KIND_CLUSTER ?= kafkatr
KCTX         ?= kind-$(KIND_CLUSTER)
IMAGE        ?= kafka-ui:dev

.PHONY: kind-traffic kind-sr kind-secure setup web-build build image test test-integration lint fmt web-dev run kind-deploy kind-forward kind-logs

setup:
	go mod download
	cd web && bun install

web-build:
	cd web && bun run build

build: web-build
	CGO_ENABLED=0 go build -o bin/kafka-ui ./cmd/kafka-ui

image:
	docker build -t $(IMAGE) .

test:
	go test -race ./...
	cd web && bun run test

test-integration:
	go test -race -tags=integration ./...

lint:
	go vet ./...
	golangci-lint run ./...
	cd web && bunx biome check . && bunx tsc -b

fmt:
	gofmt -w cmd internal web/embed.go
	cd web && bunx biome check --write .

# Frontend with HMR; proxies /api to the port-forwarded backend (make kind-forward).
web-dev:
	cd web && bun run dev

# Run the backend locally (only useful against a Kafka reachable from the host).
run: build
	./bin/kafka-ui -config config.yaml

kind-deploy: image
	kind load docker-image $(IMAGE) --name $(KIND_CLUSTER)
	kubectl --context $(KCTX) apply -f deploy/k8s/kafka-ui.yaml
	kubectl --context $(KCTX) -n kafka-ui rollout restart deploy/kafka-ui
	kubectl --context $(KCTX) -n kafka-ui rollout status deploy/kafka-ui --timeout=120s

# Dev Schema Registry (Confluent) backed by orders-cluster.
kind-sr:
	kubectl --context $(KCTX) apply -f deploy/kind/schema-registry.yaml
	kubectl --context $(KCTX) -n kafka-ui rollout status deploy/schema-registry --timeout=300s

# Demo traffic (e2e-traffic topic, ~20 msg/s, slow consumer group) for analytics.
kind-traffic:
	kubectl --context $(KCTX) apply -f deploy/kind/traffic.yaml

# Second dev cluster (SCRAM + simple authorizer) and the kafka-ui credentials for it.
kind-secure:
	kubectl --context $(KCTX) apply -f deploy/k8s/secure-cluster.yaml
	kubectl --context $(KCTX) -n kafka wait kafka/secure-cluster --for=condition=Ready --timeout=300s
	kubectl --context $(KCTX) -n kafka wait kafkauser/kafka-ui --for=condition=Ready --timeout=120s
	kubectl --context $(KCTX) -n kafka-ui create secret generic kafka-ui-secure-creds \
		--from-literal=password="$$(kubectl --context $(KCTX) -n kafka get secret kafka-ui -o jsonpath='{.data.password}' | base64 -d)" \
		--dry-run=client -o yaml | kubectl --context $(KCTX) apply -f -

kind-forward:
	kubectl --context $(KCTX) -n kafka-ui port-forward svc/kafka-ui 18080:8080

kind-logs:
	kubectl --context $(KCTX) -n kafka-ui logs -f deploy/kafka-ui
