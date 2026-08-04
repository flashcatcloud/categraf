package snmp

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"flashcat.cloud/categraf/config"
	snmppkg "flashcat.cloud/categraf/pkg/snmp"
	"flashcat.cloud/categraf/types"
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

func TestTableBuildPartialContinuesAfterValueWalkError(t *testing.T) {
	table := Table{
		Name:        "partial",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "good_before", Oid: ".1.2.3.1", Conversion: "int"},
			{Name: "bad", Oid: ".1.2.3.2", Conversion: "int"},
			{Name: "good_after", Oid: ".1.2.3.3", Conversion: "int"},
		},
	}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "10"})
			case ".1.2.3.2":
				return errors.New("timeout")
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.1", Value: "30"})
			default:
				return nil
			}
		},
	}

	rt, err := table.Build(mockConn, true, mockTranslator{})
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, int64(10), rt.Rows[0].Fields["good_before"])
	assert.Equal(t, int64(30), rt.Rows[0].Fields["good_after"])
	assert.NotContains(t, rt.Rows[0].Fields, "bad")
}

func TestTableBuildPartialDiscardsPartialWalkValuesAfterError(t *testing.T) {
	table := Table{
		Name:        "partial_atomic",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "partial", Oid: ".1.2.3.1", Conversion: "int"},
			{Name: "good_after", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				require.NoError(t, fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "10"}))
				return errors.New("timeout")
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "20"})
			default:
				return nil
			}
		},
	}

	rt, err := table.Build(mockConn, true, mockTranslator{})
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.NotContains(t, rt.Rows[0].Fields, "partial")
	assert.Equal(t, int64(20), rt.Rows[0].Fields["good_after"])
}

func TestTableBuildLegacyStillFailsOnValueWalkError(t *testing.T) {
	table := Table{
		Name:        "legacy",
		ErrorPolicy: ErrorPolicyLegacy,
		Fields: []Field{
			{Name: "bad", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return errors.New("timeout")
		},
	}

	_, err := table.Build(mockConn, true, mockTranslator{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "performing bulk walk")
}

func TestTableBuildPartialTagDependencyFailClosed(t *testing.T) {
	table := Table{
		Name:        "tag_dependency",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.1", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return errors.New("timeout")
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			default:
				return nil
			}
		},
	}

	rt, err := table.Build(mockConn, true, mockTranslator{})
	require.NoError(t, err)
	assert.Empty(t, rt.Rows)
}

func TestTableBuildPartialSecondaryProviderUnknownSkipsConsumer(t *testing.T) {
	table := Table{
		Name:        "secondary",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "secondary_map", Oid: ".1.2.3.1", SecondaryIndexTable: true},
			{Name: "primary_value", Oid: ".1.2.3.2", Conversion: "int"},
			{Name: "secondary_value", Oid: ".1.2.3.3", Conversion: "int", SecondaryIndexUse: true},
		},
	}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return errors.New("timeout")
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.7", Value: "70"})
			default:
				return nil
			}
		},
	}

	rt, err := table.Build(mockConn, true, mockTranslator{})
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, int64(10), rt.Rows[0].Fields["primary_value"])
	assert.NotContains(t, rt.Rows[0].Fields, "secondary_value")
}

func TestTableBuildPartialSecondaryProviderDoesNotSatisfyCurrentValueGate(t *testing.T) {
	table := Table{
		Name:        "secondary_provider_only",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "secondary_map", Oid: ".1.2.3.1", SecondaryIndexTable: true},
		},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
		},
	}

	result, err := table.BuildResultWithCache(conn, true, mockTranslator{}, tableBuildOptions{now: time.Now()})
	require.NoError(t, err)
	assert.Empty(t, result.Table.Rows)
	assert.Equal(t, 1, result.Stats.SkippedRows["no_current_value"])
}

func TestTableBuildPartialFilterDependencyFailClosed(t *testing.T) {
	table := Table{
		Name:        "filter_dependency",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "status", Oid: ".1.2.3.1"},
			{Name: "value", Oid: ".1.2.3.2", Conversion: "int"},
		},
		Filters: []string{"status:up"},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return errors.New("timeout")
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			default:
				return nil
			}
		},
	}

	rt, err := table.Build(mockConn, true, mockTranslator{})
	require.NoError(t, err)
	assert.Empty(t, rt.Rows)
}

