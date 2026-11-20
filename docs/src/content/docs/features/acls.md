---
title: ACLs
description: List, create and delete Kafka ACL bindings.
---

The ACL page lists every binding on the cluster: principal, permission, operation, resource type and name, pattern and host.

Admins can:

- **Add** a binding. Choose the principal (`User:alice`), host (`*` for any), resource type (topic, group, cluster, transactional ID), pattern (literal or prefixed), operation and permission (allow or deny).
- **Delete** a binding. This removes exactly that one binding.

If the cluster has no authorizer configured, the page says so instead of showing an error. Without an authorizer, Kafka allows every client to do everything.

:::note
kafka-ui needs permission to describe and alter ACLs. The simplest setup is to make kafka-ui's principal a super user on clusters where you want to manage ACLs.
:::

If an operator manages ACLs for you, deleting them in the UI may be undone. For example, Strimzi's User Operator manages the ACLs of every `KafkaUser` and restores them on its next reconcile.
