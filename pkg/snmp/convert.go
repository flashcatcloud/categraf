package snmp

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
)

// ConvertValue applies a conversion while preserving legacy float and int fallbacks.
func ConvertValue(conversion string, value interface{}) (interface{}, error) {
	converted, err := ConvertValueStrict(conversion, value)
	if err == nil {
		return converted, nil
	}

	var decimals int
	if _, scanErr := fmt.Sscanf(conversion, "float(%d)", &decimals); scanErr == nil || conversion == "float" {
		log.Printf("E! failed to extract float: %v", err)
		return float64(0), nil
	}
	if conversion == "int" {
		switch value.(type) {
		case string, []byte:
			return int64(0), nil
		}
	}
	return converted, err
}

// ConvertValueStrict applies a conversion and returns parsing errors to the caller.
func ConvertValueStrict(conversion string, value interface{}) (interface{}, error) {
	if conversion == "" {
		if bytes, ok := value.([]byte); ok {
			return string(bytes), nil
		}
		return value, nil
	}
	if conversion == "string" {
		if bytes, ok := value.([]byte); ok {
			return string(bytes), nil
		}
		return fmt.Sprintf("%v", value), nil
	}

	var decimals int
	if _, err := fmt.Sscanf(conversion, "float(%d)", &decimals); err == nil || conversion == "float" {
		divisor := math.Pow10(decimals)
		switch typed := value.(type) {
		case float32:
			return float64(typed) / divisor, nil
		case float64:
			return typed / divisor, nil
		case int:
			return float64(typed) / divisor, nil
		case int8:
			return float64(typed) / divisor, nil
		case int16:
			return float64(typed) / divisor, nil
		case int32:
			return float64(typed) / divisor, nil
		case int64:
			return float64(typed) / divisor, nil
		case uint:
			return float64(typed) / divisor, nil
		case uint8:
			return float64(typed) / divisor, nil
		case uint16:
			return float64(typed) / divisor, nil
		case uint32:
			return float64(typed) / divisor, nil
		case uint64:
			return float64(typed) / divisor, nil
		case []byte:
			return convertFloatString(string(typed), divisor)
		case string:
			return convertFloatString(typed, divisor)
		default:
			return nil, fmt.Errorf("invalid type (%T) for float conversion", value)
		}
	}

	if conversion == "int" {
		switch typed := value.(type) {
		case float32:
			return int64(typed), nil
		case float64:
			return int64(typed), nil
		case int:
			return int64(typed), nil
		case int8:
			return int64(typed), nil
		case int16:
			return int64(typed), nil
		case int32:
			return int64(typed), nil
		case int64:
			return typed, nil
		case uint:
			return int64(typed), nil
		case uint8:
			return int64(typed), nil
		case uint16:
			return int64(typed), nil
		case uint32:
			return int64(typed), nil
		case uint64:
			if typed > math.MaxInt64 {
				return nil, fmt.Errorf("uint64 value %d overflows int64 conversion", typed)
			}
			return int64(typed), nil
		case []byte:
			return convertIntString(string(typed))
		case string:
			return convertIntString(typed)
		default:
			return nil, fmt.Errorf("invalid type (%T) for int conversion", value)
		}
	}

	if conversion == "hwaddr" {
		switch typed := value.(type) {
		case []byte:
			return net.HardwareAddr(typed).String(), nil
		case string:
			if parsed, err := net.ParseMAC(typed); err == nil {
				return parsed.String(), nil
			}
			if len(typed) == 6 {
				return net.HardwareAddr([]byte(typed)).String(), nil
			}
			return nil, fmt.Errorf("invalid hwaddr string %q", typed)
		default:
			return nil, fmt.Errorf("invalid type (%T) for hwaddr conversion", value)
		}
	}

	if strings.HasPrefix(conversion, "hextoint:") {
		return convertHexToInt(conversion, value)
	}

	if conversion == "ipaddr" {
		var bytes []byte
		switch typed := value.(type) {
		case []byte:
			bytes = typed
		case string:
			if parsed := net.ParseIP(typed); parsed != nil {
				return parsed.String(), nil
			}
			bytes = []byte(typed)
		default:
			return nil, fmt.Errorf("invalid type (%T) for ipaddr conversion", value)
		}
		if len(bytes) == 0 {
			return value, nil
		}
		if len(bytes) != 4 && len(bytes) != 16 {
			return nil, fmt.Errorf("invalid length (%d) for ipaddr conversion", len(bytes))
		}
		return net.IP(bytes).String(), nil
	}

	if conversion == "percent" {
		var text string
		switch typed := value.(type) {
		case []byte:
			text = string(typed)
		case string:
			text = typed
		default:
			return nil, fmt.Errorf("invalid type (%T) for percent conversion", value)
		}
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "%"))
		if text == "" {
			return nil, fmt.Errorf("empty value for percent conversion")
		}
		converted, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, fmt.Errorf("convert to percent error: %w, value: %s", err, text)
		}
		return converted, nil
	}

	if conversion == "hw4g" || strings.Contains(conversion, ",") && strings.Contains(conversion, ":") {
		return value, nil
	}
	return nil, fmt.Errorf("invalid conversion type '%s'", conversion)
}