func TestTableBuildPartialFilterKnownAbsentDeniesRow(t *testing.T) {
	table := Table{
		Name:        "filter_known_absent",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "status", Oid: ".1.2.3.1"},
			{Name: "value", Oid: ".1.2.3.2", Conversion: "int"},
		},
		Filters: []string{"status:up"},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return nil
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			default:
				return nil
			}
		},
	}

	result, err := table.BuildResultWithCache(conn, true, mockTranslator{}, tableBuildOptions{now: time.Now()})
	require.NoError(t, err)
	assert.Empty(t, result.Table.Rows)
	assert.Equal(t, 1, result.Stats.SkippedRows["filter_deny"])
	assert.Zero(t, result.Stats.SkippedRows["filter_unknown"])
}

func TestTableBuildPartialCurrentFilterValueSatisfiesCurrentValueGate(t *testing.T) {
	table := Table{
		Name:        "filter_value",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "status", Oid: ".1.2.3.1"},
		},
		Filters: []string{"status:up"},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "up"})
		},
	}

	result, err := table.BuildResultWithCache(conn, true, mockTranslator{}, tableBuildOptions{now: time.Now()})
	require.NoError(t, err)
	require.Len(t, result.Table.Rows, 1)
	assert.Equal(t, "up", result.Table.Rows[0].Fields["status"])
	assert.Zero(t, result.Stats.SkippedRows["no_current_value"])
}

func TestTableBuildPartialFatalSNMPErrorsStopBuild(t *testing.T) {
	table := Table{
		Name:        "fatal",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "bad", Oid: ".1.2.3.1", Conversion: "int"},
			{Name: "good_after", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}
	calls := 0
	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			calls++
			return gosnmp.ErrWrongDigest
		},
	}

	_, err := table.Build(mockConn, true, mockTranslator{})
	require.Error(t, err)
	assert.True(t, isFatalSNMPError(err))
	assert.Equal(t, 1, calls)
}

func TestTableBuildPartialFatalErrorReturnsBuildStats(t *testing.T) {
	table := Table{
		Name:        "fatal_stats",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "bad", Oid: ".1.2.3.1", Conversion: "int"},
		},
	}
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return gosnmp.ErrWrongDigest
		},
	}

	result, err := table.BuildResultWithCache(conn, true, mockTranslator{}, tableBuildOptions{now: time.Now()})
	require.Error(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "snmp_fatal", result.Stats.FatalClass)
	require.Len(t, result.Stats.FieldErrors, 1)
	assert.True(t, result.Stats.FieldErrors[0].Fatal)
	assert.Equal(t, "fatal", result.Stats.FieldErrors[0].Reason)
	assert.False(t, result.Stats.isPartialResult())
}

func TestTableBuildPartialGetFatalSNMPErrorsStopBuild(t *testing.T) {
	table := Table{
		Name:        "fatal_get",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "bad", Oid: ".1.2.3.1", Conversion: "int"},
			{Name: "good_after", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}
	calls := 0
	mockConn := &mockSnmpConnection{
		get: func(oids []string) (*gosnmp.SnmpPacket, error) {
			calls++
			return nil, gosnmp.ErrInvalidMsgs
		},
	}

	_, err := table.Build(mockConn, false, mockTranslator{})
	require.Error(t, err)
	assert.True(t, isFatalSNMPError(err))
	assert.Equal(t, 1, calls)
}

func TestTableInitRejectsInvalidFilterRegex(t *testing.T) {
	table := Table{
		Name: "bad_filter",
		Fields: []Field{
			{Name: "status", Oid: ".1.2.3.1"},
		},
		Filters: []string{"status:["},
	}

	err := table.Init(mockTranslator{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "regexp compile")
}

func TestTableBuildPartialFilterUsesExactBinding(t *testing.T) {
	table := Table{
		Name:        "filter_exact",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "status", Oid: ".1.2.3.1"},
			{Name: "statusText", Oid: ".1.2.3.2"},
			{Name: "value", Oid: ".1.2.3.3", Conversion: "int"},
		},
		Filters: []string{"status:up"},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "up"})
			case ".1.2.3.2":
				return errors.New("timeout")
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.1", Value: "10"})
			default:
				return nil
			}
		},
	}

	rt, err := table.Build(mockConn, true, mockTranslator{})
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, int64(10), rt.Rows[0].Fields["value"])
}

func TestTableBuildPartialSkipsRowsMissingTagIdentity(t *testing.T) {
	table := Table{
		Name:        "row_identity",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.1", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "eth0"})
			case ".1.2.3.2":
				if err := fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"}); err != nil {
					return err
				}
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.2", Value: "20"})
			default:
				return nil
			}
		},
	}

	rt, err := table.Build(mockConn, true, mockTranslator{})
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "eth0", rt.Rows[0].Tags["ifName"])
	assert.Equal(t, int64(10), rt.Rows[0].Fields["octets"])
}

