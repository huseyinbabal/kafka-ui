---
title: Schema Registry
description: Subjects, versions, compatibility and schema-aware messages.
---

kafka-ui works with any registry that speaks the Confluent Schema Registry API. Configure it per cluster:

```yaml
clusters:
  - id: prod
    bootstrap: ["broker-1:9092"]
    schemaRegistry:
      url: https://schema-registry:8081
      username: kafka-ui               # optional basic auth
      password: ${SR_PASSWORD}
```

## Subjects

- **List:** every subject with its type (Avro, Protobuf, JSON Schema), latest version and number of versions.
- **Subject page:**
  - **Versions:** pick any version, and **compare** it with another as a line diff.
  - **References:** links to the subjects this schema references.
  - **Compatibility:** set the level per subject, or inherit the global one.
- **Admin actions:**
  - **New version:** prefilled with the latest schema. **Check compatibility** shows the registry's reasons if the new schema is incompatible.
  - **Delete:** soft delete by default. Soft-deleted schema IDs stay readable, so existing messages still decode. Permanent delete is opt-in.

A topic's **Schema** tab shows its `<topic>-key` and `<topic>-value` subjects (TopicNameStrategy). Messages that use other naming strategies still decode by their schema ID.

## In the message browser

See [Topics and messages](/kafka-ui/features/topics-and-messages/#decoding):

- Payloads in Confluent wire format decode automatically.
- The producer can serialize JSON input with a subject's schema.

Schema pages work even when the Kafka cluster is offline, because the registry is a separate service.
