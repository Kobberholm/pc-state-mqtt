# pc-state-mqtt

`pc-state-mqtt` is a Linux service for collecting PC hardware and runtime state and publishing it to hierarchical MQTT topics. Future releases will accept a deliberately constrained command protocol from the broker.

Version 0.2.0 provides core host telemetry, layered configuration, CLI, broker-free one-shot output, and MQTT/TUI publishing.

## Requirements

- Linux
- Go 1.25 or newer for building from source

Core telemetry reads world-readable `/proc` and `/sys` files and uses `statfs` for mounted filesystems; no elevated privileges are required on a standard Linux host. Docker telemetry requires access to the Docker socket. Some GPU sensors may require membership in the `video` or `render` group. Hyprland enrichment requires access to the active user session. If a sensor, mount, interface, or optional field is unavailable, collection continues and the snapshot reports a diagnostic where the source itself fails.

## Build and test

```sh
make check
make build
```

The binaries are written to `./pc-state-mqtt` and `./pc-state-mqtt-watch`. `make check` formats the code, runs `go vet`, executes all unit tests, and builds both executables.

### Local MQTT test broker

Start the localhost-only Mosquitto test broker with Docker Compose:

```sh
make mqtt/up
```

The broker listens on `tcp://localhost:1883`, permits anonymous connections, keeps no persistent state, and is intended only for local development. Override the host port when 1883 is occupied:

```sh
MQTT_TEST_PORT=1884 make mqtt/up
```

View broker logs with `make mqtt/logs` and remove the stack with `make mqtt/down`. The Compose project uses `compose.test.yaml` and the test-only configuration in `test/mosquitto/mosquitto.conf`.

## Usage

Print the current version:

```sh
./pc-state-mqtt --version
```

Produce a broker-free JSON snapshot:

```sh
./pc-state-mqtt --once
```

Connect to the configured broker and publish continuously:

```sh
./pc-state-mqtt
```

The daemon publishes immediately after connecting and every `sample_interval` thereafter. It automatically reconnects and republishes the complete current state. `availability` is retained and changes to `online` after a connection acknowledgement; graceful shutdown publishes `offline`, while an unexpected disconnect uses the retained last will.

Use an explicit configuration and override selected values:

```sh
./pc-state-mqtt --once \
  --config ./config.example.toml \
  --host-id workstation \
  --sample-interval 2s
```

CPU, memory, thermal, storage, and network data are each published as one structured metric by default. Restore separate CPU topics with `--cpu-per-core`, `PC_STATE_MQTT_CPU_PER_CORE=true`, or `cpu_per_core = true` under `[collectors]`. Restore separate memory-property topics with `--memory-per-field`, `PC_STATE_MQTT_MEMORY_PER_FIELD=true`, or `memory_per_field = true`.

Run `./pc-state-mqtt --help` for all CLI options.

### Interactive TUI

Open the feature settings and live monitoring interface with:

```sh
./pc-state-mqtt --tui
# or
make tui
```

The settings view exposes every collector configured under `[collectors]`. CPU, memory, thermal, storage, and network are marked `available`; later collectors remain `planned` and do not fabricate monitoring data.

- `Up`/`Down` or `j`/`k`: select a feature.
- `Space` or `Enter`: enable or disable the selected feature for this session.
- `s`: atomically save the collector settings to the active TOML configuration with mode `0600`; other settings and credentials are preserved.
- `m`: open the built-in live watch using the current feature selection.
- `q`: quit.

The live-watch view gives every enabled feature its own runtime row with collection state, last gather time, metric count, last send state, and a countdown to its next update. CPU, memory, thermal, and network use `sample_interval`; storage discovery uses `discovery_interval`.

Collected metric paths and values appear below the schedule as the exact publish-payload preview. The TUI connects to the configured broker asynchronously and publishes each feature update. Its `Sent` column changes from `not sent` to a timestamp only after every metric in that update receives its QoS 1 acknowledgement. Connection, collector, and publish errors remain visible without blocking unrelated features.

The live watch ticks once per second and supports scrolling with `Up`/`Down`, `j`/`k`, `Page Up`, and `Page Down`. Press `r` to gather every enabled, implemented feature immediately or `Esc`/`m` to return to feature settings.

`--tui` uses the same configuration, environment variables, and CLI overrides as one-shot and daemon modes. It cannot be combined with `--once`.

### Watch MQTT messages

`pc-state-mqtt-watch` connects to the configured broker and subscribes to all telemetry for the configured host:

```sh
./pc-state-mqtt-watch
```

The default topic filter is `pc-state/<hostname>/#`. Each received topic and payload is printed immediately. Valid JSON payloads are detected and indented automatically; other payloads are printed unchanged. Topic and payload colors are enabled by default and can be disabled explicitly:

```sh
./pc-state-mqtt-watch --color=false
```

Subscribe to another host, a broader wildcard, or a specific subtree with `--host-id` or `--topic`:

```sh
./pc-state-mqtt-watch --host-id workstation
./pc-state-mqtt-watch --topic 'pc-state/+/#'
./pc-state-mqtt-watch --topic 'pc-state/workstation/cpu/#' --color=false
```

