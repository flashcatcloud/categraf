package snmp_zabbix

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gosnmp/gosnmp"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/oliveagle/jsonpath"
)

var scriptCache = NewJSCache()

// PreprocessingContext 预处理上下文，用于维护状态
type PreprocessingContext struct {
	// 历史值缓存 key: "agent|item_key"
	historyCache map[string]*HistoryValue
	mu           sync.RWMutex
}

// HistoryValue 历史值记录
type HistoryValue struct {
	Value     interface{}
	Timestamp time.Time
	// 用于计算速率
	LastValue     interface{}
	LastTimestamp time.Time
}

// NewPreprocessingContext 创建预处理上下文
func NewPreprocessingContext() *PreprocessingContext {
	return &PreprocessingContext{
		historyCache: make(map[string]*HistoryValue),
	}
}

// GetHistory 获取历史值
func (pc *PreprocessingContext) GetHistory(key string) (*HistoryValue, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	hist, ok := pc.historyCache[key]
	if !ok {
		return nil, false
	}

	// 返回副本避免并发问题
	return &HistoryValue{
		Value:         hist.Value,
		Timestamp:     hist.Timestamp,
		LastValue:     hist.LastValue,
		LastTimestamp: hist.LastTimestamp,
	}, true
}

// UpdateHistory 更新历史值
func (pc *PreprocessingContext) UpdateHistory(key string, value interface{}, timestamp time.Time) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if hist, exists := pc.historyCache[key]; exists {
		// 保存上一次的值
		hist.LastValue = hist.Value
		hist.LastTimestamp = hist.Timestamp
		// 更新当前值
		hist.Value = value
		hist.Timestamp = timestamp
	} else {
		// 首次记录
		pc.historyCache[key] = &HistoryValue{
			Value:     value,
			Timestamp: timestamp,
		}
	}
}

// PreprocessingError 预处理错误
type PreprocessingError struct {
	Step    int
	Type    string
	Message string
	Err     error
}

