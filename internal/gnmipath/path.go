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

// Package xpath parses xpath string into gnmi Path struct.
// Copied from google/gnxi at ef18504847b0; retained locally to avoid its
// unrelated legacy Docker daemon dependencies.
package gnmipath

import (
	"fmt"

	pb "github.com/openconfig/gnmi/proto/gnmi"
)

// ToGNMIPath parses an xpath string into a gnmi Path struct defined in gnmi
// proto. Path convention can be found in
// https://github.com/openconfig/reference/blob/master/rpc/gnmi/gnmi-path-conventions.md
//
// For example, xpath /interfaces/interface[name=Ethernet1/2/3]/state/counters
// will be parsed to:
//
//	elem: <name: "interfaces" >
//	elem: <
//	    name: "interface"
//	    key: <
//	        key: "name"
//	        value: "Ethernet1/2/3"
//	    >
//	>
//	elem: <name: "state" >
//	elem: <name: "counters" >
func ToGNMIPath(xpath string) (*pb.Path, error) {
	xpathElements, err := ParseStringPath(xpath)
	if err != nil {
		return nil, err
	}
	var pbPathElements []*pb.PathElem
	for _, elem := range xpathElements {
		switch v := elem.(type) {
		case string:
			pbPathElements = append(pbPathElements, &pb.PathElem{Name: v})
		case map[string]string:
			n := len(pbPathElements)
			if n == 0 {
				return nil, fmt.Errorf("missing name before key-value list")
			}
			if pbPathElements[n-1].Key != nil {
				return nil, fmt.Errorf("two subsequent key-value lists")
			}
			pbPathElements[n-1].Key = v
		default:
			return nil, fmt.Errorf("wrong data type: %T", v)
		}
	}
	return &pb.Path{Elem: pbPathElements}, nil
}
