package snmp_trap

import (
	"fmt"
	"net"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"flashcat.cloud/categraf/pkg/snmp"
	"flashcat.cloud/categraf/types"
)

type mockTranslator struct {
	dict map[string]string
}

func (m *mockTranslator) lookup(oid string) (snmp.MibEntry, error) {
	if name, ok := m.dict[oid]; ok {
		return snmp.MibEntry{OidText: name}, nil
	}
	return snmp.MibEntry{}, fmt.Errorf("not found")
}

func TestTrapAggregation(t *testing.T) {
	slist := types.NewSampleList()
	instance := &Instance{
		Translator:     "mock",
		FieldsToLabels: []string{"ifIndex", "ifAdminStatus", "ifOperStatus"},
		TrapMapping: []TrapMapping{
			{
				Oid:   ".1.3.6.1.6.3.1.1.5.3",
				Name:  "link_down",
				Value: ".1.3.6.1.2.1.1.3",
				Varbind: []TrapVarbind{
					{Oid: ".1.3.6.1.2.1.2.2.1.1", Name: "ifIndex"},
				},
			},
		},
		transl: &mockTranslator{
			dict: map[string]string{
				".1.3.6.1.6.3.1.1.5.3":     "linkDown",
				".1.3.6.1.2.1.2.2.1.7.835": "ifAdminStatus.835",
				".1.3.6.1.2.1.2.2.1.8.835": "ifOperStatus.835",
			},
		},
	}

	packet := &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: "public",
		Variables: []gosnmp.SnmpPDU{
			{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123456)},
			{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.3"},
			{Name: ".1.3.6.1.2.1.2.2.1.1.835", Type: gosnmp.Integer, Value: 835},
			{Name: ".1.3.6.1.2.1.2.2.1.7.835", Type: gosnmp.Integer, Value: 1},
			{Name: ".1.3.6.1.2.1.2.2.1.8.835", Type: gosnmp.Integer, Value: 2},
		},
	}

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 162}

	handler := makeTrapHandler(instance, slist)
	handler(packet, addr)

	samples := slist.PopBackAll()

	// Should have exactly 1 core metric, all other varbinds were used as labels or value
	assert.Equal(t, 1, len(samples))

	sample := samples[0]
	assert.Equal(t, "snmp_trap_link_down", sample.Metric)
	assert.Equal(t, uint32(123456), sample.Value)

	labels := sample.Labels
	assert.Equal(t, "127.0.0.1", labels["source"])
	assert.Equal(t, "835", labels["ifIndex"])
	assert.Equal(t, "1", labels["ifAdminStatus"])
	assert.Equal(t, "2", labels["ifOperStatus"])
	assert.Equal(t, "link_down", labels["name"])
	assert.Equal(t, "2c", labels["version"])
}

func TestTrapDispersedFallback(t *testing.T) {
	slist := types.NewSampleList()
	instance := &Instance{
		Translator:     "mock",
		FieldsToLabels: []string{"ifIndex"}, // only ifIndex
		transl: &mockTranslator{
			dict: map[string]string{
				".1.3.6.1.6.3.1.1.5.3":     "linkDown",
				".1.3.6.1.2.1.2.2.1.1.835": "ifIndex.835",
				".1.3.6.1.2.1.1.3.0":       "sysUpTimeInstance",
			},
		},
	}

	packet := &gosnmp.SnmpPacket{
		Version:   gosnmp.Version2c,
		Community: "public",
		Variables: []gosnmp.SnmpPDU{
			{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123456)},
			{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.3"},
			{Name: ".1.3.6.1.2.1.2.2.1.1.835", Type: gosnmp.Integer, Value: 835},
		},
	}

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 162}

	handler := makeTrapHandler(instance, slist)
	handler(packet, addr)

	samples := slist.PopBackAll()

	// Should have 2 metrics (Core metric "linkDown" + Dispersed "sysUpTimeInstance")
	assert.Equal(t, 2, len(samples))

	var coreSample *types.Sample
	var sysUpTimeSample *types.Sample
	for _, s := range samples {
		if s.Metric == "snmp_trap_linkDown" {
			coreSample = s
		} else if s.Metric == "snmp_trap_sysUpTimeInstance" {
			sysUpTimeSample = s
		}
	}

	assert.NotNil(t, coreSample)
	assert.NotNil(t, sysUpTimeSample)

	// Verify Context Label Inheritance
	assert.Equal(t, "835", coreSample.Labels["ifIndex"])
	assert.Equal(t, "835", sysUpTimeSample.Labels["ifIndex"]) // Dispersed metric MUST inherit the label!
}

