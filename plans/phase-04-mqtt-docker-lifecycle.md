# Phase 4 Handoff: Docker and MQTT Lifecycle

## Outcome

Complete version `0.4.0` on branch `feature/mqtt-docker-lifecycle`: Docker Engine telemetry, production-complete MQTT scheduling and retained-topic lifecycle, and a non-mutating inbound command skeleton.

## Preconditions and current architectural reality

Do not start this phase until Phase 3 is merged into `main` at version `0.3.0`.

The original master plan assigned the MQTT client to Phase 4, but substantial MQTT work was implemented early on `feature/core-telemetry`. After Phase 3, inspect the actual merged code before writing anything:

```sh
git status --short --branch
git switch main
git --no-pager log --oneline --decorate -15
make check
```

Expected existing packages may include:

- `internal/mqttclient`: Paho connection, TLS/auth, QoS 1 token waits, retained availability, last will, reconnect signals.
- `internal/app/daemon.go`: daemon collection/publish loop.
- `internal/tui`: acknowledged live publishing.
- `internal/watcher`: companion subscriber.
- `compose.test.yaml`: localhost Mosquitto.

Read those files and tests first. Extend them. Do not create a second MQTT abstraction or duplicate TLS logic.

Create `feature/mqtt-docker-lifecycle` only from a clean, up-to-date `main`. If there are uncommitted changes, preserve them and resolve branch ownership before proceeding.

## Non-negotiable safety boundary

This phase must not execute arbitrary commands received from MQTT. It must not start, stop, restart, remove, exec into, or otherwise mutate Docker containers. The command path validates requests and returns `unsupported`; it contains no shell executor and no generic process execution hook.

## Ordered implementation tasks

### 1. Audit existing MQTT lifecycle

Document in code/tests which of these already work:

- TLS/auth configuration.
- Stable client ID.
- QoS 1 publish acknowledgement.
- Metric retain policy.
- Retained online state.
- Retained offline last will.
- Graceful retained offline publish.
- Auto-reconnect.
- Full state republish after reconnect.
- Context cancellation.
- Connection-loss diagnostics.

Add missing tests before modifying behavior. Use narrow interfaces/fakes around Paho tokens and clients.

Known likely gaps to address:

- Separate fast sample and slow discovery schedules.
- Bounded concurrent publishing for large domain objects/topic sets.
- Tracking retained topics successfully published by this process.
- Tombstoning stale retained topics after entities disappear.
- Command subscription and response handling.

### 2. Implement Docker Engine client

Create a narrow standard-library HTTP client, likely under `internal/dockerclient` or `pkg/collector/docker` with an internal transport helper.

Transport requirements:

1. Use `http.Transport.DialContext` with network `unix` and configured socket path.
2. Use a fake URL host such as `http://docker` only for HTTP request construction.
3. Bound every request with the collector context.
4. Close response bodies.
5. Reject non-2xx responses with bounded diagnostic body text.
6. Decode JSON with typed structs. Do not parse Docker JSON with maps unless fields are genuinely dynamic.
7. Make API version handling explicit. Prefer Docker's version negotiation endpoint or a conservative supported endpoint prefix.

Do not add the Docker SDK unless there is a demonstrated requirement that justifies its dependency size. The standard library API client is the intended design.

### 3. Implement Docker collector

Create:

- `pkg/collector/docker/docker.go`.
- `pkg/collector/docker/docker_test.go`.

Collect all containers, including stopped containers:

- Full immutable container ID as stable key.
- Names.
- Image name and image ID.
- Labels.
- Created/started/finished timestamps.
- State and status.
- Health status when present.
- Restart count.
- Running/paused/restarting/dead flags where available.

For running containers, request one non-streaming stats response (`stream=false`, `one-shot=true` where supported):

- CPU counters/derived percentage with documented formula and zero guards.
- Memory usage, limit, cache, and percentage.
- Block I/O cumulative bytes/operations.
- Per-network cumulative RX/TX bytes, packets, errors, and drops.

Use bounded concurrency for stats, controlled by a small constant or configuration. One failed stats call must add a container diagnostic or omit only dynamic stats; it must not remove other containers.

Default output should be one structured `docker` metric keyed by full immutable IDs unless topic-level identity retention requires a documented alternate shape.

### 4. Docker tests with a Unix-socket fake

Tests must not require a real Docker daemon.

- Start an `httptest`-style HTTP server on a temporary Unix socket.
- Verify request paths and query parameters.
- Return fixtures for container list, inspect, and stats.
- Cover running and stopped containers.
- Cover health, restart count, names, labels, and timestamps.
- Cover malformed JSON, HTTP errors, timeout, disappearing container, and one stats failure among successes.
- Verify deterministic ordering and full-ID keys.
- Verify CPU percentage zero/division guards.

An optional real-Docker smoke test may be build-tagged or environment-gated, but it cannot be required by `make check`.

### 5. Implement dual scheduling

Refactor daemon scheduling only as much as necessary. Preserve cancellation and reconnect behavior.

