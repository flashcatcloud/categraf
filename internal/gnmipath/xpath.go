/* Copyright 2017 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package xpath provides utility functions to parse gnmi xpath.
// Copied from google/gnxi at ef18504847b0; retained locally to avoid its
// unrelated legacy Docker daemon dependencies.
package gnmipath

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	idPattern = `[a-zA-Z_][a-zA-Z\d\_\-\.]*`
	// YANG identifiers must follow RFC 6020:
	// https://tools.ietf.org/html/rfc6020#section-6.2.
	idRe = regexp.MustCompile(`^(?:` + idPattern + `:)?` + idPattern + `$`)
	// The sting representation of List key value pairs must follow the
	// following pattern: [key=value], where key is the List key leaf name,
	// and value is the string representation of key leaf value.
	kvRe = regexp.MustCompile(`^\[` +
		// Key leaf name must be a valid YANG identifier.
		idPattern + `=` +
		// Key leaf value must be a non-empty string, which may contain
		// newlines. Use (?s) to turn on s flag to match newlines.
		`((?s).+)` +
		`\]$`)
)

// splitPath splits a string representation of path into []string. Path
// elements are separated by '/'. String splitting scans from left to right. A
// '[' marks the beginning of a List key value pair substring. A List key value
// pair string ends at the first ']' encountered. Neither an escaped '[', i.e.,
// `\[`, nor an escaped ']', i.e., `\]`, serves as the boundary of a List key
// value pair string.
//
// Within a List key value string, '/', '[' and ']' are treated differently:
//
//  1. A '/' does not act as a separator, and is allowed to be part of a
//     List key leaf value.
//
//  2. A '[' is allowed within a List key value. '[' and `\[` are
//     equivalent within a List key value.
//
//  3. If a ']' needs to be part of a List key value, it must be escaped as
//     '\]'. The first unescaped ']' terminates a List key value string.
//
// Outside of any List key value pair string:
//
//  1. A ']' without a matching '[' does not generate any error in this
//     API. This error is caught later by another API.
//
//  2. A '[' without an closing ']' is treated as an error, because it
//     indicates an incomplete List key leaf value string.
//
// For example, "/a/b/c" is split into []string{"a", "b", "c"}.
// "/a/b[k=eth1/1]/c" is split into []string{"a", "b[k=eth1/1]", "c"}.
// `/a/b/[k=v\]]/c` is split into []string{"a", "b", `[k=v\]]`, "c"}.
// "a/b][k=v]/c" is split into []string{"a", "b][k=v]", "c"}. The invalid List
// name "b]" error will be caught later by another API. "/a/b[k=v/c" generates
// an error because of incomplete List key value pair string.
func splitPath(str string) ([]string, error) {
	var path []string
	str += "/"
	// insideBrackets is true when at least one '[' has been found and no
	// ']' has been found. It is false when a closing ']' has been found.
	insideBrackets := false
	// begin marks the beginning of a path element, which is separated by
	// '/' unclosed between '[' and ']'.
	begin := 0
	// end marks the end of a path element, which is separated by '/'
	// unclosed between '[' and ']'.
	end := 0

	// Split the given string using unescaped '/'.
	for end < len(str) {
		switch str[end] {
		case '/':
			if !insideBrackets {
				// Current '/' is a valid path element
				// separator.
				if end > begin {
					path = append(path, str[begin:end])
				}
				end++
				begin = end
			} else {
				// Current '/' must be part of a List key value
				// string.
				end++
			}
		case '[':
			if (end == 0 || str[end-1] != '\\') && !insideBrackets {
				// Current '[' is unescacped, and is the
				// beginning of List key-value pair(s) string.
				insideBrackets = true
			}
			end++
		case ']':
			if (end == 0 || str[end-1] != '\\') && insideBrackets {
				// Current ']' is unescacped, and is the end of
				// List key-value pair(s) string.
				insideBrackets = false
			}
			end++
		default:
			end++
		}
	}

	if insideBrackets {
		return nil, fmt.Errorf("missing ] in path string: %s", str)
	}
	return path, nil
}

// parseKeyValueString parses a List key-value pair, and returns a
// map[string]string whose key is the List key leaf name and whose value is the
// string representation of List key leaf value. The input path-valur pairs are
// encoded using the following pattern: [k1=v1][k2=v2]..., where k1 and k2 must be
// valid YANG identifiers, v1 and v2 can be any non-empty strings where any ']'
// must be escapced by an '\'. Any malformed key-value pair generates an error.
// For example, given
//
//	"[k1=v1][k2=v2]",
//
// this API returns
//
//	map[string]string{"k1": "v1", "k2": "v2"}.
func parseKeyValueString(str string) (map[string]string, error) {
	keyValuePairs := make(map[string]string)
	// begin marks the beginning of a key-value pair.
	begin := 0
	// end marks the end of a key-value pair.
	end := 0
	// insideBrackets is true when at least one '[' has been found and no
	// ']' has been found. It is false when a closing ']' has been found.
	insideBrackets := false

	for end < len(str) {
		switch str[end] {
		case '[':
			if (end == 0 || str[end-1] != '\\') && !insideBrackets {
				insideBrackets = true
			}
			end++
		case ']':
			if (end == 0 || str[end-1] != '\\') && insideBrackets {
				insideBrackets = false
				keyValue := str[begin : end+1]
				// Key-value pair string must have the
				// following pattern: [k=v], where k is a valid
				// YANG identifier, and v can be any non-empty
				// string.
				if !kvRe.MatchString(keyValue) {
					return nil, fmt.Errorf("malformed List key-value pair string: %s, in: %s", keyValue, str)
				}
				keyValue = keyValue[1 : len(keyValue)-1]
				i := strings.Index(keyValue, "=")
				key, val := keyValue[:i], keyValue[i+1:]
				// Recover escaped '[' and ']'.
				val = strings.ReplaceAll(val, `\]`, `]`)
				val = strings.ReplaceAll(val, `\[`, `[`)
				keyValuePairs[key] = val
				begin = end + 1
			}
			end++
		default:
			end++
		}
	}

	if begin < end {
		return nil, fmt.Errorf("malformed List key-value pair string: %s", str)
	}

	return keyValuePairs, nil
}

// parseElement parses a split path element, and returns the parsed elements.
// Two types of path elements are supported:
//
// 1. Non-List schema node names which must be valid YANG identifiers. A valid
// schema node name is returned as it is. For example, given "abc", this API
// returns []interface{"abc"}.
//
// 2. List elements following this pattern: list-name[k1=v1], where list-name
// is the substring from the beginning of the input string to the first '[', k1
// is the substring from the letter after '[' to the first '=', and v1 is the
// substring from the letter after '=' to the first unescaped ']'. list-name
// and k1 must be valid YANG identifier, and v1 can be any non-empty string
// where ']' is escaped by '\'. A List element is parsed into two parts: List
// name and List key value pair(s). List key value pairs are saved in a
// map[string]string whose key is List key leaf name and whose value is the
// string representation of List key leaf value. For example, given
//
//	"list-name[k1=v1]",
//
// this API returns
//
//	[]interface{}{"list-name", map[string]string{"k1": "v1"}}.
//
// Multi-key List elements follow a similar pattern:
//
//	list-name[k1=v1]...[kN=vN].
func parseElement(elem string) ([]interface{}, error) {
	i := strings.Index(elem, "[")
	if i < 0 {
		if !idRe.MatchString(elem) {
			return nil, fmt.Errorf("invalid node name: %q", elem)
		}
		return []interface{}{elem}, nil
	}

	listName := elem[:i]
	if !idRe.MatchString(listName) {
		return nil, fmt.Errorf("invalid List name: %q, in: %s", listName, elem)
	}
	keyValuePairs, err := parseKeyValueString(elem[i:])
	if err != nil {
		return nil, fmt.Errorf("invalid path element %s: %v", elem, err)
	}
	return []interface{}{listName, keyValuePairs}, nil
}

// ParseStringPath parses a string path and produces a []interface{} of parsed
// path elements. Path elements in a string path are separated by '/'. Each
// path element can either be a schema node name or a List path element. Schema
// node names must be valid YANG identifiers. A List path element is encoded
// using the following pattern: list-name[key1=value1]...[keyN=valueN]. Each
// List path element generates two parsed path elements: List name and a
// map[string]string containing List key-value pairs with value(s) in string
// representation. A '/' within a List key value pair string, i.e., between a
// pair of '[' and ']', does not serve as a path separator, and is allowed to be
// part of a List key leaf value. For example, given a string path:
//
//	"/a/list-name[k=v/v]/c",
//
// this API returns:
//
//	[]interface{}{"a", "list-name", map[string]string{"k": "v/v"}, "c"}.
//
// String path parsing consists of two passes. In the first pass, the input
// string is split into []string using valid separator '/'. An incomplete List
// key value string, i.e, a '[' which starts a List key value string without a
// closing ']', in input string generates an error. In the above example, this
// pass produces:
//
//	[]string{"a", "list-name[k=v/v]", "c"}.
//
// In the second pass, each element in split []string is parsed checking syntax
// and pattern correctness. Errors are generated for invalid YANG identifiers,
// malformed List key-value string, etc.. In the above example, the second pass
// produces:
//
//	[]interface{}{"a", "list-name", map[string]string{"k": "v/v"}, "c"}.
func ParseStringPath(stringPath string) ([]interface{}, error) {
	elems, err := splitPath(stringPath)
	if err != nil {
		return nil, err
	}

	var path []interface{}
	// Check whether each path element is valid. Parse List key value
	// pairs.
	for _, elem := range elems {
		parts, err := parseElement(elem)
		if err != nil {
			return nil, fmt.Errorf("invalid string path %s: %v", stringPath, err)
		}
		path = append(path, parts...)
	}

	return path, nil
}