func TestTableBuildPartialTopLevelRequiresCompleteTagIdentity(t *testing.T) {
	table := Table{
		Name:        "top_identity",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "source", Oid: ".1.2.3.1", IsTag: true},
			{Name: "location", Oid: ".1.2.3.2", IsTag: true},
			{Name: "uptime", Oid: ".1.2.3.3", Conversion: "int"},
		},
	}

	mockConn := &mockSnmpConnection{
		host: "127.0.0.1",
		get: func(oids []string) (*gosnmp.SnmpPacket, error) {
			switch oids[0] {
			case ".1.2.3.1":
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.1", Value: "router-a"}}}, nil
			case ".1.2.3.2":
				return nil, errors.New("timeout")
			case ".1.2.3.3":
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.3", Value: "10"}}}, nil
			default:
				return nil, nil
			}
		},
	}

	rt, err := table.Build(mockConn, false, mockTranslator{})
	require.NoError(t, err)
	assert.Empty(t, rt.Rows)
}

func TestGatherTopLevelDependenciesSurviveMetricRowGate(t *testing.T) {
	table := Table{
		Name:        "top_identity",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "source", Oid: ".1.2.3.1", IsTag: true},
			{Name: "location", Oid: ".1.2.3.2", IsTag: true},
			{Name: "uptime", Oid: ".1.2.3.3", Conversion: "int"},
		},
	}
	ins := &Instance{translator: mockTranslator{}}
	rt := newAgentRuntime(time.Now())
	conn := &mockSnmpConnection{
		host: "127.0.0.1",
		get: func(oids []string) (*gosnmp.SnmpPacket, error) {
			switch oids[0] {
			case ".1.2.3.1":
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.1", Value: "router-a"}}}, nil
			case ".1.2.3.2":
				return nil, errors.New("timeout")
			case ".1.2.3.3":
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.3", Value: "10"}}}, nil
			default:
				return nil, nil
			}
		},
	}
	topTags := map[string]string{}
	err := ins.gatherTable(types.NewSampleList(), conn, rt, table, -1, topTags, map[string]string{}, false)
	require.NoError(t, err)
	assert.Equal(t, "router-a", topTags["source"])
	assert.NotContains(t, topTags, "location")
}

func TestGatherCachedTopLevelTagDrivesInheritedTableThroughRowGate(t *testing.T) {
	topTable := Table{
		Name:        "top_identity",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "source", Oid: ".1.2.3.1", IsTag: true},
			{Name: "location", Oid: ".1.2.3.2", IsTag: true},
			{Name: "uptime", Oid: ".1.2.3.3", Conversion: "int"},
		},
	}
	inheritTable := Table{
		Name:        "iface",
		ErrorPolicy: ErrorPolicyPartial,
		InheritTags: []string{
			"source",
		},
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.4", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.5", Conversion: "int"},
		},
	}
	require.NoError(t, topTable.Init(mockTranslator{}))
	require.NoError(t, inheritTable.Init(mockTranslator{}))

	rt := newAgentRuntime(time.Now())
	rt.configureDependencyCache(10*time.Minute, 0)
	ins := &Instance{
		AgentHostTag: "agent_host",
		translator:   mockTranslator{},
	}
	topTags := map[string]string{}
	extraTags := map[string]string{}
	slist := types.NewSampleList()

	conn := &mockSnmpConnection{
		host: "127.0.0.1",
		get: func(oids []string) (*gosnmp.SnmpPacket, error) {
			switch oids[0] {
			case ".1.2.3.1":
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.1", Value: "router-a"}}}, nil
			case ".1.2.3.2":
				return nil, errors.New("timeout")
			case ".1.2.3.3":
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.3", Value: "10"}}}, nil
			default:
				return nil, nil
			}
		},
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return nil
		},
	}
	require.NoError(t, ins.gatherTable(slist, conn, rt, topTable, -1, topTags, extraTags, false))
	require.Equal(t, "router-a", topTags["source"])

	conn.get = func(oids []string) (*gosnmp.SnmpPacket, error) {
		switch oids[0] {
		case ".1.2.3.1":
			return nil, errors.New("timeout")
		case ".1.2.3.2":
			return nil, errors.New("timeout")
		case ".1.2.3.3":
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.3", Value: "20"}}}, nil
		default:
			return nil, nil
		}
	}
	conn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.4":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.4.1", Value: "eth0"})
		case ".1.2.3.5":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.5.1", Value: "30"})
		default:
			return nil
		}
	}
	topTags = map[string]string{}
	slist = types.NewSampleList()
	require.NoError(t, ins.gatherTable(slist, conn, rt, topTable, -1, topTags, extraTags, false))
	require.Equal(t, "router-a", topTags["source"])
	require.NotContains(t, topTags, "location")
	require.NoError(t, ins.gatherTable(slist, conn, rt, inheritTable, 0, topTags, extraTags, true))

	sample, ok := findSample(slist, "snmp_iface_octets")
	require.True(t, ok)
	assert.Equal(t, "router-a", sample.Labels["source"])
	assert.Equal(t, "eth0", sample.Labels["ifName"])
	assert.Equal(t, int64(30), sample.Value)
}