func (e *PreprocessingError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("preprocessing step %d (%s): %s: %v", e.Step, e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("preprocessing step %d (%s): %s", e.Step, e.Type, e.Message)
}

// ApplyPreprocessingWithContext 带上下文的预处理（扩展现有函数）
func ApplyPreprocessingWithContext(value interface{}, preprocessing []PreprocessStep, context *PreprocessingContext, itemKey string, agent string) (interface{}, error) {
	if len(preprocessing) == 0 {
		return value, nil
	}

	var result interface{} = value
	var err error

	for i, step := range preprocessing {
		result, err = applyPreprocessingStep(result, step, context, itemKey, agent, i)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// 用于处理发现阶段的预处理
func ApplyDiscoveryPreprocessing(rawResult interface{}, steps []PreprocessStep) (interface{}, error) {
	var result = rawResult
	var err error

	for _, step := range steps {
		result, err = applyDiscoveryPreprocessingStep(result, step)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// 作为发现阶段预处理的分发器
func applyDiscoveryPreprocessingStep(value interface{}, step PreprocessStep) (interface{}, error) {
	// 注意：发现阶段的预处理不使用上下文
	switch step.Type {
	case "SNMP_WALK_TO_JSON", "16":
		return applySNMPWalkToJSON(value, step.Parameters)
	case "JAVASCRIPT", "21":
		return applyJavaScript(value, step.Parameters)
	case "JSONPATH", "12":
		return applyJSONPath(value, step.Parameters)
	default:
		log.Printf("W! unsupported preprocessing type in discovery phase: %s, skipping", step.Type)
		return value, nil
	}
}

// applyPreprocessingStep 应用单个预处理步骤
func applyPreprocessingStep(value interface{}, step PreprocessStep, context *PreprocessingContext, itemKey string, agent string, stepIndex int) (interface{}, error) {
	switch step.Type {
	case "MULTIPLIER", "5": // 5 是 Zabbix 中 MULTIPLIER 的数字类型
		return applyMultiplier(value, step.Parameters)

	case "HEX_TO_DECIMAL", "17": // 十六进制转十进制
		return applyHexToDecimal(value)

	case "MAC_FORMAT", "SNMP_HEX_TO_MAC": // MAC地址格式化
		return applyMacFormat(value, step.Parameters)

	case "IP_FORMAT", "SNMP_OCTETS_TO_IP": // IP地址格式化
		return applyIpFormat(value)

	case "REGEX", "11": // 正则表达式
		return applyRegex(value, step.Parameters)

	case "SIMPLE_CHANGE", "9": // 简单变化
		return applySimpleChange(value, context, fmt.Sprintf("%s|%s", agent, itemKey))

	case "CHANGE_PER_SECOND", "10": // 每秒变化率
		return applyChangePerSecond(value, context, fmt.Sprintf("%s|%s", agent, itemKey))

	case "TRIM", "LTRIM", "RTRIM": // 字符串修剪
		return applyTrim(value, step.Type)

	//case "SNMP_WALK_TO_JSON": // SNMP walk结果转JSON
	//	return applySNMPWalkToJSON(value, step.Parameters)

	case "JSONPATH", "12":
		return applyJSONPath(value, step.Parameters)

	case "JAVASCRIPT", "21":
		return applyJavaScript(value, step.Parameters)

	default:
		// 未实现的预处理类型，记录警告但不中断处理
		log.Printf("W! unsupported preprocessing type: %s at step %d, skipping", step.Type, stepIndex)
		return value, nil
	}
}

// applyMultiplier 应用乘数
func applyMultiplier(value interface{}, params []string) (interface{}, error) {
	if len(params) == 0 {
		return value, nil
	}
	multiplierStr := params[0]

	multiplier, err := strconv.ParseFloat(multiplierStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid multiplier: %v", err)
	}

	floatVal, err := ConvertToFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("cannot convert value to float: %v", err)
	}

	return floatVal * multiplier, nil
}

// applyHexToDecimal 十六进制转十进制
func applyHexToDecimal(value interface{}) (interface{}, error) {
	var hexStr string

	switch v := value.(type) {
	case string:
		hexStr = strings.TrimSpace(v)
		// 移除可能的0x前缀
		hexStr = strings.TrimPrefix(hexStr, "0x")
		hexStr = strings.TrimPrefix(hexStr, "0X")
	case []byte:
		hexStr = hex.EncodeToString(v)
	default:
		hexStr = fmt.Sprintf("%v", v)
	}

	// 移除所有非十六进制字符
	hexStr = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			return r
		}
		return -1
	}, hexStr)

	if hexStr == "" {
		return int64(0), nil
	}

	result, err := strconv.ParseInt(hexStr, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hex value '%s': %v", hexStr, err)
	}

	return result, nil
}

// applyMacFormat 格式化MAC地址
func applyMacFormat(value interface{}, params []string) (interface{}, error) {
	separator := ":"
	if len(params) > 0 {
		separator = params[0]
	}

	var macBytes []byte

	switch v := value.(type) {
	case string:
		// 尝试解析各种格式的MAC地址
		cleaned := strings.Map(func(r rune) rune {
			if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
				return r
			}
			return -1
		}, v)

		if len(cleaned) == 12 {
			// 标准6字节MAC
			for i := 0; i < 12; i += 2 {
				b, err := strconv.ParseUint(cleaned[i:i+2], 16, 8)
				if err != nil {
					return nil, fmt.Errorf("invalid MAC address format: %v", err)
				}
				macBytes = append(macBytes, byte(b))
			}
		} else {
			return v, nil // 无法解析，返回原值
		}

	case []byte:
		macBytes = v

	case net.HardwareAddr:
		macBytes = []byte(v)

	default:
		// 尝试转换为字符串再处理
		return applyMacFormat(fmt.Sprintf("%v", v), params)
	}

	if len(macBytes) != 6 {
		return value, nil // 不是标准MAC地址长度，返回原值
	}

	// 格式化MAC地址
	parts := make([]string, 6)
	for i := 0; i < 6; i++ {
		parts[i] = fmt.Sprintf("%02X", macBytes[i])
	}

	return strings.Join(parts, separator), nil
}

