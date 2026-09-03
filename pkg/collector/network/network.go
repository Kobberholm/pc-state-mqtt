package network

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pc-state-mqtt/pkg/telemetry"
)

type InterfaceDiscover func() ([]net.Interface, error)
type AddressDiscover func(net.Interface) ([]net.Addr, error)

type Collector struct {
	SysRoot    string
	Now        func() time.Time
	Interfaces InterfaceDiscover
	Addresses  AddressDiscover
}

type State struct {
	Interfaces []InterfaceState `json:"interfaces"`
}

type InterfaceState struct {
	Name             string    `json:"name"`
	Index            int       `json:"index"`
	MAC              string    `json:"mac,omitempty"`
	MTU              int       `json:"mtu"`
	Flags            []string  `json:"flags,omitempty"`
	Addresses        []Address `json:"addresses,omitempty"`
	OperationalState string    `json:"operational_state,omitempty"`
	Carrier          *bool     `json:"carrier,omitempty"`
	SpeedMbps        *float64  `json:"speed_mbps,omitempty"`
	RXBytes          uint64    `json:"rx_bytes"`
	RXPackets        uint64    `json:"rx_packets"`
	RXErrors         uint64    `json:"rx_errors"`
	RXDrops          uint64    `json:"rx_drops"`
	TXBytes          uint64    `json:"tx_bytes"`
	TXPackets        uint64    `json:"tx_packets"`
	TXErrors         uint64    `json:"tx_errors"`
	TXDrops          uint64    `json:"tx_drops"`
}

type Address struct {
	Address      string `json:"address"`
	PrefixLength int    `json:"prefix_length"`
}

func New(sysRoot string) *Collector {
	return &Collector{SysRoot: sysRoot, Now: time.Now, Interfaces: net.Interfaces, Addresses: func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }}
}
func (collector *Collector) Name() string { return "network" }

func (collector *Collector) Collect(context.Context) ([]telemetry.Metric, error) {
	discover := collector.Interfaces
	if discover == nil {
		discover = net.Interfaces
	}
	interfaces, err := discover()
	if err != nil {
		return nil, fmt.Errorf("discover network interfaces: %w", err)
	}
	addresses := collector.Addresses
	if addresses == nil {
		addresses = func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }
	}
	state := State{Interfaces: make([]InterfaceState, 0, len(interfaces))}
	for _, iface := range interfaces {
		current := InterfaceState{Name: iface.Name, Index: iface.Index, MAC: iface.HardwareAddr.String(), MTU: iface.MTU, Flags: flagNames(iface.Flags)}
		if values, addressErr := addresses(iface); addressErr == nil {
			current.Addresses = parseAddresses(values)
		}
		collector.readSysfs(&current)
		state.Interfaces = append(state.Interfaces, current)
	}
	sort.Slice(state.Interfaces, func(i, j int) bool { return state.Interfaces[i].Name < state.Interfaces[j].Name })
	metric := telemetry.Metric{Path: []string{"network"}, Value: state, ObservedAt: collector.Now().UTC()}
	return []telemetry.Metric{metric}, nil
}

func (collector *Collector) readSysfs(iface *InterfaceState) {
	root := filepath.Join(collector.SysRoot, "class/net", iface.Name)
	if value := readText(filepath.Join(root, "operstate")); value != "" {
		iface.OperationalState = value
	}
	if value, ok := readInt(filepath.Join(root, "carrier")); ok {
		carrier := value != 0
		iface.Carrier = &carrier
	}
	if value, ok := readFloat(filepath.Join(root, "speed")); ok && value >= 0 {
		iface.SpeedMbps = &value
	}
	stats := map[string]*uint64{
		"rx_bytes": &iface.RXBytes, "rx_packets": &iface.RXPackets, "rx_errors": &iface.RXErrors, "rx_dropped": &iface.RXDrops,
		"tx_bytes": &iface.TXBytes, "tx_packets": &iface.TXPackets, "tx_errors": &iface.TXErrors, "tx_dropped": &iface.TXDrops,
	}
	for name, target := range stats {
		if value, ok := readUint(filepath.Join(root, "statistics", name)); ok {
			*target = value
		}
	}
}

func parseAddresses(values []net.Addr) []Address {
	addresses := make([]Address, 0, len(values))
	for _, value := range values {
		ip, network, err := net.ParseCIDR(value.String())
		prefixLength := 0
		if err != nil || ip == nil {
			ip = net.ParseIP(strings.Split(value.String(), "%")[0])
			if ip == nil {
				continue
			}
			if ip.To4() != nil {
				prefixLength = 32
			} else {
				prefixLength = 128
			}
		} else {
			prefixLength = bitsOnes(network.Mask)
		}
		addresses = append(addresses, Address{Address: ip.String(), PrefixLength: prefixLength})
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Address < addresses[j].Address })
	return addresses
}

func bitsOnes(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
}

func flagNames(flags net.Flags) []string {
	names := make([]string, 0, 4)
	values := []struct {
		flag net.Flags
		name string
	}{{net.FlagUp, "up"}, {net.FlagBroadcast, "broadcast"}, {net.FlagLoopback, "loopback"}, {net.FlagPointToPoint, "point_to_point"}, {net.FlagMulticast, "multicast"}}
	for _, value := range values {
		if flags&value.flag != 0 {
			names = append(names, value.name)
		}
	}
	return names
}

func readText(path string) string {
	value, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func readInt(path string) (int64, bool) {
	value, err := strconv.ParseInt(readText(path), 10, 64)
	return value, err == nil
}

func readFloat(path string) (float64, bool) {
	value, err := strconv.ParseFloat(readText(path), 64)
	return value, err == nil
}

func readUint(path string) (uint64, bool) {
	value, err := strconv.ParseUint(readText(path), 10, 64)
	return value, err == nil
}
