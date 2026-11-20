---
title: Security
description: How kafka-ui protects your clusters and what to set up before exposing it.
---

## Before exposing kafka-ui

1. **Replace the example users.** Create hashes with `kafka-ui hash-password`.
2. **Serve over HTTPS** and keep `server.secureCookies: true` (the default).
3. **Keep credentials out of the config file.** Use `${ENV_VAR}` or `file:` references.
4. **Use `readOnly: true`** for clusters where nobody should change anything from the UI.
5. **Give kafka-ui's Kafka principal only the permissions it needs.** Read access is enough for a read-only deployment.

## What the server enforces

Every rule below is checked on the server. Hiding a button in the UI is never the only protection.

| Protection | Details |
|---|---|
| Authentication | Every `/api` route except login needs a session. |
| Roles | Changes need the `admin` role; `viewer` gets `403 FORBIDDEN`. |
| Read-only clusters | Changes get `403 CLUSTER_READ_ONLY`, whatever the role. |
| CSRF | Changes must send `X-Requested-With: kafka-ui`. Cookies are `SameSite=Strict`. |
| Login rate limit | 5 attempts per minute per client IP. |
| Protected topics | Topics starting with `__` can't be changed. |
| Offset resets | Only on groups without active members, with the state checked again just before committing. |
| Input validation | Topic names, partition counts, offsets, formats and regexes are checked. Regexes use RE2 (linear time). |
| Scan limits | Message scans stop at 500 results, 100,000 records or 60 seconds. |
| Secrets | Never logged, never returned by the API. |

## Audit log

Every successful change is logged at `INFO` with `msg=audit`. With `-log-format json`, a log line looks like this:

```json
{"level":"INFO","msg":"audit","action":"group.reset","user":"alice","cluster":"prod","group":"billing","topic":"invoices","strategy":"latest"}
```

The logged actions are:

| Area | Actions |
|---|---|
| Topics | `topic.create`, `topic.delete`, `topic.configs`, `topic.partitions`, `topic.purge` |
| Groups | `group.reset`, `group.delete` |
| ACLs | `acl.create`, `acl.delete` |
| Schemas | `schema.register`, `schema.compatibility`, `schema.delete` |

Produced messages are logged as `message produced`, with the user, topic, partition and offset.
