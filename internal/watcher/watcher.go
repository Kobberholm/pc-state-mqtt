package watcher

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"pc-state-mqtt/internal/mqttclient"
	"pc-state-mqtt/internal/version"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(stderr, "pc-state-mqtt-watch: determine hostname: %v\n", err)
		return 1
	}
	options, err := ParseOptions(args, hostname, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "pc-state-mqtt-watch: %v\n", err)
		return 2
	}
	if options.Version {
		fmt.Fprintln(stdout, version.Current)
		return 0
	}
	if err := Watch(ctx, options, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "pc-state-mqtt-watch: %v\n", err)
		return 1
	}
	return 0
}

func Watch(ctx context.Context, options Options, stdout, stderr io.Writer) error {
	tlsConfig, err := mqttclient.TLSConfig(options.Config.MQTT)
	if err != nil {
		return err
	}

	var outputMutex sync.Mutex
	subscribed := make(chan struct{}, 1)
	subscriptionErrors := make(chan error, 1)
	clientOptions := mqtt.NewClientOptions().
		AddBroker(options.Config.MQTT.BrokerURL).
		SetClientID(options.ClientID).
		SetUsername(options.Config.MQTT.Username).
		SetPassword(options.Config.MQTT.Password).
		SetTLSConfig(tlsConfig).
		SetAutoReconnect(true)
	clientOptions.SetConnectionLostHandler(func(_ mqtt.Client, connectionError error) {
		fmt.Fprintf(stderr, "pc-state-mqtt-watch: connection lost: %v; reconnecting\n", connectionError)
	})
	clientOptions.SetOnConnectHandler(func(client mqtt.Client) {
		token := client.Subscribe(options.Topic, 1, func(_ mqtt.Client, message mqtt.Message) {
			outputMutex.Lock()
			defer outputMutex.Unlock()
			if err := WriteMessage(stdout, message.Topic(), message.Payload(), options.Color); err != nil {
				select {
				case subscriptionErrors <- fmt.Errorf("write message: %w", err):
				default:
				}
			}
		})
		if err := mqttclient.WaitToken(ctx, token); err != nil {
			select {
			case subscriptionErrors <- fmt.Errorf("subscribe to %q: %w", options.Topic, err):
			default:
			}
			return
		}
		select {
		case subscribed <- struct{}{}:
		default:
		}
	})

	client := mqtt.NewClient(clientOptions)
	if err := mqttclient.WaitToken(ctx, client.Connect()); err != nil {
		return fmt.Errorf("connect to %s: %w", options.Config.MQTT.BrokerURL, err)
	}
	defer client.Disconnect(250)

	select {
	case <-subscribed:
		fmt.Fprintf(stderr, "pc-state-mqtt-watch: subscribed to %s\n", options.Topic)
	case err := <-subscriptionErrors:
		return err
	case <-ctx.Done():
		return nil
	}

	select {
	case err := <-subscriptionErrors:
		return err
	case <-ctx.Done():
		return nil
	}
}
