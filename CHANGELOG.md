# Changelog

## Unreleased

- Collect CPU identity and online state, thermal sensors, mounted filesystem and block-device telemetry, and network interface counters.
- Register the Phase 2 collectors in one-shot output and the TUI, with deterministic structured output and isolated optional-source failures.

## 0.2.0 - 2026-09-03

- Complete core host telemetry for CPU identity/online state, thermal, storage, and network data.

- Collect aggregate and per-logical-CPU utilization from procfs with optional per-core clock data from sysfs, grouped into one JSON object by default with configurable separate per-core topics.
- Collect memory, cache, buffers, and swap values from procfs with byte-normalized output, grouped into one JSON object by default with configurable separate property topics.
- Run collectors concurrently with independent timeouts, deterministic output, and partial-failure diagnostics.
- Include enabled CPU and memory metrics in broker-free one-shot snapshots.
- Add a localhost-only Mosquitto Docker Compose stack with a quiet process health check and Makefile targets for MQTT integration testing.
- Add the `pc-state-mqtt-watch` companion client with reconnecting topic subscriptions, automatic JSON formatting, TLS/auth configuration, and default-on terminal colors.
- Add a `--tui` feature settings interface with atomic configuration saving and a scrollable, automatically refreshing system monitor.
- Show per-feature gather/send state and independent update countdowns in the TUI live watch, with explicit event-driven cadence support for future collectors.
- Publish telemetry to MQTT at QoS 1 with retained metadata and availability, automatic reconnect and full republish, and graceful or last-will offline state.
- Connect the TUI live watch to MQTT and record per-feature send timestamps only after broker acknowledgements.
- Treat CLI help as a successful command instead of reporting `flag: help requested`.

## 0.1.0 - 2026-09-03

- Establish the Go module, package boundaries, and feature-branch development plan.
- Add the versioned telemetry schema, topic-safe identifiers, JSON metric envelopes, and one-shot snapshot output.
- Add layered TOML, environment, and CLI configuration with validation and credential redaction.
- Add version/help handling, cancellation-aware application startup, build targets, tests, example configuration, and initial documentation.