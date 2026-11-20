# kafka-ui

A web UI for running and watching Apache Kafka clusters. One Go binary serves the
API and an embedded React (shadcn/ui) app. It talks to Kafka only over the Kafka
protocol, so it works with any distribution (Strimzi, Confluent, MSK, plain Apache Kafka).

## Features

- **Multi-cluster** with health checks; unreachable clusters are shown offline instead of hanging, and a cluster can be marked `readOnly`.
- **Login** with `admin` (everything) and `viewer` (read-only) roles; server-side sessions, CSRF protection, login rate limiting, audit log of every change.
- **Brokers and topics:** overview, partitions with replicas/ISR, configs; create, delete, edit configs, add partitions, purge.
- **Message browser:** newest / oldest / from offset / from time / live tail, partition selection, server-side substring or regex filters, results streamed while scanning; String, JSON, Base64, Hex, and Avro / Protobuf / JSON Schema via Schema Registry. Producer with keys, headers and schemas.
- **Consumer groups:** members, assignments, per-partition lag; offset reset with a mandatory preview (only on groups without active members); delete.
- **ACLs:** list, create, delete (shows a notice when the cluster has no authorizer).
- **Schema Registry** (Confluent API): subjects, versions with diff, compatibility levels and checks, register, soft/hard delete.
- **Analytics:** throughput (msg/s, bytes/s), consumer lag with catch-up estimates, top topics, partition skew, hygiene checks (replication factor, `min.insync.replicas`, idle topics, empty groups). Sampled every 5s into SQLite (raw 6h, 1-minute rollups for 7 days).
- **Live topology:** brokers, topics and consumer groups with their consumers, with message flow drawn in proportion to msg/s, updated every 2s.

## Quick start (local binary)

Requirements: Go 1.27+, [Bun](https://bun.sh) 1.3+.

```sh
make setup                       # Go modules + web dependencies
cp config.example.yaml config.yaml
# edit clusters in config.yaml, then:
make run                         # builds the UI and starts on :8080
```

Open http://localhost:8080 and sign in. The example users are `admin/admin` and `viewer/viewer`: **change them before exposing the UI**.

Create a password hash for a new user:

```sh
echo -n 's3cret' | ./bin/kafka-ui hash-password
```

and put it under `auth.users` with `role: admin` or `role: viewer`.

## Configuration

See [`config.example.yaml`](config.example.yaml) for every option (SASL PLAIN/SCRAM, TLS/mTLS, Schema Registry with basic auth, retention, sampling intervals).

- Secrets can be written inline, as `${ENV_VAR}`, or as `file:/path`. They are never logged or returned by the API.
- Overrides via environment: `KAFKAUI_CONFIG` (config path), `KAFKAUI_SERVER_ADDR`, `KAFKAUI_STORAGE_PATH`, `KAFKAUI_LOG_FORMAT` (`text` or `json`).
- A cluster that cannot be set up (e.g. a missing credential) is listed as offline with the reason; the rest of the UI keeps working.

## Running in Kubernetes (kind dev setup)

The repository is developed against a local kind cluster called `kafkatr` with Strimzi:

```sh
make kind-deploy      # build the image, load it into kind, apply deploy/k8s/kafka-ui.yaml
make kind-forward     # port-forward to http://127.0.0.1:18080
make kind-sr          # optional: Confluent Schema Registry (deploy/kind/schema-registry.yaml)
make kind-secure      # optional: second cluster with SCRAM + ACLs (deploy/k8s/secure-cluster.yaml)
make kind-traffic     # optional: demo producer + slow consumer for analytics/live view
```

`deploy/k8s/kafka-ui.yaml` is a dev manifest (Secret with config, PVC for the SQLite database, Deployment, Service). It runs as non-root with a read-only root filesystem. For production, replace the dev users and enable `secureCookies` behind HTTPS.

## Development

```sh
make web-dev          # Vite dev server with HMR on :5173, proxying /api to 127.0.0.1:18080 (KAFKAUI_API to change)
make lint             # golangci-lint, Biome, tsc
make test             # Go unit tests (race) + Vitest
make test-integration # Go tests against Kafka, an ACL-enabled Kafka and Schema Registry in Docker (testcontainers)
make coverage         # unit + integration coverage per package
make e2e              # Playwright against the running deployment (KAFKAUI_E2E_URL, default http://127.0.0.1:18080)
```

Layout:

```
cmd/kafka-ui/          entry point
internal/api/          HTTP handlers, middleware, SSE
internal/auth/         users, sessions, rate limiting
internal/cluster/      one Kafka client per cluster, health, read-only guard
internal/kafka/        topics, messages, groups, ACLs, hygiene (no HTTP)
internal/serde/        payload decoding/encoding, Schema Registry wire format
internal/schemaregistry/  Schema Registry API wrapper
internal/analytics/    collector, SQLite store, rollups, queries
internal/live/         topology snapshots and their SSE hub
internal/itest/        integration tests (-tags=integration)
web/                   React app (Vite, TanStack Router/Query/Table, shadcn/ui); web/e2e Playwright specs
```

The design, decisions and acceptance criteria live in [`SPEC.md`](SPEC.md).

## Limitations

- Producers can't be listed through the Kafka protocol; the UI shows them only as topic ingress.
- Broker host metrics (CPU, heap) and JMX are out of scope.
- Bytes/s is estimated from log size growth, so retention deletes can lower it.
- JSON Schema payloads are checked for valid JSON on produce, not validated against the schema.
