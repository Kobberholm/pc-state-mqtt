# pc-state-mqtt

`pc-state-mqtt` is a Linux service for collecting PC hardware and runtime state and publishing it to hierarchical MQTT topics. Future releases will accept a deliberately constrained command protocol from the broker.

Version 0.1.0 establishes the telemetry schema, layered configuration, CLI, and broker-free one-shot output. Hardware collectors and live MQTT publishing are implemented in the following planned feature releases; daemon mode currently exits with an explicit not-implemented error.

## Requirements

- Linux
- Go 1.25 or newer for building from source

Most planned metrics use world-readable procfs and sysfs files. Docker telemetry requires access to the Docker socket. Some GPU sensors may require membership in the `video` or `render` group. Hyprland enrichment requires access to the active user session.

## Build and test

```sh
make check
make build
```

The binary is written to `./pc-state-mqtt`. `make check` formats the code, runs `go vet`, executes all unit tests, and builds the executable.

## Usage

Print the current version:

```sh
./pc-state-mqtt --version
```

Produce a broker-free JSON snapshot:

```sh
./pc-state-mqtt --once
```

Use an explicit configuration and override selected values:

```sh
./pc-state-mqtt --once \
  --config ./config.example.toml \
  --host-id workstation \
  --sample-interval 2s
```

Run `./pc-state-mqtt --help` for all CLI options.

## Configuration

The default configuration path is `$XDG_CONFIG_HOME/pc-state-mqtt/config.toml`, normally `~/.config/pc-state-mqtt/config.toml`. See `config.example.toml` for every setting.

Configuration precedence, from lowest to highest, is:

1. Built-in defaults.
2. TOML configuration.
3. `PC_STATE_MQTT_*` environment variables.
4. Explicit CLI options.

Common environment variables include `PC_STATE_MQTT_BROKER_URL`, `PC_STATE_MQTT_USERNAME`, `PC_STATE_MQTT_PASSWORD`, `PC_STATE_MQTT_HOST_ID`, `PC_STATE_MQTT_TOPIC_ROOT`, and `PC_STATE_MQTT_SAMPLE_INTERVAL`. Individual collectors use variables such as `PC_STATE_MQTT_COLLECTOR_DOCKER=false`.

Prefer `PC_STATE_MQTT_PASSWORD` over storing a password in TOML. If a configuration file contains credentials, restrict it to mode `0600`. Configuration diagnostics redact passwords.

Client certificate authentication requires both `cert_file` and `key_file`. `ca_file` can be configured independently for broker certificate verification.

## Telemetry contract

Leaf topics use this shape:

```text
pc-state/<host>/<domain>/<entity>/<property>
```

Topic segments retain letters, digits, `.`, `_`, and `-`. Other characters are percent-encoded, so identifiers containing `/`, spaces, or `%` cannot alter the hierarchy.

Every leaf payload uses the same JSON envelope:

```json
{
  "value": 42,
  "observed_at": "2026-09-03T11:30:00Z",
  "unit": "percent"
}
```

The schema version is `1.0`. Identity, metadata, and availability topics will be retained at QoS 1. Frequently changing samples will be unretained at QoS 1. Cumulative kernel counters are published as cumulative values so subscribers can derive rates over their preferred interval.

## Planned collectors

- CPU utilization, identity, online state, and per-core clocks.
- Memory, cache, and swap usage.
- Thermal, fan, voltage, and power sensors from hwmon and thermal zones.
- Mounted filesystem usage, block-device identity, and disk I/O counters.
- Network interfaces, addresses, link state, and traffic counters.
- GPU identity, utilization, clocks, VRAM, temperature, fan, and power.
- DRM connector and EDID monitor identity, modes, and optional Hyprland layout.
- Docker container identity, state, health, CPU, memory, block I/O, and network state.

Unavailable optional data sources will produce diagnostics without stopping successful collectors.

## Commands

The initial command topic contract will validate requests and respond with `unsupported`. It will not contain a shell executor, Docker mutations, display controls, or any other path that changes system state. Command execution is reserved for a later design and release.

## Development workflow

Development is performed on `feature/*` branches. Every feature merge must update this README and `CHANGELOG.md` as applicable, increment the minor version exactly once, pass `make check`, and then merge into `main`.

See `PLAN.md` for the complete staged implementation plan.