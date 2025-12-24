package snmp_zabbix

import (
	"testing"
)

func TestExpandMacros_Robustness(t *testing.T) {
	tmpl := &ZabbixTemplate{}

	testCases := []struct {
		name     string
		text     string
		context  map[string]string
		expected string
	}{
		{
			name: "Standard Key",
			text: "OID.{#SNMPINDEX}",
			context: map[string]string{
				"SNMPINDEX": "100",
			},
			expected: "OID.100",
		},
		{
			name: "Key with Braces",
			text: "OID.{#SNMPINDEX}",
			context: map[string]string{
				"{SNMPINDEX}": "101",
			},
			expected: "OID.101",
		},
		{
			name: "Key with Hash and Braces",
			text: "OID.{#SNMPINDEX}",
			context: map[string]string{
				"{#SNMPINDEX}": "102",
			},
			expected: "OID.102",
		},
		{
			name: "Key with Hash only",
			text: "OID.{#SNMPINDEX}",
			context: map[string]string{
				"#SNMPINDEX": "103",
			},
			expected: "OID.103",
		},
		{
			name: "Mixed Context Keys",
			text: "IF.{#IFNAME}.IDX.{#IFINDEX}",
			context: map[string]string{
				"IFNAME":      "eth0",
				"{#IFINDEX}": "10",
			},
			expected: "IF.eth0.IDX.10",
		},
		{
			name: "Expand without Hash",
			text: "Description: {IFALIAS}",
			context: map[string]string{
				"IFALIAS": "My Interface",
			},
			expected: "Description: My Interface",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tmpl.ExpandMacros(tc.text, tc.context)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}