// applyIpFormat 格式化IP地址
func applyIpFormat(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		// 已经是字符串格式，尝试验证
		if ip := net.ParseIP(v); ip != nil {
			return v, nil
		}
		// 可能是十六进制或其他格式
		return parseIPFromString(v)

	case []byte:
		if len(v) == 4 {
			// IPv4
			return net.IPv4(v[0], v[1], v[2], v[3]).String(), nil
		} else if len(v) == 16 {
			// IPv6
			return net.IP(v).String(), nil
		}
		return value, nil

	case net.IP:
		return v.String(), nil

	case uint32:
		// 32位整数转IPv4
		return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String(), nil

	default:
		// 尝试从OID或其他格式提取
		return extractIPFromValue(fmt.Sprintf("%v", v))
	}
}

// parseIPFromString 从字符串解析IP
func parseIPFromString(s string) (interface{}, error) {
	// 尝试从OID格式提取 (如 1.3.6.1.2.1.4.22.1.2.1.192.168.1.1)
	parts := strings.Split(s, ".")
	if len(parts) >= 4 {
		// 尝试最后4个部分作为IP
		if len(parts) >= 4 {
			ipParts := parts[len(parts)-4:]
			octets := make([]byte, 4)
			for i, part := range ipParts {
				val, err := strconv.Atoi(part)
				if err != nil || val < 0 || val > 255 {
					break
				}
				octets[i] = byte(val)
				if i == 3 {
					return net.IPv4(octets[0], octets[1], octets[2], octets[3]).String(), nil
				}
			}
		}
	}

	return s, nil
}

// extractIPFromValue 从值中提取IP地址
func extractIPFromValue(s string) (interface{}, error) {
	// 正则匹配IP地址
	ipRegex := regexp.MustCompile(`(\d{1,3}\.){3}\d{1,3}`)
	if match := ipRegex.FindString(s); match != "" {
		if ip := net.ParseIP(match); ip != nil {
			return match, nil
		}
	}

	return s, nil
}

// applyRegex 应用正则表达式
func applyRegex(value interface{}, params []string) (interface{}, error) {
	if len(params) < 1 {
		return nil, fmt.Errorf("regex pattern not specified")
	}
	pattern := params[0]
	output := "\\0"
	if len(params) > 1 {
		output = params[1]
	}

	strVal := fmt.Sprintf("%v", value)

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %v", err)
	}

	matches := re.FindStringSubmatch(strVal)
	if len(matches) == 0 {
		return "", nil
	}

	// 替换输出模板中的反向引用
	result := output
	for i := 0; i < len(matches) && i < 10; i++ {
		placeholder := fmt.Sprintf("\\%d", i)
		result = strings.ReplaceAll(result, placeholder, matches[i])
	}

	return result, nil
}

// applySimpleChange 计算简单变化（当前值 - 上次值）
func applySimpleChange(value interface{}, context *PreprocessingContext, key string) (interface{}, error) {
	if context == nil {
		return nil, fmt.Errorf("context required for SIMPLE_CHANGE")
	}

	floatVal, err := ConvertToFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("cannot convert value to float: %v", err)
	}

	history, exists := context.GetHistory(key)
	if !exists || history.Value == nil {
		// 首次采集，无法计算变化
		context.UpdateHistory(key, floatVal, time.Now())
		return nil, fmt.Errorf("no previous value available")
	}

	prevFloat, err := ConvertToFloat64(history.Value)
	if err != nil {
		// 上次值无法转换，更新为新值
		context.UpdateHistory(key, floatVal, time.Now())
		return nil, fmt.Errorf("cannot convert previous value: %v", err)
	}

	change := floatVal - prevFloat
	context.UpdateHistory(key, floatVal, time.Now())

	return change, nil
}