The watcher uses the same configuration file, broker URL, TLS files, username, password, topic root, host ID, and `PC_STATE_MQTT_*` environment variables as the publisher. Its default client ID adds `-watch` to the configured publisher client ID; override it with `--client-id` when running multiple watchers. `make watch` starts it with defaults.

## Configuration

The default configuration path is `$XDG_CONFIG_HOME/pc-state-mqtt/config.toml`, normally `~/.config/pc-state-mqtt/config.toml`. See `config.example.toml` for every setting.

Configuration precedence, from lowest to highest, is:

1. Built-in defaults.
2. TOML configuration.
3. `PC_STATE_MQTT_*` environment variables.
4. Explicit CLI options.

Common environment variables include `PC_STATE_MQTT_BROKER_URL`, `PC_STATE_MQTT_USERNAME`, `PC_STATE_MQTT_PASSWORD`, `PC_STATE_MQTT_HOST_ID`, `PC_STATE_MQTT_TOPIC_ROOT`, and `PC_STATE_MQTT_SAMPLE_INTERVAL`. Individual collectors use variables such as `PC_STATE_MQTT_COLLECTOR_DOCKER=false`.

The CPU collector defaults to one MQTT topic and JSON object containing `total` and `cores`. Set `PC_STATE_MQTT_CPU_PER_CORE=true` to emit the previous separate property topics instead.

The memory collector similarly defaults to one `memory` object. Set `PC_STATE_MQTT_MEMORY_PER_FIELD=true` to emit the previous separate memory-property topics instead.

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

By default, the `pc-state/<host>/cpu` envelope groups aggregate and logical-CPU readings:

```json
{
  "value": {
    "identity": {
      "model_name": "Example CPU",
      "vendor_id": "GenuineIntel",
      "architecture": "amd64",
      "logical_cpu_count": 8,
      "online_cpu_count": 8,
      "online_cpus": ["0", "1", "2", "3", "4", "5", "6", "7"]
    },
    "total": { "usage_percent": 18.75 },
    "cores": {
      "0": {
        "usage_percent": 12.5,
        "online": true,
        "clock_current_mhz": 2400,
        "clock_min_mhz": 800,
        "clock_max_mhz": 4800
      }
    }
  },
  "observed_at": "2026-09-03T11:30:00Z"
}
```

With per-core output enabled, the collector instead emits separate paths such as `cpu/total/usage_percent`, `cpu/0/usage_percent`, and `cpu/0/clock_current_mhz` as in earlier development builds.

The default `pc-state/<host>/memory` envelope groups byte-normalized memory and swap readings:

```json
{
  "value": {
    "total_bytes": 34359738368,
    "available_bytes": 17179869184,
    "used_bytes": 17179869184,
    "cached_bytes": 4294967296,
    "buffers_bytes": 134217728,
    "swap_total_bytes": 8589934592,
    "swap_free_bytes": 6442450944,
    "swap_used_bytes": 2147483648
  },
  "observed_at": "2026-09-03T11:30:00Z"
}
```

With per-field output enabled, the collector emits the previous paths such as `memory/total_bytes`, `memory/available_bytes`, and `memory/used_bytes`.

The `thermal` object contains sorted `sensors` entries with `chip`, `name`, `type`, `value`, and explicit units (`C`, `RPM`, `V`, or `W`). Hwmon is preferred and thermal zones are a fallback.

The `storage` object contains `mounts` (total, available, and used bytes), `blocks` (model, vendor, rotational state, sector size, and capacity), and cumulative `disk_io` counters. Pseudo-filesystems are excluded by default; no rates are invented.

The `network` object contains sorted interfaces with kernel name/index, MAC, MTU, flags, IPv4/IPv6 addresses and prefix lengths, operational state, carrier/speed, and cumulative RX/TX byte, packet, error, and drop counters. Link-local addresses are retained.

The schema version is `1.0`. Identity, metadata, and availability topics are retained at QoS 1. Frequently changing samples are unretained at QoS 1. Cumulative kernel counters are published as cumulative values so subscribers can derive rates over their preferred interval.

## Planned collectors

- CPU identity, online state, utilization, and logical-core clocks are available in one combined object by default or separate per-core topics when enabled.
- Memory, cache, and swap usage are available in development as one combined object by default or separate property topics when enabled.
- GPU identity, utilization, clocks, VRAM, temperature, fan, and power.
- DRM connector and EDID monitor identity, modes, and optional Hyprland layout.
- Docker container identity, state, health, CPU, memory, block I/O, and network state.

Unavailable optional data sources will produce diagnostics without stopping successful collectors.

## Commands

The initial command topic contract will validate requests and respond with `unsupported`. It will not contain a shell executor, Docker mutations, display controls, or any other path that changes system state. Command execution is reserved for a later design and release.

## Development workflow

Development is performed on `feature/*` branches. Every feature merge must update this README and `CHANGELOG.md` as applicable, increment the minor version exactly once, pass `make check`, and then merge into `main`.

See [PLAN.md](PLAN.md) for the staged roadmap. The self-contained files under [plans](plans) provide explicit per-phase handoffs for agents without prior project context.