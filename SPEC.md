# Spec: Kafka UI

> Status: **APPROVED, in progress** (M0–M6 done, 2026-11-20; open items: integration/E2E tests, 500-node FPS check). Living document: update this first whenever scope or a decision changes.

## 1. Objective

A single-binary web UI for running and watching Apache Kafka clusters. One Go monolith serves a REST/SSE API and an embedded React (shadcn/ui) SPA.

**Users:** developers and platform engineers who need to see what is happening in one or more Kafka clusters (topics, messages, consumer lag, ACLs, schemas) and make day-to-day changes without the CLI tools.

**What success looks like:** against the local `kafkatr` kind cluster, an engineer can log in, switch clusters, find a topic, read and produce messages, see why `ledger` is lagging, reset its offsets, and watch the brokers and the connected consumers move in real time on a 3D topology view.

### Assumptions (correct these if wrong)
1. Kafka >= 3.x, KRaft mode. The reference cluster is Strimzi `orders-cluster`, Kafka 4.3.2, 3 dual-role nodes.
2. The UI talks to Kafka only over the Kafka protocol (admin + consumer + producer APIs). It does **not** depend on Strimzi CRDs or the K8s API.
3. Broker host metrics (CPU, JVM heap) are out of scope for v1. "Analytics" means numbers we can get from the Kafka protocol: offsets, log dir sizes, group state, and so on.
4. Producers cannot be enumerated through the Kafka admin API. The live view shows *consumers* exactly; producer activity shows up as topic ingress rate only.
5. The spec is written in English; UI copy is English in v1 (i18n-ready, no translations yet).

## 2. Features & Acceptance Criteria

Features are grouped into milestones (see section 10). Each bullet is an acceptance criterion.

### F1. Multi-cluster
- Clusters are defined in a YAML config. Each one has: bootstrap servers, optional SASL (PLAIN, SCRAM-SHA-256/512), optional TLS (CA, client cert, skip-verify), optional Schema Registry, and a `readOnly` flag.
- A cluster switcher sits in the top bar; every URL is cluster-scoped (`/clusters/:clusterId/...`).
- One cluster that cannot be reached must not break the others. It shows as `offline` with the last error.
- A cluster marked `readOnly` rejects every mutating API call with `403`, whatever the user's role.

### F2. Authentication & authorization (UI)
- Username/password login. Users come from config with bcrypt password hashes.
- Two roles: `admin` (everything) and `viewer` (read-only: no create, delete, produce, reset or config edits).
- Session: a server-side session ID in an `HttpOnly; Secure; SameSite=Strict` cookie; idle timeout is configurable (default 12h).
- Every `/api/*` route except `/api/v1/auth/login` and `/healthz` requires a session. Mutating routes also need the `X-Requested-With: kafka-ui` header (CSRF defense).
- Failed logins are rate limited per IP (5/min).

### F3. Cluster & broker overview
- Brokers: ID, host:port, rack, controller flag, partition leader count, replica count, log dir size (from `DescribeLogDirs`).
- Cluster summary: Kafka version, cluster ID, number of topics/partitions, under-replicated partitions, offline partitions, total size.
- Broker detail: the broker's configs (read-only in v1) and the partitions it leads.

### F4. Topics
- List with search, internal-topic toggle, and columns for partitions, RF, size, message count (sum of HWM − LWM), ingress msg/s, URP count.
- Detail tabs: Overview, Partitions (leader, replicas, ISR, LWM/HWM, size), Configs (overrides highlighted), Consumers (groups reading this topic and their lag), Messages, Schema (if SR is configured).
- Create topic (name, partitions, RF, configs) with validation.
- Delete topic: the user must type the topic name to confirm.
- Edit topic configs; increase partitions (decrease is not possible and is explained in the UI).
- Purge topic (delete records up to HWM), behind a confirmation.

