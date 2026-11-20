---
title: Kubernetes
description: Deploy kafka-ui next to your Kafka cluster.
---

[`deploy/k8s/kafka-ui.yaml`](https://github.com/huseyinbabal/kafka-ui/blob/main/deploy/k8s/kafka-ui.yaml) is a complete example deployment. It contains:

- a `kafka-ui` namespace
- a Secret holding `config.yaml`
- a PersistentVolumeClaim for the SQLite database (sessions and analytics)
- a Deployment: one replica, `Recreate` strategy, non-root user, read-only root filesystem, readiness and liveness probes on `/healthz`
- a ClusterIP Service on port 8080

## Steps

1. **Pick an image.** Use a release image, `ghcr.io/huseyinbabal/kafka-ui:<version>`, or build and push your own:

   ```sh
   docker build -t registry.example.com/kafka-ui:1.0 .
   docker push registry.example.com/kafka-ui:1.0
   ```

2. **Edit the manifest:**
   - Set `image:` to your image.
   - Replace the example users and clusters in the `kafka-ui-config` Secret.
   - Keep credentials out of the config. Reference them as `${ENV_VAR}` and add those variables from another Secret:

   ```yaml
   env:
     - name: PROD_KAFKA_PASSWORD
       valueFrom:
         secretKeyRef: { name: kafka-ui-credentials, key: kafka-password }
   ```

3. **Apply it** and put the Service behind your ingress with TLS:

   ```sh
   kubectl apply -f deploy/k8s/kafka-ui.yaml
   kubectl -n kafka-ui rollout status deploy/kafka-ui
   ```

Behind HTTPS, keep `server.secureCookies` at its default (`true`).

## Notes

- **One replica.** Sessions and analytics live in one SQLite file on the PersistentVolumeClaim. The Deployment uses the `Recreate` strategy, so two pods never write to it at the same time.
- **Network access.** kafka-ui must reach the addresses your brokers advertise, and the Schema Registry URL if you configure one.
- **Strimzi with SCRAM.** Create a `KafkaUser` with `scram-sha-512` authentication. Copy its generated password into a Secret in kafka-ui's namespace, then reference it as an environment variable as shown above. The repository's [`deploy/k8s/secure-cluster.yaml`](https://github.com/huseyinbabal/kafka-ui/blob/main/deploy/k8s/secure-cluster.yaml) and the `make kind-secure` target show the whole flow.
