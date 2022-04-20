package netx

import (
	"fmt"
	"net"
)

func LocalAddressByInterfaceName(interfaceName string) (net.Addr, error) {
	i, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, err
	}

	addrs, err := i.Addrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if naddr, ok := addr.(*net.IPNet); ok {
			// leaving port set to zero to let kernel pick
			return &net.TCPAddr{IP: naddr.IP}, nil
		}
	}

	return nil, fmt.Errorf("cannot create local address for interface %q", interfaceName)
}