### F5. Message browser & producer
- Seek modes: oldest, newest (last N), specific offset (per partition), timestamp.
- Partition selector (all or a subset). Results stream to the UI over SSE so a long scan shows progress and can be cancelled.
- Server-side filters: key / value / header substring or regex. Scans stop at a configurable limit (default: 500 matches or 100k scanned records).
- Deserializers, auto-detected with manual override: String, JSON (pretty printed), Hex/Base64, Avro / Protobuf / JSON Schema through Schema Registry (Confluent wire format).
- Each message row shows partition, offset, timestamp, key, value preview, headers, size. Expanding a row shows the full content with copy buttons.
- Produce: key, value, headers, partition (optional), serializer (String / JSON / SR schema). Admin only.

### F6. Consumer groups
- List: group ID, state, members, protocol, topics, total lag.
- Detail: members (client ID, host, assigned partitions) and a per-partition table with committed offset, HWM and lag; lag trend chart (from F9).
- Reset offsets for a topic or partitions: earliest, latest, specific offset, timestamp, shift by ±N. Only allowed when the group is `Empty`; the UI explains why otherwise. A dry-run preview comes first.
- Delete group (only when it is `Empty`).

### F7. ACLs
- List ACLs with filters (principal, resource type, resource name, pattern type, operation).
- Create and delete ACL bindings. Admin only.
- If the cluster has no authorizer (`SECURITY_DISABLED`), the page says so instead of failing.

### F8. Schema Registry (Confluent-compatible API; works with Confluent SR and Apicurio ccompat)
- Subjects list with search, latest version, schema type, compatibility level.
- Subject detail: versions, schema diff between two versions, references.
- Register a new version (with a compatibility check first), change compatibility (global and per subject), delete a version or subject (soft, then optional hard).
- Topic → subject link through TopicNameStrategy.

### F9. Analytics
- A per-cluster background collector samples every `collector.interval` (default 5s):
  - per-partition LWM/HWM → topic ingress msg/s
  - per-group committed offsets → consume rate and lag per group/topic/partition
  - log dir sizes per broker/topic (every 60s)
  - URP / offline partition counts
- Samples go into embedded SQLite with retention (default 7d) and downsampling (raw 5s for 6h, 1m rollups for 7d).
- Dashboards (shadcn charts / Recharts):
  - cluster throughput over time (msg/s, and bytes/s where it can be estimated from size deltas)
  - top N topics by ingress / size / growth
  - consumer lag over time, worst groups, "time to catch up" estimate (lag ÷ net consume rate)
  - partition skew: hot partitions, uneven leader distribution across brokers
  - hygiene: idle topics (no ingress in X), empty/dead groups, topics with RF < 2 or `min.insync.replicas` misconfigured
- Time range picker: 15m / 1h / 6h / 24h / 7d.

### F10. Live topology (3D)
- Route `/clusters/:id/live` shows a real-time 3D force graph (`react-force-graph-3d` / three.js):
  - **Nodes:** brokers (controller highlighted), topics, consumer groups, consumer members (client ID @ host).
  - **Edges:** topic → leader broker; consumer member → topic, built from partition assignments.
  - Node size encodes throughput or size; edge particles flow at a speed that tracks msg/s; lag is a red-to-green color scale on group nodes.
- The server pushes a snapshot over SSE every 2s (configurable). The client diffs it so the graph animates instead of re-laying out.
- Joins and leaves (e.g. scaling `analytics` from 3 to 4 replicas, a rebalance) show up within one or two ticks, with a short highlight animation.
- Clicking a node opens a side panel with its details and links to the matching F3/F4/F6 page.
- Toggles: 2D/3D, hide internal topics, filter by topic/group, pause.
- Must stay above 30 fps with 500 nodes on a recent laptop.

### Non-goals (v1)
Kafka Connect / MirrorMaker management, ksqlDB, Strimzi CRD awareness, OIDC/LDAP login, JMX/Prometheus broker metrics, alerting, editing broker configs, multi-tenant RBAC beyond admin/viewer.

