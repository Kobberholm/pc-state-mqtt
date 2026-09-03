package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnescapeMountPath(t *testing.T) {
	if got := unescapeMountPath(`/var/lib/my\040data\011x\134y`); got != "/var/lib/my data\tx\\y" {
		t.Fatalf("path = %q", got)
	}
}

func TestParseMountInfoFiltersPseudoFilesystems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mountinfo")
	contents := "36 25 8:1 / /var/lib/my\\040data rw,relatime - ext4 /dev/sda1 rw\n37 25 0:1 / /proc rw - proc proc rw\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := parseMountInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "/var/lib/my data" || entries[0].Filesystem != "ext4" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestReadDiskStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diskstats")
	row := "8 0 sda 10 0 20 30 40 0 50 60 0 70 0\n"
	if err := os.WriteFile(path, []byte(row), 0o600); err != nil {
		t.Fatal(err)
	}
	stats, err := readDiskStats(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].ReadBytes != 20*512 || stats[0].WriteOperations != 40 {
		t.Fatalf("stats = %#v", stats)
	}
}
