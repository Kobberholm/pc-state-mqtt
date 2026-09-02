package mqttclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/pkg/telemetry"
)

type Client struct {
	client      mqtt.Client
	topicRoot   string
	hostID      string
	connections chan struct{}
	errors      chan error
}

func Connect(ctx context.Context, configuration config.Config) (*Client, error) {
	tlsConfig, err := TLSConfig(configuration.MQTT)
	if err != nil {
		return nil, err
	}
	connections := make(chan struct{}, 1)
	connectionErrors := make(chan error, 1)
	will := telemetry.Metric{
		Path: []string{"availability"}, Value: "offline", ObservedAt: time.Now().UTC(), Retention: telemetry.Retained,
	}
	willTopic, err := will.Topic(configuration.TopicRoot, configuration.HostID)
	if err != nil {
		return nil, err
	}
	willPayload, err := will.Payload()
	if err != nil {
		return nil, err
	}

	options := mqtt.NewClientOptions().
		AddBroker(configuration.MQTT.BrokerURL).
		SetClientID(configuration.MQTT.ClientID).
		SetUsername(configuration.MQTT.Username).
		SetPassword(configuration.MQTT.Password).
		SetTLSConfig(tlsConfig).
		SetAutoReconnect(true).
		SetConnectTimeout(10*time.Second).
		SetKeepAlive(30*time.Second).
		SetPingTimeout(10*time.Second).
		SetWill(willTopic, string(willPayload), 1, true)
	options.SetOnConnectHandler(func(connectedClient mqtt.Client) {
		go func() {
			online := telemetry.Metric{
				Path: []string{"availability"}, Value: "online", ObservedAt: time.Now().UTC(), Retention: telemetry.Retained,
			}
			payload, payloadError := online.Payload()
			if payloadError == nil {
				payloadError = WaitToken(context.Background(), connectedClient.Publish(willTopic, 1, true, payload))
			}
			if payloadError != nil {
				select {
				case connectionErrors <- fmt.Errorf("publish online availability: %w", payloadError):
				default:
				}
				return
			}
			select {
			case connections <- struct{}{}:
			default:
			}
		}()
	})
	options.SetConnectionLostHandler(func(_ mqtt.Client, connectionError error) {
		select {
		case connectionErrors <- connectionError:
		default:
		}
	})

	client := mqtt.NewClient(options)
	if err := WaitToken(ctx, client.Connect()); err != nil {
		return nil, fmt.Errorf("connect to %s: %w", configuration.MQTT.BrokerURL, err)
	}
	return &Client{
		client: client, topicRoot: configuration.TopicRoot, hostID: configuration.HostID,
		connections: connections, errors: connectionErrors,
	}, nil
}

func (client *Client) Connections() <-chan struct{} { return client.connections }

func (client *Client) Errors() <-chan error { return client.errors }

func (client *Client) Connected() bool { return client.client.IsConnectionOpen() }

func (client *Client) PublishMetrics(ctx context.Context, metrics []telemetry.Metric) (int, error) {
	if !client.Connected() {
		return 0, fmt.Errorf("MQTT client is disconnected")
	}
	published := 0
	for _, metric := range metrics {
		topic, err := metric.Topic(client.topicRoot, client.hostID)
		if err != nil {
			return published, err
		}
		payload, err := metric.Payload()
		if err != nil {
			return published, err
		}
		if err := WaitToken(ctx, client.client.Publish(topic, 1, metric.Retention == telemetry.Retained, payload)); err != nil {
			return published, fmt.Errorf("publish %s: %w", topic, err)
		}
		published++
	}
	return published, nil
}

func (client *Client) PublishAvailability(ctx context.Context, status string) error {
	metric := telemetry.Metric{
		Path: []string{"availability"}, Value: status, ObservedAt: time.Now().UTC(), Retention: telemetry.Retained,
	}
	_, err := client.PublishMetrics(ctx, []telemetry.Metric{metric})
	return err
}

func (client *Client) Close(ctx context.Context) error {
	var err error
	if client.Connected() {
		err = client.PublishAvailability(ctx, "offline")
	}
	client.client.Disconnect(250)
	return err
}

func TLSConfig(configuration config.MQTT) (*tls.Config, error) {
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

func WaitToken(ctx context.Context, token mqtt.Token) error {
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
