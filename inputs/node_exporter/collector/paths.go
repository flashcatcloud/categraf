// Copyright 2015 The Prometheus Authors
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

package collector

import (
	"github.com/prometheus/procfs"
	"path/filepath"
	"strings"
)

var (
	// The path of the proc filesystem.
	procPath     = new(string) // kingpin.Flag("path.procfs", "procfs mountpoint.").Default(procfs.DefaultMountPoint).String()
	sysPath      = new(string) // kingpin.Flag("path.sysfs", "sysfs mountpoint.").Default("/sys").String()
	rootfsPath   = new(string) // kingpin.Flag("path.rootfs", "rootfs mountpoint.").Default("/").String()
	udevDataPath = new(string) // kingpin.Flag("path.udev.data", "udev data path.").Default("/run/udev/data").String()
)

func procFilePath(name string) string {
	return filepath.Join(*procPath, name)
}

func sysFilePath(name string) string {
	return filepath.Join(*sysPath, name)
}

func rootfsFilePath(name string) string {
	return filepath.Join(*rootfsPath, name)
}

func udevDataFilePath(name string) string {
	return filepath.Join(*udevDataPath, name)
}

func rootfsStripPrefix(path string) string {
	if *rootfsPath == "/" {
		return path
	}
	stripped := strings.TrimPrefix(path, *rootfsPath)
	if stripped == "" {
		return "/"
	}
	return stripped
}

func pathInit(params map[string]string) error {
	path, ok := params["path.procfs"]
	if !ok {
		*procPath = procfs.DefaultMountPoint
	} else {
		*procPath = path
	}

	sPath, ok := params["path.sysfs"]
	if !ok {
		*sysPath = "/sys"
	} else {
		*sysPath = sPath
	}
	rPath, ok := params["path.rootfs"]
	if !ok {
		*rootfsPath = "/"
	} else {
		*rootfsPath = rPath
	}
	uPath, ok := params["path.udev.data"]
	if !ok {
		*udevDataPath = "/run/udev/data"
	} else {
		*udevDataPath = uPath
	}
	return nil
}
