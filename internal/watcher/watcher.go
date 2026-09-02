package watcher

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"pc-state-mqtt/internal/config"
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
	tlsConfig, err := newTLSConfig(options.Config.MQTT)
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
		if err := waitToken(ctx, token); err != nil {
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
	if err := waitToken(ctx, client.Connect()); err != nil {
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

func newTLSConfig(configuration config.MQTT) (*tls.Config, error) {
	if configuration.CAFile == "" && configuration.CertFile == "" {
		return nil, nil
	}
	tlsConfiguration := &tls.Config{MinVersion: tls.VersionTLS12}
	if configuration.CAFile != "" {
		contents, err := os.ReadFile(configuration.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read MQTT CA certificate: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(contents) {
			return nil, fmt.Errorf("MQTT CA file contains no certificates")
		}
		tlsConfiguration.RootCAs = roots
	}
	if configuration.CertFile != "" {
		certificate, err := tls.LoadX509KeyPair(configuration.CertFile, configuration.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load MQTT client certificate: %w", err)
		}
		tlsConfiguration.Certificates = []tls.Certificate{certificate}
	}
	return tlsConfiguration, nil
}

func waitToken(ctx context.Context, token mqtt.Token) error {
	finished := make(chan struct{})
	go func() {
		token.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		return token.Error()
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(15 * time.Second):
		return fmt.Errorf("MQTT operation timed out")
	}
}
