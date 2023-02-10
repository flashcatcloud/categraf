//go:build !linux

package sockstat

// ParseNetSockstat retrieves IPv4 socket statistics.
func ParseNetSockstat() (*NetSockstat, error) {
	return nil, nil
}

// ParseNetSockstat6 retrieves IPv6 socket statistics.
//
// If IPv6 is disabled on this kernel, the returned error can be checked with
// os.IsNotExist.
func ParseNetSockstat6() (*NetSockstat, error) {
	return nil, nil
}
