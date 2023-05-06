//go:build linux

package netstat

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

// Copyright 2022 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

func (p Proc) Netstat() (*ProcNetstat, error) {
	filename := p.path("net/netstat")
	data, err := file.ReadBytes(filename)
	if err != nil {
		return nil, err
	}
	procNetstat, err := parseNetstat(bytes.NewReader(data), filename)
	procNetstat.PID = p.PID
	return &procNetstat, err
}

// parseNetstat parses the metrics from proc/<pid>/net/netstat file
// and returns a ProcNetstat structure.
func parseNetstat(r io.Reader, fileName string) (ProcNetstat, error) {
	var (
		scanner     = bufio.NewScanner(r)
		procNetstat = ProcNetstat{
			TcpExt: make(map[string]interface{}),
			IpExt:  make(map[string]interface{}),
		}
	)

	for scanner.Scan() {
		nameParts := strings.Split(scanner.Text(), " ")
		scanner.Scan()
		valueParts := strings.Split(scanner.Text(), " ")
		// Remove trailing :.
		protocol := strings.TrimSuffix(nameParts[0], ":")
		if len(nameParts) != len(valueParts) {
			return procNetstat, fmt.Errorf("mismatch field count mismatch in %s: %s",
				fileName, protocol)
		}
		for i := 1; i < len(nameParts); i++ {
			value, err := strconv.ParseFloat(valueParts[i], 64)
			if err != nil {
				return procNetstat, err
			}
			key := nameParts[i]

			switch protocol {
			case "TcpExt":
				procNetstat.TcpExt[key] = &value
			case "IpExt":
				procNetstat.IpExt[key] = &value
			}
		}
	}
	return procNetstat, scanner.Err()
}
