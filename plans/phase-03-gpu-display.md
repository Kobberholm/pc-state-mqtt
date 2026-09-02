# Phase 3 Handoff: GPU and Display Telemetry

## Outcome

Implement version `0.3.0` on branch `feature/gpu-display-telemetry`: stable GPU identity and metrics, DRM connector/EDID state, and optional Hyprland logical display enrichment.

## Preconditions

Do not begin implementation until Phase 2 is merged into `main` at version `0.2.0`.

Before editing:

```sh
git status --short --branch
git switch main
git pull --ff-only  # only if a remote is configured and this is safe
git switch -c feature/gpu-display-telemetry
make check
```

If the worktree is dirty, do not switch branches or create a branch. Inspect and preserve existing changes first. Never reset or checkout user changes.

## Repository conventions to preserve

- Collector implementations live under `pkg/collector/<domain>` and implement `collector.Collector`.
- Filesystem roots come from `config.Config`; tests use temporary fixture roots.
- Collector failures become diagnostics and do not suppress other domains.
- Default domain output should be a small number of structured objects, consistent with combined CPU and memory output. Add leaf-topic compatibility modes only when there is a concrete consumer need.
- Metric paths use raw stable identifiers as segments; `pkg/telemetry` performs escaping.
- Fast counters are cumulative where the kernel exposes cumulative values.
- TUI `Sent` timestamps mean QoS 1 acknowledgement, not merely queued output.

## Source material

A sibling repository exists at `../gpu-outputs`. It contains tested DRM discovery and EDID parsing patterns:

- `../gpu-outputs/pkg/drm/discovery.go` and tests.
- `../gpu-outputs/pkg/drm/edid.go` and tests.
- `../gpu-outputs/pkg/hyprland/monitors.go` and tests.
- `../gpu-outputs/pkg/pciids/database.go` and tests.

Read and adapt algorithms, fixtures, and edge-case handling. Do not import `gpu-outputs` as a module and do not copy unrelated CLI behavior. Keep this repository independently buildable.

Captured host command output is available under `../command-output`, including Hyprland and PCI examples. Treat captures as fixtures/reference, not as runtime dependencies.

## Ordered implementation tasks

### 1. Configuration and contracts

Add only options that are needed by the implementation. Likely options include:

- Enable/disable GPU, Display, and Hyprland collectors using existing collector toggles.
- Optional `nvidia-smi` executable path or enrichment toggle if command discovery needs configuration.
- Hyprland socket/signature override only if environment discovery is insufficient.

For every option:

1. Add a TOML field.
2. Add `PC_STATE_MQTT_*` environment handling.
3. Add a CLI flag only when useful for diagnostics/one-shot execution.
4. Add precedence/default tests.
5. Add it to `config.example.toml` and README.

### 2. DRM GPU discovery

Create `pkg/collector/gpu` and focused internal parsing helpers as needed.

Requirements:

1. Scan `<sys_root>/class/drm/card*` but exclude connector entries when discovering cards.
2. Resolve each card's backing device symlink.
3. Use lowercase PCI BDF such as `0000:03:00.0` as the stable GPU ID when available.
4. Fall back to DRM card name only when no PCI identity exists.
5. Collect driver, vendor ID, device ID, subsystem IDs, and human-readable names when locally available.
6. Do not make a network request for PCI names. A packaged/local database is optional; numeric IDs are required.
7. Sort GPUs by stable ID.
8. Test multiple cards, non-PCI devices, symlink failures, duplicate paths, and hot removal.

Suggested default metric shape:

```json
{
  "value": {
    "0000:03:00.0": {
      "drm_card": "card1",
      "driver": "amdgpu",
      "vendor_id": "1002",
      "device_id": "..."
    }
  }
}
```

Use explicit unit-bearing field names for nested values.

### 3. Portable GPU telemetry

Read DRM/sysfs/hwmon first:

- Utilization when exposed by the driver.
- Current/min/max clocks when exposed.
- VRAM total/used when exposed.
- Temperature, fan RPM, and power from the hwmon device associated with the GPU.

Rules:

- Associate hwmon entries by resolved device path, not enumeration order or label alone.
- Preserve source-specific missing fields by omission.
- Normalize units consistently.
- Do not fail the whole GPU because one sensor is absent or malformed.
- Keep expensive command execution out of fast paths unless bounded by context and timeout.

### 4. Optional vendor enrichment

Implement vendor enrichment behind narrow interfaces so unit tests do not invoke host commands.

For NVIDIA:

- Invoke `nvidia-smi` only if available.
- Request explicit CSV fields with `--format=csv,noheader,nounits` or XML with a real XML parser.
- Parse UUID, PCI BDF, utilization, clocks, VRAM, temperature, fan, and power where available.
- Bound execution with collector context.
- Missing executable is not a fatal collector error if sysfs data exists.

