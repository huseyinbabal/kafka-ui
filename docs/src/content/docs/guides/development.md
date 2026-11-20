---
title: Development
description: Build, test and contribute to kafka-ui.
---

## Setup

You need Go 1.27+, Bun 1.3+, and Docker (for integration tests).

```sh
make setup        # Go modules and web dependencies
make run          # build and start with ./config.yaml
make web-dev      # Vite dev server with hot reload on :5173, proxying /api (KAFKAUI_API)
```

The backend is in `internal/`. The React app is in `web/`, and it is embedded into the binary at build time.

## Tests

| Command | What it runs |
|---|---|
| `make lint` | golangci-lint, Biome, TypeScript |
| `make test` | Go unit tests (race detector), Vitest |
| `make test-integration` | Go tests against a real Kafka, an ACL-enabled Kafka and Schema Registry in Docker (testcontainers) |
| `make coverage` | unit and integration coverage, merged per package |
| `make e2e` | Playwright against a running instance (`KAFKAUI_E2E_URL`, default `http://127.0.0.1:18080`) |

E2E tests only create and change resources whose names start with `e2e-`.

## A full local environment with kind

The repository has manifests for a local [kind](https://kind.sigs.k8s.io) cluster running Strimzi. Set `KIND_CLUSTER` to your cluster's name.

```sh
make kind-deploy    # build the image, load it into kind, deploy deploy/k8s/kafka-ui.yaml
make kind-forward   # http://127.0.0.1:18080
make kind-sr        # Confluent Schema Registry
make kind-secure    # a second cluster with SCRAM authentication and ACLs
make kind-traffic   # a demo producer, plus a fast and a slow consumer group
make demo           # re-record the demo GIF (needs traffic)
```

## Documentation

This site lives in `docs/`, built with [Starlight](https://starlight.astro.build):

```sh
cd docs && bun install && bun run dev
```

It is published to GitHub Pages on every push to `main`.

## CI and releases

GitHub Actions run these workflows:

| Workflow | When | What |
|---|---|---|
| CI | every pull request and push to `main` | Go lint and unit tests, integration tests, web lint/type check/tests/build, docs build, and a Docker build check |
| Release image | a GitHub release is published | a multi-arch image (`linux/amd64`, `linux/arm64`) pushed to `ghcr.io/huseyinbabal/kafka-ui` |
| Docs | push to `main` that changes `docs/` | build this site and publish it to GitHub Pages |

To release, publish a GitHub release with a tag like `v1.2.0`. The image is then tagged:

- `1.2.0`
- `1.2`
- `1` (not for `0.x` releases)
- `latest` (not for prereleases)

`kafka-ui version` prints the release tag.

```sh
docker pull ghcr.io/huseyinbabal/kafka-ui:latest
```

## Contributing

[`SPEC.md`](https://github.com/huseyinbabal/kafka-ui/blob/main/SPEC.md) describes the behavior, design rules and contribution guidelines. In short:

- Run `make lint test` before opening a pull request, and `make test-integration` when you change code that talks to Kafka.
- Enforce validation, roles and read-only checks on the server.
- Open an issue first for new dependencies, API or config changes, and anything that relaxes a safety rule.
