//go:build linux
// +build linux

package linux_sysctl_fs

import (
	"bytes"
	"errors"
	"os"
	"path"
	"strconv"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/pkg/osx"
	"flashcat.cloud/categraf/types"
	"k8s.io/klog/v2"
)

const inputName = "linux_sysctl_fs"

type SysctlFS struct {
	config.PluginConfig

	path string
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &SysctlFS{
			path: path.Join(osx.GetHostProc(), "/sys/fs"),
		}
	})
}

func (s *SysctlFS) Clone() inputs.Input {
	return &SysctlFS{
		path: path.Join(osx.GetHostProc(), "/sys/fs"),
	}
}

func (s *SysctlFS) Name() string {
	return inputName
}

func (s *SysctlFS) Gather(slist *types.SampleList) {
	fields := map[string]interface{}{}

	for _, n := range []string{"aio-nr", "aio-max-nr", "dquot-nr", "dquot-max", "super-nr", "super-max"} {
		if err := s.gatherOne(n, fields); err != nil {
			klog.ErrorS(err, "failed to gather sysctl fs metric", "name", n)
		}
	}

	err := s.gatherList("inode-state", fields, "inode-nr", "inode-free-nr", "inode-preshrink-nr")
	if err != nil {
		klog.ErrorS(err, "failed to gather sysctl inode-state")
	}

	err = s.gatherList("dentry-state", fields, "dentry-nr", "dentry-unused-nr", "dentry-age-limit", "dentry-want-pages")
	if err != nil {
		klog.ErrorS(err, "failed to gather sysctl dentry-state")
	}

	err = s.gatherList("file-nr", fields, "file-nr", "", "file-max")
	if err != nil {
		klog.ErrorS(err, "failed to gather sysctl file-nr")
	}

	slist.PushSamples(inputName, fields)
}

func (s *SysctlFS) gatherOne(name string, fields map[string]interface{}) error {
	bs, err := os.ReadFile(s.path + "/" + name)
	if err != nil {
		// Ignore non-existing entries
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	v, err := strconv.ParseUint(string(bytes.TrimRight(bs, "\n")), 10, 64)
	if err != nil {
		return err
	}

	fields[name] = v
	return nil
}

func (s *SysctlFS) gatherList(file string, fields map[string]interface{}, fieldNames ...string) error {
	bs, err := os.ReadFile(s.path + "/" + file)
	if err != nil {
		// Ignore non-existing entries
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	bsplit := bytes.Split(bytes.TrimRight(bs, "\n"), []byte{'\t'})
	for i, name := range fieldNames {
		if i >= len(bsplit) {
			break
		}
		if name == "" {
			continue
		}

		v, err := strconv.ParseUint(string(bsplit[i]), 10, 64)
		if err != nil {
			return err
		}
		fields[name] = v
	}

	return nil
}
