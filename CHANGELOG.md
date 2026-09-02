# Changelog

## Unreleased

- Collect aggregate and per-logical-CPU utilization from procfs with optional per-core clock data from sysfs.
- Collect memory, cache, buffers, and swap values from procfs with byte-normalized output.
- Run collectors concurrently with independent timeouts, deterministic output, and partial-failure diagnostics.
- Include enabled CPU and memory metrics in broker-free one-shot snapshots.

## 0.1.0 - 2026-09-03

- Establish the Go module, package boundaries, and feature-branch development plan.
- Add the versioned telemetry schema, topic-safe identifiers, JSON metric envelopes, and one-shot snapshot output.
- Add layered TOML, environment, and CLI configuration with validation and credential redaction.
- Add version/help handling, cancellation-aware application startup, build targets, tests, example configuration, and initial documentation.