## 3. Tech Stack

| Layer | Choice | Notes |
|---|---|---|
| Language | Go 1.27 | matches the local toolchain |
| HTTP | `net/http` (1.22+ pattern routing) | no framework; small middleware chain |
| Kafka client | `github.com/twmb/franz-go` (`kgo`, `kadm`) | pure Go, full admin API, SASL/TLS |
| Schema Registry | `franz-go/pkg/sr` + `hamba/avro`, `bufbuild/protocompile` | wire-format decode |
| Storage | SQLite through `modernc.org/sqlite` (no CGO) | sessions + analytics samples |
| Config | YAML + env overrides (`KAFKAUI_*`), secrets read from env/files | |
| Logging | `log/slog` (JSON in prod) | |
| Realtime | Server-Sent Events | one-way push is all we need; easy through proxies |
| Frontend | Vite + React 19 + TypeScript (strict) | built output embedded with `go:embed` |
| UI | shadcn/ui + Tailwind CSS v4, lucide icons, dark mode default | |
| Data/routing | TanStack Query + TanStack Router, TanStack Table for grids, TanStack Virtual for message lists | |
| Charts | shadcn charts (Recharts) | |
| 3D | `react-force-graph-3d` (three.js) | |
| Forms | react-hook-form + zod | |
| FE tooling | Bun (package manager/runner), Biome (lint+format), Vitest + Testing Library, Playwright | |
| Deploy | Multi-stage Dockerfile → distroless image; K8s manifests in `deploy/` | runs inside kind |

Versions: latest stable when the project is initialized, pinned in `go.mod` / `bun.lock`.

## 4. Commands

```bash
# one-time
make setup              # go mod download && cd web && bun install

# development
make web-dev            # Vite dev server on :5173, proxies /api -> 127.0.0.1:18080 (override: KAFKAUI_API)
make kind-deploy        # build image, `kind load docker-image --name kafkatr`, apply deploy/k8s, rollout restart
make kind-forward       # kubectl -n kafka-ui port-forward svc/kafka-ui 18080:8080  (host :8080 is taken locally)
make kind-logs          # kubectl -n kafka-ui logs -f deploy/kafka-ui

# build
make build              # bun run build (web/dist) && go build -o bin/kafka-ui ./cmd/kafka-ui
make image              # docker build -t kafka-ui:dev .

# quality
make lint               # golangci-lint run ./... && cd web && bunx biome check .
make fmt                # gofmt -w . && cd web && bunx biome format --write .
make test               # go test -race ./... && cd web && bun run test
make test-integration   # go test -race -tags=integration ./...   (testcontainers: apache/kafka)
make test-e2e           # cd web && bunx playwright test         (against port-forwarded kind deploy)
```

Dev loop: the backend runs inside kind (it needs in-cluster DNS for the broker advertised addresses). The frontend runs locally with HMR and proxies to the port-forwarded backend.

## 5. Project Structure

```
cmd/kafka-ui/main.go          → entrypoint: load config, wire deps, start server
internal/
  config/                     → YAML + env loading, validation
  auth/                       → users, bcrypt, sessions, roles
  cluster/                    → cluster registry: one kgo/kadm client per cluster, health, readOnly guard
  apierr/                     → typed errors -> HTTP status + stable code
  kafka/                      → domain services over kadm/kgo, one file per area (brokers, topics, groups, ...); no HTTP
  serde/                      → (de)serializers: string, json, hex, avro, protobuf, jsonschema
  schemaregistry/             → SR client wrapper
  analytics/
    collector/                → periodic sampling per cluster
    store/                    → SQLite schema, writes, rollups, queries
  live/                       → topology snapshot builder + SSE broadcaster
  api/                        → router, middleware (auth, csrf, logging, recover), handlers, SPA fallback
  storage/                    → SQLite open/migrations (embedded SQL)
web/
  embed.go                    → go:embed of web/dist (package web)
  src/
    components/ui/            → shadcn generated components (do not hand-edit beyond theming)
    components/               → shared app components (layout, cluster switcher, data-table...)
    features/<feature>/       → topics, messages, groups, brokers, acls, schemas, analytics, live
    lib/                      → api client, sse helper, formatters
    routes/                   → TanStack Router route tree
  e2e/                        → Playwright specs
deploy/
  k8s/                        → namespace, configmap, secret (example), deployment, service
  kind/                       → helper manifests for the local cluster (e.g. schema registry)
config.example.yaml
Dockerfile
Makefile
SPEC.md
```