Required behavior:

- Immediate full collection after an acknowledged connection.
- Fast domains at `sample_interval`.
- Discovery/identity domains at `discovery_interval`.
- Event-driven domains publish on events when implemented.
- Reconnect triggers complete republish, including retained identity.
- No overlapping collection of the same domain if one run exceeds its interval.
- Expensive Docker discovery and stats are bounded.
- Diagnostics go to stderr and do not terminate the daemon.

Prefer a scheduler with injected clock/tickers or explicit trigger channels so unit tests do not sleep. Do not build tests around multi-second real timers.

### 6. Retained-topic ownership and cleanup

Implement stale retained-topic tombstoning conservatively.

Rules:

1. Track only retained topics whose QoS 1 publish token succeeded in this process.
2. Keep ownership scoped under the exact configured `<topic_root>/<host_id>/` prefix.
3. After a successful complete discovery pass, compare previously acknowledged retained topics with the new retained set.
4. Publish a retained zero-length payload to stale topics at QoS 1.
5. Remove a topic from the ownership set only after the tombstone acknowledgement succeeds.
6. Never tombstone topics outside the configured host prefix.
7. Never tombstone merely because a collector failed or timed out. Cleanup requires an authoritative successful discovery result.
8. Rebuild or republish ownership safely after reconnect.

Add tests for disappeared Docker containers/displays/GPUs, collector failure, reconnect, changed host/topic root, publish failure, and malicious/out-of-root topic input.

### 7. Non-executing command skeleton

Create `internal/command` with typed request/response structures.

Suggested request fields:

```json
{
  "command_id": "uuid-or-safe-id",
  "action": "future.action",
  "parameters": {},
  "requested_at": "2026-09-03T11:30:00Z"
}
```

Validation requirements:

- Bounded payload size.
- Required command ID and action.
- Safe command ID length/characters for response topic use.
- Valid timestamp within a documented age/skew window.
- Parameters must be a JSON object with bounded depth/size.
- Duplicate command IDs rejected using a bounded TTL cache.
- Malformed/expired/duplicate requests produce a structured rejection when a safe response topic can be formed.
- Valid requests always return status `unsupported` in this release.

MQTT behavior:

- Subscribe to `<root>/<host>/command/#` on every connection.
- Publish responses to `<root>/<host>/command-response/<escaped-command-id>` at QoS 1.
- Responses are not retained unless the documented contract explicitly requires it.
- Message callbacks must hand work to bounded processing; do not block Paho callback internals indefinitely.

Do not include an executor interface that accepts arbitrary strings. Do not invoke `os/exec`.

### 8. Integration tests

Use the existing Compose broker and Makefile targets:

```sh
make mqtt/up
make test-integration
make mqtt/down
```

Integration tests should be opt-in through a build tag and/or `MQTT_TEST_BROKER`. They must use unique client IDs and topic roots per test.

Cover:

- Initial online state.
- Valid JSON telemetry.
- QoS 1 completion.
- Reconnect/full republish after broker restart if practical.
- Graceful offline.
- Last-will offline after forced termination.
- Stale retained-topic tombstone.
- Command malformed/expired/duplicate/unsupported responses.

Tests must clean up retained topics they create.

### 9. TUI and watcher regression

- TUI must continue to record send time only after all publishes for that feature are acknowledged.
- TUI connection errors must remain visible and non-blocking.
- Watcher must still resubscribe after reconnect and pretty-print JSON.
- Shared TLS/token handling must remain in one package.
- Unique watcher/publisher client IDs are required in tests and docs.

### 10. Documentation and release

Update README with:

- Docker socket permissions and read-only behavior.
- Exact Docker JSON schema.
- Sampling vs discovery cadence.
- Retained-topic cleanup semantics.
- Availability lifecycle.
- Command request/response schema and explicit non-mutating guarantee.
- Broker integration-test workflow.

Update changelog and `config.example.toml`. Set version to `0.4.0` exactly once at completion.

## Required validation

```sh
make check
make mqtt/up
make test-integration
make mqtt/down
./pc-state-mqtt --once | jq '.metrics.docker'
```

If a local Docker socket is available, run a read-only smoke test. Do not create, mutate, or remove containers as part of normal validation unless an isolated opt-in test explicitly owns them.

Run `git diff --check`. Confirm no credentials, socket data, generated binaries, or broker persistence files are staged.

## Definition of done

- Docker telemetry covers list/identity/state and bounded non-streaming stats.
- Unit tests use a fake Unix-socket Engine API.
- Fast/discovery scheduling is deterministic and tested.
- MQTT reconnect, full republish, online/offline, and QoS acknowledgement work.
- Stale retained topics are cleaned only with proven ownership and authoritative discovery.
- Commands are validated and answered `unsupported`; no mutation or arbitrary execution path exists.
- TUI and watcher regressions pass.
- README/changelog/config examples are complete.
- Version is `0.4.0`.
- Unit and available integration tests pass.
- Branch is merged into `main` only after completion.
