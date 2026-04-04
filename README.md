# diasoft-gateway

Go gateway for dual-mode diploma verification: public verify flow, JWT-based private API for the new frontend, Kafka-driven read-model projection, and the canonical platform Swagger file.

## Responsibilities

- public diploma verification API
- private JWT API for university/student/hr flows
- QR verification page rendering
- public share-link page rendering
- PostgreSQL read-model storage
- Redis cache and rate limiting
- Kafka consumer for upstream diploma and share-link events
- recovery worker for failed event processing
- BFF/facade over `diasoft-registry` internal gateway endpoints
- shared OpenAPI document for gateway public/private routes and registry backoffice routes

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

## Private API

- `POST /api/v1/auth/login`
- `GET /api/v1/auth/me`
- `GET /api/v1/university/diplomas`
- `POST /api/v1/university/diplomas/upload`
- `GET /api/v1/university/imports/{jobId}`
- `GET /api/v1/university/imports/{jobId}/errors`
- `POST /api/v1/university/diplomas/{id}/revoke`
- `GET /api/v1/university/diplomas/{id}/qr`
- `GET /api/v1/student/diploma`
- `POST /api/v1/student/share-link`
- `DELETE /api/v1/student/share-link/{token}`

Private API uses gateway-issued JWT tokens and proxies source-of-truth mutations/reads to `diasoft-registry` through `/internal/gateway/*` service-auth endpoints.

University upload accepts only `.csv` and `.xlsx` files. The endpoint is asynchronous: a successful call returns `202 Accepted` with an import job id, and row-level issues are exposed later through `GET /api/v1/university/imports/{jobId}` and `GET /api/v1/university/imports/{jobId}/errors`.

## Swagger / OpenAPI

Canonical platform Swagger lives in [api/openapi/openapi.yaml](api/openapi/openapi.yaml).
It documents:

- gateway public API
- gateway private API
- registry backoffice/internal API

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
