package gnmi

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
)

func string2float32(base64Str string) (float32, error) {
	// 解码Base64字符串
	decodedBytes, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return 0.0, fmt.Errorf("base64 string %s decode error: %v", base64Str, err)
	}

	// 确保解码后的字节切片长度为4（32位）
	if len(decodedBytes) != 4 {
		return 0.0, fmt.Errorf("length after decoding is not 4 bytes, data type is not float32")
	}

	// 将字节切片转换为uint32
	bits := binary.BigEndian.Uint32(decodedBytes)

	// 将uint32转换为float32
	return math.Float32frombits(bits), nil
}

func bytes2float32(bytes []uint8) (float32, error) {
	if len(bytes) != 4 {
		return 0.0, fmt.Errorf("length after decoding is not 4 bytes, data type is not float32")
	}

	// 将字节切片转换为uint32
	bits := binary.BigEndian.Uint32(bytes)

	// 将uint32转换为float32
	return math.Float32frombits(bits), nil
}

// convertIEEEFloat32 decodes an ieeefloat32-encoded metric value. Depending on
// the device encoding the raw value arrives as a base64-encoded string
// (json/json_ietf) or a raw 4-byte slice (proto/bytes). A value of any other
// type, or a decode failure, returns an error instead of silently producing a
// misleading zero value.
func convertIEEEFloat32(raw interface{}) (float32, error) {
	switch v := raw.(type) {
	case string:
		return string2float32(v)
	case []uint8:
		return bytes2float32(v)
	default:
		return 0.0, fmt.Errorf("ieeefloat32 conversion: unsupported value type %T", raw)
	}
}