func TestTableBuildPartialUsesCachedTagDependency(t *testing.T) {
	table := Table{
		Name:        "tag_cache",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.1", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}
	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: base}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "eth0"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	mockConn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return errors.New("timeout")
		case ".1.2.3.2":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "20"})
		default:
			return nil
		}
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "eth0", rt.Rows[0].Tags["ifName"])
	assert.Equal(t, int64(20), rt.Rows[0].Fields["octets"])

	opts.now = base.Add(11 * time.Minute)
	rt, err = table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	assert.Empty(t, rt.Rows)
}

func TestTableBuildResultRecordsDependencyStatesAndCacheEvents(t *testing.T) {
	table := Table{
		Name:        "stats",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.1", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}
	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: base}
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "eth0"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildResultWithCache(conn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	conn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return errors.New("timeout")
		case ".1.2.3.2":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "20"})
		default:
			return nil
		}
	}
	opts.now = base.Add(time.Minute)
	result, err := table.BuildResultWithCache(conn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Stats.FailedFields)
	require.NotEmpty(t, result.Stats.FieldErrors)
	assert.Equal(t, "ifName", result.Stats.FieldErrors[0].FieldName)
	require.NotEmpty(t, result.Stats.DependencyStates)
	assert.Equal(t, DependencyStateCached, result.Stats.DependencyStates[0].State)
	require.NotEmpty(t, result.Stats.CacheEvents)
	assert.Equal(t, dependencyCacheResultHit, result.Stats.CacheEvents[0].Result)
}

func TestTableBuildPartialTopLevelTagConversionErrorUsesCacheWithoutRefreshingBadValue(t *testing.T) {
	table := Table{
		Name:        "top_tag_cache",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{
				Name:       "source",
				Oid:        ".1.2.3.1",
				IsTag:      true,
				Conversion: "int",
				ConvertRules: []ConvertRule{
					{Match: "bad", Conversion: "int"},
				},
			},
		},
	}
	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(false), topLevel: true, now: base}

	mockConn := &mockSnmpConnection{
		host: "127.0.0.1",
		get: func(oids []string) (*gosnmp.SnmpPacket, error) {
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.1", Value: "1"}}}, nil
		},
	}
	_, err := table.BuildWithCache(mockConn, false, mockTranslator{}, opts)
	require.NoError(t, err)

	mockConn.get = func(oids []string) (*gosnmp.SnmpPacket, error) {
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.1", Value: "bad"}}}, nil
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(mockConn, false, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "1", rt.Rows[0].Tags["source"])

	mockConn.get = func(oids []string) (*gosnmp.SnmpPacket, error) {
		return nil, errors.New("timeout")
	}
	opts.now = base.Add(2 * time.Minute)
	rt, err = table.BuildWithCache(mockConn, false, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "1", rt.Rows[0].Tags["source"])
}

func TestTableBuildPartialDependencyCacheDisabledDoesNotWrite(t *testing.T) {
	table := Table{
		Name:        "tag_cache_disabled",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.1", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}
	cache := newDependencyCache(0)
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: time.Now()}

	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "eth0"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	mockConn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return errors.New("timeout")
		case ".1.2.3.2":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "20"})
		default:
			return nil
		}
	}
	rt, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	assert.Empty(t, rt.Rows)
}

