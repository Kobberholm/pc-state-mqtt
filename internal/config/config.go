package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Duration struct {
	time.Duration
}

func (duration *Duration) UnmarshalText(value []byte) error {
	parsed, err := time.ParseDuration(string(value))
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", value, err)
	}
	duration.Duration = parsed
	return nil
}

func (duration Duration) MarshalText() ([]byte, error) {
	return []byte(duration.String()), nil
}

type MQTT struct {
	BrokerURL string `toml:"broker_url"`
	ClientID  string `toml:"client_id"`
	Username  string `toml:"username"`
	Password  string `toml:"password"`
	CAFile    string `toml:"ca_file"`
	CertFile  string `toml:"cert_file"`
	KeyFile   string `toml:"key_file"`
}

type Collectors struct {
	CPU            bool `toml:"cpu"`
	CPUPerCore     bool `toml:"cpu_per_core"`
	Memory         bool `toml:"memory"`
	MemoryPerField bool `toml:"memory_per_field"`
	Thermal        bool `toml:"thermal"`
	Storage        bool `toml:"storage"`
	Network        bool `toml:"network"`
	GPU            bool `toml:"gpu"`
	Display        bool `toml:"display"`
	Hyprland       bool `toml:"hyprland"`
	Docker         bool `toml:"docker"`
}

type Config struct {
	HostID            string     `toml:"host_id"`
	TopicRoot         string     `toml:"topic_root"`
	SampleInterval    Duration   `toml:"sample_interval"`
	DiscoveryInterval Duration   `toml:"discovery_interval"`
	CollectorTimeout  Duration   `toml:"collector_timeout"`
	ProcRoot          string     `toml:"proc_root"`
	SysRoot           string     `toml:"sys_root"`
	DockerSocket      string     `toml:"docker_socket"`
	MQTT              MQTT       `toml:"mqtt"`
	Collectors        Collectors `toml:"collectors"`
}

type LookupEnv func(string) (string, bool)

func Defaults(hostname string) Config {
	return Config{
		HostID:            hostname,
		TopicRoot:         "pc-state",
		SampleInterval:    Duration{5 * time.Second},
		DiscoveryInterval: Duration{60 * time.Second},
		CollectorTimeout:  Duration{3 * time.Second},
		ProcRoot:          "/proc",
		SysRoot:           "/sys",
		DockerSocket:      "/var/run/docker.sock",
		MQTT: MQTT{
			BrokerURL: "tcp://localhost:1883",
			ClientID:  "pc-state-mqtt-" + hostname,
		},
		Collectors: Collectors{
			CPU: true, Memory: true, Thermal: true, Storage: true, Network: true,
			GPU: true, Display: true, Hyprland: true, Docker: true,
		},
	}
}

func DefaultPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find user config directory: %w", err)
	}
	return filepath.Join(directory, "pc-state-mqtt", "config.toml"), nil
}

func Load(path string, required bool, hostname string, lookup LookupEnv) (Config, error) {
	configuration := Defaults(hostname)
	if path != "" {
		contents, err := os.ReadFile(path)
		if err != nil && (required || !errors.Is(err, os.ErrNotExist)) {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
		if err == nil {
			if err := toml.Unmarshal(contents, &configuration); err != nil {
				return Config{}, fmt.Errorf("parse config %q: %w", path, err)
			}
		}
	}

	if lookup == nil {
		lookup = os.LookupEnv
	}
	if err := configuration.applyEnvironment(lookup); err != nil {
		return Config{}, err
	}
	if err := configuration.Validate(); err != nil {
		return Config{}, err
	}
	return configuration, nil
}

func Save(path string, configuration Config) error {
	if path == "" {
		return fmt.Errorf("config path must not be empty")
	}
	if err := configuration.Validate(); err != nil {
		return err
	}
	contents, err := toml.Marshal(configuration)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return writeFile(path, contents)
}

func SaveCollectors(path string, collectors Collectors) error {
	if path == "" {
		return fmt.Errorf("config path must not be empty")
	}
	document := make(map[string]any)
	contents, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read config %q: %w", path, err)
	}
	if err == nil {
		if err := toml.Unmarshal(contents, &document); err != nil {
			return fmt.Errorf("parse config %q: %w", path, err)
		}
	}
	document["collectors"] = collectors
	contents, err = toml.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode collector settings: %w", err)
	}
	return writeFile(path, contents)
}

