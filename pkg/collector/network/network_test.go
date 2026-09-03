package network

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorUsesInjectedInterfacesAndSysfs(t *testing.T) {
	root := t.TempDir()
	stats := filepath.Join(root, "class/net/eth0/statistics")
	if err := os.MkdirAll(stats, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "class/net/eth0/operstate"), "up\n")
	write(filepath.Join(root, "class/net/eth0/carrier"), "1\n")
	write(filepath.Join(root, "class/net/eth0/speed"), "1000\n")
	write(filepath.Join(stats, "rx_bytes"), "42\n")
	iface := net.Interface{Name: "eth0", Index: 2, MTU: 1500, Flags: net.FlagUp}
	collector := New(root)
	collector.Interfaces = func() ([]net.Interface, error) { return []net.Interface{iface}, nil }
	collector.Addresses = func(net.Interface) ([]net.Addr, error) {
		return []net.Addr{cidrAddr("192.0.2.4/24"), cidrAddr("fe80::1/64")}, nil
	}
	collector.Now = func() time.Time { return time.Unix(1, 0) }
	metrics, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := metrics[0].Value.(State)
	if len(state.Interfaces) != 1 || state.Interfaces[0].RXBytes != 42 || len(state.Interfaces[0].Addresses) != 2 {
		t.Fatalf("state = %#v", state)
	}
}

type cidrAddr string

func (value cidrAddr) Network() string { return "ip+net" }
func (value cidrAddr) String() string  { return string(value) }
