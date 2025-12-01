package snmp_zabbix

import "testing"

func TestZabbixTemplate_ExpandMacros(t *testing.T) {
	template := &ZabbixTemplate{}

	tests := []struct {
		name     string
		text     string
		context  map[string]string
		expected string
	}{
		{
			name: "macro with braces in context key",
			text: "1.3.6.1.4.1.2011.5.25.31.1.1.10.1.7.{#SNMPINDEX}",
			context: map[string]string{
				"{#SNMPINDEX}": "67108873",
			},
			expected: "1.3.6.1.4.1.2011.5.25.31.1.1.10.1.7.67108873",
		},
		{
			name: "macro without braces in context key",
			text: "1.3.6.1.2.1.2.2.1.8.{#SNMPINDEX}",
			context: map[string]string{
				"SNMPINDEX": "12345",
			},
			expected: "1.3.6.1.2.1.2.2.1.8.12345",
		},
		{
			name: "multiple macros with braces",
			text: "{#ENT_NAME}: Temperature ({#SNMPINDEX})",
			context: map[string]string{
				"{#ENT_NAME}":  "MPU Board",
				"{#SNMPINDEX}": "67108873",
			},
			expected: "MPU Board: Temperature (67108873)",
		},
		{
			name: "interface name expansion",
			text: "Interface {#IFNAME}({#IFALIAS}): Bits received",
			context: map[string]string{
				"{#IFNAME}":  "eth0",
				"{#IFALIAS}": "LAN",
			},
			expected: "Interface eth0(LAN): Bits received",
		},
		{
			name: "OID with interface index macro",
			text: "1.3.6.1.2.1.31.1.1.1.6.{#SNMPINDEX}",
			context: map[string]string{
				"{#SNMPINDEX}": "1",
				"{#IFINDEX}":   "1",
			},
			expected: "1.3.6.1.2.1.31.1.1.1.6.1",
		},
		{
			name: "no macro in text",
			text: "1.3.6.1.2.1.1.1.0",
			context: map[string]string{
				"{#SNMPINDEX}": "1",
			},
			expected: "1.3.6.1.2.1.1.1.0",
		},
		{
			name:     "empty context",
			text:     "1.3.6.1.2.1.2.2.1.8.{#SNMPINDEX}",
			context:  map[string]string{},
			expected: "1.3.6.1.2.1.2.2.1.8.{#SNMPINDEX}",
		},
		{
			name: "malformed macro key only prefix",
			text: "1.3.6.1.2.1.2.2.1.8.{#SNMPINDEX}",
			context: map[string]string{
				"{#SNMPINDEX": "12345", // Missing closing brace - should not be normalized
			},
			expected: "1.3.6.1.2.1.2.2.1.8.{#SNMPINDEX}", // No match, macro stays unexpanded
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := template.ExpandMacros(tt.text, tt.context)
			if result != tt.expected {
				t.Errorf("ExpandMacros() = %q, want %q", result, tt.expected)
			}
		})
	}
}
