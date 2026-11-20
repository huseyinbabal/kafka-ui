---
title: Topics and messages
description: Browse, search, decode and produce messages; manage topics.
---

## Topics

The topic list shows partitions, replication factor, under-replicated partitions, message count and size. Internal topics are hidden unless you turn them on.

A topic page has these tabs:

- **Partitions:** leader, replicas (out-of-sync replicas highlighted), start and end offsets, size.
- **Consumers:** groups with commits on this topic, and their lag.
- **Throughput:** messages per second and size over time.
- **Configs:** all configs, with overrides highlighted. Admins can edit a value or reset it to the default.
- **Messages:** the message browser.
- **Schema:** shown when Schema Registry is configured.

Admins can also create topics, add partitions, purge all messages, and delete topics.

- Delete and purge ask you to type the topic name.
- Topics starting with `__` can't be changed.
- If Kafka rejects a change, for example an invalid config value, its own error message is shown.

## Message browser

![Message browser with an Avro message decoded](/kafka-ui/screenshots/message-browser.png)

| Option | Values |
|---|---|
| Seek | **Newest**, **Oldest**, **From offset**, **From time**, **Live tail** |
| Partitions | all, or any subset |
| Limit | 20 to 500 results |
| Filter | substring (case-insensitive) or regex, on key, value, headers or anywhere |
| Key / value format | Auto, String, JSON, Base64, Hex, Schema Registry |

- **Server-side scanning.** Results stream to the browser as they are found, with a running count of scanned and matched records. Press **Stop** at any time.
- **Limits.** A scan stops at the result limit, after 100,000 records, or after 60 seconds. Live tail runs for up to 15 minutes. The reason a scan stopped is shown.
- **Safe regex.** Patterns use Go's RE2 engine, which runs in linear time, so no pattern can hang the server.
- **Newest first.** "Newest" reads backwards from the end of each partition and shows the most recent messages first.
- **Message detail.** Click a row for the full key, value and headers, with copy buttons. JSON is pretty-printed without losing precision on large integers.

### Decoding

With **Auto**, each payload is detected as JSON, text or binary (shown as Base64). With Schema Registry configured, payloads in the Confluent wire format are decoded automatically:

- **Avro:** fields in schema order; unions are shown unwrapped.
- **Protobuf:** including nested messages and imports.
- **JSON Schema.**

If decoding fails, for example because a schema was deleted, the raw bytes are shown together with the reason.

## Producing messages

Admins can produce from the topic page.

- **Key:** optional; it can also be null.
- **Value** and **headers.**
- **Partition:** pick one, or leave it on auto. Auto hashes the key with the same algorithm as the Java client.
- **Formats:** String, JSON (validated), Base64, Hex, or **Schema Registry**.
  - With Schema Registry, you pick a subject. The default is `<topic>-value`.
  - You write the value as JSON. For Avro, union values may be bare (`"rush"`) or wrapped (`{"string": "rush"}`). For Protobuf, you can choose the message type.

Producing never creates a topic: an unknown topic returns `TOPIC_NOT_FOUND`.
