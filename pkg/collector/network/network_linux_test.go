package network

import (
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNetlinkAttrs(t *testing.T) {
	data := make([]byte, 20)
	binary.NativeEndian.PutUint16(data[0:2], 9)
	binary.NativeEndian.PutUint16(data[2:4], unix.IFLA_IFNAME|0x8000)
	copy(data[4:], "eth0\x00")
	binary.NativeEndian.PutUint16(data[12:14], 8)
	binary.NativeEndian.PutUint16(data[14:16], unix.IFLA_MTU)

	attrs, err := netlinkAttrs(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 2 || attrs[0].kind != unix.IFLA_IFNAME || string(attrs[0].value) != "eth0\x00" {
		t.Fatalf("attrs = %#v", attrs)
	}
	if attrs[1].kind != unix.IFLA_MTU || len(attrs[1].value) != 4 {
		t.Fatalf("attrs = %#v", attrs)
	}
}
