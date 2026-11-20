---
title: Architecture
description: How kafka-ui is put together.
---

```
browser ──HTTP/SSE──▶ api ──▶ kafka ──────────▶ Kafka (franz-go)
                       │      schemaregistry ─▶ Schema Registry
                       │      serde
                       ├──▶ auth ─────────────▶ SQLite (sessions)
                       ├──▶ analytics ────────▶ SQLite (samples, rollups)
                       └──▶ live
```

- **One process.** A Go binary serves the API and the embedded React app. It keeps one Kafka client per cluster, a background health check, an analytics collector, and a live-topology builder that runs only while someone is watching.
- **Kafka protocol only.** It uses the [franz-go](https://github.com/twmb/franz-go) client and admin APIs. It needs no JMX, no operator, and no Kubernetes API.
- **Storage.** SQLite through a pure-Go driver (no CGO) holds sessions and analytics. Migrations are embedded.
- **Streaming.** Message scans and the live view use Server-Sent Events, so long scans show progress and can be cancelled.

| Package | Responsibility |
|---|---|
| `internal/api` | HTTP handlers, middleware (auth, CSRF, logging, recovery), SSE, SPA serving |
| `internal/auth` | users, sessions, login rate limit |
| `internal/cluster` | clients per cluster, health checks, read-only guard |
| `internal/config` | YAML, environment overrides, secrets, validation |
| `internal/kafka` | topics, messages, groups, ACLs, hygiene (no HTTP) |
| `internal/serde` | payload decoding and encoding, Schema Registry wire format |
| `internal/schemaregistry` | Schema Registry API |
| `internal/analytics` | collector, storage, rollups, queries |
| `internal/live` | topology snapshots and their subscribers |
| `web/` | React, TypeScript, Vite, TanStack Query/Router/Table, shadcn/ui, Recharts |
