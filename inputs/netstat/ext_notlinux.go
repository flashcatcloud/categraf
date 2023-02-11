//go:build !linux

package netstat

func (p Proc) Netstat() (*ProcNetstat, error) {
	return nil, nil
}
