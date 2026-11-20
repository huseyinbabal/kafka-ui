---
title: Kubernetes
description: Install kafka-ui into a Kubernetes cluster with one manifest.
---

Every release publishes a `release.yaml` with the image pinned to that version. It creates:

- a `kafka-ui` namespace
- a PersistentVolumeClaim for the SQLite database (sessions and analytics)
- a Deployment: one replica, non-root, read-only root filesystem, health probes
- a ClusterIP Service on port 8080

## Install

**1. Apply the manifest.**

```sh
kubectl apply -f https://github.com/huseyinbabal/kafka-ui/releases/latest/download/release.yaml
```

To pin a version, use `releases/download/v0.1.0/release.yaml` instead.

**2. Add your configuration.** The pod waits until this Secret exists. Start from [`config.example.yaml`](https://github.com/huseyinbabal/kafka-ui/blob/main/config.example.yaml) and see [Configuration](/kafka-ui/getting-started/configuration/).

```sh
kubectl -n kafka-ui create secret generic kafka-ui-config --from-file=config.yaml
```

**3. Optionally, add credentials.** Passwords that `config.yaml` references as `${VAR}` can live in a second Secret. Each key becomes an environment variable in the pod:

```sh
kubectl -n kafka-ui create secret generic kafka-ui-env \
  --from-literal=PROD_KAFKA_PASSWORD='...' \
  --from-literal=SR_PASSWORD='...'
```

**4. Open it.**

```sh
kubectl -n kafka-ui rollout status deploy/kafka-ui
kubectl -n kafka-ui port-forward svc/kafka-ui 8080:8080
```

For shared use, put the `kafka-ui` Service behind your ingress with TLS. Keep `server.secureCookies` at its default (`true`).

## Upgrade

Apply the newer `release.yaml`. Your configuration is in Secrets that the manifest doesn't contain, so it is left untouched:

```sh
kubectl apply -f https://github.com/huseyinbabal/kafka-ui/releases/download/v0.2.0/release.yaml
```

## Change the configuration

The pod reads its config at startup:

```sh
kubectl -n kafka-ui create secret generic kafka-ui-config --from-file=config.yaml \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n kafka-ui rollout restart deploy/kafka-ui
```

## Uninstall

```sh
kubectl delete -f https://github.com/huseyinbabal/kafka-ui/releases/latest/download/release.yaml
```

This deletes the namespace, including the Secrets and the analytics database.

## Notes

- **One replica.** Sessions and analytics live in one SQLite file on a ReadWriteOnce volume. The Deployment uses the `Recreate` strategy, so two pods never write to it at the same time.
- **Network access.** kafka-ui must reach the addresses your brokers advertise, and the Schema Registry URL if you configure one. Inside the cluster, these are usually service DNS names, such as `my-cluster-kafka-bootstrap.kafka.svc:9092` for Strimzi.
- **Strimzi with SCRAM.**
  1. Create a `KafkaUser` with `scram-sha-512` authentication.
  2. Copy its generated password into `kafka-ui-env`:

     ```sh
     kubectl -n kafka-ui create secret generic kafka-ui-env --from-literal=KAFKA_PASSWORD="$(
       kubectl -n kafka get secret kafka-ui -o jsonpath='{.data.password}' | base64 -d)"
     ```

  3. Reference it in `config.yaml` as `password: ${KAFKA_PASSWORD}`.
- **Customizing.** The manifest is plain YAML. Download it and adjust resources, storage size or the namespace, or patch it with Kustomize.
