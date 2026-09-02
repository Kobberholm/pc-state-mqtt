package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPrecedenceAndRedaction(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.toml")
	contents := []byte(`
host_id = "from-file"
topic_root = "file-root"
sample_interval = "10s"

[mqtt]
broker_url = "tcp://file:1883"
username = "file-user"
password = "file-secret"

[collectors]
docker = false
`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	environment := map[string]string{
		"PC_STATE_MQTT_HOST_ID":          "from-env",
		"PC_STATE_MQTT_SAMPLE_INTERVAL":  "7s",
		"PC_STATE_MQTT_PASSWORD":         "env-secret",
		"PC_STATE_MQTT_COLLECTOR_DOCKER": "true",
		"PC_STATE_MQTT_CPU_PER_CORE":     "true",
		"PC_STATE_MQTT_MEMORY_PER_FIELD": "true",
	}
	configuration, err := Load(path, true, "default-host", func(key string) (string, bool) {
		value, exists := environment[key]
		return value, exists
	})
	if err != nil {
		t.Fatal(err)
	}

	if configuration.HostID != "from-env" || configuration.TopicRoot != "file-root" {
		t.Fatalf("unexpected configuration: %+v", configuration)
	}
	if configuration.SampleInterval.Duration != 7*time.Second {
		t.Fatalf("sample interval = %s", configuration.SampleInterval.Duration)
	}
	if !configuration.Collectors.Docker || !configuration.Collectors.CPUPerCore || !configuration.Collectors.MemoryPerField {
		t.Fatal("environment did not override collector settings")
	}
	if output := configuration.String(); strings.Contains(output, "env-secret") || !strings.Contains(output, "<redacted>") {
		t.Fatalf("configuration string exposes secret: %s", output)
	}
}

func TestLoadMissingOptionalFileUsesDefaults(t *testing.T) {
	configuration, err := Load(filepath.Join(t.TempDir(), "missing.toml"), false, "host", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if configuration.MQTT.BrokerURL != "tcp://localhost:1883" || !configuration.Collectors.CPU || configuration.Collectors.CPUPerCore || configuration.Collectors.MemoryPerField {
		t.Fatalf("unexpected defaults: %+v", configuration)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{"empty host", func(configuration *Config) { configuration.HostID = "" }},
		{"zero interval", func(configuration *Config) { configuration.SampleInterval.Duration = 0 }},
		{"certificate without key", func(configuration *Config) { configuration.MQTT.CertFile = "client.crt" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := Defaults("host")
			test.change(&configuration)
			if err := configuration.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSaveWritesSecureReloadableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	configuration := Defaults("saved-host")
	configuration.Collectors.GPU = false
	if err := Save(path, configuration); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("permissions = %o, want 600", permissions)
	}
	loaded, err := Load(path, true, "other-host", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HostID != "saved-host" || loaded.Collectors.GPU {
		t.Fatalf("loaded config = %+v", loaded)
	}
}

func TestSaveCollectorsPreservesOtherConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	contents := []byte("host_id = \"saved-host\"\n[mqtt]\npassword = \"secret\"\n[collectors]\ncpu = true\n")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	collectors := Defaults("host").Collectors
	collectors.CPU = false
	if err := SaveCollectors(path, collectors); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path, true, "other-host", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HostID != "saved-host" || loaded.MQTT.Password != "secret" || loaded.Collectors.CPU {
		t.Fatalf("loaded config = %+v", loaded)
	}
}