Tests sit next to the code (`foo_test.go`, `Foo.test.tsx`). Integration tests carry the `//go:build integration` tag.

## 6. Code Style

**Go:** gofmt + golangci-lint (errcheck, govet, staticcheck, revive, gosec). Handlers stay thin and services know nothing about HTTP. Every Kafka call takes a `context.Context` with a timeout. Wrap errors with `%w`. No globals.

```go
// internal/api/topics.go
func (h *TopicsHandler) Create(w http.ResponseWriter, r *http.Request) {
	c, err := h.clusters.Writable(r.PathValue("cluster"))
	if err != nil {
		writeError(w, err) // maps ErrClusterNotFound -> 404, ErrReadOnly -> 403
		return
	}
	var req CreateTopicRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, apierr.BadRequest("invalid body", err))
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, apierr.BadRequest(err.Error(), err))
		return
	}
	topic, err := h.topics.Create(r.Context(), c, req.toSpec())
	if err != nil {
		writeError(w, fmt.Errorf("create topic %q: %w", req.Name, err))
		return
	}
	writeJSON(w, http.StatusCreated, topic)
}
```

**API conventions:** REST under `/api/v1/clusters/{cluster}/...`. JSON uses camelCase. Errors look like `{"error":{"code":"TOPIC_EXISTS","message":"..."}}`. List endpoints return `{"items":[...]}`. Streams (`/messages/stream`, `/live`) are SSE.

**TypeScript/React:** strict TS, function components, no `any`. Server state lives in TanStack Query (no global store for it). Each feature owns its queries in `features/<x>/api.ts`. File names are kebab-case, components PascalCase.

```tsx
// web/src/features/topics/topic-list.tsx
export function TopicList({ clusterId }: { clusterId: string }) {
  const { data, isPending, error } = useTopics(clusterId)
  if (error) return <ErrorState error={error} />
  return (
    <DataTable
      columns={topicColumns}
      data={data?.items ?? []}
      loading={isPending}
      searchKey="name"
      emptyState={<EmptyState title="No topics" />}
    />
  )
}
```

Every page handles its loading, empty and error states. Destructive actions always go through a shadcn `AlertDialog`.

## 7. Testing Strategy

| Level | Tool | Scope | Where |
|---|---|---|---|
| Go unit | `testing` + `go-cmp` | serde, config validation, lag/rate math, rollups, auth/session, handler mapping with fake services | `internal/**/_test.go` |
| Go integration | testcontainers-go (`apache/kafka` KRaft) + Confluent SR container | kafka services end to end: create/describe topics, produce/consume, groups and offset reset, ACLs (authorizer-enabled container), SR decode | `-tags=integration` |
| Frontend unit | Vitest + Testing Library + MSW | components, hooks, formatters, the live-graph diff logic | `web/src/**/*.test.tsx` |
| E2E | Playwright | login, topic CRUD, browse/produce, group reset, live view renders, against the kind deployment | `web/e2e/` |

- Bug fixes start with a failing test.
- Coverage target: ≥ 80% for `internal/kafka`, `internal/serde`, `internal/analytics`, `internal/auth`. No hard gate elsewhere.
- E2E tests only use topics/groups prefixed `e2e-` and clean them up. They never touch `orders`, `payments`, `clicks` or `emails` beyond reading.

