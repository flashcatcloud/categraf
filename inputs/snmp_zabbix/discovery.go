package snmp_zabbix

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gosnmp/gosnmp"
)

type DiscoveryEngine struct {
	client   *SNMPClientManager
	template *ZabbixTemplate
	cache    map[string]DiscoveryCacheEntry

	discoveryCache   map[string][]DiscoveryItem
	discoveryCacheMu sync.RWMutex
}

type DiscoveryCacheEntry struct {
	Data      []DiscoveryItem
	Timestamp time.Time
	TTL       time.Duration
}

type DiscoveryItem struct {
	Macros map[string]string // 发现的宏键值对
	Index  string            // 索引值
}

type MonitorItem struct {
	Key         string
	OID         string
	Type        string
	Name        string
	Units       string
	Agent       string
	Description string
	ValueType   string

	Delay            time.Duration
	LastCollect      time.Time
	IsDiscovered     bool
	Tags             map[string]string
	DiscoveryRuleKey string

	IsLabelProvider bool   // 是否为标签提供者
	LabelKey        string // 作为标签输出时，使用的key (e.g., "alias")
	DiscoveryIndex  string // 从 {#SNMPINDEX} 等宏解析出的唯一索引

	Preprocessing []PreprocessStep `json:"preprocessing,omitempty"`
}

func extractLabelKey(itemKey string) string {
	if idx := strings.Index(itemKey, "["); idx > 0 {
		return itemKey[:idx] // a.b.c.alias[params] -> a.b.c.alias
	}
	return itemKey // if [ is not found, return the original string
}

func NewDiscoveryEngine(client *SNMPClientManager, template *ZabbixTemplate) *DiscoveryEngine {
	return &DiscoveryEngine{
		client:   client,
		template: template,
		cache:    make(map[string]DiscoveryCacheEntry),
	}
}

func (d *DiscoveryEngine) ExecuteDiscovery(ctx context.Context, agent string, rule DiscoveryRule) ([]DiscoveryItem, error) {

	cacheKey := fmt.Sprintf("%s:%s", agent, rule.Key)

	// 检查缓存
	if entry, exists := d.cache[cacheKey]; exists {
		if time.Since(entry.Timestamp) < entry.TTL {
			return entry.Data, nil
		}
	}
	// 执行SNMP发现，现在返回一个通用的 interface{} 类型以处理不同类型的原始结果
	rawResult, err := d.performSNMPDiscovery(ctx, agent, rule)
	if err != nil {
		return nil, fmt.Errorf("SNMP discovery failed for rule '%s': %w", rule.Key, err)
	}

	var discoveries []DiscoveryItem
	// 如果发现规则本身有预处理步骤，则执行它们
	if len(rule.Preprocessing) > 0 {
		var valueForPreprocessing interface{} = rawResult
		if discoveryItems, ok := rawResult.([]DiscoveryItem); ok {
			flattenedData := make([]map[string]string, 0, len(discoveryItems))
			for _, item := range discoveryItems {
				flatItem := make(map[string]string)
				// 复制所有宏到顶层
				for k, v := range item.Macros {
					flatItem[k] = v
				}
				flattenedData = append(flattenedData, flatItem)
			}

			// Zabbix LLD 格式是一个包含 "data" 键的对象
			lldWrapper := map[string]interface{}{
				"data": flattenedData,
			}
			// 将其序列化为 JSON 字节
			jsonBytes, err := json.Marshal(lldWrapper)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal initial discovery result to JSON: %w", err)
			}
			// 预处理的正确输入应该是这个 JSON 字符串
			valueForPreprocessing = string(jsonBytes)
			//log.Printf("DEBUG: Serialized discovery result for preprocessing: %s", valueForPreprocessing)
		}

		processedValue, err := ApplyDiscoveryPreprocessing(valueForPreprocessing, rule.Preprocessing)
		if err != nil {
			return nil, fmt.Errorf("discovery preprocessing failed for rule '%s': %w", rule.Key, err)
		}

		// 预处理的最终结果应该是JSON字符串
		jsonStr, ok := processedValue.(string)
		if !ok {
			return nil, fmt.Errorf("expected JSON string from discovery preprocessing, but got %T", processedValue)
		}

		// 将JSON结果反序列化为DiscoveryItem
		// Zabbix LLD格式是一个包含 "data" 键的对象
		var lldResult struct {
			Data []map[string]string `json:"data"`
		}
		// 首先尝试解析标准LLD格式
		if err := json.Unmarshal([]byte(jsonStr), &lldResult); err != nil {
			// 如果失败，尝试直接解析为数组
			var rawDiscoveries []map[string]string
			if errUnmarshal := json.Unmarshal([]byte(jsonStr), &rawDiscoveries); errUnmarshal != nil {
				return nil, fmt.Errorf("failed to unmarshal final discovery JSON from both LLD and raw array formats: %w", errUnmarshal)
			}
			lldResult.Data = rawDiscoveries
		}

		for _, itemMacros := range lldResult.Data {
			discoveries = append(discoveries, DiscoveryItem{Macros: itemMacros})
		}

	} else {
		// 如果没有预处理，假定结果是 []DiscoveryItem
		var ok bool
		discoveries, ok = rawResult.([]DiscoveryItem)
		if !ok {
			return nil, fmt.Errorf("discovery result was not []DiscoveryItem and no preprocessing was defined")
		}
	}

	// 应用过滤器
	filtered := d.applyDiscoveryFilter(discoveries, rule.Filter)
	log.Printf("I! filtered discovery results: %d items", len(filtered))
	ttl := parseZabbixDelay(rule.Delay)
	if ttl == 0 {
		ttl = time.Hour // 默认缓存1小时
	}

	// 缓存结果
	d.cache[cacheKey] = DiscoveryCacheEntry{
		Data:      filtered,
		Timestamp: time.Now(),
		TTL:       ttl,
	}

	return filtered, nil
}

