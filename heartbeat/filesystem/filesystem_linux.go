// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package filesystem

var dfOptions = []string{"-lP"}
var expectedLength = 6

func updatefileSystemInfo(values []string) map[string]string {
	return map[string]string{
		"name":       values[0],
		"kb_size":    values[1],
		"mounted_on": values[5],
	}
}
