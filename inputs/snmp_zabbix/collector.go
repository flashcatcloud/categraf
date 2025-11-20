package snmp_zabbix

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"flashcat.cloud/categraf/types"
)

type SNMPCollector struct {
	client *SNMPClientManager
	config *Config
	mu     sync.RWMutex

	preprocessingCtx *PreprocessingContext
	labelCache       *LabelCache
	labels           map[string]string
	mappings         map[string]map[string]string
}

type CollectionResult struct {
	Agent  string
	Key    string
	Value  interface{}
	Type   string
	Tags   map[string]string
	Fields map[string]interface{}
	Time   time.Time
	Error  error
}

func NewSNMPCollector(client *SNMPClientManager, config *Config, labelCache *LabelCache, labels map[string]string, mappings map[string]map[string]string) *SNMPCollector {
	return &SNMPCollector{
		client:           client,
		config:           config,
		preprocessingCtx: NewPreprocessingContext(),
		labelCache:       labelCache,
		labels:           labels,
		mappings:         mappings,
	}
}

func (c *SNMPCollector) CollectItems(ctx context.Context, items []MonitorItem, slist *types.SampleList) error {
	agentItems := c.groupItemsByAgent(items)

	var wg sync.WaitGroup
	// 使用带缓冲的 channel 防止阻塞
	resultChan := make(chan CollectionResult, len(items)+10)

	// 并发收集每个agent的数据
	for agent, agentItemList := range agentItems {
		wg.Add(1)
		go func(agent string, items []MonitorItem) {
			defer wg.Done()
			c.collectFromAgent(ctx, agent, items, resultChan)
		}(agent, agentItemList)
	}

	// 等待所有goroutine完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理收集结果
	for result := range resultChan {
		if result.Error != nil {
			// 记录错误但继续处理其他结果
			// 降低日志级别或增加频率限制，防止日志刷屏
			if c.config.DebugMode {
				log.Printf("D! collected agent: %s, key: %s, error: %v", result.Agent, result.Key, result.Error)
			}
			continue
		}

		c.addMetricToAccumulator(result, slist)
	}

	return nil
}

func (c *SNMPCollector) groupItemsByAgent(items []MonitorItem) map[string][]MonitorItem {
	agentItems := make(map[string][]MonitorItem)

	for _, item := range items {
		agent := item.Agent
		if agent == "" {
			// 如果item没有指定agent，使用配置中的第一个agent
			if len(c.config.Agents) > 0 {
				agent = c.config.GetAgentAddress(c.config.Agents[0])
			}
		}
		agentItems[agent] = append(agentItems[agent], item)
	}

	return agentItems
}

func (c *SNMPCollector) collectFromAgent(ctx context.Context, agent string, items []MonitorItem, resultChan chan<- CollectionResult) {
	// 获取Agent级别的锁，确保同一时间只有一个采集任务在运行
	unlock := c.client.acquire(agent)
	defer unlock()

	client, err := c.client.GetClient(agent)
	if err != nil {
		for _, item := range items {
			resultChan <- CollectionResult{
				Agent: agent,
				Key:   item.Key,
				Error: fmt.Errorf("failed to get SNMP client for agent %s: %w", agent, err),
			}
		}
		return
	}

	// 批量大小
	batchSize := c.config.MaxRepetitions
	if batchSize <= 0 {
		batchSize = 10 // 默认值
	}

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batchItems := items[i:end]

		// 如果 batchSize 为 1，直接使用单个采集逻辑，避免 GetBulk 的开销
		if batchSize == 1 {
			c.collectIndividually(client, agent, batchItems, resultChan)
			continue
		}

		oids := make([]string, len(batchItems))
		for j, item := range batchItems {
			oids[j] = item.OID
		}

		// 尝试批量请求
		results, err := c.bulkGet(client, oids)
		if err != nil {
			// 只有在调试模式下才打印详细的批量失败日志
			if c.config.DebugMode {
				log.Printf("D! bulk request failed for agent %s (size: %d): %v. Falling back to individual collection.", agent, len(oids), err)
			}
			// 降级：尝试单独请求这个批次的OID
			c.collectIndividually(client, agent, batchItems, resultChan)
			continue
		}

		for j, result := range results {
			if j >= len(batchItems) {
				break
			}

			item := batchItems[j]
			c.processSingleResult(agent, item, result, resultChan)
		}
	}
}

func (c *SNMPCollector) collectIndividually(client *gosnmp.GoSNMP, agent string, items []MonitorItem, resultChan chan<- CollectionResult) {
	for _, item := range items {
		// 使用 Get 而不是 BulkGet
		pduResult, err := client.Get([]string{item.OID})

		var bulkResult BulkGetResult
		bulkResult.OID = item.OID

		if err != nil {
			bulkResult.Error = err
		} else if len(pduResult.Variables) > 0 {
			bulkResult.Value = pduResult.Variables[0]
		} else {
			bulkResult.Error = fmt.Errorf("no data returned for OID %s", item.OID)
		}

		c.processSingleResult(agent, item, bulkResult, resultChan)
	}
}

