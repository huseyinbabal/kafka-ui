---
title: Quick start
description: Run kafka-ui with Docker or from source in a few minutes.
---

kafka-ui is a single binary with the web app embedded. All it needs is a config file that lists your clusters, and a writable place for its SQLite database.

## With Docker

Images for `linux/amd64` and `linux/arm64` are published to GitHub Container Registry with every release.

```sh
curl -fsSLo config.yaml https://raw.githubusercontent.com/huseyinbabal/kafka-ui/main/config.example.yaml
# edit config.yaml: point `clusters` at your brokers
docker run --rm -p 8080:8080 \
  -v "$PWD/config.yaml:/etc/kafka-ui/config.yaml:ro" \
  -v kafka-ui-data:/data \
  ghcr.io/huseyinbabal/kafka-ui:latest -config /etc/kafka-ui/config.yaml
```

To build the image yourself, run `docker build -t kafka-ui .` in a clone of the repository.

The image runs as a non-root user. Sessions and analytics are written to the `/data` volume, so keep that volume if you want analytics history to survive restarts.

## From source

You need Go 1.27+ and [Bun](https://bun.sh) 1.3+.

```sh
git clone https://github.com/huseyinbabal/kafka-ui && cd kafka-ui
make setup                          # Go modules and web dependencies
cp config.example.yaml config.yaml
make run                            # builds the UI and the binary, then starts on :8080
```

`make build` produces `bin/kafka-ui`. Copy that file anywhere: the web app is embedded in it.

## First login

Open http://localhost:8080. The example config ships two users:

| User | Password | Role |
|---|---|---|
| `admin` | `admin` | everything |
| `viewer` | `viewer` | read-only |

:::caution
Replace these users before exposing the UI anywhere. Create a password hash with:

```sh
echo -n 'my-secret' | ./bin/kafka-ui hash-password
```
:::

## Point it at your cluster

The smallest useful config:

```yaml
auth:
  users:
    - username: admin
      passwordHash: "$2a$10$..."    # from hash-password
      role: admin
clusters:
  - id: local
    name: Local
    bootstrap: ["localhost:9092"]
```

kafka-ui connects to the addresses your brokers **advertise**. If the brokers advertise in-cluster names, such as Kubernetes service DNS, run kafka-ui where those names resolve, for example in the same cluster. See [Kubernetes](/kafka-ui/getting-started/kubernetes/).

Next: [Configuration](/kafka-ui/getting-started/configuration/) covers SASL, TLS, Schema Registry and read-only clusters.