func TestTableBuildPartialIsolatesDependencyCacheForSameNameTables(t *testing.T) {
	tableA := Table{
		Name:        "same",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.1", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.2", Conversion: "int"},
		},
	}
	tableB := Table{
		Name:        "same",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "ifName", Oid: ".1.2.3.3", IsTag: true},
			{Name: "octets", Oid: ".1.2.3.4", Conversion: "int"},
		},
	}
	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "eth-a"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.1", Value: "eth-b"})
			case ".1.2.3.4":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.4.1", Value: "20"})
			default:
				return nil
			}
		},
	}
	_, err := tableA.BuildWithCache(conn, true, mockTranslator{}, tableBuildOptions{dependencyCache: cache, tableID: tableA.cacheIDWithIndex(true, 0), tableIndex: 0, now: base})
	require.NoError(t, err)
	_, err = tableB.BuildWithCache(conn, true, mockTranslator{}, tableBuildOptions{dependencyCache: cache, tableID: tableB.cacheIDWithIndex(true, 1), tableIndex: 1, now: base})
	require.NoError(t, err)

	conn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return errors.New("timeout")
		case ".1.2.3.2":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "11"})
		default:
			return nil
		}
	}
	rt, err := tableA.BuildWithCache(conn, true, mockTranslator{}, tableBuildOptions{dependencyCache: cache, tableID: tableA.cacheIDWithIndex(true, 0), tableIndex: 0, now: base.Add(time.Minute)})
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "eth-a", rt.Rows[0].Tags["ifName"])
	assert.Equal(t, int64(11), rt.Rows[0].Fields["octets"])
}

func TestTableBuildPartialUsesCachedFilterDependencyWithoutEmittingCachedValue(t *testing.T) {
	table := Table{
		Name:        "filter_cache",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "status", Oid: ".1.2.3.1"},
			{Name: "value", Oid: ".1.2.3.2", Conversion: "int"},
		},
		Filters: []string{"status:up"},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: base}
	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "up"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	mockConn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return errors.New("timeout")
		case ".1.2.3.2":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "20"})
		default:
			return nil
		}
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, int64(20), rt.Rows[0].Fields["value"])
	assert.NotContains(t, rt.Rows[0].Fields, "status")
}

func TestTableBuildPartialUsesCachedSecondaryIndexTagDependency(t *testing.T) {
	table := Table{
		Name:        "secondary_tag_cache",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "secondary_map", Oid: ".1.2.3.1", SecondaryIndexTable: true},
			{Name: "ifName", Oid: ".1.2.3.2", IsTag: true, SecondaryIndexUse: true},
			{Name: "value", Oid: ".1.2.3.3", Conversion: "int"},
		},
	}
	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: base}
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.7", Value: "eth0"})
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.1", Value: "10"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildWithCache(conn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	conn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
		case ".1.2.3.2":
			return errors.New("timeout")
		case ".1.2.3.3":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.1", Value: "11"})
		default:
			return nil
		}
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(conn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "eth0", rt.Rows[0].Tags["ifName"])
	assert.Equal(t, int64(11), rt.Rows[0].Fields["value"])
}

func TestTableBuildPartialUsesCachedSecondaryIndexTagWithOuterJoin(t *testing.T) {
	table := Table{
		Name:        "secondary_tag_outer_cache",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "secondary_map", Oid: ".1.2.3.1", SecondaryIndexTable: true, SecondaryOuterJoin: true},
			{Name: "ifName", Oid: ".1.2.3.2", IsTag: true, SecondaryIndexUse: true},
			{Name: "value", Oid: ".1.2.3.3", Conversion: "int"},
		},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: base}
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.7", Value: "eth0"})
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.1", Value: "10"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildWithCache(conn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	conn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
		case ".1.2.3.2":
			return errors.New("timeout")
		case ".1.2.3.3":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.1", Value: "11"})
		default:
			return nil
		}
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(conn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "eth0", rt.Rows[0].Tags["ifName"])
	assert.Equal(t, int64(11), rt.Rows[0].Fields["value"])
	assert.NotContains(t, rt.Rows[0].Tags, "index")
}

func TestTableBuildPartialUsesCachedFilterDependencyWithSecondaryIndexUse(t *testing.T) {
	table := Table{
		Name:             "filter_secondary_cache",
		ErrorPolicy:      ErrorPolicyPartial,
		FilterExpression: "status",
		Fields: []Field{
			{Name: "secondary_map", Oid: ".1.2.3.1", SecondaryIndexTable: true},
			{Name: "value", Oid: ".1.2.3.2", Conversion: "int"},
			{Name: "status", Oid: ".1.2.3.3", SecondaryIndexUse: true},
		},
		Filters: []string{"status:status:up"},
	}
	require.NoError(t, table.Init(mockTranslator{}))

	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: base}
	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.7", Value: "up"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	mockConn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
		case ".1.2.3.2":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "11"})
		case ".1.2.3.3":
			return errors.New("timeout")
		default:
			return nil
		}
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, int64(11), rt.Rows[0].Fields["value"])
	assert.NotContains(t, rt.Rows[0].Fields, "status")
}

