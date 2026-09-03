# Phase 2: Core Host Telemetry (Completed)

## Outcome

Version `0.2.0` is complete on branch `copilot/verify-phase-1-baseline`. CPU, memory, thermal, storage, and network state are collected with deterministic output and isolated failures. The MQTT/TUI work present on the branch is validated, documented, and ready to merge into `main`.

## Start here

Run:

```sh
git status --short --branch
git --no-pager log --oneline --decorate -12
make check
```

Completion evidence as of 2026-09-03:

- `make check` passes formatting, vet, all unit tests, and both builds.
- Default and compatibility `--once` snapshots succeed without diagnostics.
- The local Mosquitto broker retains `online` after connection and `offline` after SIGINT shutdown.
- CPU, memory, thermal, storage, and network collectors are implemented, registered, fixture-tested, documented, and exposed in the TUI.
- Version `0.2.0` is set in `internal/version/version.go`; only the merge into `main` remains.

## Existing implementation to preserve

### Aggregation

`pkg/collector/collector.go` runs collectors concurrently with a timeout per collector, deterministic metric ordering, and independent diagnostics. New collectors must implement the existing `collector.Collector` interface and must work through this aggregator.

### CPU

Files:

- `pkg/collector/cpu/cpu.go`.
- `pkg/collector/cpu/cpu_test.go`.

Current behavior:

- Reads paired `/proc/stat` samples and calculates utilization.
- Reads available cpufreq current/min/max values from sysfs.
- Defaults to one metric at path `cpu`; its value contains `total` and `cores`.
- `cpu_per_core = true`, `PC_STATE_MQTT_CPU_PER_CORE=true`, or `--cpu-per-core` restores separate property topics.

Remaining CPU requirements:

1. Add stable CPU identity: model name, vendor ID, architecture where available, and logical CPU count.
2. Add online state from `/sys/devices/system/cpu/online` or per-CPU `online` files. CPU 0 may lack an `online` file and should be treated as online when present in procfs.
3. Keep the combined object as the default output. Extend that object rather than creating default leaf topics.
4. Keep per-core compatibility output working.
5. Handle malformed procfs rows, counter resets, CPU hot-remove races, missing cpufreq, and missing optional identity fields without panics.

### Memory

Files:

- `pkg/collector/memory/memory.go`.
- `pkg/collector/memory/memory_test.go`.

Current behavior:

- Reads `/proc/meminfo`.
- Normalizes memory, cache, buffers, and swap values to bytes.
- Defaults to one metric at path `memory`.
- `memory_per_field = true`, `PC_STATE_MQTT_MEMORY_PER_FIELD=true`, or `--memory-per-field` restores separate property topics.

Preserve this behavior. Missing optional swap/cache/buffer values should be omitted from the combined object, not fabricated. `MemTotal` and `MemAvailable` remain required.

### MQTT and TUI already on this branch

Files include:

- `internal/mqttclient/client.go` and tests.
- `internal/app/daemon.go` and tests.
- `internal/tui/tui.go` and tests.
- `internal/watcher`.
- `compose.test.yaml` and `test/mosquitto/mosquitto.conf`.

Preserve:

- QoS 1 publishing.
- Retain policy from each `telemetry.Metric`.
- Retained online/offline availability and last will.
- Reconnect and complete republish.
- TUI `Sent` timestamps only after publish acknowledgement.
- Slash-style Makefile targets `mqtt/up`, `mqtt/down`, and `mqtt/logs`.

Before completing Phase 2, decide whether discovery scheduling and retained-topic cleanup can be completed here without delaying collectors. If deferred, document the exact remaining lifecycle work in Phase 4 and do not claim it is implemented.

## Ordered implementation tasks

### 1. Revalidate current uncommitted work

Run:

```sh
make check
./pc-state-mqtt --once | jq .
./pc-state-mqtt --once --cpu-per-core | jq .
./pc-state-mqtt --once --memory-per-field | jq .
```

Fix only regressions attributable to current Phase 2 work. Do not refactor unrelated code.

### 2. Finish CPU identity and online state

- Extend fixture data under temporary proc/sys roots in tests.
- Prefer `/proc/cpuinfo` for model/vendor and `runtime.GOARCH` for architecture unless a more direct existing abstraction is present.
- Parse Linux CPU range syntax such as `0-3,8,10-11` in a focused helper with table-driven tests.
- Keep field names explicit and unit-bearing where needed.
- Ensure output ordering does not depend on map iteration.

### 3. Implement thermal collector

Create:

- `pkg/collector/thermal/thermal.go`.
- `pkg/collector/thermal/thermal_test.go`.

Requirements:

