# kafka-ui specification

This document describes what kafka-ui does, how it is built, and the rules contributors follow. It describes the current behavior. When a change alters behavior described here, update this file in the same pull request.

## 1. Goals

kafka-ui is a single-binary web UI for operating and observing Apache Kafka clusters. A Go server exposes a REST + Server-Sent Events API and serves an embedded React app.

It is for developers and platform engineers who want to see what is happening in their clusters and make day-to-day changes without the CLI tools. Typical questions it answers:

- Which topics exist, how are they replicated, and what is in them?
- Why is this consumer group lagging, and will it catch up?
- Who is consuming what, right now?

**Principles**

- **Kafka protocol only.** Admin, consumer and producer APIs. No dependency on a distribution, an operator (Strimzi, Confluent for Kubernetes) or the Kubernetes API.
- **One binary, no external services.** State (sessions, analytics) lives in an embedded SQLite file.
- **Safe by default.** All checks are enforced on the server. Destructive actions need explicit confirmation. Internal topics are never mutated.

**Non-goals:** Kafka Connect / MirrorMaker management, ksqlDB, broker host metrics (CPU, heap, JMX), alerting, editing broker configs, OIDC/LDAP login, fine-grained RBAC. These are not ruled out forever, but they need a design discussion first (see section 9).

Requires Kafka 3.x or newer. KRaft and ZooKeeper-mode clusters both work.

## 2. Features

### 2.1 Clusters