## 8. Boundaries

**Always**
- Run `make lint test` before calling a task done.
- Validate every input on the server (topic names, partition counts, offsets, regex length).
- Enforce role and `readOnly` checks on the server, never just in the UI.
- Give every Kafka call a timeout. Cap message scans (count and time).
- Keep secrets out of logs and out of API responses (SASL passwords, SR credentials).
- Update SPEC.md when a decision changes.

**Ask first**
- Any change to the `kafkatr` kind cluster (Strimzi CRs, listeners, enabling an authorizer, deploying Schema Registry).
- Mutating existing topics/groups in kind (`orders`, `payments`, `clicks`, `emails`, the groups `analytics`, `ledger`, `mailer`...), e.g. offset resets on real groups.
- Adding a dependency that is not in the stack table.
- Changing the API contract shape or the config file format once implemented.

**Never**
- Commit secrets, real credentials or `.env` files.
- Delete or restart the existing workloads in the `kafka` namespace.
- Hand-edit generated shadcn primitives beyond theming tokens.
- Remove or skip failing tests without approval.

## 9. Success Criteria (v1 done)

Each item is checked against the `kafkatr` kind cluster:
1. `make build` produces one binary that serves the UI and API; the image runs in namespace `kafka-ui` in kind.
2. Two clusters are configured (`orders-cluster` plus a second, e.g. the integration container or an unreachable one). The switcher works; the unreachable one shows `offline` while the other keeps working.
3. A `viewer` login cannot mutate anything: the API returns 403 and the UI hides or disables the actions.
4. The topics list shows `clicks` (4 partitions, RF 3), `orders`, `payments`, `emails` with correct partition/ISR data.
5. Reading the last 100 messages of `orders` renders JSON pretty-printed; a 100k-record filtered scan streams progress and can be cancelled.
6. Groups `analytics`, `ledger`, `ledger-retry`, `ledger-dlq`, `mailer` appear with members and per-partition lag that matches `kafka-consumer-groups.sh --describe` within one collector tick.
7. An offset reset on an `e2e-` group (dry run, then apply) works; resetting a group that is not `Empty` is refused with a clear message.
8. The ACL page lists, creates and deletes bindings on an authorizer-enabled cluster, and shows a "no authorizer" notice on `orders-cluster` as it is today.
9. With Schema Registry deployed, an Avro-encoded topic decodes in the browser and a new schema version can be registered.
10. Analytics shows 1h throughput for `clicks` and the lag trend for `ledger`; the data survives a pod restart (PVC-backed SQLite).
11. Live view: `kubectl scale deploy/analytics --replicas=4` shows a 4th consumer node within 5s; scaling back removes it. At least 30 fps with the reference cluster.
12. `make lint test test-integration` pass.

## 10. Milestones

| # | Milestone | Contents |
|---|---|---|
| M0 | Skeleton | repo layout, config, server, embed, shadcn shell + layout, Dockerfile, kind deploy, CI-able Makefile |
| M1 | Core read | multi-cluster registry, auth/roles, overview/brokers, topics read, groups read |
| M2 | Messages | browser (SSE stream, filters, string/json/hex), producer |
| M3 | Mutations | topic create/delete/config/partitions/purge, group reset/delete, ACLs |
| M4 | Schema Registry | SR client, Avro/Proto/JSON Schema serde, subjects UI |
| M5 | Analytics | collector, SQLite store and rollups, dashboards |
| M6 | Live 3D | snapshot builder, SSE, force graph, side panel |

Each milestone gets a plan and a task breakdown (spec-driven phases 2–3) before implementation starts.

## 11. Open Questions