1. Scan configurable `<sys_root>/class/hwmon/hwmon*` first.
2. Resolve chip identity from `name`, device symlink/uevent information, and stable hardware path where possible. Do not use enumeration order alone as identity.
3. Read channels for temperature (`temp*_input`), fan (`fan*_input`), voltage (`in*_input`), and power (`power*_input`).
4. Preserve `*_label` when present; otherwise use a deterministic channel name.
5. Apply Linux sysfs units correctly: millidegrees Celsius, RPM, millivolts/microvolts as documented by the source, and microwatts. Normalize to explicit output units.
6. Use `/sys/class/thermal/thermal_zone*` only as fallback. Deduplicate zones already represented by hwmon.
7. Ignore files that disappear during collection as hot-remove races, but report meaningful permission/parser failures as diagnostics through the collector error boundary.
8. Add malformed, missing, duplicate-label, fallback, and hot-remove fixture tests.

### 4. Implement storage collector

Create:

- `pkg/collector/storage/storage.go`.
- `pkg/collector/storage/storage_test.go`.

Requirements:

1. Parse configurable `<proc_root>/self/mountinfo`; correctly unescape octal mount escapes such as `\040`.
2. Exclude pseudo-filesystems by default. Keep the exclusion list explicit and tested.
3. Use `unix.Statfs` or an established Go syscall API to report total, available, and used bytes for real mounts. If adding `golang.org/x/sys/unix`, add it through the module toolchain and keep usage narrow.
4. Scan `<sys_root>/class/block` for block identity, model/vendor, rotational state, logical sector size, and capacity.
5. Parse `<proc_root>/diskstats` for cumulative read/write operations, sectors, bytes, and I/O time. Never invent rates.
6. Use stable block names and mount paths in combined structured output. Topic-segment escaping remains the telemetry package's responsibility.
7. Test escaped paths, bind mounts, pseudo-filesystem filtering, malformed rows, missing sysfs files, and device removal races.

### 5. Implement network collector

Create:

- `pkg/collector/network/network.go`.
- `pkg/collector/network/network_test.go`.

Requirements:

1. Use `net.Interfaces` for names, indexes, MAC addresses, MTU, flags, and IP addresses.
2. Make interface discovery injectable for tests; do not require the host's live interfaces in unit tests.
3. Read `<sys_root>/class/net/<name>` for operational state, carrier, speed, and cumulative statistics.
4. Include IPv4 and IPv6 addresses with prefix length. Preserve link-local addresses.
5. Publish cumulative RX/TX bytes, packets, errors, and drops. Do not calculate rates.
6. Use kernel interface names as stable IDs and deterministic sorting.
7. Handle missing speed (common for virtual interfaces), permission failures, and interface removal without failing unrelated interfaces.
8. Test physical, loopback, virtual, IPv6, malformed counter, and hot-remove cases.

### 6. Register collectors and TUI availability

- Add new constructors to `internal/systemstate/systemstate.go` using configured proc/sys roots.
- Add each collector to full collection and `GatherFeature`.
- Mark Thermal, Storage, and Network as implemented in `internal/tui/tui.go` only after their collectors work.
- Use sample cadence for thermal/network and discovery cadence where appropriate for storage identity/mount discovery. If one collector mixes fast counters and slow identity, document and implement a clear scheduling compromise rather than silently collecting expensive discovery every fast tick.
- Add system-state and TUI tests for registration and enabled/disabled behavior.

### 7. Documentation and release

Update:

- `README.md`: exact collected fields, permissions, combined object schemas, compatibility options, diagnostics, and examples.
- `CHANGELOG.md`: concise Phase 2 additions.
- `config.example.toml`: every new option.
- `internal/version/version.go`: change to `0.2.0` exactly once, only at completion.

## Required validation

Run all of the following:

```sh
make check
./pc-state-mqtt --once | jq .
./pc-state-mqtt --once --cpu-per-core --memory-per-field | jq .
make mqtt/up
PC_STATE_MQTT_CLIENT_ID=pc-state-mqtt-phase2-test ./pc-state-mqtt --host-id phase2-test
# In another terminal:
./pc-state-mqtt-watch --client-id pc-state-mqtt-phase2-watch --topic 'pc-state/phase2-test/#' --color=false
make mqtt/down
```

For the daemon smoke test, stop the publisher with SIGINT and verify retained `availability` becomes `offline`. Use unique client IDs to avoid Mosquitto disconnect loops caused by duplicate IDs.

Also test permission degradation with fixture roots or unreadable fixture files. Optional missing data must not crash `--once`.

## Git and merge procedure

1. Stay on `feature/core-telemetry`.
2. Preserve all user and prior-agent changes in the dirty worktree.
3. Make focused commits when practical; do not commit generated binaries.
4. Verify `git diff --check` and `make check`.
5. Bump to `0.2.0` only after all acceptance criteria pass.
6. Commit README and changelog updates with the phase.
7. Merge into `main` only when the phase is complete and tests pass. Do not merge a partial collector set.

## Definition of done

- CPU identity, online state, utilization, and clocks work in combined and compatibility modes.
- Memory combined and compatibility modes work.
- Thermal, storage, and network collectors are implemented, registered, fixture-tested, and visible in the TUI.
- Collector failures remain isolated.
- MQTT/TUI changes currently in the worktree remain functional and documented accurately.
- `make check` and host smoke tests pass.
- Version is `0.2.0`.
- README and changelog describe actual behavior, not planned behavior.
- The completed feature branch is merged into `main`.
