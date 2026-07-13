package snmp

import (
	"fmt"
	"regexp"
)

// ConvertRule maps or extracts values before a plugin applies its conversion.
type ConvertRule struct {
	Match      string      `toml:"match"`
	Regex      string      `toml:"regex"`
	Extract    string      `toml:"extract"`
	Value      interface{} `toml:"value"`
	Conversion string      `toml:"conversion"`

	compiledRegex *regexp.Regexp `toml:"-"`
}

// ConvertRuleResult describes the first matching rule.
type ConvertRuleResult struct {
	Matched    bool
	FixedValue bool
	Value      interface{}
	Conversion string
}

// InitConvertRules validates rules and compiles their regular expressions.
func InitConvertRules(rules []ConvertRule) error {
	for i := range rules {
		rule := &rules[i]
		if rule.Match == "" && rule.Regex == "" {
			return fmt.Errorf("convert_rule[%d]: match or regex must be set", i)
		}
		if rule.Match != "" && rule.Regex != "" {
			return fmt.Errorf("convert_rule[%d]: match and regex are mutually exclusive", i)
		}
		if rule.Extract != "" && rule.Regex == "" {
			return fmt.Errorf("convert_rule[%d]: extract can only be used with regex", i)
		}
		if rule.Value != nil {
			if rule.Extract != "" || rule.Conversion != "" {
				return fmt.Errorf("convert_rule[%d]: value cannot be used with extract or conversion", i)
			}
			switch rule.Value.(type) {
			case string, int, int64, float64, bool:
			default:
				return fmt.Errorf("convert_rule[%d]: value must be a scalar type", i)
			}
		}
		if rule.Regex != "" {
			re, err := regexp.Compile(rule.Regex)
			if err != nil {
				return fmt.Errorf("convert_rule[%d]: invalid regex: %w", i, err)
			}
			rule.compiledRegex = re
		}
	}
	return nil
}

// MatchConvertRule applies rules in order and returns the first match.
func MatchConvertRule(rules []ConvertRule, value interface{}, defaultConversion string) ConvertRuleResult {
	raw := rawString(value)
	for i := range rules {
		rule := &rules[i]
		candidate, matched := rule.match(raw)
		if !matched {
			continue
		}
		if rule.Value != nil {
			return ConvertRuleResult{Matched: true, FixedValue: true, Value: rule.Value}
		}
		conversion := rule.Conversion
		if conversion == "" {
			conversion = defaultConversion
		}
		return ConvertRuleResult{Matched: true, Value: candidate, Conversion: conversion}
	}
	return ConvertRuleResult{}
}

func (r *ConvertRule) match(raw string) (string, bool) {
	if r.Match != "" {
		return raw, raw == r.Match
	}
	if r.compiledRegex == nil {
		return "", false
	}
	if r.Extract == "" {
		return raw, r.compiledRegex.MatchString(raw)
	}
	indices := r.compiledRegex.FindStringSubmatchIndex(raw)
	if indices == nil {
		return "", false
	}
	return string(r.compiledRegex.ExpandString(nil, r.Extract, raw, indices)), true
}

func rawString(value interface{}) string {
	if bs, ok := value.([]byte); ok {
		return string(bs)
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}
