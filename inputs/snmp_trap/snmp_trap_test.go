package snmp_trap

import (
	"fmt"
	"net"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"

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
