//go:build !linux
// +build !linux

package systemd

import (
	"flashcat.cloud/categraf/types"
)

func (s *Systemd) Init() error {
	return nil
}

func (s *Systemd) Gather(slist *types.SampleList) {
}
