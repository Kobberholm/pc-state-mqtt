package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"pc-state-mqtt/internal/config"
)

type Options struct {
	Config     config.Config
	ConfigPath string
	Once       bool
	TUI        bool
	Version    bool
}

func Parse(args []string, hostname string, output io.Writer) (Options, error) {
	configPath, explicitConfig, err := findConfigPath(args)
	if err != nil {
		return Options{}, err
	}
	if configPath == "" {
		configPath, err = config.DefaultPath()
		if err != nil {
			return Options{}, err
		}
	}

	configuration, err := config.Load(configPath, explicitConfig, hostname, os.LookupEnv)
	if err != nil {
		return Options{}, err
	}

	flags := flag.NewFlagSet("pc-state-mqtt", flag.ContinueOnError)
	flags.SetOutput(output)
	var options Options
	options.Config = configuration
	options.ConfigPath = configPath
	flags.BoolVar(&options.Once, "once", false, "collect once and print JSON without connecting to MQTT")
	flags.BoolVar(&options.TUI, "tui", false, "open the interactive feature and monitoring interface")
	flags.BoolVar(&options.Version, "version", false, "print the application version")
	flags.String("config", configPath, "path to TOML configuration")
	flags.StringVar(&options.Config.HostID, "host-id", configuration.HostID, "host identifier used in MQTT topics")
	flags.StringVar(&options.Config.TopicRoot, "topic-root", configuration.TopicRoot, "MQTT topic root")
	flags.StringVar(&options.Config.MQTT.BrokerURL, "broker-url", configuration.MQTT.BrokerURL, "MQTT broker URL")
	flags.StringVar(&options.Config.MQTT.Username, "mqtt-username", configuration.MQTT.Username, "MQTT username")
	flags.StringVar(&options.Config.MQTT.Password, "mqtt-password", configuration.MQTT.Password, "MQTT password (prefer PC_STATE_MQTT_PASSWORD)")
	flags.DurationVar(&options.Config.SampleInterval.Duration, "sample-interval", configuration.SampleInterval.Duration, "metric sample interval")
	flags.DurationVar(&options.Config.DiscoveryInterval.Duration, "discovery-interval", configuration.DiscoveryInterval.Duration, "hardware discovery interval")
	flags.DurationVar(&options.Config.CollectorTimeout.Duration, "collector-timeout", configuration.CollectorTimeout.Duration, "timeout for each collector")
	flags.BoolVar(&options.Config.Collectors.CPUPerCore, "cpu-per-core", configuration.Collectors.CPUPerCore, "publish CPU properties as separate per-core topics")
	flags.BoolVar(&options.Config.Collectors.MemoryPerField, "memory-per-field", configuration.Collectors.MemoryPerField, "publish memory properties as separate topics")
	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if flags.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if options.Once && options.TUI {
		return Options{}, fmt.Errorf("--once and --tui cannot be used together")
	}
	if err := options.Config.Validate(); err != nil {
		return Options{}, err
	}
	return options, nil
}

func findConfigPath(args []string) (string, bool, error) {
	for index, argument := range args {
		if argument == "--config" {
			if index+1 >= len(args) {
				return "", false, fmt.Errorf("--config requires a path")
			}
			return args[index+1], true, nil
		}
		if strings.HasPrefix(argument, "--config=") {
			path := strings.TrimPrefix(argument, "--config=")
			if path == "" {
				return "", false, fmt.Errorf("--config requires a path")
			}
			return path, true, nil
		}
	}
	return "", false, nil
}