1. ~~Schema Registry in kind~~ (resolved: Confluent cp-schema-registry 8.0.0). Was: nothing is deployed yet. Proposal: deploy Apicurio Registry (ccompat API) or Confluent `cp-schema-registry` into namespace `kafka-ui` (or `kafka`). Which one? (Ask-first: this changes the cluster.)
2. ~~ACLs in kind~~ (resolved: second cluster `secure-cluster`). Was: `orders-cluster` has no authorizer and only a plain internal listener. Do we enable `authorization: simple` on it (it needs care, since the existing apps connect anonymously), or test ACLs only against the integration container plus a second Strimzi cluster?
3. **UI users:** are the two built-in roles (admin/viewer) enough for v1, or do you need per-cluster roles (e.g. admin on dev, viewer on prod)?
4. ~~Second real cluster~~ (resolved: yes). Was: should we create a small second Strimzi `Kafka` in kind for multi-cluster testing?
5. ~~Git~~: private repo github.com/huseyinbabal/kafka-ui.

## 12. Decisions Log
- 2026-11-20: Roles stay admin/viewer for v1 (default; per-cluster roles revisit later).
- 2026-11-20: Active controller comes from DescribeQuorum leader; KRaft Metadata returns a random broker as "controller" (KIP-590).
- 2026-11-20: Group lag totals exclude partitions without a committed offset (matches `kafka-consumer-groups.sh`).
- 2026-11-20: Offline clusters fail fast with 503 `CLUSTER_OFFLINE` based on the 15s health check instead of waiting on Kafka timeouts.
- 2026-11-20: TanStack Table pinned to v8 (v9 API is a rewrite).
- 2026-11-20: Message streams use fetch + a small SSE parser (not EventSource) so pre-stream errors return JSON and Stop aborts the request. Events: `message`, `progress` (≤4/s), `done` {scanned, matched, reason}, `error`.
- 2026-11-20: Seek modes oldest/newest/offset/timestamp plus `live` tail (15 min cap). Bounded scans cap at 60s and 100k records; newest reads backwards in growing windows per partition and sorts each window newest-first.
- 2026-11-20: Substring filters are case-insensitive; regex is Go RE2 (linear time, no ReDoS). Streamed payload text is capped at 64 KB; the single-message endpoint returns full content.
- 2026-11-20: Produce checks topic existence first (never auto-creates) and picks the partition like the Java client (murmur2 of the key) when none is given. Produce is admin-only and refused on readOnly clusters; each produce is logged with the user.
- 2026-11-20: JSON is pretty-printed client-side without JSON.parse so 64-bit integers are shown exactly.
- 2026-11-20: Dev data for scans lives in `e2e-messages` (3 partitions, ~120k perf-test records); business topics are only read.
- 2026-11-20: Mutations never touch `__`-prefixed topics (403 `TOPIC_PROTECTED`). Topic and group deletes, and purges, need the name typed in the UI.
- 2026-11-20: Offset reset and group delete only run on `Empty` groups (reset also allows `Dead`, i.e. not yet created). The state is checked again right before the commit; a dry run is required in the UI before Apply. `shift` skips partitions without a commit.
- 2026-11-20: Mutations are logged as `msg=audit` with action, user and cluster. The broker's own error message (e.g. invalid config value) is passed to the UI.
- 2026-11-20: The ACL page shows "No authorizer" on `SECURITY_DISABLED`. ACL deletes match one exact entry (literal/prefixed only; ANY/MATCH are not exposed).
- 2026-11-20: Second dev cluster `secure-cluster` (namespace `kafka`, 1 node, SCRAM-SHA-512, simple authorizer, `kafka-ui` is a super user) from `deploy/k8s/secure-cluster.yaml`; `make kind-secure` creates it and copies the kafka-ui password into the `kafka-ui` namespace. ACL CRUD verified against it.
- 2026-11-20: A cluster whose client cannot be created (e.g. missing credential env var) is listed as offline with the reason instead of stopping the app.
- 2026-11-20: Dev Schema Registry is Confluent `cp-schema-registry:8.0.0` in namespace `kafka-ui` (`deploy/kind/schema-registry.yaml`, `make kind-sr`), storing schemas in `_schemas` on orders-cluster. Configured for `orders` and `orders-readonly`; `secure` has none.
- 2026-11-20: Auto decoding tries the registry only for payloads in Confluent wire format (magic 0 + positive schema id). On failure the raw bytes are shown with the reason; failed schema lookups are cached for 30s so big scans don't hammer the registry. Avro JSON keeps schema field order; bytes/fixed render as base64; Avro unions are shown unwrapped.
- 2026-11-20: Producing with "Schema Registry" takes a subject (+ version, + protobuf message name; default first message). Avro input accepts bare or {"type": value} union values. JSON Schema payloads are only checked for valid JSON, not validated against the schema.
- 2026-11-20: Interop verified both ways with Confluent's console tools (Avro and Protobuf, incl. 2^53+1 longs).
- 2026-11-20: Schema endpoints are not gated on Kafka health (the registry is a separate service). Subject delete is soft by default; permanent delete is opt-in.
- 2026-11-20: Analytics store: per-topic offset/size sums, per-group-topic committed sum + lag, and cluster URP/offline counts. Raw samples are kept 6h, then rolled into minute buckets (max per minute: counters keep their last value, lag/URP keep spikes) kept for `storage.retention` (7d). Compaction runs every 5 minutes.
- 2026-11-20: Rates are derived at query time from counter deltas, timed by the actual samples in each bucket (a partly filled last bucket is not under-reported). Charts get about 240 points per range. Bytes/s is estimated from log size growth, sampled every 30s, and charted in ≥1-minute buckets.
- 2026-11-20: Group catch-up = lag ÷ (consume − ingress) over the trailing 5 minutes; "not consuming" when the consume rate is 0, "falling behind" when consume ≤ ingress. Groups not sampled in the last two buckets (deleted) are dropped from the list.
- 2026-11-20: Skew (hot partitions over 1.5× their fair share, leaders/replicas/ingress per broker) comes from the collector's in-memory last minute, not the DB. Idle topics report where their evidence starts (first sample or range start).
- 2026-11-20: Charts use the dataviz reference palette (validated light/dark, first three slots) as `--series-1..3`; one y-axis per chart, so msg/s and bytes/s are separate charts.
- 2026-11-20: Demo traffic: `deploy/kind/traffic.yaml` (`make kind-traffic`) runs a ~20 msg/s producer into `e2e-traffic` (6 partitions) and a bursty slow consumer group `e2e-slow-consumer`.
- 2026-11-20: Live view: one snapshot builder per watched cluster (`internal/live.Hub`), only while someone watches; snapshots every `collector.liveInterval` (2s) over SSE, slow clients get the newest one. Nodes: brokers, topics, groups, members; links: topic→leader broker, group→member, member→topic (assignments), group→topic (committed offsets, so member-less groups stay near their topics). Topic/broker rates come from the analytics collector (last minute); group/member rates from commit deltas between snapshots.
- 2026-11-20: The client merges snapshots into the existing graph (node and link objects keep identity and positions); the layout only gets new data when nodes/links are added or removed. New members start next to their group and are drawn 3× larger for 4s; joins/leaves go to a "Recent changes" list. Group color = lag status (good/warning/serious/critical, dataviz status palette) with legend and labels.
- 2026-11-20: The live page and three.js load lazily (separate ~430 KB gz chunk). Kafka-provided labels are HTML-escaped in tooltips.
- 2026-11-20: Success criterion 11 verified with the e2e demo consumer (scaling `e2e-traffic-consumer` 1→3 showed the new members within one tick after their JVMs joined) rather than `analytics`, which belongs to the existing workloads (ask first). Measured ~30 fps in the automation browser with ~35 nodes, which looks like a browser frame cap; the 500-node budget is not verified yet.
