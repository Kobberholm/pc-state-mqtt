package watcher

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/pkg/telemetry"
)

type Options struct {
	Config   config.Config
	Topic    string
	ClientID string
	Color    bool
	Version  bool
}

func ParseOptions(args []string, hostname string, output io.Writer) (Options, error) {
	configPath, explicitConfig, err := watcherConfigPath(args)
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
	options := Options{Config: configuration, ClientID: configuration.MQTT.ClientID + "-watch", Color: true}

	flags := flag.NewFlagSet("pc-state-mqtt-watch", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.String("config", configPath, "path to TOML configuration")
	flags.StringVar(&options.Config.HostID, "host-id", configuration.HostID, "host identifier used in the default topic")
	flags.StringVar(&options.Config.TopicRoot, "topic-root", configuration.TopicRoot, "MQTT topic root used by the default topic")
	flags.StringVar(&options.Config.MQTT.BrokerURL, "broker-url", configuration.MQTT.BrokerURL, "MQTT broker URL")
	flags.StringVar(&options.Config.MQTT.Username, "mqtt-username", configuration.MQTT.Username, "MQTT username")
	flags.StringVar(&options.Config.MQTT.Password, "mqtt-password", configuration.MQTT.Password, "MQTT password (prefer PC_STATE_MQTT_PASSWORD)")
	flags.StringVar(&options.ClientID, "client-id", options.ClientID, "MQTT client identifier")
	flags.StringVar(&options.Topic, "topic", "", "MQTT topic filter (default: <topic-root>/<host-id>/#)")
	flags.BoolVar(&options.Color, "color", true, "use ANSI colors in terminal output")
	flags.BoolVar(&options.Version, "version", false, "print the application version")
	if err := flags.Parse(args); err != nil {
		return Options{}, err
	}
	if flags.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if options.Topic == "" {
		options.Topic = strings.Join([]string{
			telemetry.Segment(options.Config.TopicRoot),
			telemetry.Segment(options.Config.HostID),
			"#",
		}, "/")
	}
	if strings.TrimSpace(options.ClientID) == "" {
		return Options{}, fmt.Errorf("MQTT client ID must not be empty")
	}
	if strings.TrimSpace(options.Topic) == "" {
		return Options{}, fmt.Errorf("MQTT topic filter must not be empty")
	}
	return options, nil
}

func watcherConfigPath(args []string) (string, bool, error) {
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