func (c *SNMPCollector) processSingleResult(agent string, item MonitorItem, result BulkGetResult, resultChan chan<- CollectionResult) {
	if item.IsLabelProvider {
		if result.Error == nil {
			rawValue := c.convertSNMPValue(result.Value, item)
			processedValue := rawValue
			if len(item.Preprocessing) > 0 {
				processed, err := ApplyPreprocessingWithContext(
					rawValue,
					item.Preprocessing,
					c.preprocessingCtx,
					item.Key,
					agent,
				)
				if err == nil {
					processedValue = processed
				}
			}

			if strVal, ok := processedValue.(string); ok {
				c.labelCache.Set(agent, item.DiscoveryRuleKey, item.DiscoveryIndex, item.LabelKey, strVal)
			}
		}
		return
	}

	collectionResult := CollectionResult{
		Agent: agent,
		Key:   item.Key,
		Time:  time.Now(),
		Tags:  c.buildTags(agent, item),
	}

	if result.Error != nil {
		collectionResult.Error = result.Error
	} else {
		rawValue := c.convertSNMPValue(result.Value, item)
		processedValue := rawValue
		if len(item.Preprocessing) > 0 {
			processed, err := ApplyPreprocessingWithContext(
				rawValue,
				item.Preprocessing,
				c.preprocessingCtx,
				item.Key,
				agent,
			)
			if err != nil {
				if !strings.Contains(err.Error(), "no previous value") && c.config.DebugMode {
					log.Printf("D! preprocessing failed for %s: %v", item.Key, err)
				}
				processedValue = rawValue
			} else {
				processedValue = processed
			}
		}

		finalValue, fields, err := c.processValue(processedValue, item)
		if err != nil {
			collectionResult.Error = err
		} else {
			if floatVal, convErr := ConvertToFloat64(finalValue); convErr == nil {
				fields["value"] = floatVal
			} else {
				fields["value"] = finalValue
			}

			collectionResult.Value = finalValue
			collectionResult.Fields = fields
			collectionResult.Type = item.Type
		}
	}
	resultChan <- collectionResult
}

type BulkGetResult struct {
	OID   string
	Value gosnmp.SnmpPDU
	Error error
}

func (c *SNMPCollector) bulkGet(client *gosnmp.GoSNMP, oids []string) ([]BulkGetResult, error) {
	// 这里的实现假设 oids 数量已经由 caller 控制好了。

	result, err := client.Get(oids)
	if err != nil {
		return nil, err
	}

	valueMap := make(map[string]gosnmp.SnmpPDU, len(result.Variables))
	for _, pdu := range result.Variables {
		normalizedOID := strings.TrimPrefix(pdu.Name, ".")
		valueMap[normalizedOID] = pdu
	}

	results := make([]BulkGetResult, len(oids))
	for i, oid := range oids {
		results[i] = BulkGetResult{OID: oid}

		// 尝试多种形式的OID匹配（有没有前导点）
		oidNoDot := strings.TrimPrefix(oid, ".")

		if pdu, ok := valueMap[oidNoDot]; ok {
			if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
				results[i].Error = fmt.Errorf("OID not found on device: %s", oid)
			} else {
				results[i].Value = pdu
			}
		} else {
			results[i].Error = fmt.Errorf("no response for OID: %s", oid)
		}
	}

	return results, nil
}

func (c *SNMPCollector) processValue(value interface{}, item MonitorItem) (interface{}, map[string]interface{}, error) {
	fields := make(map[string]interface{})

	switch item.ValueType {
	case "float":
		if floatVal, err := c.convertToFloat(value); err == nil {
			fields["value"] = floatVal
			return floatVal, fields, nil
		} else {
			return value, fields, fmt.Errorf("failed to convert value to float for key %s: %w", item.Key, err)
		}
	case "uint":
		if uintVal, err := c.convertToUint(value); err == nil {
			fields["value"] = uintVal
			return uintVal, fields, nil
		} else {
			return value, fields, fmt.Errorf("failed to convert value to uint for key %s: %w", item.Key, err)
		}
	case "string":
		strVal := fmt.Sprintf("%v", value)
		fields["value"] = strVal
		return strVal, fields, nil
	default:
		if floatVal, err := c.convertToFloat(value); err == nil {
			fields["value"] = floatVal
			return floatVal, fields, nil
		} else {
			strVal := fmt.Sprintf("%v", value)
			fields["value"] = strVal
			return strVal, fields, nil
		}
	}
}

