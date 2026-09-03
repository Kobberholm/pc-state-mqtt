package network

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	netlinkHeaderSize = 16
	nlaHeaderSize     = 4
	ifInfoSize        = 16
	ifAddrSize        = 8
	nlaTypeMask       = 0x3fff
)

func netlinkInterfaces() ([]net.Interface, error) {
	messages, err := netlinkDump(unix.RTM_GETLINK, make([]byte, ifInfoSize))
	if err != nil {
		return nil, err
	}
	interfaces := make([]net.Interface, 0, len(messages))
	for _, message := range messages {
		if len(message.Data) < ifInfoSize {
			continue
		}
		index := int(int32(binary.NativeEndian.Uint32(message.Data[4:8])))
		flags := net.Flags(binary.NativeEndian.Uint32(message.Data[8:12]))
		iface := net.Interface{Index: index, Flags: flags}
		attrs, err := netlinkAttrs(message.Data[ifInfoSize:])
		if err != nil {
			return nil, fmt.Errorf("parse netlink interface %d: %w", index, err)
		}
		for _, attr := range attrs {
			switch attr.kind {
			case unix.IFLA_IFNAME:
				iface.Name = strings.TrimRight(string(attr.value), "\x00")
			case unix.IFLA_ADDRESS:
				iface.HardwareAddr = append(net.HardwareAddr(nil), attr.value...)
			case unix.IFLA_MTU:
				if len(attr.value) >= 4 {
					iface.MTU = int(binary.NativeEndian.Uint32(attr.value))
				}
			}
		}
		if iface.Name != "" {
			interfaces = append(interfaces, iface)
		}
	}
	return interfaces, nil
}

func netlinkAddresses(iface net.Interface) ([]net.Addr, error) {
	messages, err := netlinkDump(unix.RTM_GETADDR, make([]byte, ifAddrSize))
	if err != nil {
		return nil, err
	}
	addresses := make([]net.Addr, 0)
	for _, message := range messages {
		if len(message.Data) < ifAddrSize {
			continue
		}
		index := int(binary.NativeEndian.Uint32(message.Data[4:8]))
		if index != iface.Index {
			continue
		}
		family := message.Data[0]
		prefixLength := int(message.Data[1])
		attrs, err := netlinkAttrs(message.Data[ifAddrSize:])
		if err != nil {
			return nil, fmt.Errorf("parse netlink addresses for %s: %w", iface.Name, err)
		}
		var address []byte
		for _, attr := range attrs {
			if family == unix.AF_INET && attr.kind == unix.IFA_LOCAL {
				address = attr.value
				break
			}
			if attr.kind == unix.IFA_ADDRESS {
				address = attr.value
			}
		}
		if len(address) != net.IPv4len && len(address) != net.IPv6len {
			continue
		}
		bits := net.IPv6len * 8
		if family == unix.AF_INET {
			bits = net.IPv4len * 8
		}
		ip := append(net.IP(nil), address...)
		addresses = append(addresses, &net.IPNet{IP: ip, Mask: net.CIDRMask(prefixLength, bits)})
	}
	return addresses, nil
}

type netlinkAttr struct {
	kind  uint16
	value []byte
}

func netlinkAttrs(data []byte) ([]netlinkAttr, error) {
	var attrs []netlinkAttr
	for len(data) > 0 {
		if len(data) < nlaHeaderSize {
			break
		}
		length := int(binary.NativeEndian.Uint16(data[0:2]))
		if length < nlaHeaderSize || length > len(data) {
			break
		}
		attrs = append(attrs, netlinkAttr{
			kind:  binary.NativeEndian.Uint16(data[2:4]) & nlaTypeMask,
			value: data[nlaHeaderSize:length],
		})
		aligned := (length + 3) &^ 3
		if aligned > len(data) {
			break
		}
		data = data[aligned:]
	}
	return attrs, nil
}

func netlinkDump(messageType uint16, payload []byte) ([]syscall.NetlinkMessage, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("open route netlink: %w", err)
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("bind route netlink: %w", err)
	}

	request := make([]byte, netlinkHeaderSize+len(payload))
	binary.NativeEndian.PutUint32(request[0:4], uint32(len(request)))
	binary.NativeEndian.PutUint16(request[4:6], messageType)
	binary.NativeEndian.PutUint16(request[6:8], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(request[8:12], 1)
	copy(request[netlinkHeaderSize:], payload)
	if err := unix.Sendto(fd, request, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, fmt.Errorf("request route netlink dump: %w", err)
	}

	var messages []syscall.NetlinkMessage
	buffer := make([]byte, 1<<16)
	for {
		length, _, err := unix.Recvfrom(fd, buffer, 0)
		if err != nil {
			return nil, fmt.Errorf("read route netlink dump: %w", err)
		}
		parts, err := syscall.ParseNetlinkMessage(buffer[:length])
		if err != nil {
			return nil, fmt.Errorf("parse route netlink dump: %w", err)
		}
		for _, part := range parts {
			switch part.Header.Type {
			case unix.NLMSG_DONE:
				return messages, nil
			case unix.NLMSG_ERROR:
				if len(part.Data) < 4 {
					return nil, fmt.Errorf("malformed route netlink error")
				}
				code := int32(binary.NativeEndian.Uint32(part.Data[:4]))
				if code != 0 {
					return nil, fmt.Errorf("route netlink dump: %w", unix.Errno(-code))
				}
				return messages, nil
			case unix.NLMSG_NOOP:
				continue
			default:
				messages = append(messages, syscall.NetlinkMessage{
					Header: part.Header,
					Data:   append([]byte(nil), part.Data...),
				})
			}
		}
	}
}
