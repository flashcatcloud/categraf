// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"os/exec"
	"regexp"
	"strings"
)

func getMemoryInfo() (memoryInfo map[string]string, err error) {
	memoryInfo = make(map[string]string)

	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err == nil {
		memoryInfo["total"] = strings.Trim(string(out), "\n")
	}

	out, err = exec.Command("sysctl", "-n", "vm.swapusage").Output()
	if err == nil {
		swap := regexp.MustCompile("total = ").Split(string(out), 2)[1]
		memoryInfo["swap_total"] = strings.Split(swap, " ")[0]
	}

	return
}