func (d *DiscoveryEngine) performSNMPDiscovery(ctx context.Context, agent string, rule DiscoveryRule) (interface{}, error) {
	unlock := d.client.acquire(agent)
	defer unlock()
	client, err := d.client.GetClient(agent)
	if err != nil {
		return nil, fmt.Errorf("failed to get SNMP client: %w", err)
	}

	snmpOID := strings.TrimSpace(rule.SNMPOID)
	if snmpOID == "" {
		return nil, fmt.Errorf("empty SNMP OID for discovery rule %s", rule.Key)
	}
	if strings.HasPrefix(snmpOID, "walk[") {
		return d.performMultiOidWalkDiscovery(ctx, client, snmpOID)
	} else if strings.HasPrefix(snmpOID, "discovery[") {
		return d.performZabbixDependentDiscovery(ctx, client, snmpOID, rule)
	} else {
		return d.performStandardDiscovery(ctx, client, snmpOID, rule)
	}
}

// performMultiOidWalkDiscovery 处理 walk[...] 语法
// 改为顺序执行，防止复用 client 导致的 Request ID 错乱
func (d *DiscoveryEngine) performMultiOidWalkDiscovery(ctx context.Context, client *gosnmp.GoSNMP, discoveryOID string) ([][]gosnmp.SnmpPDU, error) {
	// 解析出 walk[] 中的所有 OID
	content := discoveryOID[5 : len(discoveryOID)-1]
	oidStrings := strings.Split(content, ",")

	if len(oidStrings) == 0 {
		return nil, fmt.Errorf("no OIDs found in walk[] directive: %s", discoveryOID)
	}

	var allPdus [][]gosnmp.SnmpPDU

	// 用于临时存储结果，保证顺序
	results := make([][][]gosnmp.SnmpPDU, len(oidStrings))

	for i, oidStr := range oidStrings {
		oid := strings.TrimSpace(oidStr)
		if err := d.validateOID(oid); err != nil {
			return nil, fmt.Errorf("invalid OID '%s' in walk[]: %w", oid, err)
		}

		// 这里不需要 acquire 锁，因为上层 performSNMPDiscovery 已经持有锁了
		pdus, err := client.BulkWalkAll(oid)
		if err != nil {
			return nil, fmt.Errorf("SNMP walk failed for OID %s: %w", oid, err)
		}

		results[i] = append(results[i], pdus)
	}

	for _, pduGroup := range results {
		if len(pduGroup) > 0 {
			allPdus = append(allPdus, pduGroup[0])
		}
	}

	return allPdus, nil
}

