package storage

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"pc-state-mqtt/pkg/telemetry"
)

var pseudoFilesystems = map[string]bool{
	"autofs": true, "bdev": true, "binfmt_misc": true, "bpf": true,
	"cgroup": true, "cgroup2": true, "configfs": true, "debugfs": true,
	"devpts": true, "devtmpfs": true, "efivarfs": true, "fusectl": true,
	"hugetlbfs": true, "mqueue": true, "nsfs": true, "proc": true,
	"pstore": true, "ramfs": true, "rootfs": true, "rpc_pipefs": true,
	"securityfs": true, "selinuxfs": true, "sysfs": true, "tmpfs": true,
	"tracefs": true,
}

type Collector struct {
	ProcRoot string
	SysRoot  string
	Now      func() time.Time
	Statfs   func(string, *unix.Statfs_t) error
}

type State struct {
	Mounts []Mount     `json:"mounts"`
	Blocks []Block     `json:"blocks"`
	DiskIO []DiskStats `json:"disk_io"`
}

type Mount struct {
	Path           string `json:"path"`
	Filesystem     string `json:"filesystem"`
	Source         string `json:"source,omitempty"`
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
}

type Block struct {
	Name              string `json:"name"`
	Model             string `json:"model,omitempty"`
	Vendor            string `json:"vendor,omitempty"`
	Rotational        *bool  `json:"rotational,omitempty"`
	LogicalSectorSize uint64 `json:"logical_sector_size,omitempty"`
	CapacityBytes     uint64 `json:"capacity_bytes,omitempty"`
}

type DiskStats struct {
	Name                  string `json:"name"`
	ReadOperations        uint64 `json:"read_operations"`
	WriteOperations       uint64 `json:"write_operations"`
	ReadSectors           uint64 `json:"read_sectors"`
	WriteSectors          uint64 `json:"write_sectors"`
	ReadBytes             uint64 `json:"read_bytes"`
	WriteBytes            uint64 `json:"write_bytes"`
	ReadTimeMilliseconds  uint64 `json:"read_time_milliseconds"`
	WriteTimeMilliseconds uint64 `json:"write_time_milliseconds"`
	IOTimeMilliseconds    uint64 `json:"io_time_milliseconds"`
}

type mountEntry struct {
	Path       string
	Filesystem string
	Source     string
}

func New(procRoot, sysRoot string) *Collector {
	return &Collector{ProcRoot: procRoot, SysRoot: sysRoot, Now: time.Now, Statfs: unix.Statfs}
}
func (collector *Collector) Name() string { return "storage" }

func (collector *Collector) Collect(context.Context) ([]telemetry.Metric, error) {
	mounts, mountErr := collector.readMounts()
	blocks, blockErr := collector.readBlocks()
	diskIO, diskErr := readDiskStats(filepath.Join(collector.ProcRoot, "diskstats"))
	var failures []error
	for _, err := range []error{mountErr, blockErr, diskErr} {
		if err != nil {
			failures = append(failures, err)
		}
	}
	state := State{Mounts: mounts, Blocks: blocks, DiskIO: diskIO}
	metric := telemetry.Metric{Path: []string{"storage"}, Value: state, ObservedAt: collector.Now().UTC()}
	if len(failures) > 0 {
		return []telemetry.Metric{metric}, errors.Join(failures...)
	}
	return []telemetry.Metric{metric}, nil
}

func (collector *Collector) readMounts() ([]Mount, error) {
	entries, err := parseMountInfo(filepath.Join(collector.ProcRoot, "self/mountinfo"))
	if err != nil {
		return nil, err
	}
	mounts := make([]Mount, 0, len(entries))
	var failures []error
	statfs := collector.Statfs
	if statfs == nil {
		statfs = unix.Statfs
	}
	for _, entry := range entries {
		var stat unix.Statfs_t
		if err := statfs(entry.Path, &stat); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if os.IsPermission(err) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
				continue
			}
			failures = append(failures, fmt.Errorf("statfs %s: %w", entry.Path, err))
			continue
		}
		total := uint64(stat.Blocks) * uint64(stat.Bsize)
		free := uint64(stat.Bfree) * uint64(stat.Bsize)
		available := uint64(stat.Bavail) * uint64(stat.Bsize)
		used := uint64(0)
		if total > free {
			used = total - free
		}
		mounts = append(mounts, Mount{Path: entry.Path, Filesystem: entry.Filesystem, Source: entry.Source, TotalBytes: total, AvailableBytes: available, UsedBytes: used})
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })
	if len(failures) > 0 {
		return mounts, errors.Join(failures...)
	}
	return mounts, nil
}

