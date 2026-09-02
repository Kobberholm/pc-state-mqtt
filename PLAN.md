# pc-state-mqtt Implementation Plan

Create an independent Go 1.25 repository that continuously collects Linux PC state and publishes it as versioned, timestamped leaf metrics over MQTT. Keep Linux integrations root-free where possible, degrade individual collectors cleanly, support broker-free one-shot JSON diagnostics, ship user and system systemd units, and reserve an inbound command contract that never executes commands in the initial release.

## Phase 1: Repository and contracts (`feature/bootstrap`, version `0.1.0`)

1. Create the idiomatic top-level layout: `cmd/pc-state-mqtt`, `internal/app`, `internal/cli`, `internal/config`, `internal/mqttclient`, `internal/command`, and public `pkg/telemetry` plus `pkg/collector` domain packages. Use module name `pc-state-mqtt` and Go 1.25.
2. Define the public telemetry contract in `pkg/telemetry`: a complete nested `Snapshot` for one-shot output; a flattened `Metric` containing path segments, typed value, unit, observation time, and retain class; collector/result/error types; deterministic topic-segment escaping; schema version `1.0`; and stable JSON field names. Use UTC RFC3339Nano timestamps and explicit units.
3. Define MQTT topics under configurable `<topic_root>/<host_id>` (default `pc-state/<hostname>`): `meta/*`, domain leaf topics, retained `availability`, inbound `command/#`, and `command-response/<command_id>`. Encode every metric as `{value, observed_at, unit?}`. Use QoS 1; retain metadata, identity, and availability; leave rapidly changing samples unretained.
4. Implement configuration with precedence `defaults < TOML < PC_STATE_MQTT_* environment < explicitly supplied CLI flags`. Default to `$XDG_CONFIG_HOME/pc-state-mqtt/config.toml`, broker `tcp://localhost:1883`, a 5-second sample interval, a 60-second discovery interval, all collectors enabled, `/proc`, `/sys`, and `/var/run/docker.sock`. Support username/password, TLS, host/topic overrides, collector toggles, timeouts, and intervals. Never log credentials.
5. Implement standard-library flag CLI modes: normal daemon publishing, `--once` broker-free nested JSON, `--config`, MQTT/config overrides, `--version`, and help. Keep `cmd/pc-state-mqtt/main.go` as a thin signal-aware entrypoint delegating to `internal/app`.
6. Add Makefile targets `build`, `run`, `test`, `test-integration`, `fmt`, `vet`, `check`, `install`, and later systemd installation helpers. Add `config.example.toml`, `README.md`, and `CHANGELOG.md`; expose version `0.1.0` from one constant and `--version`.
7. Add tests for topic escaping, JSON schema, CLI behavior, configuration precedence, TLS and duration validation, secret redaction, and cancellation. Run `make check`, update documentation and changelog, commit the feature branch, and merge it into `main`.

## Phase 2: Core host collectors (`feature/core-telemetry`, version `0.2.0`)

1. Build a concurrent aggregate collector with per-collector timeouts, deterministic output ordering, injected clock/filesystem roots, and isolated errors. Individual collector failures appear in diagnostics but do not suppress successful domains.
2. Implement CPU collection from `/proc/stat` and sysfs: aggregate/per-logical-CPU utilization, identity, online state, model/vendor, and current/min/max clocks. Handle counter resets and missing cpufreq data.
3. Implement memory and swap from `/proc/meminfo`: total, available, used, cached, buffers, swap total/free/used, normalized to bytes.
4. Implement thermal collection from `/sys/class/hwmon` first and `/sys/class/thermal` as a deduplicated fallback. Preserve chip and sensor labels, temperatures, fan RPM, voltage, and power readings where available.
5. Implement storage from `/proc/self/mountinfo`, `statfs`, `/sys/class/block`, and `/proc/diskstats`: real mounts, filesystem usage, block identity/capacity, and cumulative I/O counters. Exclude pseudo-filesystems by default.
6. Implement network collection with Go network APIs and `/sys/class/net`: interface identity, MAC, MTU, addresses, link state/speed, and cumulative byte/packet/error/drop counters.
7. Add fixture-driven parser and sysfs tests for malformed data, hot-remove races, resets, duplicate labels, escaped mount paths, IPv6, and permission failures.
8. Update README and changelog, bump to `0.2.0`, run `make check` and a host `--once` smoke test, then merge into `main`.

## Phase 3: GPU and display state (`feature/gpu-display-telemetry`, version `0.3.0`)

