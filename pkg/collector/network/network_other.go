//go:build !linux

package network

import (
	"fmt"
	"net"
)

func netlinkInterfaces() ([]net.Interface, error) {
	return nil, fmt.Errorf("route netlink discovery is only supported on Linux")
}

func netlinkAddresses(net.Interface) ([]net.Addr, error) {
	return nil, fmt.Errorf("route netlink discovery is only supported on Linux")
}
