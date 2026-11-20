---
title: Clusters and access
description: Multiple clusters, health, read-only mode, users and roles.
---

## Multiple clusters

Every cluster in the config appears in the switcher at the top of the sidebar. Every URL is scoped to one cluster: `/clusters/{id}/...`.

- **Health:** kafka-ui checks every cluster every 15 seconds. An unreachable cluster shows **offline** with the last error. Requests to it fail immediately with `503 CLUSTER_OFFLINE` instead of waiting for Kafka timeouts. The other clusters are not affected.
- **Broken config:** a cluster whose client can't be created, for example because of a missing credential, also shows as offline with the reason.
- **Overview page:** cluster ID, Kafka version, active controller, topic and partition counts, under-replicated and offline partitions, size per broker.
- **Controller:** on KRaft clusters, the controller shown is the real quorum leader.

## Read-only clusters

`readOnly: true` makes the server reject every change on that cluster with `403 CLUSTER_READ_ONLY`, for every user. The UI hides the actions too, but the server check is what counts.

## Users and roles

| Role | Can |
|---|---|
| `admin` | everything: produce, create/delete/edit topics, reset offsets, delete groups, manage ACLs and schemas |
| `viewer` | read everything; no changes |

Roles apply to all clusters. Combine a `viewer` user with read-only clusters for extra safety.

## Sessions

- Login creates a server-side session. The browser only holds a random token in an `HttpOnly`, `SameSite=Strict` cookie, which is also `Secure` unless you disable it.
- Sessions expire after `auth.sessionIdleTimeout` of inactivity (default 12h).
- Logging out deletes the session on the server.

See [Security](/kafka-ui/guides/security/) for CSRF protection, rate limiting and the audit log.
