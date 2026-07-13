package snmp

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snmppkg "flashcat.cloud/categraf/pkg/snmp"
)

func TestFieldConvertRules(t *testing.T) {
	tests := []struct {
		name        string
		conversion  string
		rules       []ConvertRule
		value       interface{}
		expected    interface{}
		expectError bool
	}{
		{
			name:       "exact mapping",
			conversion: "float",
			rules:      []ConvertRule{{Match: "offline", Value: int64(-1)}},
			value:      []byte("offline"),
			expected:   int64(-1),
		},
		{
			name:       "legacy fallback",
			conversion: "float",
			rules:      []ConvertRule{{Match: "offline", Value: int64(-1)}},
			value:      "34%",
			expected:   float64(34),
		},
		{
			name:       "regex extraction",
			conversion: "int",
			rules:      []ConvertRule{{Regex: `^fan:\s*(.*)$`, Extract: "$1", Conversion: "float"}},
			value:      "fan: 34%",
			expected:   float64(34),
		},
		{
			name:       "first match wins",
			conversion: "float",
			rules: []ConvertRule{
				{Regex: ".*", Value: int64(1)},
				{Regex: ".*", Value: int64(2)},
			},
			value:    "anything",
			expected: int64(1),
		},
		{
			name:        "strict matched conversion",
			conversion:  "float",
			rules:       []ConvertRule{{Match: "bad", Conversion: "float"}},
			value:       "bad",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := Field{
				Name:         "speed",
				Oid:          "1.2.3.4",
				Conversion:   tt.conversion,
				ConvertRules: tt.rules,
			}
			require.NoError(t, field.init(mockTranslator{}))

			got, err := field.convertValue(mockTranslator{}, gosnmp.SnmpPDU{Value: tt.value})
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "convert_rule")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestLegacyFieldConversionsRemainAvailable(t *testing.T) {
	tests := []struct {
		conversion string
		value      interface{}
		expected   interface{}
	}{
		{conversion: "byte", value: []byte("1 MB"), expected: float64(1000 * 1000)},
		{conversion: "enum", value: 1, expected: "enum"},
		{conversion: "percent", value: "36%", expected: float64(36)},
		{conversion: "float", value: "offline", expected: float64(0)},
	}
	for _, test := range tests {
		got, err := fieldConvert(mockTranslator{}, test.conversion, gosnmp.SnmpPDU{Value: test.value})
		require.NoError(t, err)
		assert.Equal(t, test.expected, got)
	}
}

func TestTableBuildWithConvertRules(t *testing.T) {
	config := `
name = "fan"
oid = "1.2.3"
index_as_tag = true

[[field]]
oid = "1.2.3.4"
name = "speed"
conversion = "float"

[[field.convert_rule]]
match = "offline"
value = -1
`
	var table Table
	require.NoError(t, toml.Unmarshal([]byte(config), &table))
	require.NoError(t, table.Init(mockTranslator{}))

	connection := &mockSnmpConnection{
		get: func([]string) (*gosnmp.SnmpPacket, error) {
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.4", Value: []byte("offline")}}}, nil
		},
		walk: func(_ string, fn gosnmp.WalkFunc) error {
			require.NoError(t, fn(gosnmp.SnmpPDU{Name: ".1.2.3.4.1", Value: []byte("offline")}))
			require.NoError(t, fn(gosnmp.SnmpPDU{Name: ".1.2.3.4.2", Value: []byte("34%")}))
			return nil
		},
	}

	got, err := table.Build(connection, false, mockTranslator{})
	require.NoError(t, err)
	require.Len(t, got.Rows, 1)
	assert.Equal(t, int64(-1), got.Rows[0].Fields["speed"])

	got, err = table.Build(connection, true, mockTranslator{})
	require.NoError(t, err)
	require.Len(t, got.Rows, 2)
	values := map[string]interface{}{}
	for _, row := range got.Rows {
		values[row.Tags["index"]] = row.Fields["speed"]
	}
	assert.Equal(t, int64(-1), values["1"])
	assert.Equal(t, float64(34), values["2"])

	connection.walk = func(_ string, fn gosnmp.WalkFunc) error {
		return fn(gosnmp.SnmpPDU{Name: ".1.2.3.4.1", Value: []byte("fan: bad")})
	}
	table.Fields[0].ConvertRules = []ConvertRule{{Regex: `^fan:\s*(.*)$`, Extract: "$1", Conversion: "float"}}
	require.NoError(t, snmppkg.InitConvertRules(table.Fields[0].ConvertRules))
	_, err = table.Build(connection, true, mockTranslator{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "convert_rule")
}

type mockTranslator struct{}

func (mockTranslator) SnmpTranslate(oid string) (string, string, string, string, error) {
	return "", oid, "mockName", "", nil
}

func (mockTranslator) SnmpTable(oid string) (string, string, string, []Field, error) {
	return "", oid, "mockTable", nil, nil
}

func (mockTranslator) SnmpFormatEnum(string, interface{}, bool) (string, error) {
	return "enum", nil
}

func (mockTranslator) SetDebugMode(bool) {}

type mockSnmpConnection struct {
	get  func([]string) (*gosnmp.SnmpPacket, error)
	walk func(string, gosnmp.WalkFunc) error
}

func (*mockSnmpConnection) Host() string { return "127.0.0.1" }

func (m *mockSnmpConnection) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	return m.get(oids)
}

func (m *mockSnmpConnection) Walk(oid string, fn gosnmp.WalkFunc) error {
	return m.walk(oid, fn)
}
