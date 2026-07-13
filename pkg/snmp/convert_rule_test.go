package snmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRules(t *testing.T) {
	rules := []ConvertRule{
		{Match: "offline", Value: int64(-1)},
		{Regex: `^fan:\s*(.*)$`, Extract: "$1", Conversion: "float"},
	}
	require.NoError(t, InitConvertRules(rules))

	fixed := MatchConvertRule(rules, []byte("offline"), "float")
	assert.True(t, fixed.Matched)
	assert.True(t, fixed.FixedValue)
	assert.Equal(t, int64(-1), fixed.Value)

	extracted := MatchConvertRule(rules, "fan: 34%", "int")
	assert.True(t, extracted.Matched)
	assert.False(t, extracted.FixedValue)
	assert.Equal(t, "34%", extracted.Value)
	assert.Equal(t, "float", extracted.Conversion)

	assert.False(t, MatchConvertRule(rules, "unknown", "float").Matched)
}

func TestInitConvertRulesErrors(t *testing.T) {
	tests := []ConvertRule{
		{},
		{Match: "a", Regex: "a"},
		{Regex: "("},
		{Match: "a", Extract: "$1"},
		{Regex: ".*", Extract: "$1", Value: int64(1)},
		{Match: "a", Value: int64(1), Conversion: "float"},
		{Match: "a", Value: []int{1}},
	}
	for i := range tests {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			require.Error(t, InitConvertRules([]ConvertRule{tests[i]}))
		})
	}
}
