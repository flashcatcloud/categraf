// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

type WKSTA_INFO_100 struct {
	wki100_platform_id  uint32
	wki100_computername string
	wki100_langroup     string
	wki100_ver_major    uint32
	wki100_ver_minor    uint32
}

type SERVER_INFO_101 struct {
	sv101_platform_id   uint32
	sv101_name          string
	sv101_version_major uint32
	sv101_version_minor uint32
	sv101_type          uint32
	sv101_comment       string
}

func platGetVersion(outdata *byte) (maj uint64, min uint64, err error) {
	return 0, 0, nil
}

func platGetServerInfo(data *byte) (si101 SERVER_INFO_101) {
	return SERVER_INFO_101{}
}
