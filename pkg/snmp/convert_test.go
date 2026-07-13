package snmp

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertValueStrict(t *testing.T) {
	tests := []struct {
		conversion string
		value      interface{}
		expected   interface{}
	}{
		{"", []byte("router"), "router"},
		{"string", []byte("router"), "router"},
		{"float", "speed: 34%", float64(34)},
		{"float(2)", "1234", float64(12.34)},
		{"int", "10k", int64(10 * 1024)},
		{"hwaddr", []byte{0, 17, 34, 51, 68, 85}, "00:11:22:33:44:55"},
		{"ipaddr", []byte{10, 95, 5, 53}, "10.95.5.53"},
		{"hextoint:BigEndian:uint16", []byte{0x12, 0x34}, uint16(0x1234)},
		{"percent", "98.5%", float64(98.5)},
	}
	for _, test := range tests {
		got, err := ConvertValueStrict(test.conversion, test.value)
		require.NoError(t, err)
		assert.Equal(t, test.expected, got)
	}
}

func TestConvertValueFallbackAndErrors(t *testing.T) {
	got, err := ConvertValue("float", "offline")
	require.NoError(t, err)
	assert.Equal(t, float64(0), got)

	_, err = ConvertValueStrict("float", "offline")
	require.Error(t, err)

	_, err = ConvertValueStrict("int", uint64(math.MaxInt64)+1)
	require.Error(t, err)

	_, err = ConvertValueStrict("hextoint:BigEndian:uint32", []byte{0x12, 0x34})
	require.Error(t, err)
}