func TestTableBuildPartialUsesCachedSecondaryMapping(t *testing.T) {
	table := Table{
		Name:        "secondary_cache",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "secondary_map", Oid: ".1.2.3.1", SecondaryIndexTable: true},
			{Name: "primary_value", Oid: ".1.2.3.2", Conversion: "int"},
			{Name: "secondary_value", Oid: ".1.2.3.3", Conversion: "int", SecondaryIndexUse: true},
		},
	}
	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(true), now: base}
	mockConn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			switch oid {
			case ".1.2.3.1":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.1.1", Value: "7"})
			case ".1.2.3.2":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "10"})
			case ".1.2.3.3":
				return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.7", Value: "70"})
			default:
				return nil
			}
		},
	}
	_, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)

	mockConn.walk = func(oid string, fn gosnmp.WalkFunc) error {
		switch oid {
		case ".1.2.3.1":
			return errors.New("timeout")
		case ".1.2.3.2":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.2.1", Value: "11"})
		case ".1.2.3.3":
			return fn(gosnmp.SnmpPDU{Name: ".1.2.3.3.7", Value: "71"})
		default:
			return nil
		}
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(mockConn, true, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, int64(11), rt.Rows[0].Fields["primary_value"])
	assert.Equal(t, int64(71), rt.Rows[0].Fields["secondary_value"])
	assert.NotContains(t, rt.Rows[0].Fields, "secondary_map")
}

func TestTableBuildPartialUsesCachedTopLevelTag(t *testing.T) {
	table := Table{
		Name:        "top",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "source", Oid: ".1.2.3.1", IsTag: true},
		},
	}
	cache := newDependencyCache(10 * time.Minute)
	base := time.Now()
	opts := tableBuildOptions{dependencyCache: cache, tableID: table.cacheID(false), topLevel: true, now: base}
	mockConn := &mockSnmpConnection{
		host: "127.0.0.1",
		get: func(oids []string) (*gosnmp.SnmpPacket, error) {
			return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.2.3.1", Value: "router-a"}}}, nil
		},
	}
	_, err := table.BuildWithCache(mockConn, false, mockTranslator{}, opts)
	require.NoError(t, err)

	mockConn.get = func(oids []string) (*gosnmp.SnmpPacket, error) {
		return nil, errors.New("timeout")
	}
	opts.now = base.Add(time.Minute)
	rt, err := table.BuildWithCache(mockConn, false, mockTranslator{}, opts)
	require.NoError(t, err)
	require.Len(t, rt.Rows, 1)
	assert.Equal(t, "router-a", rt.Rows[0].Tags["source"])
}

func TestTableBuildDoesNotMutateSecondaryProviderOrder(t *testing.T) {
	table := Table{
		Name:        "secondary_order",
		ErrorPolicy: ErrorPolicyPartial,
		Fields: []Field{
			{Name: "value", Oid: ".1.2.3.2", Conversion: "int"},
			{Name: "secondary_map", Oid: ".1.2.3.1", SecondaryIndexTable: true},
			{Name: "secondary_value", Oid: ".1.2.3.3", Conversion: "int", SecondaryIndexUse: true},
		},
	}
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return nil
		},
	}

	_, err := table.Build(conn, true, mockTranslator{})
	require.NoError(t, err)
	assert.Equal(t, "value", table.Fields[0].Name)
	assert.Equal(t, "secondary_map", table.Fields[1].Name)
}

func TestDependencyCacheDisablePreventsConcurrentWritesAfterDrop(t *testing.T) {
	cache := newDependencyCache(10 * time.Minute)
	cache.disable()
	cache.replaceTableField("table", "ifName", map[string]interface{}{".1": "eth0"}, time.Now())

	_, ok, _ := cache.tableField("table", "ifName", time.Now())
	assert.False(t, ok)
}

func TestPrepareAgentForGatherRespectsDisableUpWhenUnhealthy(t *testing.T) {
	ins := &Instance{
		Agents:        []string{"127.0.0.1"},
		DisableUp:     true,
		agentRuntimes: []*agentRuntime{newAgentRuntime(time.Now())},
	}
	rt := ins.agentRuntimes[0]
	rt.mu.Lock()
	rt.healthy = false
	rt.nextProbeAt = time.Now().Add(time.Hour)
	rt.mu.Unlock()

	slist := types.NewSampleList()
	collect, override := ins.prepareAgentForGather(slist, 0, "127.0.0.1", map[string]string{})

	assert.False(t, collect)
	assert.Nil(t, override)
	assert.Equal(t, 0, slist.Len())
}

