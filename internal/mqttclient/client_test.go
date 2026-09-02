package mqttclient

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"pc-state-mqtt/internal/config"
)

type completedToken struct {
	err error
}

func (token completedToken) Wait() bool                       { return true }
func (token completedToken) WaitTimeout(_ time.Duration) bool { return true }
func (token completedToken) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
func (token completedToken) Error() error { return token.err }

var _ mqtt.Token = completedToken{}

func TestTLSConfigWithoutFilesIsNil(t *testing.T) {
	tlsConfig, err := TLSConfig(config.MQTT{})
	if err != nil {
		t.Fatal(err)
	}
	if tlsConfig != nil {
		t.Fatalf("TLS config = %#v", tlsConfig)
	}
}

func TestTLSConfigRejectsMissingCAFile(t *testing.T) {
	_, err := TLSConfig(config.MQTT{CAFile: filepath.Join(t.TempDir(), "missing.pem")})
	if err == nil {
		t.Fatal("expected missing CA error")
	}
}

func TestWaitTokenReturnsTokenError(t *testing.T) {
	expected := errors.New("publish failed")
	if err := WaitToken(context.Background(), completedToken{err: expected}); !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
}