func TestTrapStructuredVarbindConversion(t *testing.T) {
	slist := types.NewSampleList()
	instance := &Instance{
		Varbind: []VarbindConfig{
			{Oid: ".1.3.6.1.4.1.1", Name: "device_name", Conversion: "string", AsLabel: true},
			{Oid: ".1.3.6.1.4.1.2", Name: "device_ip", Conversion: "ipaddr", AsLabel: true},
		},
		TrapMapping: []TrapMapping{{Oid: ".1.3.6.1.6.3.1.1.5.3", Name: "device_event"}},
		transl: &mockTranslator{dict: map[string]string{
			".1.3.6.1.6.3.1.1.5.3": "linkDown",
		}},
	}
	packet := &gosnmp.SnmpPacket{
		Version: gosnmp.Version2c,
		Variables: []gosnmp.SnmpPDU{
			{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.3"},
			{Name: ".1.3.6.1.4.1.1", Type: gosnmp.OctetString, Value: []byte("router-01")},
			{Name: ".1.3.6.1.4.1.2", Type: gosnmp.OctetString, Value: []byte{10, 95, 5, 53}},
		},
	}

	makeTrapHandler(instance, slist)(packet, &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	samples := slist.PopBackAll()
	assert.Len(t, samples, 1)
	assert.Equal(t, "router-01", samples[0].Labels["device_name"])
	assert.Equal(t, "10.95.5.53", samples[0].Labels["device_ip"])
}

func TestTrapConvertRulesForCoreValue(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected interface{}
	}{
		{name: "fixed mapping", raw: "offline", expected: int64(-1)},
		{name: "legacy conversion fallback", raw: "34%", expected: float64(34)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			slist := types.NewSampleList()
			instance := &Instance{
				TrapMapping: []TrapMapping{{
					Oid:   ".1.3.6.1.6.3.1.1.5.3",
					Name:  "fan_event",
					Value: ".1.3.6.1.4.1.10",
					Varbind: []TrapVarbind{{
						Oid:        ".1.3.6.1.4.1.10",
						Conversion: "float",
						ConvertRules: []snmp.ConvertRule{{
							Match: "offline",
							Value: int64(-1),
						}},
					}},
				}},
				transl: &mockTranslator{dict: map[string]string{
					".1.3.6.1.6.3.1.1.5.3": "linkDown",
				}},
			}
			require.NoError(t, instance.initConvertRules())
			packet := &gosnmp.SnmpPacket{
				Version: gosnmp.Version2c,
				Variables: []gosnmp.SnmpPDU{
					{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.3"},
					{Name: ".1.3.6.1.4.1.10.1", Type: gosnmp.OctetString, Value: []byte(test.raw)},
				},
			}

			makeTrapHandler(instance, slist)(packet, &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
			samples := slist.PopBackAll()
			require.Len(t, samples, 1)
			assert.Equal(t, test.expected, samples[0].Value)
		})
	}
}

func TestTrapGlobalVarbindConvertRule(t *testing.T) {
	slist := types.NewSampleList()
	instance := &Instance{
		Varbind: []VarbindConfig{{
			Oid:        ".1.3.6.1.4.1.20",
			Name:       "fan_speed",
			Conversion: "float",
			ConvertRules: []snmp.ConvertRule{{
				Regex: `(?i)^\s*offline\s*$`,
				Value: int64(-1),
			}},
		}},
		transl: &mockTranslator{dict: map[string]string{}},
	}
	require.NoError(t, instance.initConvertRules())
	packet := &gosnmp.SnmpPacket{
		Version: gosnmp.Version2c,
		Variables: []gosnmp.SnmpPDU{{
			Name: ".1.3.6.1.4.1.20.1", Type: gosnmp.OctetString, Value: []byte(" OFFLINE "),
		}},
	}

	makeTrapHandler(instance, slist)(packet, &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	samples := slist.PopBackAll()
	require.Len(t, samples, 1)
	assert.Equal(t, "snmp_trap_fan_speed_1", samples[0].Metric)
	assert.Equal(t, int64(-1), samples[0].Value)
}

func TestTrapConvertRuleValidation(t *testing.T) {
	instance := &Instance{Varbind: []VarbindConfig{{
		Oid:          ".1.3.6.1.4.1.20",
		ConvertRules: []snmp.ConvertRule{{Regex: "("}},
	}}}
	require.Error(t, instance.initConvertRules())
}

func TestTrapConvertRuleTOML(t *testing.T) {
	config := `
[[varbind]]
oid = ".1.3.6.1.4.1.20"
name = "fan_speed"
conversion = "float"

[[varbind.convert_rule]]
match = "offline"
value = -1

[[trap_mapping]]
oid = ".1.3.6.1.6.3.1.1.5.3"

[[trap_mapping.varbind]]
oid = ".1.3.6.1.4.1.20"
name = "fan_speed"

[[trap_mapping.varbind.convert_rule]]
regex = '^fan:\s*(.*)$'
extract = "$1"
conversion = "float"
`
	var instance Instance
	require.NoError(t, toml.Unmarshal([]byte(config), &instance))
	require.NoError(t, instance.initConvertRules())
	require.Len(t, instance.Varbind, 1)
	require.Len(t, instance.Varbind[0].ConvertRules, 1)
	require.Len(t, instance.TrapMapping, 1)
	require.Len(t, instance.TrapMapping[0].Varbind[0].ConvertRules, 1)
}