// applyChangePerSecond 计算每秒变化率
func applyChangePerSecond(value interface{}, context *PreprocessingContext, key string) (interface{}, error) {
	if context == nil {
		return nil, fmt.Errorf("context required for CHANGE_PER_SECOND")
	}

	floatVal, err := ConvertToFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("cannot convert value to float: %v", err)
	}

	now := time.Now()
	history, exists := context.GetHistory(key)
	if !exists || history.Value == nil {
		// 首次采集，无法计算速率
		context.UpdateHistory(key, floatVal, now)
		return nil, fmt.Errorf("no previous value available to calculate rate")
	}

	prevFloat, err := ConvertToFloat64(history.Value)
	if err != nil {
		context.UpdateHistory(key, floatVal, now)
		return nil, fmt.Errorf("cannot convert previous value: %v", err)
	}

	// 计算时间差（秒）
	timeDiff := now.Sub(history.Timestamp).Seconds()
	if timeDiff <= 0 {
		context.UpdateHistory(key, floatVal, now)
		return nil, fmt.Errorf("invalid time difference: %f seconds", timeDiff)
	}

	var change float64
	if floatVal < prevFloat {
		maxValue := getMaxCounterValue(value)
		change = (float64(maxValue) - prevFloat) + floatVal
	} else {
		change = floatVal - prevFloat
	}

	// 计算速率
	context.UpdateHistory(key, floatVal, now)

	return change / timeDiff, nil
}

// applyTrim 应用字符串修剪
func applyTrim(value interface{}, trimType string) (interface{}, error) {
	strVal := fmt.Sprintf("%v", value)

	switch trimType {
	case "TRIM":
		return strings.TrimSpace(strVal), nil
	case "LTRIM":
		return strings.TrimLeft(strVal, " \t\n\r"), nil
	case "RTRIM":
		return strings.TrimRight(strVal, " \t\n\r"), nil
	default:
		return strVal, nil
	}
}

