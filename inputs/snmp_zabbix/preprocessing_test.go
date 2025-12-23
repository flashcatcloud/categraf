package snmp_zabbix

import (
	"reflect"
	"testing"
)

func TestApplyJavaScript_JSON(t *testing.T) {
	type testCase struct {
		name   string
		value  interface{}
		script string
		check  func(t *testing.T, res interface{}, err error)
	}

	tests := []testCase{
		{
			name:  "valid JSON.parse",
			value: `{"data": 123}`,
			script: `
		var obj = JSON.parse(value);
		return obj.data;
	`,
			check: func(t *testing.T, res interface{}, err error) {
				if err != nil {
					t.Fatalf("JSON.parse failed: %v", err)
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
			},
		},
		{
			name:  "valid JSON.stringify",
			value: nil,
			script: `
		var obj = {a: 1, b: "test"};
		return JSON.stringify(obj);
	`,
			check: func(t *testing.T, res interface{}, err error) {
				if err != nil {
					t.Fatalf("JSON.stringify failed: %v", err)
				}
				s, ok := res.(string)
				if !ok {
					t.Fatalf("Expected string result, got %T %v", res, res)
				}
				// Verify naive string contains keys (order may vary)
				if !reflect.DeepEqual(s, `{"a":1,"b":"test"}`) && !reflect.DeepEqual(s, `{"b":"test","a":1}`) {
					t.Errorf("Unexpected JSON string: %s", s)
				}
			},
		},
		{
			name:  "user scenario JSON.parse",
			value: `{"data": "hello"}`,
			script: `
		value = JSON.parse(value);
		return value.data;
	`,
			check: func(t *testing.T, res interface{}, err error) {
				if err != nil {
					t.Fatalf("User scenario failed: %v", err)
				}
				if res != "hello" {
					t.Errorf("Expected 'hello', got %v", res)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			res, err := applyJavaScript(tc.value, []string{tc.script})
			tc.check(t, res, err)
		})
	}
}

func TestApplyJavaScript_InvalidJSON(t *testing.T) {
	script := `JSON.parse(value)`
	_, err := applyJavaScript(`{invalid}`, []string{script})
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