func (c *SNMPCollector) convertSNMPValue(pdu gosnmp.SnmpPDU, item MonitorItem) interface{} {
	hasJsPreprocessing := false
	hasIpMacPreprocessing := false
	for _, step := range item.Preprocessing {
		switch step.Type {
		case "JAVASCRIPT", "21":
			hasJsPreprocessing = true
		case "MAC_FORMAT", "SNMP_HEX_TO_MAC", "IP_FORMAT", "SNMP_OCTETS_TO_IP":
			hasIpMacPreprocessing = true
		}
	}

	switch pdu.Type {
	case gosnmp.OctetString:
		if bytes, ok := pdu.Value.([]byte); ok {
			if hasJsPreprocessing {
				return bytesToHexSpacedStr(bytes)
			} else if hasIpMacPreprocessing {
				return bytes
			}
			str := string(bytes)
			if c.isPrintableString(str) {
				return str
			}
			return fmt.Sprintf("%x", bytes)
		}
		return fmt.Sprintf("%v", pdu.Value)
	case gosnmp.Integer:
		return pdu.Value
	case gosnmp.Counter32:
		if val, ok := pdu.Value.(uint32); ok {
			return uint64(val)
		}
		return pdu.Value
	case gosnmp.Counter64:
		return pdu.Value
	case gosnmp.Gauge32:
		return pdu.Value
	case gosnmp.TimeTicks:
		if ticks, ok := pdu.Value.(uint32); ok {
			return float64(ticks) / 100.0
		}
		return pdu.Value
	case gosnmp.ObjectIdentifier:
		return fmt.Sprintf("%v", pdu.Value)
	case gosnmp.IPAddress:
		return fmt.Sprintf("%v", pdu.Value)
	case gosnmp.Opaque:
		if bytes, ok := pdu.Value.([]byte); ok {
			return fmt.Sprintf("%x", bytes)
		}
		return fmt.Sprintf("%v", pdu.Value)
	default:
		return fmt.Sprintf("%v", pdu.Value)
	}
}

func (c *SNMPCollector) isPrintableString(s string) bool {
	for _, r := range s {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

func (c *SNMPCollector) convertToFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

func (c *SNMPCollector) convertToUint(value interface{}) (uint64, error) {
	switch v := value.(type) {
	case uint64:
		return v, nil
	case uint32:
		return uint64(v), nil
	case uint:
		return uint64(v), nil
	case int64:
		if v >= 0 {
			return uint64(v), nil
		}
		return 0, fmt.Errorf("negative value cannot be converted to uint: %d", v)
	case int32:
		if v >= 0 {
			return uint64(v), nil
		}
		return 0, fmt.Errorf("negative value cannot be converted to uint: %d", v)
	case int:
		if v >= 0 {
			return uint64(v), nil
		}
		return 0, fmt.Errorf("negative value cannot be converted to uint: %d", v)
	case string:
		return strconv.ParseUint(v, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to uint64", value)
	}
}

func (c *SNMPCollector) buildTags(agent string, item MonitorItem) map[string]string {
	tags := make(map[string]string)

	tags["snmp_agent"] = agent
	tags["snmp_host"] = getHostFromAgentStr(agent)
	tags["oid"] = item.OID
	tags["plugin"] = inputName

	if item.IsDiscovered && item.DiscoveryIndex != "" {
		if cachedLabels := c.labelCache.Get(agent, item.DiscoveryRuleKey, item.DiscoveryIndex); cachedLabels != nil {
			for key, value := range cachedLabels {
				tags[key] = value
			}
		}
	}

	if strings.Contains(item.Key, "[") && strings.Contains(item.Key, "]") {
		start := strings.Index(item.Key, "[")
		end := strings.Index(item.Key, "]")
		if start < end {
			param := item.Key[start+1 : end]
			keyBase := item.Key[:start]

			if strings.Contains(keyBase, "if.") {
				tags["interface_index"] = param
				keyIdx := strings.LastIndex(param, ".")
				if keyIdx != -1 && keyIdx != len(param)-1 {
					tags["snmp_index"] = param[keyIdx+1:]
				}
			} else if strings.Contains(keyBase, "fs.") {
				tags["filesystem_index"] = param
			} else {
				tags["index"] = param
			}
		}
	}

	if item.Key != "" {
		tags["item_key"] = item.Key
	}
	if item.Name != "" {
		tags["item"] = item.Name
	}
	for k, v := range item.Tags {
		tags[k] = v
	}
	for k, v := range c.labels {
		tags[k] = v
	}
	if kvs, ok := c.mappings[agent]; ok {
		for k, v := range kvs {
			tags[k] = v
		}
	}

	return tags
}

func (c *SNMPCollector) addMetricToAccumulator(result CollectionResult, slist *types.SampleList) {
	measurement := c.buildMeasurementName(result.Key)
	for _, fv := range result.Fields {
		sample := types.NewSample("", measurement, fv, result.Tags).SetTime(result.Time)
		slist.PushFront(sample)
	}
}

func (c *SNMPCollector) buildMeasurementName(key string) string {
	measurement := strings.ReplaceAll(key, ".", "_")
	if idx := strings.Index(measurement, "["); idx != -1 {
		measurement = measurement[:idx]
	}
	return "snmp_" + measurement
}

func bytesToHexSpacedStr(data []byte) string {
	hexParts := make([]string, len(data))
	for i, b := range data {
		hexParts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(hexParts, " ")
}