- Clusters are defined in YAML. Each cluster has:
  - bootstrap servers
  - optional SASL (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
  - optional TLS/mTLS
  - an optional Schema Registry
  - a `readOnly` flag
- Every URL is scoped to one cluster: `/clusters/{id}/...`.
- A background check probes each cluster every 15s.
  - An unreachable cluster shows as `offline` with the last error.
  - Requests to an offline cluster fail fast with `503 CLUSTER_OFFLINE` instead of waiting for Kafka timeouts.
- A cluster whose client cannot be created (for example, a missing credential) is listed as offline with the reason. The rest of the app keeps working.
- `readOnly` clusters reject every mutation with `403 CLUSTER_READ_ONLY`, whatever the user's role.
- The active controller is the KRaft quorum leader (`DescribeQuorum`). In KRaft, the controller field in Metadata points at a random broker. ZooKeeper clusters fall back to Metadata.

### 2.2 Authentication and roles

- Users are defined in config with bcrypt hashes. `kafka-ui hash-password` creates a hash from stdin.
- There are two roles:
  - `admin`: can do everything.
  - `viewer`: read-only. No create, delete, produce, reset, config or ACL changes.
- Sessions are server-side.
  - The cookie is random, `HttpOnly`, `SameSite=Strict`, and `Secure` unless disabled.
  - Only a hash of the token is stored.
  - Sessions have a sliding idle timeout (default 12h).
- Every `/api/*` route except login requires a session.
- Mutating requests must carry `X-Requested-With: kafka-ui` (CSRF defense).
- Login attempts are limited to 5 per minute per client IP.
- Every successful mutation is logged as `msg=audit` with the action, user, cluster and target.

### 2.3 Brokers and topics

- **Overview:** cluster ID, version, controller, topic and partition counts, under-replicated and offline partitions, total size.
- **Brokers:** leaders, replicas, size, disk usage, configs (read-only).
- **Topic list:** partitions, replication factor, under-replicated partitions, message count (end − start offsets), size. Internal topics are hidden unless toggled on.
- **Topic detail:** partitions (leader, replicas, ISR, offsets, size), consuming groups with lag, configs (overrides highlighted), throughput charts, messages, and Schema Registry subjects.
- **Topic changes:**
  - **Create:** name, partitions, replication factor, configs. A validate-only mode is available.
  - **Edit configs:** set a value, or reset it to the default.
  - **Add partitions:** partitions cannot be removed.
  - **Purge:** deletes records up to the current end offset.
  - **Delete topic.**
- Delete and purge require typing the topic name. Topics starting with `__` cannot be changed (`403 TOPIC_PROTECTED`).
- The broker's own error message, such as an invalid config value, is passed to the user.

### 2.4 Message browser and producer

- **Seek modes:**
  - newest
  - oldest
  - from offset
  - from timestamp
  - live tail
- **Scope:** all partitions or a subset.
- **Streaming:** results stream over SSE as they are found, with progress events. Stopping the request cancels the scan.
- **Limits:**
  - up to 500 results per scan (default 100)
  - up to 100,000 scanned records
  - 60 seconds per bounded scan, 15 minutes for live tail
  - 64 KB of text per streamed payload; the single-message endpoint returns the full content
- **Newest** reads backwards in growing windows per partition and returns the results newest first.
- **Filters** run on the server over key, value or headers:
  - **Substring:** case-insensitive.
  - **Regex:** Go RE2, linear time, so a user-supplied pattern cannot cause catastrophic backtracking.
- **Decoding:** each payload is decoded as String, JSON, Base64 or Hex. Auto-detection is the default and can be overridden.
  - **Schema Registry formats:** with a registry configured, payloads in Confluent wire format (magic byte 0 + schema ID) decode as Avro, Protobuf (including nested messages and references) or JSON Schema.
  - **Failures:** if decoding fails, the raw bytes are shown with the reason.
  - **Large integers:** JSON is pretty-printed without parsing numbers, so 64-bit integers are shown exactly.
- **Producer** (admin only):
  - key, value, headers, partition (or chosen from the key like the Java client)
  - formats: String, JSON, Base64, Hex, or a Schema Registry subject
  - for Avro, union values may be bare or wrapped as `{"type": value}`
  - the topic must exist; producing never auto-creates a topic

### 2.5 Consumer groups

- **List:** state, members, protocol, topics, total lag.
- **Lag rule:** lag counts only partitions with a committed offset, which matches `kafka-consumer-groups.sh`.
- **Detail:** members with client ID, host and assignments; per-partition committed offset, end offset and lag; lag trend.
- **Reset offsets:** per topic, optionally per partition. Strategies:
  - earliest
  - latest
  - offset
  - timestamp
  - shift by ±N (partitions without a commit are skipped)
- **Reset rules:**
  - Only allowed when the group has no active members (state `Empty`, or `Dead` for a group that does not exist yet).
  - The state is checked again right before the commit.
  - The UI requires a preview (dry run) before Apply.
- **Delete:** only for `Empty` groups.

### 2.6 ACLs

- List, create and delete ACL bindings. Delete matches one exact binding; `LITERAL` and `PREFIXED` patterns are supported.
- If the cluster has no authorizer (`SECURITY_DISABLED`), the page shows a notice instead of an error.

### 2.7 Schema Registry

- Uses the Confluent API. Works with Confluent Schema Registry and other compatible registries.
- **Subjects:** type, latest version, version count.
- **Subject detail:** versions, line diff between two versions, references.
- **Changes:**
  - Register a new version, with a compatibility check that shows the registry's reasons.
  - Set or reset the per-subject compatibility level.
  - Delete a subject: soft by default, permanent on request.
- A topic's **Schema** tab shows its `<topic>-key` / `<topic>-value` subjects (TopicNameStrategy).
- Registry endpoints do not depend on Kafka health.
- Failed schema lookups are cached for 30s, so large scans do not hammer the registry.

### 2.8 Analytics

- **Sampling:** a collector samples every cluster every `collector.interval` (default 5s):
  - per-topic end and start offset sums
  - per group and topic: committed offset sum and lag
  - URP and offline partition counts
  - log sizes (every 30s)
- **Storage:** samples are stored in SQLite.
  - Raw samples are kept for 6 hours, then rolled into 1-minute buckets.
  - Rollups are kept for `storage.retention` (default 7 days).
  - Rollups keep the maximum per minute: counters keep their last value, and lag/URP keep their spikes.
- **Rates** are derived at query time from counter deltas, timed by the actual sample times.
  - Charts get about 240 points per range (15m, 1h, 6h, 24h, 7d).
  - Bytes/s is estimated from log size growth and charted in buckets of at least 1 minute.
- **Dashboards:**
  - **Throughput:** messages in/s and bytes in/s, excluding internal topics.
  - **Lag:** total consumer lag over time.
  - **Groups:** consume rate, ingress rate and catch-up estimate. Catch-up is lag ÷ (consume − ingress) over the last 5 minutes. A group is labeled "not consuming" when its consume rate is 0, and "falling behind" when it consumes no faster than messages arrive.
  - **Topics:** ingress, produced count, size and growth.
  - **Partition skew:** partitions above 1.5× their fair share of ingress, and leaders/replicas/ingress per broker, over the last minute.
  - **Hygiene:**
    - replication factor below 2
    - `min.insync.replicas` ≥ RF, or 1 with RF ≥ 3
    - under-replicated topics
    - idle topics (no new messages in the observed window)
    - empty groups

### 2.9 Live topology

- **Layout:** a 2D flow view, updated every `collector.liveInterval` (default 2s) over SSE:
  - **Brokers** are cards along the top: controller badge, leader count, ingress.
  - **Topics** are the middle column: in/out rate bars and leader brokers.
  - **Consumer groups** are the right column: lag status, lag, consume rate, and consumers as `client@host`.
- **Message flow:** lines run from producers into topics and from topics to groups. Their width and the speed of their moving dashes follow msg/s, and each line shows its rate as text. Animation stops when the user prefers reduced motion.
- **Group status** is shown as a colored border plus a text label:
  - no lag
  - behind
  - far behind (≥ 10k)
  - lag with no consumers: the group lags and consumed nothing in the last 30s
- **Changes:** new consumers are highlighted, and joins/leaves are listed under "Recent changes".
- **Controls:** filter, hide internal topics, pause, and a details panel for every node.
- **Rates:**
  - Consume rates use a 30s sliding window of commits, so consumers that commit in batches do not flicker to 0.
  - Topic and broker rates come from the analytics collector (last minute).
- **Server load:** one snapshot builder runs per watched cluster, only while someone is watching. Slow clients receive only the newest snapshot.
- **Performance target:** stays above 30 fps with 500 nodes.

## 3. Architecture

```
browser ──HTTP/SSE──▶ internal/api ──▶ internal/kafka ─────────▶ Kafka (franz-go kgo/kadm)
                          │            internal/schemaregistry ─▶ Schema Registry (franz-go sr)
                          │            internal/serde
                          ├──▶ internal/auth ──────────────────▶ SQLite (sessions)
                          ├──▶ internal/analytics (collector) ─▶ SQLite (samples, rollups)
                          └──▶ internal/live (snapshot hub)
```

| Layer | Choice |
|---|---|
| Server | Go, `net/http` with pattern routing, `log/slog` |
| Kafka | `twmb/franz-go` (`kgo`, `kadm`, `sr`) |
| Serialization | `hamba/avro`, `bufbuild/protocompile` + `google.golang.org/protobuf` |
| Storage | SQLite through `modernc.org/sqlite` (pure Go, no CGO); embedded SQL migrations |
| Frontend | Vite, React, TypeScript (strict), Tailwind CSS v4, shadcn/ui, lucide icons |
| Data and routing | TanStack Query, TanStack Router, TanStack Table |
| Charts | Recharts through shadcn charts, loaded lazily |
| Tooling | Bun, Biome, Vitest, Playwright, golangci-lint |
| Packaging | `go:embed` of the built app; multi-stage Dockerfile → distroless, non-root image |

Package layout:

```
cmd/kafka-ui/            entry point, hash-password command
internal/api/            handlers, middleware (auth, CSRF, logging, recover), SSE, SPA fallback
internal/apierr/         typed errors → HTTP status + stable code
internal/auth/           users, sessions, login rate limit
internal/cluster/        one client per cluster, health checks, read-only guard
internal/config/         YAML loading, env overrides, secrets, validation
internal/kafka/          topics, messages, groups, ACLs, hygiene; no HTTP
internal/serde/          payload decoding/encoding, Schema Registry wire format
internal/schemaregistry/ Schema Registry API wrapper
internal/analytics/      collector, store, rollups, queries
internal/live/           topology snapshots and their subscriber hub
internal/storage/        SQLite open + migrations
internal/itest/          integration tests (build tag: integration)
web/src/features/<x>/    one folder per area (topics, messages, groups, acls, schemas, analytics, live…)
web/src/components/ui/   generated shadcn primitives
web/e2e/                 Playwright specs
deploy/                  Kubernetes manifests (deploy/k8s) and local dev helpers (deploy/kind)
```

## 4. API conventions

- REST under `/api/v1`. Cluster resources are under `/api/v1/clusters/{cluster}/...`. JSON uses camelCase.
- Lists return `{"items": [...]}`.
- Errors return `{"error": {"code": "TOPIC_EXISTS", "message": "..."}}`. Codes are stable and safe to match on. Common ones:
  - `UNAUTHORIZED`, `FORBIDDEN`, `CSRF`
  - `CLUSTER_NOT_FOUND`, `CLUSTER_OFFLINE`, `CLUSTER_READ_ONLY`
  - `TOPIC_NOT_FOUND`, `TOPIC_EXISTS`, `TOPIC_PROTECTED`
  - `GROUP_NOT_EMPTY`, `GROUP_NOT_FOUND`
  - `SECURITY_DISABLED`
  - `SCHEMA_INCOMPATIBLE`, `SUBJECT_NOT_FOUND`
  - `KAFKA_TIMEOUT`
  - Kafka protocol error names (for example `INVALID_CONFIG`)
- Streams use Server-Sent Events.
  - `GET .../topics/{topic}/messages/stream` emits `message`, `progress`, `done` (with `scanned`, `matched` and `reason`), or `error`.
  - `GET .../live` emits `hello`, then `snapshot` or `error`.
  - Errors before the stream starts are returned as normal JSON errors.
- Mutations use POST, PUT, PATCH or DELETE and require the CSRF header and the `admin` role.

## 5. Configuration

`config.example.yaml` documents every option. Rules:

- Secrets may be inline, `${ENV_VAR}`, or `file:/path`. They are never logged or returned by the API.
- Environment overrides:
  - `KAFKAUI_CONFIG`
  - `KAFKAUI_SERVER_ADDR`
  - `KAFKAUI_STORAGE_PATH`
  - `KAFKAUI_LOG_FORMAT` (`text` or `json`)
- Cluster IDs match `^[a-z0-9][a-z0-9-]{0,62}$`.
- The example config is loaded by a test, so it must stay valid.

## 6. Code style

**Go**

- Code is gofmt-formatted and passes golangci-lint.
- Handlers are thin. They decode, resolve the cluster, call a service, and write JSON. Services in `internal/kafka`, `serde`, `analytics` and `live` know nothing about HTTP.
- Every Kafka call takes a context with a timeout.
- Errors are wrapped with `%w`. User-facing errors are `*apierr.Error` with a stable code.
- There are no package-level mutable globals.

```go
func (s *Server) createTopic(w http.ResponseWriter, r *http.Request) {
	mutate(s, w, r, func(ctx context.Context, c *cluster.Cluster, req kafka.CreateTopicRequest) (any, error) {
		t, err := kafka.CreateTopic(ctx, c.Admin, req) // validation and broker errors become apierr codes
		if err == nil && !req.ValidateOnly {
			s.audit(r, "topic.create", "topic", req.Name)
		}
		return t, err
	})
}
```

**TypeScript/React**

- Strict TypeScript, function components, no `any`.
- Server state lives in TanStack Query. Each feature owns its hooks in `features/<x>/api.ts`.
- File names are kebab-case, components PascalCase.
- Every page handles its loading, empty and error states.
- Destructive actions go through a confirmation dialog; deletes and purges require typing the name.
- Pure logic (merging snapshots, formatting, diffing) lives in plain `.ts` modules with unit tests.
- Charts follow the palette in `index.css` (`--series-1..3` for categories, fixed status colors with text labels) and use one y-axis per chart.

## 7. Testing

| Level | Tool | Command |
|---|---|---|
| Go unit | `testing` | `make test` |
| Frontend unit | Vitest | `make test` |
| Integration | testcontainers-go: Kafka, an ACL-enabled Kafka and Schema Registry in Docker | `make test-integration` |
| Coverage | merged unit + integration (`go tool covdata`) | `make coverage` |
| E2E | Playwright against a running instance (`KAFKAUI_E2E_URL`) | `make e2e` |

- **Integration tests** cover the Kafka-facing code end to end:
  - topics, browse, produce, purge
  - groups and offset reset
  - ACLs
  - Schema Registry round trips
  - the collector and live snapshots
  - the HTTP API with SSE
- **Coverage target:** ≥ 80% (unit + integration) for `internal/kafka`, `internal/serde`, `internal/analytics` and `internal/auth`.
- **E2E tests** only create and change resources whose names start with `e2e-`. They log in once per role, because logins are rate-limited.
- A bug fix starts with a failing test.

## 8. Contributing rules

**Always**

- Run `make lint test` (and `make test-integration` when Kafka-facing code changes) before opening a pull request.
- Validate input on the server: names, partition counts, offsets, regexes, formats.
- Enforce role and `readOnly` checks on the server, never only in the UI.
- Give every Kafka and registry call a timeout, and cap anything that scans.
- Keep secrets out of logs, API responses and the repository.
- Update this spec and `config.example.yaml` when behavior or configuration changes.

**Discuss first** (open an issue)

- New dependencies.
- Changes to the API contract, error codes or the config format.
- New mutating features, or relaxing a safety rule (protected topics, `Empty`-only resets, confirmations).

**Never**

- Commit credentials or real configs (`config.yaml` is git-ignored).
- Hand-edit the generated shadcn primitives in `web/src/components/ui` beyond theming.
- Remove or skip failing tests to make a build pass.

## 9. Known limitations and ideas

- **Producers:** they can't be listed through the Kafka protocol, so they appear only as topic ingress.
- **Roles:** there are only `admin` and `viewer`, global for all clusters. Per-cluster roles are a likely next step.
- **JSON Schema:** payloads are checked for valid JSON on produce, not validated against the schema.
- **Bytes/s:** it is an estimate from log size growth, so retention deletes can lower it.
- **Login:** there is no SSO (OIDC/LDAP) yet.
