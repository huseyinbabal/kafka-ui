---
title: Configuration
description: Every option in config.yaml.
---

kafka-ui reads one YAML file, `config.yaml` by default. Pass another path with `-config` or `KAFKAUI_CONFIG`. The annotated [`config.example.yaml`](https://github.com/huseyinbabal/kafka-ui/blob/main/config.example.yaml) is kept valid by a test, so it is always a good starting point.

## Secrets

Any password field accepts three forms:

| Form | Example |
|---|---|
| inline | `password: s3cret` |
| environment variable | `password: ${KAFKA_PASSWORD}` |
| file | `password: file:/etc/kafka-ui/kafka-password` |

Secrets are never logged and never returned by the API. If a referenced variable or file is missing, that cluster is shown as offline with the reason, and the rest of the app keeps working.

## Server

```yaml
server:
  addr: ":8080"          # KAFKAUI_SERVER_ADDR
  secureCookies: true    # default; set false only for plain-HTTP setups
```

## Users

```yaml
auth:
  sessionIdleTimeout: 12h
  users:
    - username: alice
      passwordHash: "$2a$10$..."   # echo -n 'secret' | kafka-ui hash-password
      role: admin                  # admin | viewer
```

- `admin` can do everything.
- `viewer` is read-only.

See [Clusters and access](/kafka-ui/features/clusters-and-access/) for the details.

## Storage and sampling

```yaml
storage:
  path: kafka-ui.db       # KAFKAUI_STORAGE_PATH; relative paths land in /data in the Docker image
  retention: 168h         # analytics history (1-minute resolution)
collector:
  interval: 5s            # analytics sampling
  liveInterval: 2s        # live topology updates
```

## Clusters

```yaml
clusters:
  - id: prod                       # [a-z0-9-], appears in URLs
    name: Production
    bootstrap: ["broker-1:9093", "broker-2:9093"]
    readOnly: true                 # refuse every change on this cluster
    sasl:
      mechanism: SCRAM-SHA-512     # PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512
      username: kafka-ui
      password: ${PROD_KAFKA_PASSWORD}
    tls:
      enabled: true
      caFile: /etc/kafka-ui/ca.pem
      certFile: /etc/kafka-ui/client.pem       # optional, for mTLS
      keyFile: /etc/kafka-ui/client-key.pem
      insecureSkipVerify: false
    schemaRegistry:
      url: https://schema-registry:8081
      username: kafka-ui                       # optional basic auth
      password: file:/etc/kafka-ui/sr-password
```

You can list the same Kafka cluster twice, once with `readOnly: true`. This is useful when you want a "safe" entry for day-to-day browsing.

## Environment variables

| Variable | Overrides |
|---|---|
| `KAFKAUI_CONFIG` | config file path (`-config`) |
| `KAFKAUI_SERVER_ADDR` | `server.addr` |
| `KAFKAUI_STORAGE_PATH` | `storage.path` |
| `KAFKAUI_LOG_FORMAT` | `text` (default) or `json` (`-log-format`) |
