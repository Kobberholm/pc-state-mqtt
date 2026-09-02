PREFIX ?= /usr/local
COMPOSE ?= docker compose
MQTT_TEST_COMPOSE_FILE ?= compose.test.yaml

.PHONY: build check fmt install mqtt-down mqtt-logs mqtt-up run test test-integration vet

build:
	go build -o pc-state-mqtt ./cmd/pc-state-mqtt

check: fmt vet test build

fmt:
	go fmt ./...

install:
	go install ./cmd/pc-state-mqtt

mqtt-up:
	$(COMPOSE) -f $(MQTT_TEST_COMPOSE_FILE) up -d --wait

mqtt-down:
	$(COMPOSE) -f $(MQTT_TEST_COMPOSE_FILE) down --remove-orphans

mqtt-logs:
	$(COMPOSE) -f $(MQTT_TEST_COMPOSE_FILE) logs -f mqtt

run:
	go run ./cmd/pc-state-mqtt --once

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

vet:
	go vet ./...