func (d *DiscoveryEngine) performZabbixDependentDiscovery(ctx context.Context, client *gosnmp.GoSNMP, discoveryOID string, rule DiscoveryRule) ([]DiscoveryItem, error) {
	macroOIDPairs, err := d.parseZabbixDiscoveryOID(discoveryOID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Zabbix discovery OID: %w", err)
	}

	allResults := make(map[string]map[string]string)

	for _, pair := range macroOIDPairs {
		type walkResult struct {
			pdus []gosnmp.SnmpPDU
			err  error
		}

		resultChan := make(chan walkResult, 1)

		go func() {
			pdus, err := client.BulkWalkAll(pair.OID)
			resultChan <- walkResult{pdus: pdus, err: err}
		}()

		var results []gosnmp.SnmpPDU
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("SNMP walk for OID %s was canceled or timed out: %w", pair.OID, ctx.Err())
		case res := <-resultChan:
			if res.err != nil {
				log.Printf("Warning: SNMP walk failed for OID %s: %v", pair.OID, res.err)
				continue
			}
			results = res.pdus
		}

		for _, result := range results {
			index := d.extractIndexFromOID(result.Name, pair.OID)
			if index == "" {
				continue
			}
			if allResults[index] == nil {
				allResults[index] = make(map[string]string)
			}
			value := d.convertSNMPValueToString(result)
			allResults[index][pair.Macro] = value
		}
	}

	var discoveries []DiscoveryItem
	for index, macros := range allResults {
		macros["{#SNMPINDEX}"] = index
		macros["{#IFINDEX}"] = index
		discovery := DiscoveryItem{
			Macros: macros,
			Index:  index,
		}
		discoveries = append(discoveries, discovery)
	}

	return discoveries, nil
}

func (d *DiscoveryEngine) parseZabbixDiscoveryOID(discoveryOID string) ([]MacroOIDPair, error) {
	// 移除 "discovery[" 前缀和 "]" 后缀
	if !strings.HasPrefix(discoveryOID, "discovery[") || !strings.HasSuffix(discoveryOID, "]") {
		return nil, fmt.Errorf("invalid discovery OID format: %s", discoveryOID)
	}

	content := discoveryOID[10 : len(discoveryOID)-1] // 移除 "discovery[" 和 "]"

	// 分割参数
	parts := strings.Split(content, ",")
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("discovery OID must have even number of parameters (macro,oid pairs): %s", discoveryOID)
	}

	var pairs []MacroOIDPair
	for i := 0; i < len(parts); i += 2 {
		macro := strings.TrimSpace(parts[i])
		oid := strings.TrimSpace(parts[i+1])

		// 验证宏格式
		if !strings.HasPrefix(macro, "{#") || !strings.HasSuffix(macro, "}") {
			return nil, fmt.Errorf("invalid macro format: %s", macro)
		}

		// 验证 OID 格式
		if err := d.validateOID(oid); err != nil {
			return nil, fmt.Errorf("invalid OID %s for macro %s: %w", oid, macro, err)
		}

		pairs = append(pairs, MacroOIDPair{
			Macro: macro,
			OID:   oid,
		})
	}

	return pairs, nil
}

