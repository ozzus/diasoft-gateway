# diasoft-gateway

Public Go service for diploma verification, QR landing pages, public share-link resolution, Kafka-driven read-model projection, and the canonical platform Swagger file.

## Responsibilities

- public diploma verification API
- QR verification page rendering
- public share-link page rendering
- PostgreSQL read-model storage
- Redis cache and rate limiting
- Kafka consumer for upstream diploma and share-link events
- recovery worker for failed event processing
- shared OpenAPI document for gateway public routes and registry internal routes

## Binaries

- `cmd/diasoft-gateway` — HTTP API
- `cmd/diasoft-gateway-consumer` — Kafka projector
- `cmd/diasoft-gateway-dlq-replayer` — recovery worker for failed events

## Stack

- Go
- PostgreSQL
- Redis
- Kafka
- OpenTelemetry
- Prometheus

## Public API

- `POST /api/v1/public/verify`
- `GET /api/v1/public/verify/{verificationToken}`
- `GET /api/v1/public/share-links/{shareToken}`
- `GET /v/{verificationToken}`
- `GET /s/{shareToken}`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

## Swagger / OpenAPI

Canonical platform Swagger lives in [api/openapi/openapi.yaml](api/openapi/openapi.yaml).
It now documents both:

- gateway public API
- registry internal API

`diasoft-registry` and `diasoft-web` no longer keep separate local OpenAPI yaml files as the primary source of truth.

## Runtime notes

- API metrics are exposed on the main HTTP port.
- Consumer and recovery worker expose health and metrics on their metrics port.
- Gateway supports PostgreSQL-backed reads, Redis caching, rate limiting, and trusted-proxy-aware client IP resolution.
- Kafka failures are redirected to a dead-letter topic and can be reprocessed by the recovery worker.

## Run

```bash
go run ./cmd/diasoft-gateway
```

```bash
go run ./cmd/diasoft-gateway-consumer
```

```bash
go run ./cmd/diasoft-gateway-dlq-replayer
```

## Test

```bash
go test ./...
```

```bash
go test -tags=integration ./...
```

Integration tests require a working Docker daemon.