func writeFile(path string, contents []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*.toml")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary config: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func (configuration *Config) applyEnvironment(lookup LookupEnv) error {
	stringsByKey := map[string]*string{
		"PC_STATE_MQTT_HOST_ID":       &configuration.HostID,
		"PC_STATE_MQTT_TOPIC_ROOT":    &configuration.TopicRoot,
		"PC_STATE_MQTT_BROKER_URL":    &configuration.MQTT.BrokerURL,
		"PC_STATE_MQTT_CLIENT_ID":     &configuration.MQTT.ClientID,
		"PC_STATE_MQTT_USERNAME":      &configuration.MQTT.Username,
		"PC_STATE_MQTT_PASSWORD":      &configuration.MQTT.Password,
		"PC_STATE_MQTT_CA_FILE":       &configuration.MQTT.CAFile,
		"PC_STATE_MQTT_CERT_FILE":     &configuration.MQTT.CertFile,
		"PC_STATE_MQTT_KEY_FILE":      &configuration.MQTT.KeyFile,
		"PC_STATE_MQTT_DOCKER_SOCKET": &configuration.DockerSocket,
	}
	for key, target := range stringsByKey {
		if value, exists := lookup(key); exists {
			*target = value
		}
	}

	durationsByKey := map[string]*Duration{
		"PC_STATE_MQTT_SAMPLE_INTERVAL":    &configuration.SampleInterval,
		"PC_STATE_MQTT_DISCOVERY_INTERVAL": &configuration.DiscoveryInterval,
		"PC_STATE_MQTT_COLLECTOR_TIMEOUT":  &configuration.CollectorTimeout,
	}
	for key, target := range durationsByKey {
		if value, exists := lookup(key); exists {
			if err := target.UnmarshalText([]byte(value)); err != nil {
				return fmt.Errorf("environment %s: %w", key, err)
			}
		}
	}

	collectorsByKey := map[string]*bool{
		"CPU": &configuration.Collectors.CPU, "MEMORY": &configuration.Collectors.Memory,
		"THERMAL": &configuration.Collectors.Thermal, "STORAGE": &configuration.Collectors.Storage,
		"NETWORK": &configuration.Collectors.Network, "GPU": &configuration.Collectors.GPU,
		"DISPLAY": &configuration.Collectors.Display, "HYPRLAND": &configuration.Collectors.Hyprland,
		"DOCKER": &configuration.Collectors.Docker,
	}
	if value, exists := lookup("PC_STATE_MQTT_CPU_PER_CORE"); exists {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("environment PC_STATE_MQTT_CPU_PER_CORE: %w", err)
		}
		configuration.Collectors.CPUPerCore = parsed
	}
	if value, exists := lookup("PC_STATE_MQTT_MEMORY_PER_FIELD"); exists {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("environment PC_STATE_MQTT_MEMORY_PER_FIELD: %w", err)
		}
		configuration.Collectors.MemoryPerField = parsed
	}
	for name, target := range collectorsByKey {
		key := "PC_STATE_MQTT_COLLECTOR_" + name
		if value, exists := lookup(key); exists {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("environment %s: %w", key, err)
			}
			*target = parsed
		}
	}
	return nil
}

func (configuration Config) Validate() error {
	if strings.TrimSpace(configuration.HostID) == "" {
		return fmt.Errorf("host ID must not be empty")
	}
	if strings.Trim(configuration.TopicRoot, " /") == "" {
		return fmt.Errorf("topic root must not be empty")
	}
	if configuration.SampleInterval.Duration <= 0 || configuration.DiscoveryInterval.Duration <= 0 || configuration.CollectorTimeout.Duration <= 0 {
		return fmt.Errorf("intervals and collector timeout must be positive")
	}
	if configuration.MQTT.BrokerURL == "" {
		return fmt.Errorf("MQTT broker URL must not be empty")
	}
	if (configuration.MQTT.CertFile == "") != (configuration.MQTT.KeyFile == "") {
		return fmt.Errorf("MQTT client certificate and key must be configured together")
	}
	return nil
}

func (configuration Config) String() string {
	return fmt.Sprintf("host_id=%q topic_root=%q broker_url=%q client_id=%q username=%q password=%s",
		configuration.HostID, configuration.TopicRoot, configuration.MQTT.BrokerURL,
		configuration.MQTT.ClientID, configuration.MQTT.Username, redacted(configuration.MQTT.Password))
}

func redacted(value string) string {
	if value == "" {
		return "<empty>"
	}
	return "<redacted>"
}
