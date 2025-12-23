package snmp_zabbix

import (
	"reflect"
	"testing"
)

func TestApplyJavaScript_JSON(t *testing.T) {
	// 1. Valid JSON.parse
	scriptParse := `
		var obj = JSON.parse(value);
		return obj.data;
	`
	jsonStr := `{"data": 123}`
	res, err := applyJavaScript(jsonStr, []string{scriptParse})
	if err != nil {
		t.Errorf("JSON.parse failed: %v", err)
	}
	if val, ok := res.(int64); ok {
		if val != 123 {
			t.Errorf("Expected 123, got %d", val)
		}
	} else if val, ok := res.(float64); ok { // Goja might return float
		if val != 123 {
			t.Errorf("Expected 123, got %f", val)
		}
	} else {
		t.Errorf("Unexpected result type: %T %v", res, res)
	}

	// 2. Valid JSON.stringify
	scriptStringify := `
		var obj = {a: 1, b: "test"};
		return JSON.stringify(obj);
	`
	resStr, err := applyJavaScript(nil, []string{scriptStringify})
	if err != nil {
		t.Errorf("JSON.stringify failed: %v", err)
	}
	// Verify naive string contains keys
	s := resStr.(string)
	if !reflect.DeepEqual(s, `{"a":1,"b":"test"}`) && !reflect.DeepEqual(s, `{"b":"test","a":1}`) {
		t.Errorf("Unexpected JSON string: %s", s)
	}

	// 3. User scenario from conversation
	scriptUserScenario := `
		value = JSON.parse(value);
		return value.data;
	`
	resUser, err := applyJavaScript(`{"data": "hello"}`, []string{scriptUserScenario})
	if err != nil {
		t.Errorf("User scenario failed: %v", err)
	}
	if resUser != "hello" {
		t.Errorf("Expected 'hello', got %v", resUser)
	}
}

func TestApplyJavaScript_InvalidJSON(t *testing.T) {
	script := `JSON.parse(value)`
	_, err := applyJavaScript(`{invalid}`, []string{script})
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