func (d *DiscoveryEngine) performStandardDiscovery(ctx context.Context, client *gosnmp.GoSNMP, snmpOID string, rule DiscoveryRule) ([]DiscoveryItem, error) {
	if err := d.validateOID(snmpOID); err != nil {
		return nil, fmt.Errorf("invalid SNMP OID '%s': %w", snmpOID, err)
	}

	type walkResult struct {
		pdus []gosnmp.SnmpPDU
		err  error
	}

	resultChan := make(chan walkResult, 1)

	go func() {
		pdus, err := client.BulkWalkAll(snmpOID)
		resultChan <- walkResult{pdus: pdus, err: err}
	}()

	var results []gosnmp.SnmpPDU
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("SNMP walk for OID %s was canceled or timed out: %w", snmpOID, ctx.Err())
	case res := <-resultChan:
		if res.err != nil {
			return nil, fmt.Errorf("SNMP walk failed for OID %s: %w", snmpOID, res.err)
		}
		results = res.pdus
	}

	var discoveries []DiscoveryItem
	for _, result := range results {
		discovery := d.parseStandardDiscoveryResult(result, rule, snmpOID)
		if discovery != nil {
			discoveries = append(discoveries, *discovery)
		}
	}

	return discoveries, nil
}

func (d *DiscoveryEngine) extractIndexFromOID(fullOID, baseOID string) string {
	// 移除前导点
	fullOID = strings.TrimPrefix(fullOID, ".")
	baseOID = strings.TrimPrefix(baseOID, ".")

	if !strings.HasPrefix(fullOID, baseOID) {
		return ""
	}

	// 提取索引部分
	if len(fullOID) <= len(baseOID) {
		return ""
	}

	index := fullOID[len(baseOID):]
	index = strings.TrimPrefix(index, ".")

	return index
}

func isPrintableString(b []byte) bool {
	for _, c := range string(b) {
		// unicode.IsPrint 会将空格也视为可打印
		if !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}

func (d *DiscoveryEngine) convertSNMPValueToString(pdu gosnmp.SnmpPDU) string {
	switch pdu.Type {
	case gosnmp.OctetString:
		if bytes, ok := pdu.Value.([]byte); ok {
			if isPrintableString(bytes) {
				return string(bytes)
			} else {
				hexParts := make([]string, len(bytes))
				for i, b := range bytes {
					hexParts[i] = fmt.Sprintf("%02X", b)
				}
				return strings.Join(hexParts, " ")
			}
		}
		return fmt.Sprintf("%v", pdu.Value)
	case gosnmp.Integer, gosnmp.Counter32, gosnmp.Counter64, gosnmp.Gauge32:
		return gosnmp.ToBigInt(pdu.Value).String()
	default:
		// Fallback for other types
		return fmt.Sprintf("%v", pdu.Value)
	}
}

func (d *DiscoveryEngine) parseStandardDiscoveryResult(result gosnmp.SnmpPDU, rule DiscoveryRule, baseOID string) *DiscoveryItem {
	macros := make(map[string]string)

	// 提取索引
	index := d.extractIndexFromOID(result.Name, baseOID)
	if index != "" {
		macros["{#SNMPINDEX}"] = index
	}

	// 添加值
	value := d.convertSNMPValueToString(result)

	// 根据发现规则类型设置相应的宏
	switch {
	case strings.Contains(rule.Key, "net.if"):
		macros["{#IFINDEX}"] = index
		macros["{#IFNAME}"] = value
		macros["{#IFDESCR}"] = value
	case strings.Contains(rule.Key, "vfs.fs"):
		macros["{#FSINDEX}"] = index
		macros["{#FSNAME}"] = value
		macros["{#FSPATH}"] = value
	default:
		macros["{#VALUE}"] = value
	}

	return &DiscoveryItem{
		Macros: macros,
	}
}

type MacroOIDPair struct {
	Macro string
	OID   string
}

func (d *DiscoveryEngine) validateOID(oid string) error {
	if oid == "" {
		return fmt.Errorf("empty OID")
	}

	// 移除可能的前导点
	oid = strings.TrimPrefix(oid, ".")

	// 检查是否是有效的点分十进制格式
	parts := strings.Split(oid, ".")
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("empty OID component at position %d", i)
		}

		// 检查是否为数字
		if _, err := strconv.Atoi(part); err != nil {
			return fmt.Errorf("invalid OID component '%s' at position %d: must be numeric", part, i)
		}
	}

	return nil
}

