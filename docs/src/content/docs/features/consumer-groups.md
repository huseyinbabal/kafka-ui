---
title: Consumer groups
description: Members, lag, lag trends, offset resets and group deletion.
---

## Lag

The group list shows state, members, topics and total lag. A group page shows:

- **Members:** client ID, host, and assigned partitions.
- **Partition lag:** per partition, the committed offset, end offset and lag, plus the member that owns it.
- **Lag trend:** total lag and consume rate over time, from 15 minutes to 7 days.

Lag only counts partitions that have a committed offset. This matches `kafka-consumer-groups.sh --describe`.

## Resetting offsets

Admins can move a group's committed offsets for one topic:

| Strategy | Moves to |
|---|---|
| Earliest | the start of each partition |
| Latest | the end of each partition (skip everything) |
| Specific offset | the given offset, kept within each partition's range |
| Timestamp | the first message at or after the time |
| Shift by | current ± N; partitions without a commit are skipped |

The safety rules:

1. **Preview first.** The dialog shows current and new offsets per partition. **Apply** only works after a preview with the same settings.
2. **Only idle groups.** Offsets can only change while the group has no active members. Stop the consumers first. A group with members is refused with `GROUP_NOT_EMPTY`, and the state is checked again right before committing.
3. **Audited.** Every applied reset is logged with the user.

## Deleting a group

Admins can delete a group that has no active members. This removes its committed offsets: if the consumers start again, they begin from their `auto.offset.reset` position. You confirm by typing the group name.