func convertFloatString(value string, divisor float64) (float64, error) {
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed / divisor, nil
	}
	parsed, err := heuristicDataExtract(value)
	if err != nil {
		return 0, err
	}
	return parsed / divisor, nil
}

func convertIntString(value string) (int64, error) {
	lower := strings.ToLower(value)
	multiplier := int64(1)
	numeric := lower
	for _, unit := range []struct {
		suffix     string
		multiplier int64
	}{
		{"gb", 1024 * 1024 * 1024}, {"g", 1024 * 1024 * 1024},
		{"tb", 1024 * 1024 * 1024 * 1024}, {"t", 1024 * 1024 * 1024 * 1024},
		{"mb", 1024 * 1024}, {"m", 1024 * 1024},
		{"kb", 1024}, {"k", 1024},
	} {
		if strings.HasSuffix(lower, unit.suffix) {
			numeric = strings.TrimSuffix(lower, unit.suffix)
			multiplier = unit.multiplier
			break
		}
	}
	parsed, err := strconv.ParseInt(numeric, 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed * multiplier, nil
}

func convertHexToInt(conversion string, value interface{}) (interface{}, error) {
	parts := strings.Split(conversion, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid conversion type '%s'", conversion)
	}
	bytes, ok := value.([]byte)
	if !ok {
		return value, nil
	}
	widths := map[string]int{"uint16": 2, "uint32": 4, "uint64": 8}
	width, ok := widths[parts[2]]
	if !ok {
		return nil, fmt.Errorf("invalid bit value (%s) for hex to int conversion", parts[2])
	}
	if len(bytes) < width {
		return nil, fmt.Errorf("invalid length (%d) for %s hex to int conversion; need at least %d bytes", len(bytes), parts[2], width)
	}

	var order binary.ByteOrder
	switch parts[1] {
	case "LittleEndian":
		order = binary.LittleEndian
	case "BigEndian":
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf("invalid Endian value (%s) for hex to int conversion", parts[1])
	}
	switch parts[2] {
	case "uint16":
		return order.Uint16(bytes), nil
	case "uint32":
		return order.Uint32(bytes), nil
	default:
		return order.Uint64(bytes), nil
	}
}

func heuristicDataExtract(value string) (float64, error) {
	if value == "" {
		return 0, fmt.Errorf("empty string, cannot extract float value")
	}
	start, end := -1, -1
	hasDot, hasExponent, hasDigit := false, false, false
	for i := 0; i < len(value); i++ {
		char := value[i]
		if char > 127 {
			if start != -1 {
				end = i
				break
			}
			continue
		}
		if start == -1 {
			switch {
			case char >= '0' && char <= '9':
				start, hasDigit = i, true
			case char == '-' || char == '+':
				if i+1 < len(value) && ((value[i+1] >= '0' && value[i+1] <= '9') || value[i+1] == '.') {
					start = i
				}
			case char == '.' && i+1 < len(value) && value[i+1] >= '0' && value[i+1] <= '9':
				start, hasDot = i, true
			}
			continue
		}
		switch {
		case char >= '0' && char <= '9':
			hasDigit = true
		case char == '.' && !hasDot && !hasExponent:
			hasDot = true
		case (char == 'e' || char == 'E') && !hasExponent && hasDigit:
			hasExponent = true
			if i+1 < len(value) && (value[i+1] == '+' || value[i+1] == '-') {
				i++
			}
		default:
			end = i
			i = len(value)
		}
	}
	if start != -1 && end == -1 {
		end = len(value)
	}
	if start == -1 || !hasDigit {
		return 0, fmt.Errorf("no valid number found in string: %s", value)
	}
	number := value[start:end]
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse number %q from string %q: %w", number, value, err)
	}
	return parsed, nil
}
