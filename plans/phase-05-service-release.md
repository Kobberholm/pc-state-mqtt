# Phase 5 Handoff: Service Packaging and Release Hardening

## Outcome

Ship version `0.5.0` on branch `feature/service-packaging` with hardened system and user systemd units, parameterized installation/removal, complete operator documentation, and release-grade validation.

## Preconditions

Do not begin until Phase 4 is merged into `main` at version `0.4.0`.

Start only from a clean worktree:

```sh
git status --short --branch
git switch main
git switch -c feature/service-packaging
make check
```

If changes already exist, preserve them and determine ownership before switching branches. Never reset or discard user work.

## Packaging principles

- Install but never enable or start services automatically.
- Support both a system service and a per-user service.
- Keep service units static and auditable; do not generate them at runtime.
- Do not embed credentials in unit files.
- Use configuration/environment files documented with secure permissions.
- The normal `install` target installs binaries only. Unit installation uses explicit targets.
- Honor `PREFIX`, `DESTDIR`, and configurable unit directories so distro packaging works without writing to the live root.

## Ordered implementation tasks

### 1. Audit runtime requirements

Before writing units, inspect:

- CLI flags and default config path.
- Whether daemon mode logs only to stdout/stderr.
- Shutdown timing and SIGTERM handling.
- Docker socket access.
- DRM/hwmon permissions.
- Hyprland environment/socket discovery.
- Writable paths, if any.
- Network/TLS certificate access.

Document every required filesystem or socket access. Hardening directives must not block intended collectors silently.

### 2. Add system service unit

Create a unit under a repository packaging directory such as `packaging/systemd/pc-state-mqtt.service`.

Recommended baseline behavior:

- `Type=simple` or `Type=exec`.
- `ExecStart` points to the installed binary and an explicit system config path, typically `/etc/pc-state-mqtt/config.toml`.
- `Restart=on-failure` with a modest delay.
- `TimeoutStopSec` long enough for the final QoS 1 offline publish.
- `After=network-online.target` and `Wants=network-online.target` when remote brokers are expected.
- No automatic `User=` assumption unless the packaging contract creates one. Prefer documenting a dedicated account or parameterization.

Evaluate and test hardening directives individually:

- `NoNewPrivileges=true`.
- `PrivateTmp=true`.
- `ProtectSystem=strict` or `full`.
- `ProtectHome=true` or `read-only`, considering TLS/config paths.
- `ProtectKernelTunables=true`.
- `ProtectKernelModules=true`.
- `ProtectControlGroups=true`.
- `RestrictSUIDSGID=true`.
- `LockPersonality=true`.
- `MemoryDenyWriteExecute=true` if Go runtime compatibility is verified.
- Restrict address families to those actually needed, including `AF_UNIX`, `AF_INET`, and `AF_INET6`.

Do not blindly paste a hardening template. For each omitted protection, document the collector requirement that prevents it.

The unit must not include `WantedBy` behavior that causes installation to enable it. An `[Install]` section is acceptable; installation targets must not call `systemctl enable`.

### 3. Add user service unit

Create `packaging/systemd/user/pc-state-mqtt.service` or another clearly separated user-unit path.

Requirements:

- Use `%h/.config/pc-state-mqtt/config.toml` or rely on the application's XDG default.
- Preserve access to the user session and Hyprland environment.
- Do not assume Docker socket access; document group/rootless Docker requirements.
- Use appropriate restart and shutdown settings.
- Apply hardening that is valid for user managers.
- Do not enable or start automatically.

Explain when to choose system vs user service:

- System service for host telemetry independent of login.
- User service for Hyprland/session enrichment and user-scoped credentials.

### 4. Expand Makefile installation

Preserve existing targets, especially `mqtt/up`, `mqtt/down`, and `mqtt/logs`.

Add parameterized variables, for example:

```make
PREFIX ?= /usr/local
DESTDIR ?=
BINDIR ?= $(PREFIX)/bin
SYSTEMD_SYSTEM_UNIT_DIR ?= $(PREFIX)/lib/systemd/system
SYSTEMD_USER_UNIT_DIR ?= $(PREFIX)/lib/systemd/user
```

Required targets:

- `install`: install both binaries only.
- `uninstall`: remove installed binaries only.
- `install-systemd-system`: install system unit explicitly.
- `uninstall-systemd-system`.
- `install-systemd-user`: install user unit explicitly.
- `uninstall-systemd-user`.
- Optional aggregate unit install/remove targets if names are unambiguous.

Rules:

- Use the `install` command with explicit modes (`0755` binaries, `0644` units).
- Create destination directories.
- Never call `sudo` inside the Makefile.
- Never call `systemctl enable`, `start`, `restart`, or `daemon-reload` automatically.
- Work correctly with `DESTDIR` staging.
- Add a dry staging test using `DESTDIR=$(mktemp -d)`.

### 5. Configuration packaging

Decide whether to install `config.example.toml` as documentation or provide an explicit `install-config-example` target. Do not overwrite an existing live config.

