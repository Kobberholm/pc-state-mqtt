PREFIX ?= /usr/local

.PHONY: build check fmt install run test test-integration vet

build:
	go build -o pc-state-mqtt ./cmd/pc-state-mqtt

check: fmt vet test build

fmt:
	go fmt ./...

install:
	go install ./cmd/pc-state-mqtt

run:
	go run ./cmd/pc-state-mqtt --once

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

vet:
	go vet ./...