# syntax=docker/dockerfile:1
FROM oven/bun:1.3 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kafka-ui ./cmd/kafka-ui && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kafka-ui /kafka-ui
# Owned by the nonroot user so a named volume mounted here is writable. As the
# working directory, it also receives the default relative storage path (kafka-ui.db).
COPY --from=build --chown=65532:65532 /out/data /data
WORKDIR /data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/kafka-ui"]