Document:

- System config location.
- User config location.
- Mode `0600` when credentials are present.
- Environment variable alternatives for secrets.
- TLS CA/client certificate permissions.
- Stable client ID requirements.

Do not create a default password or anonymous-broker assumption in production service docs.

### 6. Complete README

README must cover, with commands that match the repository:

- Build and test.
- One-shot diagnostics.
- Daemon usage.
- TUI and watcher usage.
- Topic hierarchy and JSON envelopes.
- Combined CPU/memory and any later combined-domain schemas.
- Configuration precedence and every environment variable/CLI compatibility option.
- Collector sources and units.
- Permissions and expected degradation.
- Docker socket risks and read-only application behavior.
- DRM/hwmon group access.
- Hyprland user-session requirement.
- TLS/auth setup.
- Availability/reconnect/retained cleanup.
- Non-mutating command skeleton.
- System and user service installation, enable/start steps performed manually by the operator.
- Troubleshooting duplicate MQTT client IDs, unavailable files, permission errors, and malformed config.

Do not describe planned behavior as implemented.

### 7. Release checklist

Create `RELEASING.md` with an explicit checklist:

1. Clean worktree and expected branch.
2. Version changed exactly once for the phase.
3. README and changelog updated.
4. `go mod tidy` causes no unexplained dependency churn.
5. `make check` passes.
6. Integration tests pass when dependencies are available.
7. One-shot JSON validates repeatedly.
8. MQTT online/offline and reconnect verified.
9. Service units pass verification.
10. Staged install/uninstall tested.
11. No credentials or host-specific paths in staged changes.
12. Git diff reviewed before merge.

Also add the invariant to contributor/development documentation: each completed feature merge increments the minor version exactly once.

### 8. Systemd verification

Run, when available:

```sh
systemd-analyze verify packaging/systemd/pc-state-mqtt.service
systemd-analyze --user verify packaging/systemd/user/pc-state-mqtt.service
```

If user verification cannot run due to the environment, verify syntax with the system command where possible and report the limitation.

Test staged installation without root:

```sh
stage=$(mktemp -d)
make DESTDIR="$stage" install install-systemd-system install-systemd-user
find "$stage" -type f -printf '%m %p\n' | sort
make DESTDIR="$stage" uninstall uninstall-systemd-system uninstall-systemd-user
find "$stage" -type f -print
rm -rf "$stage"
```

The final `find` should show no installed files owned by these targets. Empty directories may remain.

### 9. Runtime hardening validation

On a system with systemd and the required resources:

- Run the system unit manually after placing a test config.
- Confirm SIGTERM produces retained offline availability.
- Confirm restart reconnects and republishes.
- Confirm inaccessible optional proc/sys/hwmon/DRM sources create diagnostics rather than crashes.
- Confirm user service can access Hyprland when launched from the user manager.
- Confirm system service documentation does not promise Hyprland access.
- Check `systemd-analyze security` and record intentional exposure.

Do not weaken protections merely to eliminate every score warning; functionality and documented least privilege matter more than a perfect score.

### 10. Soak and resource checks

Run a representative daemon soak long enough to observe repeated sample/discovery intervals and at least one broker interruption. Monitor:

- Stable goroutine count.
- Stable memory use.
- No unbounded retained-topic ownership growth.
- No duplicate client reconnect loop.
- No overlapping expensive collectors.
- No repeated noisy diagnostics for expected missing optional files.

A practical local soak is 30-60 minutes. Record what was actually tested in the changelog or release notes, not necessarily in source code.

### 11. Final release work

- Set `internal/version/version.go` to `0.5.0` exactly once.
- Add a dated `0.5.0` changelog section only when release/merge conventions call for it; otherwise keep accurate Unreleased entries until merge.
- Run `go mod tidy` and inspect changes.
- Run all validation below.
- Commit focused changes.
- Merge `feature/service-packaging` into `main` only after every required check passes.
- Do not create a Git tag unless explicitly requested.

## Required validation

```sh
make check
make test-integration  # when broker/Docker prerequisites are available
git diff --check
./pc-state-mqtt --version
for i in 1 2 3; do ./pc-state-mqtt --once | jq -e . >/dev/null; done
systemd-analyze verify packaging/systemd/pc-state-mqtt.service
```

Also run staged install/uninstall and a broker online/offline test using existing `make mqtt/up` and `make mqtt/down` targets.

## Definition of done

- System and user service units exist, are hardened deliberately, and verify successfully where tooling permits.
- Neither installation nor units enable/start services automatically.
- Makefile install/remove targets honor `PREFIX`, `DESTDIR`, and unit-directory overrides.
- Staged install/uninstall works without root.
- README is complete and accurate for operators.
- `RELEASING.md` contains a reproducible checklist.
- Permission degradation, MQTT lifecycle, repeated JSON output, and resource behavior are validated.
- Version is `0.5.0`.
- `make check` and applicable integration checks pass.
- Branch is merged into `main`; no tag is created unless requested.