For AMD and Intel:

- Prefer documented sysfs files.
- Add driver-specific parsing only for fields unavailable through portable DRM/sysfs.
- Isolate each parser and add fixture tests.

Merge enrichment into the GPU identified by normalized PCI BDF. Never join by list index.

### 5. EDID parser and DRM connectors

Create `pkg/collector/display` or a reusable `pkg/edid` parser if ownership is clearer.

Collect:

- DRM connector name and owning GPU/card.
- Connector type.
- `status` (connected/disconnected/unknown).
- Enabled/disabled state.
- EDID manufacturer, product/model, serial, physical size, display name, and checksum validity.
- Advertised modes from `modes`.
- Current kernel mode when exposed.

Requirements:

1. Parse binary EDID structurally; do not search raw bytes with ad hoc strings.
2. Handle base block plus useful extension metadata without requiring every extension type.
3. A malformed EDID must produce a connector with an EDID diagnostic, not remove the connector.
4. Disconnected connectors remain represented with identity fields that are available.
5. Use connector names such as `DP-1` as stable IDs within the owning GPU.
6. Test malformed length/checksum, missing EDID, disconnected connector, duplicate monitor models, and multi-GPU connector ownership.

### 6. Hyprland enrichment

Create a narrow interface around Hyprland queries. The sibling `gpu-outputs` repository and local `external-projects/hyprland-go` are references, but avoid adding a large dependency unless it clearly reduces complexity.

Collect logical state:

- Monitor name/description.
- Position.
- Pixel dimensions.
- Scale.
- Transform.
- Refresh rate.
- Focus state.
- Active workspace.
- Disabled monitor definitions when available.

Rules:

- DRM remains authoritative for physical identity and EDID.
- Join Hyprland state to DRM connectors by connector/monitor name where possible.
- Hyprland absence, missing environment variables, or unavailable socket must degrade cleanly.
- Parse JSON with `encoding/json`, not text slicing.
- Bound all IPC with context.
- Test present, absent, malformed JSON, disabled monitor, transformed monitor, and unmatched monitor cases.

### 7. Integration and TUI

- Register GPU, Display, and Hyprland in `internal/systemstate`.
- Mark them available in `internal/tui/tui.go` only after implementation.
- Use sample cadence for dynamic GPU metrics.
- Use discovery cadence for connector/EDID identity.
- Hyprland should become event-driven if a reliable event socket is implemented. Otherwise use documented polling and leave event support for a later change; do not label polling as event-driven.
- Ensure reconnect/full republish includes these domains.

### 8. Documentation and release

Update README with:

- Exact default JSON structures.
- Stable ID rules and fallback behavior.
- Optional vendor tools.
- Permissions (`video`/`render`) and Hyprland session requirements.
- Missing-data and diagnostics behavior.

Update changelog and `config.example.toml`. Set `internal/version/version.go` to `0.3.0` exactly once at completion.

## Required tests

At minimum:

- Stable multi-GPU ordering.
- PCI and non-PCI ID fallback.
- Driver symlink and hot-remove races.
- GPU sensor unit conversion.
- Missing/malformed optional vendor command output.
- EDID valid and malformed fixtures.
- Connected and disconnected connectors.
- Multi-GPU connector ownership.
- Hyprland present/absent/malformed state.
- System-state collector registration.
- TUI availability and cadence.
- JSON serialization of combined domain objects.

## Validation

```sh
make check
./pc-state-mqtt --once | jq '.metrics | {gpu, display, hyprland}'
```

Run on at least one real host with DRM devices. If NVIDIA hardware is unavailable, validate its parser with captured fixtures and clearly state that live NVIDIA validation was not possible.

If MQTT is available:

```sh
make mqtt/up
./pc-state-mqtt-watch --client-id pc-state-mqtt-phase3-watch --topic 'pc-state/phase3-test/#' --color=false
PC_STATE_MQTT_CLIENT_ID=pc-state-mqtt-phase3-publisher ./pc-state-mqtt --host-id phase3-test
make mqtt/down
```

Use SIGINT and verify retained offline availability.

## Definition of done

- GPU IDs are stable and tested across multiple cards.
- Portable GPU metrics work; optional vendor enrichment degrades cleanly.
- DRM connector and EDID state is complete enough to identify real monitors.
- Hyprland enrichment is optional and cannot break DRM telemetry.
- All new collectors are registered and accurately represented in the TUI.
- README and changelog match actual behavior.
- Version is `0.3.0`.
- `make check` and available host/MQTT smoke tests pass.
- Branch `feature/gpu-display-telemetry` is merged into `main` only after completion.