func TestGatherPushesHealthStateWhenUnhealthyCollectionIsSkipped(t *testing.T) {
	ins := &Instance{
		Agents:           []string{"127.0.0.1"},
		DisableUp:        true,
		MaxFailCount:     3,
		RecoveryInterval: config.Duration(time.Minute),
		agentRuntimes:    []*agentRuntime{newAgentRuntime(time.Now())},
	}
	rt := ins.agentRuntimes[0]
	rt.mu.Lock()
	rt.healthy = false
	rt.nextProbeAt = time.Now().Add(time.Hour)
	rt.mu.Unlock()

	slist := types.NewSampleList()
	ins.Gather(slist)

	sample, ok := findSample(slist, "snmp_health_state")
	require.True(t, ok)
	assert.Equal(t, 0, sample.Value)
}

func TestRuntimeConnectionWalkUsesOnFinishAsResponseEvidence(t *testing.T) {
	rt := newAgentRuntime(time.Now())
	rt.consecutiveFails = 2
	rt.gatherStats = collectionStats{}
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			rt.stats.recordSent()
			rt.stats.recordRecv()
			rt.stats.recordFinish()
			return nil
		},
	}
	rc := &runtimeConnection{rt: rt, conn: conn}

	require.NoError(t, rc.Walk(".1.2.3", func(pdu gosnmp.SnmpPDU) error { return nil }))
	assert.Equal(t, 1, rt.gatherStats.requestsSent)
	assert.Equal(t, 1, rt.gatherStats.rawResponses)
	assert.Equal(t, 1, rt.gatherStats.responsesObserved)
	rt.completeGather(3, time.Minute)

	rt.mu.Lock()
	defer rt.mu.Unlock()
	assert.True(t, rt.healthy)
	assert.Equal(t, 0, rt.consecutiveFails)
}

func TestPushCollectionStatsUsesOperationDimensions(t *testing.T) {
	rt := newAgentRuntime(time.Now())
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			rt.stats.recordSent()
			rt.stats.recordRecv()
			rt.stats.recordFinish()
			return nil
		},
	}
	rc := &runtimeConnection{rt: rt, conn: conn}

	require.NoError(t, rc.Walk(".1.2.3", func(pdu gosnmp.SnmpPDU) error { return nil }))
	stats := rt.completeGather(3, time.Minute)
	slist := types.NewSampleList()
	(&Instance{}).pushCollectionStats(slist, rt, "127.0.0.1", stats, time.Second)

	requestSample, ok := findSampleWithLabels(slist, "snmp_request_total", map[string]string{
		"operation": "walk",
	})
	require.True(t, ok)
	assert.Equal(t, float64(1), requestSample.Value)
	assert.NotContains(t, requestSample.Labels, "result")
	operationSample, ok := findSampleWithLabels(slist, "snmp_operation_total", map[string]string{
		"operation": "walk",
		"result":    "success",
	})
	require.True(t, ok)
	assert.Equal(t, float64(1), operationSample.Value)
	responseSample, ok := findSampleWithLabels(slist, "snmp_response_observed_total", map[string]string{
		"operation": "walk",
	})
	require.True(t, ok)
	assert.Equal(t, float64(1), responseSample.Value)
}

func TestRuntimeStoreProbeConnectionRejectsClosedRuntime(t *testing.T) {
	rt := newAgentRuntime(time.Now())
	rt.close()
	closed := 0
	conn := &mockSnmpConnection{
		close: func() error {
			closed++
			return nil
		},
	}

	assert.False(t, rt.storeProbeConnection(conn))
	assert.Equal(t, 1, closed)
}

func TestPushBuildStatsUsesCumulativeCountersAndAggregatesLabels(t *testing.T) {
	ins := &Instance{}
	rt := newAgentRuntime(time.Now())
	stats := BuildStats{
		TableName: "interfaces",
		Partial:   true,
		FieldErrors: []FieldError{
			{Operation: "walk", Reason: "transport", Dependency: true},
			{Operation: "walk", Reason: "transport", Dependency: true},
		},
	}

	first := types.NewSampleList()
	ins.pushBuildStats(first, rt, "127.0.0.1", stats)
	sample, ok := findSample(first, "snmp_field_error_total")
	require.True(t, ok)
	assert.Equal(t, float64(2), sample.Value)

	second := types.NewSampleList()
	ins.pushBuildStats(second, rt, "127.0.0.1", stats)
	sample, ok = findSample(second, "snmp_field_error_total")
	require.True(t, ok)
	assert.Equal(t, float64(4), sample.Value)
}

