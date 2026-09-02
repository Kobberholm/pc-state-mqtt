# Changelog

## Unreleased

- Collect aggregate and per-logical-CPU utilization from procfs with optional per-core clock data from sysfs.
- Collect memory, cache, buffers, and swap values from procfs with byte-normalized output.
- Run collectors concurrently with independent timeouts, deterministic output, and partial-failure diagnostics.
- Include enabled CPU and memory metrics in broker-free one-shot snapshots.
- Add a localhost-only Mosquitto Docker Compose stack with a quiet process health check and Makefile targets for MQTT integration testing.
- Add the `pc-state-mqtt-watch` companion client with reconnecting topic subscriptions, automatic JSON formatting, TLS/auth configuration, and default-on terminal colors.
- Add a `--tui` feature settings interface with atomic configuration saving and a scrollable, automatically refreshing system monitor.
- Show per-feature gather/send state and independent update countdowns in the TUI live watch, with explicit event-driven cadence support for future collectors.
- Treat CLI help as a successful command instead of reporting `flag: help requested`.

## 0.1.0 - 2026-09-03

- Establish the Go module, package boundaries, and feature-branch development plan.
- Add the versioned telemetry schema, topic-safe identifiers, JSON metric envelopes, and one-shot snapshot output.
- Add layered TOML, environment, and CLI configuration with validation and credential redaction.
- Add version/help handling, cancellation-aware application startup, build targets, tests, example configuration, and initial documentation.