func parseMountInfo(path string) ([]mountEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read mountinfo: %w", err)
	}
	defer file.Close()
	var entries []mountEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), " - ", 2)
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 6 || len(right) < 2 {
			continue
		}
		entry := mountEntry{Path: unescapeMountPath(left[4]), Filesystem: right[0], Source: unescapeMountPath(right[1])}
		if skipMount(entry) {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan mountinfo: %w", err)
	}
	return entries, nil
}

func skipMount(entry mountEntry) bool {
	filesystem := strings.ToLower(entry.Filesystem)
	if pseudoFilesystems[filesystem] || filesystem == "overlay" || filesystem == "fuse-overlayfs" {
		return true
	}
	// Docker's network namespaces are mounted as nsfs below one of these paths.
	path := filepath.Clean(entry.Path)
	return strings.HasPrefix(path, "/run/docker/netns/") ||
		strings.HasPrefix(path, "/var/run/docker/netns/")
}

func unescapeMountPath(value string) string {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && index+3 < len(value) {
			number, err := strconv.ParseUint(value[index+1:index+4], 8, 8)
			if err == nil {
				result.WriteByte(byte(number))
				index += 3
				continue
			}
		}
		result.WriteByte(value[index])
	}
	return result.String()
}

func (collector *Collector) readBlocks() ([]Block, error) {
	directories, err := filepath.Glob(filepath.Join(collector.SysRoot, "class/block", "*"))
	if err != nil {
		return nil, fmt.Errorf("scan block devices: %w", err)
	}
	sort.Strings(directories)
	blocks := make([]Block, 0, len(directories))
	for _, directory := range directories {
		name := filepath.Base(directory)
		block := Block{Name: name, Model: readTrimmed(filepath.Join(directory, "device/model")), Vendor: readTrimmed(filepath.Join(directory, "device/vendor"))}
		if value, ok := readUint(filepath.Join(directory, "queue/logical_block_size")); ok {
			block.LogicalSectorSize = value
		}
		if sectors, ok := readUint(filepath.Join(directory, "size")); ok {
			sectorSize := block.LogicalSectorSize
			if sectorSize == 0 {
				sectorSize = 512
			}
			block.CapacityBytes = sectors * sectorSize
		}
		if value, ok := readUint(filepath.Join(directory, "queue/rotational")); ok {
			rotational := value != 0
			block.Rotational = &rotational
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func readDiskStats(path string) ([]DiskStats, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read diskstats: %w", err)
	}
	defer file.Close()
	var stats []DiskStats
	var failures []error
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			continue
		}
		numbers := make([]uint64, len(fields)-3)
		valid := true
		for i := range numbers {
			numbers[i], err = strconv.ParseUint(fields[i+3], 10, 64)
			if err != nil {
				valid = false
				break
			}
		}
		if !valid {
			failures = append(failures, fmt.Errorf("parse diskstats row %q", fields[2]))
			continue
		}
		stats = append(stats, DiskStats{Name: fields[2], ReadOperations: numbers[0], ReadSectors: numbers[2], ReadBytes: numbers[2] * 512, ReadTimeMilliseconds: numbers[3], WriteOperations: numbers[4], WriteSectors: numbers[6], WriteBytes: numbers[6] * 512, WriteTimeMilliseconds: numbers[7], IOTimeMilliseconds: numbers[9]})
	}
	if err := scanner.Err(); err != nil {
		return stats, fmt.Errorf("scan diskstats: %w", err)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Name < stats[j].Name })
	if len(failures) > 0 {
		return stats, errors.Join(failures...)
	}
	return stats, nil
}

func readTrimmed(path string) string {
	value, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func readUint(path string) (uint64, bool) {
	value, err := strconv.ParseUint(readTrimmed(path), 10, 64)
	return value, err == nil
}
