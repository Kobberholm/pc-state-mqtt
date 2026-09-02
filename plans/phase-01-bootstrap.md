# Phase 1 Handoff: Repository and Contracts

## Outcome

Establish version `0.1.0` of `pc-state-mqtt`: repository structure, telemetry contract, layered configuration, broker-free CLI output, tests, and project documentation.

## Current status

This phase is complete and merged into `main` at merge commit `464f0dc`. The implementation commit is `f2f3897` on `feature/bootstrap`. Do not reimplement or recommit this phase. Use this file to understand the baseline required by later phases and to verify that later work has not regressed it.

The active development branch may be ahead of `main` and may have uncommitted work. Always run `git status --short --branch` before doing anything. Never discard or overwrite uncommitted changes.

## Required baseline

- Go module: `pc-state-mqtt`, Go 1.25.
- Thin executable: `cmd/pc-state-mqtt/main.go`.
- Application orchestration: `internal/app`.
- CLI parsing: `internal/cli` using the standard `flag` package.
- Configuration: `internal/config`, parsed with `github.com/pelletier/go-toml/v2`.
- Version constant: `internal/version/version.go`.
- Public telemetry contract: `pkg/telemetry`.
- Public collector contract and aggregation: `pkg/collector`.
- Primary documents: `README.md`, `CHANGELOG.md`, `PLAN.md`, `config.example.toml`.

## Contract that later phases must preserve

### Configuration precedence

The effective configuration must be resolved in this order, from lowest to highest priority:

1. Built-in defaults.
2. TOML file.
3. `PC_STATE_MQTT_*` environment variables.
4. Explicit CLI flags.

Defaults include:

- Broker: `tcp://localhost:1883`.
- Topic root: `pc-state`.
- Host ID: local hostname.
- Sample interval: 5 seconds.
- Discovery interval: 60 seconds.
- Collector timeout: 3 seconds.
- Proc root: `/proc`.
- Sys root: `/sys`.
- Docker socket: `/var/run/docker.sock`.
- All collectors enabled.

Passwords must never be included in diagnostic strings. TLS client certificate and key must be configured as a pair. Configuration files written by the app must use mode `0600`.

### Telemetry contract

- Base topic: `<topic_root>/<host_id>`.
- A metric owns path segments below that base topic.
- Unsafe topic-segment characters are percent-encoded; a source identifier must not be able to add topic hierarchy.
- Payload envelope: `{ "value": ..., "observed_at": "...", "unit": "..." }` where `unit` is omitted when empty.
- Timestamps are UTC RFC3339Nano.
- Schema version is `1.0`.
- QoS is 1.
- Identity, metadata, and availability are retained.
- Fast-changing telemetry is not retained.
- Collector failures appear as diagnostics and must not remove successful collector data.

### CLI contract

- `--once`: collect and print indented JSON without connecting to MQTT.
- `--version`: print only the current version.
- `--help`: print help and exit successfully.
- `--config`: use an explicit TOML path and fail if it does not exist.
- Unexpected positional arguments fail.
- `cmd/pc-state-mqtt/main.go` remains signal-aware and delegates to `internal/app`.

## Verification procedure

Run from the repository root:

```sh
make check
./pc-state-mqtt --version
./pc-state-mqtt --once | jq .
./pc-state-mqtt --help
```

Expected results:

- `make check` formats, vets, tests, and builds successfully.
- Version output is `0.1.0` on the Phase 1 merge commit.
- `--once` returns a valid snapshot with `schema_version`, `host_id`, `observed_at`, `metrics`, and optional diagnostics.
- Help exits with status 0.

Focused tests that protect this phase are in:

- `pkg/telemetry/telemetry_test.go`.
- `pkg/collector/collector_test.go`.
- `internal/config/config_test.go`.
- `internal/cli/cli_test.go`.
- `internal/app/app_test.go`.

## Rules for agents starting a later phase

1. Do not alter the topic escaping or envelope schema casually. Treat those as public API changes.
2. Preserve configuration precedence when adding every new option.
3. Add TOML, environment, and CLI coverage for user-facing options where applicable.
4. Keep filesystem roots and clocks injectable for fixture tests.
5. Keep executables thin; place behavior in `internal` or reusable contracts in `pkg`.
6. Update `README.md` and `CHANGELOG.md` in every feature phase.
7. Increment the minor version exactly once when a phase is complete and ready to merge, not for intermediate commits.
8. Use existing Makefile targets. Do not replace `make mqtt/up`, `make mqtt/down`, or `make mqtt/logs` with ad hoc Compose commands in documentation.

## Definition of done

This phase is already done when commit `464f0dc` is reachable from `main`, its baseline tests still pass, and later work has not broken the contracts above.