1. Adapt the tested DRM sysfs and EDID patterns from the sibling `gpu-outputs` project without cross-module imports. Identify GPUs by topic-safe PCI BDF when available, falling back to the DRM card name.
2. Collect portable GPU utilization, clocks, VRAM, temperature, fan, and power data from DRM/sysfs/hwmon. Add optional `nvidia-smi` enrichment and driver-specific AMD/Intel parsing where needed.
3. Publish DRM connector state and monitor identity: connector/card relationship, connected/enabled state, EDID manufacturer/model/serial/physical size, advertised modes, and current kernel mode.
4. Add optional Hyprland logical display enrichment: position, dimensions, scale, transform, refresh rate, focus, active workspace, and disabled monitors. DRM remains authoritative for physical identity.
5. Test multi-GPU ordering, stable IDs, disconnected connectors, malformed EDID, missing optional commands, and Hyprland-present/absent behavior.
6. Update README and changelog, bump to `0.3.0`, run focused and one-shot checks, then merge into `main`.

## Phase 4: Docker and MQTT lifecycle (`feature/mqtt-docker-lifecycle`, version `0.4.0`)

1. Implement Docker collection against the Docker Engine HTTP API over a configurable Unix socket using the standard library transport. List all containers and gather bounded-concurrency, non-streaming stats for running containers: immutable ID, names, image, labels, state, health, timestamps, restart count, CPU, memory, block I/O, and network counters.
2. Wrap `github.com/eclipse/paho.mqtt.golang` behind a narrow internal interface. Configure client identity, TLS/auth, keepalive, retained QoS-1 offline last will, reconnect/backoff, context-aware publish waits, and reconnect callbacks.
3. Implement lifecycle orchestration: immediate collection, fast sampling, slower discovery, bounded publishes, signal-aware shutdown, final retained offline state, and stderr logging. Track only retained topics successfully published by this process and tombstone stale ones within its host root.
4. Add the non-executing command skeleton. Subscribe to `<root>/<host>/command/#`, validate bounded requests with command ID/action/parameters/timestamp, reject malformed/duplicate/expired requests, and publish an `unsupported` response for valid requests. Include no mutating executor.
5. Test initial connection, reconnect/full republish, scheduling, partial failures, timeouts, shutdown, LWT, retained-topic cleanup, command rejection, and Docker Unix-socket responses. Add opt-in Mosquitto integration tests via `MQTT_TEST_BROKER`.
6. Update README and changelog, bump to `0.4.0`, run all available checks and broker smoke tests, then merge into `main`.

## Phase 5: Service packaging and release hardening (`feature/service-packaging`, version `0.5.0`)

1. Add hardened systemd system and user units. Neither unit is enabled automatically. Document Docker socket group access, DRM/hwmon `video`/`render` access, and the user-session requirement for Hyprland enrichment.
2. Add Makefile installation and removal targets parameterized by `PREFIX`, `DESTDIR`, and unit directories. Keep normal `install` limited to the binary and expose explicit service-unit targets.
3. Complete README coverage for setup, topic and payload contracts, configuration precedence, collectors, permissions, security, services, troubleshooting, and future command compatibility.
4. Add a release checklist requiring README and changelog updates and one minor version increment for every feature merge.
5. Bump to `0.5.0`; run `make check`, applicable integration tests, repeated one-shot schema validation, systemd verification, permission-degradation checks, MQTT reconnect/LWT tests, and a resource soak before merging.

## Key decisions

- Use top-level `cmd`, `internal`, and `pkg`; there is no `src` directory.
- Linux is the supported platform. Root is not required; inaccessible optional data sources degrade with diagnostics.
- Publish hierarchical leaf topics with a consistent timestamped JSON envelope.
- Use QoS 1, retained identity/meta/availability, unretained high-frequency samples, and an MQTT last will.
- Publish cumulative kernel/device counters; consumers can derive rates. Calculate CPU percentages from bounded paired samples.
- Prefer direct procfs/sysfs/network APIs and a small Docker Engine HTTP client. Paho and a TOML parser are the intended runtime libraries.
- GPU IDs use PCI addresses where possible; Docker uses full immutable IDs; interfaces use kernel names; sensors use resolved hardware identity/channel; displays use DRM connector names.
- DRM is authoritative for physical display identity; Hyprland is optional logical configuration enrichment.
- Include systemd user and system services, but do not enable either automatically.
- Initial command support only validates requests and responds `unsupported`; no command can change system state.
- Every feature branch updates `README.md` and `CHANGELOG.md` as applicable, increments the minor version exactly once, passes `make check`, and is merged into `main` only after validation.