func TestPushBuildStatsSeparatesFatalFromPartial(t *testing.T) {
	ins := &Instance{}
	rt := newAgentRuntime(time.Now())
	stats := BuildStats{
		TableName:  "interfaces",
		Partial:    true,
		FatalClass: "snmp_fatal",
		FieldErrors: []FieldError{
			{Operation: "walk", Reason: "fatal", Fatal: true},
		},
	}

	slist := types.NewSampleList()
	ins.pushBuildStats(slist, rt, "127.0.0.1", stats)

	_, partialFound := findSample(slist, "snmp_partial_table_total")
	assert.False(t, partialFound)
	fatalSample, ok := findSampleWithLabels(slist, "snmp_fatal_table_total", map[string]string{
		"fatal_class": "snmp_fatal",
	})
	require.True(t, ok)
	assert.Equal(t, float64(1), fatalSample.Value)
}

func TestRuntimeConnectionPermanentSocketErrorClosesCachedConnection(t *testing.T) {
	rt := newAgentRuntime(time.Now())
	closed := 0
	conn := &mockSnmpConnection{
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return net.ErrClosed
		},
		close: func() error {
			closed++
			return nil
		},
	}
	require.NoError(t, rt.storeConnection(conn))
	rc := &runtimeConnection{rt: rt, conn: conn}

	err := rc.Walk(".1.2.3", func(pdu gosnmp.SnmpPDU) error { return nil })
	require.Error(t, err)
	assert.Equal(t, 1, closed)
	assert.Nil(t, rt.cachedConnection())
}

func TestGatherDoesNotMutateMappingsAndMappingsOverrideLabels(t *testing.T) {
	agent := "127.0.0.1"
	mappings := map[string]map[string]string{
		agent: {
			"site": "mapped",
			"rack": "r1",
		},
	}
	rt := newAgentRuntime(time.Now())
	conn := &mockSnmpConnection{
		host: agent,
		walk: func(oid string, fn gosnmp.WalkFunc) error {
			return fn(gosnmp.SnmpPDU{Name: oid + ".1", Value: "10"})
		},
	}
	require.NoError(t, rt.storeConnection(conn))
	ins := &Instance{
		InstanceConfig: config.InstanceConfig{
			InternalConfig: config.InternalConfig{
				Labels: map[string]string{
					"site": "global",
					"env":  "prod",
				},
			},
		},
		Agents:        []string{agent},
		AgentHostTag:  "agent_host",
		DisableUp:     true,
		Mappings:      mappings,
		translator:    mockTranslator{},
		agentRuntimes: []*agentRuntime{rt},
		Tables: []Table{
			{
				Name: "iface",
				Fields: []Field{
					{Name: "value", Oid: ".1.2.3.1", Conversion: "int"},
				},
			},
		},
	}

	slist := types.NewSampleList()
	ins.Gather(slist)

	sample, ok := findSampleWithLabels(slist, "snmp_iface_value", map[string]string{
		"site": "mapped",
		"rack": "r1",
		"env":  "prod",
	})
	require.True(t, ok)
	assert.Equal(t, int64(10), sample.Value)
	assert.Equal(t, map[string]string{"site": "mapped", "rack": "r1"}, mappings[agent])
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
	host  string
	get   func([]string) (*gosnmp.SnmpPacket, error)
	walk  func(string, gosnmp.WalkFunc) error
	close func() error
}

func (m *mockSnmpConnection) Host() string {
	if m.host != "" {
		return m.host
	}
	return "127.0.0.1"
}

func (m *mockSnmpConnection) Close() error {
	if m.close != nil {
		return m.close()
	}
	return nil
}

func (m *mockSnmpConnection) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	return m.get(oids)
}

func (m *mockSnmpConnection) Walk(oid string, fn gosnmp.WalkFunc) error {
	return m.walk(oid, fn)
}

func findSample(slist *types.SampleList, metric string) (*types.Sample, bool) {
	slist.RLock()
	defer slist.RUnlock()
	for e := slist.L.Front(); e != nil; e = e.Next() {
		sample, ok := e.Value.(*types.Sample)
		if ok && sample.Metric == metric {
			return sample, true
		}
	}
	return nil, false
}

func findSampleWithLabels(slist *types.SampleList, metric string, labels map[string]string) (*types.Sample, bool) {
	slist.RLock()
	defer slist.RUnlock()
	for e := slist.L.Front(); e != nil; e = e.Next() {
		sample, ok := e.Value.(*types.Sample)
		if !ok || sample.Metric != metric {
			continue
		}
		matched := true
		for k, v := range labels {
			if sample.Labels[k] != v {
				matched = false
				break
			}
		}
		if matched {
			return sample, true
		}
	}
	return nil, false
}
