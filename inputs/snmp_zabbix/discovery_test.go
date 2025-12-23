package snmp_zabbix

import (
	"encoding/json"
	"testing"
)

// TestExtractIndexFromOID verifies the logic for extracting the index from a full OID given a base OID.
// This ensures we handle dot separators correctly.
func TestExtractIndexFromOID(t *testing.T) {
	d := &DiscoveryEngine{}

	testCases := []struct {
		name     string
		fullOID  string
		baseOID  string
		expected string
	}{
		{
			name:     "Standard case with extra dot",
			fullOID:  "1.3.6.1.4.1.61167.1.1.101",
			baseOID:  "1.3.6.1.4.1.61167.1",
			expected: "1.101",
		},
		{
			name:     "Case with leading dot in full OID",
			fullOID:  ".1.3.6.1.4.1.61167.1.1.101",
			baseOID:  "1.3.6.1.4.1.61167.1",
			expected: "1.101",
		},
		{
			name:     "Direct suffix (101)",
			fullOID:  "1.3.6.1.4.1.61167.1.101",
			baseOID:  "1.3.6.1.4.1.61167.1",
			expected: "101",
		},
		{
			name:     "Base and Full match exactly (no index)",
			fullOID:  "1.3.6.1.4.1.61167.1",
			baseOID:  "1.3.6.1.4.1.61167.1",
			expected: "",
		},
		{
			name:     "False prefix match (e.g. 1.3 vs 1.33)",
			fullOID:  "1.3.6.1.4.1.61167.12",
			baseOID:  "1.3.6.1.4.1.61167.1",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := d.extractIndexFromOID(tc.fullOID, tc.baseOID)
			if res != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, res)
			}
		})
	}
}

// TestRealWorldData verifies that the Zabbix JS script (and our parsing logic) works with
// the data format observed in the user's real environment (snmpwalk output).
func TestRealWorldDataAndJS(t *testing.T) {
	// Real data from user snmpwalk
	// Base OID: 1.3.6.1.4.1.61167.1
	// Full OID: .1.3.6.1.4.1.61167.1.102.11
	// Expected Index: 102.11
	
	baseOID := "1.3.6.1.4.1.61167.1"
	fullOID := ".1.3.6.1.4.1.61167.1.102.11"
	
	d := &DiscoveryEngine{}
	index := d.extractIndexFromOID(fullOID, baseOID)
	
	t.Logf("Extracted Index: '%s'", index)
	
	if index != "102.11" {
		t.Errorf("Index extraction failed. Expected '102.11', got '%s'", index)
	}

	// This assumes the user template JS logic. 
	// Even if we didn't modify the JS in the template, we want to ensure it works for this valid index.
	script := `
value = JSON.parse(value);
inputArray = value.data;
var transformedArray = inputArray.reduce(function (acc, item) {
	var snmpIndex = item["{#SNMPINDEX}"];
	// Logic from template: split by dot and take second part
	var parts = snmpIndex.split('.');
	var slotindex = parts.length > 1 ? parts[1] : undefined;

	// Note: In our robust fix plan we suggested handling single part too, 
	// but here we verify that for strict "102.11" it works even with basic logic,
	// confirming that protocol version was the main blocker.
	
	if (slotindex && !acc.some(function(existingItem) { return existingItem["{#SLOTINDEX}"] === slotindex; })) {
		acc.push(Object.assign(
			{},
			item,
			{ "{#SLOTINDEX}": slotindex}
		));
	}
	
	return acc;
}, []);

return JSON.stringify(transformedArray);
`
	inputItem := map[string]string{
		"{#SNMPINDEX}": index, // "102.11"
		"{#SNMPVALUE}": "Fan 1",
	}
	lldWrapper := map[string]interface{}{
		"data": []map[string]string{inputItem},
	}
	jsonBytes, _ := json.Marshal(lldWrapper)
	
	res, err := applyJavaScript(string(jsonBytes), []string{script})
	if err != nil {
		t.Fatalf("JS Execution failed: %v", err)
	}
	
	resStr := res.(string)
	t.Logf("JS Output: %s", resStr)
	
	var output []map[string]interface{}
	json.Unmarshal([]byte(resStr), &output)
	
	if len(output) > 0 {
		t.Logf("Capture Success! SlotIndex: %v", output[0]["{#SLOTINDEX}"])
		if output[0]["{#SLOTINDEX}"] != "11" {
			t.Errorf("SlotIndex mismatch. Expected '11', got '%v'", output[0]["{#SLOTINDEX}"])
		}
	} else {
		t.Errorf("Capture Failed! Output is empty. JS logic rejected the valid index.")
	}
}
