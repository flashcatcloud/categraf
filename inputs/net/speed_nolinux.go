//go:build !linux

package net

func Speed(iface string) (int64, error) {
	return 0, nil
}