// applySNMPWalkToJSON 将SNMP walk结果转换为JSON
func applySNMPWalkToJSON(value interface{}, params []string) (interface{}, error) {
	allPdus, ok := value.([][]gosnmp.SnmpPDU)
	if !ok {
		return nil, fmt.Errorf("SNMP_WALK_TO_JSON expects [][]gosnmp.SnmpPDU input, got %T", value)
	}

	// 解析参数: ['{#MACRO1}', 'OID1', '0', '{#MACRO2}', 'OID2', '0', ...]
	if len(params)%3 != 0 {
		return nil, fmt.Errorf("invalid parameters for SNMP_WALK_TO_JSON, must be triplets of macro, oid, is_json_path")
	}

	type macroOid struct {
		macro   string
		oid     string
		walkIdx int
	}

	var mappings []macroOid
	for i := 0; i < len(params); i += 3 {
		mappings = append(mappings, macroOid{
			macro: params[i],
			oid:   strings.TrimPrefix(params[i+1], "."),
		})
	}

	// 确保 walk 的结果数量与 OID 映射数量匹配
	if len(allPdus) != len(mappings) {
		return nil, fmt.Errorf("number of SNMP walk results (%d) does not match number of OID mappings in parameters (%d)", len(allPdus), len(mappings))
	}
	for i := range mappings {
		mappings[i].walkIdx = i
	}

	// 使用 SNMP Index 作为 key 来聚合结果
	indexedResults := make(map[string]map[string]interface{})

	// 提取器函数
	extract := func(fullOID, baseOID string) string {
		fullOID = strings.TrimPrefix(fullOID, ".")
		if !strings.HasPrefix(fullOID, baseOID) {
			return ""
		}
		if len(fullOID) <= len(baseOID) {
			return ""
		}
		index := fullOID[len(baseOID):]
		return strings.TrimPrefix(index, ".")
	}

	for _, mapping := range mappings {
		pduSet := allPdus[mapping.walkIdx]
		for _, pdu := range pduSet {
			index := extract(pdu.Name, mapping.oid)
			if index == "" {
				continue
			}

			if _, exists := indexedResults[index]; !exists {
				indexedResults[index] = make(map[string]interface{})
				indexedResults[index]["{#SNMPINDEX}"] = index
			}

			// 转换 SNMP 值
			switch pdu.Type {
			case gosnmp.OctetString:
				indexedResults[index][mapping.macro] = string(pdu.Value.([]byte))
			default:
				indexedResults[index][mapping.macro] = gosnmp.ToBigInt(pdu.Value).String()
			}
		}
	}

	// 将 map 转换为 LLD JSON 所需的 slice 格式
	var lldData []map[string]interface{}
	for _, v := range indexedResults {
		lldData = append(lldData, v)
	}

	// Zabbix LLD 格式是一个包含 "data" 键的对象
	result := map[string]interface{}{
		"data": lldData,
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal discovery results to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

func ConvertToFloat64(value interface{}) (float64, error) {
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
		// 尝试解析为数字
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, nil
		}
		// 可能是十六进制
		if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
			if i, err := strconv.ParseInt(v[2:], 16, 64); err == nil {
				return float64(i), nil
			}
		}
		return 0, fmt.Errorf("cannot convert string '%s' to float64", v)
	case []byte:
		return ConvertToFloat64(string(v))
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

// getMaxCounterValue determines the maximum value for a counter based on its SNMP type.
func getMaxCounterValue(value interface{}) uint64 {
	// Default to a 32-bit counter max value, which is most common.
	// 2^32 - 1
	maxValue := uint64(4294967295)

	switch value.(type) {
	case uint64:
		// If the value is already a uint64, assume it's a 64-bit counter.
		// 2^64 - 1
		maxValue = uint64(18446744073709551615)
	}
	return maxValue
}

func applyJSONPath(value interface{}, params []string) (interface{}, error) {
	if len(params) == 0 {
		return nil, errors.New("JSONPath preprocessing requires a 'path' parameter")
	}
	path := params[0]

	var rawBytes []byte
	switch v := value.(type) {
	case string:
		rawBytes = []byte(v)
	case []byte:
		rawBytes = v
	default:
		return nil, fmt.Errorf("JSONPath requires string or []byte input, got %T", value)
	}

	var jsonData interface{}
	if err := json.Unmarshal(rawBytes, &jsonData); err != nil {
		return nil, fmt.Errorf("failed to parse input as JSON: %w", err)
	}

	result, err := jsonpath.JsonPathLookup(jsonData, path)
	if err != nil {
		return nil, fmt.Errorf("JSONPath lookup failed for path '%s': %w", path, err)
	}

	if result == nil {
		return nil, fmt.Errorf("JSONPath '%s' not found in JSON data", path)
	}

	// 如果结果是map或slice，Zabbix会将其序列化回JSON字符串
	if _, isMap := result.(map[string]interface{}); isMap {
		jsonBytes, _ := json.Marshal(result)
		return string(jsonBytes), nil
	}
	if _, isSlice := result.([]interface{}); isSlice {
		jsonBytes, _ := json.Marshal(result)
		return string(jsonBytes), nil
	}

	return result, nil
}

func wrapJavaScript(script string) string {
	trimmedScript := strings.TrimSpace(script)

	if strings.HasPrefix(trimmedScript, "function") || strings.HasPrefix(trimmedScript, "(function") {
		return script
	}

	// 自动添加 "return "。
	if !strings.Contains(trimmedScript, "return") && !strings.Contains(trimmedScript, ";") {
		trimmedScript = "return " + trimmedScript
	}

	// 使用IIFE（Immediately Invoked Function Expression）包裹脚本，注入 "value" 变量。
	return fmt.Sprintf("(function(value){ %s; })(value);", trimmedScript)
}

func applyJavaScript(value interface{}, params []string) (interface{}, error) {
	if len(params) == 0 {
		return nil, errors.New("JavaScript preprocessing requires a 'script' parameter")
	}
	script := params[0]

	// Zabbix 7.0+ 模板可能会有新的JS函数，比如iregsub，这里我们用Go的正则模拟
	// 这是一个简化的实现，仅用于支持当前模板
	if strings.Contains(script, "iregsub") {
		re := regexp.MustCompile(`iregsub\("([^"]+)",\s*"([^"]+)"\)`)
		matches := re.FindStringSubmatch(script)
		if len(matches) == 3 {
			pattern := matches[1]
			replacement := matches[2]
			strValue := fmt.Sprintf("%v", value)

			goRe, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex in iregsub: %w", err)
			}
			return goRe.ReplaceAllString(strValue, replacement), nil
		}
	}

	// 从缓存获取或编译脚本
	wrappedScript := wrapJavaScript(script)
	program, err := scriptCache.Get(wrappedScript)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to compile javascript: %w (original script: %q, wrapped: %q)",
			err, script, wrappedScript,
		)
	}

	// 为每次执行创建独立的、带安全控制的运行时
	vm := goja.New()

	// 设置执行超时
	const timeout = 5 * time.Second
	timer := time.AfterFunc(timeout, func() {
		vm.Interrupt(errors.New("javascript execution timed out"))
	})
	defer timer.Stop() // 确保定时器被清理
	valueToInject := value

	// 检查 value 是否为 Zabbix LLD JSON 字符串 "{"data": [...]}"
	if strVal, ok := value.(string); ok {
		trimmedVal := strings.TrimSpace(strVal)
		// 必须是对象格式，且看起来像 LLD 格式
		if strings.HasPrefix(trimmedVal, `{"data":`) && strings.HasSuffix(trimmedVal, "}") {
			// 尝试解析
			var lldWrapper map[string]interface{}
			if err := json.Unmarshal([]byte(trimmedVal), &lldWrapper); err == nil {
				if data, exists := lldWrapper["data"]; exists {
					// 解析成功，并且 'data' 键存在
					// 将要注入的值替换为 'data' 键的内容 (通常是一个 LLD 数组)
					valueToInject = data
					log.Printf("D! auto-unwrapped LLD JSON 'data' field for javascript preprocessing")
				}
			}
			// 如果解析失败或 'data' 键不存在, valueToInject 将保持为原始字符串，这是安全的
		}
	}

	// 注入输入值
	err = vm.Set("value", valueToInject)
	if err != nil {
		return nil, fmt.Errorf("failed to set 'value' in javascript vm: %w", err)
	}
	console := vm.NewObject()
	console.Set("log", func(call goja.FunctionCall) goja.Value {
		log.Printf("I! JS-LOG: %s", call.Argument(0).String())
		return goja.Undefined()
	})
	console.Set("error", func(call goja.FunctionCall) goja.Value {
		log.Printf("E! JS-ERROR: %s", call.Argument(0).String())
		return goja.Undefined()
	})
	console.Set("warn", func(call goja.FunctionCall) goja.Value {
		log.Printf("W! JS-WARN: %s", call.Argument(0).String())
		return goja.Undefined()
	})
	vm.Set("console", console)

	vm.Set("JSON", map[string]interface{}{
		"parse": func(s string) (interface{}, error) {
			var v interface{}
			err := json.Unmarshal([]byte(s), &v)
			return v, err
		},
		"stringify": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
	})

	// 执行预编译的程序
	result, err := vm.RunProgram(program)
	if err != nil {
		return nil, fmt.Errorf("javascript execution failed: %w", err)
	}

	// 从goja.Value转换回Go原生类型
	return result.Export(), nil
}
