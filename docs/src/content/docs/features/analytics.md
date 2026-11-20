---
title: Analytics
description: Throughput, lag, catch-up estimates, skew and hygiene checks.
---

![Analytics page](/kafka-ui/screenshots/analytics.png)

## How data is collected

A background collector samples every cluster every `collector.interval` (default 5s):

- topic offsets (for message rates)
- consumer group commits and lag
- under-replicated and offline partitions
- log sizes (every 30s)

Samples go into the SQLite database:

- Full-resolution data is kept for 6 hours.
- After that, data is kept at 1-minute resolution for `storage.retention` (default 7 days). Lag spikes survive the rollup.
- Put the database on a persistent volume to keep history across restarts.

Charts cover 15m, 1h, 6h, 24h or 7d, with about 240 points each.

## Dashboards

**Messages in / s** and **Bytes in / s.** Throughput over all non-internal topics. Bytes per second is estimated from log size growth, so retention deletes can lower it.

**Consumer lag.** Total lag across all groups over time.

**Groups by lag.** For each group, the table shows:

| Column | Meaning |
|---|---|
| Lag | current lag, in messages |
| Consume/s | the group's rate over the last 5 minutes |
| Ingress/s | how fast messages arrive on the group's topics |
| Catch up in | lag ÷ (consume − ingress); shows **falling behind** when it consumes no faster than messages arrive, and **not consuming** when idle |

**Topics.** Message rate, messages produced, size and growth over the selected range.

**Partition skew.**
- Leaders, replicas and ingress per broker.
- Partitions that take more than 1.5× their fair share of a topic's ingress, measured over the last minute.

**Hygiene.** Replication and housekeeping issues:

| Check | Why it matters |
|---|---|
| Replication factor 1 | losing one broker loses data |
| `min.insync.replicas` ≥ RF | one broker down blocks `acks=all` producers (or always, if greater) |
| `min.insync.replicas` = 1 with RF ≥ 3 | `acks=all` writes can be acknowledged by a single replica |
| Under-replicated partitions | replicas are behind |
| Idle topics | no new messages in the observed window |
| Empty groups | no members; their commits remain until they expire |

Topic pages have a **Throughput** tab, and group pages a **Lag trend** chart, built from the same data.