func (d *DiscoveryEngine) applyDiscoveryFilter(discoveries []DiscoveryItem, filter DiscoveryFilter) []DiscoveryItem {
	if len(filter.Conditions) == 0 {
		return discoveries
	}

	var filtered []DiscoveryItem

	for _, discovery := range discoveries {
		if d.template != nil && d.template.ValidateFilter(filter, discovery.Macros) {
			filtered = append(filtered, discovery)
		}
	}

	return filtered
}

func (d *DiscoveryEngine) ApplyItemPrototypes(discoveries []DiscoveryItem, rule DiscoveryRule) []MonitorItem {
	var items []MonitorItem
	prototypes := rule.ItemPrototypes

	for _, discovery := range discoveries {
		discoveryIndex := ""
		if idx, ok := discovery.Macros["{#SNMPINDEX}"]; ok {
			discoveryIndex = idx
		} else if idx, ok := discovery.Macros["{#IFINDEX}"]; ok {
			// Fallback for different index macro names
			discoveryIndex = idx
		}

		for _, prototype := range prototypes {
			if prototype.Status == "DISABLED" ||
				prototype.Status == "UNSUPPORTED" {
				continue
			}
			delay := parseZabbixDelay(prototype.Delay)
			tags := map[string]string{}
			for _, tag := range prototype.Tags {
				tags[tag.Tag] = tag.Value
			}

			item := MonitorItem{
				Key:              d.expandMacros(prototype.Key, discovery.Macros),
				OID:              d.expandMacros(prototype.SNMPOID, discovery.Macros),
				Type:             ConvertZabbixItemType(prototype.Type),
				Name:             d.expandMacros(prototype.Name, discovery.Macros),
				Units:            prototype.Units,
				Description:      d.expandMacros(prototype.Description, discovery.Macros),
				ValueType:        ConvertZabbixValueType(prototype.ValueType),
				Delay:            delay,
				IsDiscovered:     true,
				Tags:             tags,
				DiscoveryRuleKey: rule.Key,
				Preprocessing:    prototype.Preprocessing,
				DiscoveryIndex:   discoveryIndex,
			}
			switch prototype.ValueType {
			case "CHAR", "1", "TEXT", "4":
				item.IsLabelProvider = true
				item.LabelKey = extractLabelKey(prototype.Key)
			default:
				item.IsLabelProvider = false
			}
			items = append(items, item)
		}
	}

	return items
}

func parseZabbixDelay(delayStr string) time.Duration {
	if delayStr == "" {
		return 60 * time.Second // 默认60秒
	}

	// Zabbix支持多种格式：
	// - 简单数字（秒）: "30"
	// - 带单位的: "30s", "5m", "1h"
	// - 灵活间隔: "30;wd1-5,h9-18:60;wd6-7:300"

	// 简单处理，支持基本格式
	if strings.Contains(delayStr, ";") {
		// 复杂的灵活间隔，取第一个值
		parts := strings.Split(delayStr, ";")
		delayStr = parts[0]
	}

	// 尝试解析数字（秒）
	if seconds, err := strconv.Atoi(delayStr); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// 尝试解析带单位的时间
	if duration, err := time.ParseDuration(delayStr); err == nil {
		return duration
	}

	return 60 * time.Second
}

func (d *DiscoveryEngine) expandMacros(text string, macros map[string]string) string {
	result := text
	for macro, value := range macros {
		macroWithBraces := fmt.Sprintf("{#%s}", strings.Trim(macro, "{}#"))
		result = strings.ReplaceAll(result, macroWithBraces, value)
	}

	if d.template != nil {
		result = d.template.ExpandMacros(result, macros)
	}
	return result
}

func (d *DiscoveryEngine) containsMacros(text string) bool {
	// 检查是否包含 宏 {#MACRO} 或 {$MACRO}
	return strings.Contains(text, "{#") || strings.Contains(text, "{$")
}
