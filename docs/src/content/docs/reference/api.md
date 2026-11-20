---
title: HTTP API
description: The REST and SSE API behind the UI.
---

The UI uses a JSON API under `/api/v1`. You can script against it with the same session cookie.

## Conventions

- **Login:** `POST /api/v1/auth/login` with `{"username": "...", "password": "..."}` sets the `kui_session` cookie.
- **Changes** (`POST`, `PUT`, `PATCH`, `DELETE`) need the `admin` role and the header `X-Requested-With: kafka-ui`.
- **Lists** return `{"items": [...]}`.
- **Errors** return `{"error": {"code": "...", "message": "..."}}` with a stable code (see below).
- **Streams** are Server-Sent Events.

```sh
curl -c jar -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' http://localhost:8080/api/v1/auth/login
curl -b jar http://localhost:8080/api/v1/clusters/local/topics
```

## Endpoints

All cluster paths start with `/api/v1/clusters/{cluster}`. ✱ marks changes (admin only).

| Method | Path | |
|---|---|---|
| GET | `/healthz` | liveness (no auth) |
| GET | `/api/v1/auth/me` · POST `/api/v1/auth/logout` | session |
| GET | `/api/v1/clusters` | clusters with health |
| GET | `…/overview` | cluster and broker summary |
| GET | `…/brokers/{id}/configs` | broker configs |
| GET | `…/topics` · `…/topics/{topic}` · `…/topics/{topic}/consumers` | topics |
| POST ✱ | `…/topics` | create (`validateOnly` supported) |
| DELETE ✱ | `…/topics/{topic}` | delete |
| PATCH ✱ | `…/topics/{topic}/configs` | `{"changes":[{"name":"retention.ms","value":"3600000"}]}`; `null` resets |
| PUT ✱ | `…/topics/{topic}/partitions` | `{"count": 12}` |
| POST ✱ | `…/topics/{topic}/purge` | delete all records |
| GET | `…/topics/{topic}/messages/stream` | SSE scan (see below) |
| GET | `…/topics/{topic}/messages/{partition}/{offset}` | one message, full content |
| POST ✱ | `…/topics/{topic}/messages` | produce |
| GET | `…/groups` · `…/groups/{group}` | groups with lag |
| POST ✱ | `…/groups/{group}/reset-offsets` | `{"topic":"t","strategy":"latest","dryRun":true}` |
| DELETE ✱ | `…/groups/{group}` | delete an empty group |
| GET / POST ✱ | `…/acls` | list / create |
| POST ✱ | `…/acls/delete` | delete one binding |
| GET | `…/schemas/subjects` · `…/schemas/subjects/{subject}` | subjects |
| POST | `…/schemas/subjects/{subject}/compatibility-check` | check a schema |
| POST ✱ | `…/schemas/subjects/{subject}/versions` | register |
| PUT ✱ | `…/schemas/subjects/{subject}/compatibility` | `{"level":"FULL"}`; `""` inherits |
| DELETE ✱ | `…/schemas/subjects/{subject}?permanent=true` | delete |
| GET | `…/analytics/overview?range=1h` | throughput, lag, top topics, groups |
| GET | `…/analytics/topics/{topic}` · `…/analytics/groups/{group}` | time series |
| GET | `…/analytics/skew` · `…/analytics/hygiene` | skew and hygiene |
| GET | `…/live` | SSE topology snapshots |

## Message stream

`GET …/topics/{topic}/messages/stream` accepts these query parameters:

| Parameter | Values |
|---|---|
| `mode` | `newest` (default), `oldest`, `offset`, `timestamp`, `live` |
| `limit` | 1–500 (default 100) |
| `partitions` | e.g. `0,2` (default: all) |
| `offset` | start offset, for `mode=offset` |
| `timestamp` | Unix milliseconds, for `mode=timestamp` |
| `q`, `qField`, `regex` | filter text; `any`, `key`, `value` or `header`; `true` for regex |
| `keyFormat`, `valueFormat` | `auto`, `string`, `json`, `base64`, `hex`, `schema` |

It emits these events:

- `message`: one record.
- `progress`: `{"scanned": n, "matched": n}`.
- `done`: `{"scanned", "matched", "reason"}`, where `reason` is `end`, `limit`, `scanLimit`, `timeout` or `cancelled`.
- `error`: something went wrong after the stream started.

Close the connection to cancel a scan.

## Error codes

| Code | Status | Meaning |
|---|---|---|
| `UNAUTHORIZED` / `INVALID_CREDENTIALS` / `RATE_LIMITED` | 401 / 401 / 429 | login and session |
| `FORBIDDEN` / `CSRF` / `CLUSTER_READ_ONLY` / `TOPIC_PROTECTED` | 403 | not allowed |
| `CLUSTER_NOT_FOUND` / `TOPIC_NOT_FOUND` / `GROUP_NOT_FOUND` / `MESSAGE_NOT_FOUND` / `SUBJECT_NOT_FOUND` | 404 | not found |
| `TOPIC_EXISTS` / `GROUP_NOT_EMPTY` / `SECURITY_DISABLED` / `SCHEMA_INCOMPATIBLE` | 409 | conflict |
| `BAD_REQUEST` / Kafka error names such as `INVALID_CONFIG` | 400 | invalid input or rejected by Kafka |
| `CLUSTER_OFFLINE` | 503 | cluster unreachable |
| `KAFKA_TIMEOUT` / `KAFKA_ERROR` / `SCHEMA_REGISTRY_ERROR` | 504 / 502 | upstream failure |
