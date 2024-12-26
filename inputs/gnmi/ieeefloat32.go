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
