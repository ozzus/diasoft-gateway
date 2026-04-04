# Gateway Architecture

## Purpose

`diasoft-gateway` is the public-facing read service of the diploma verification platform.
It serves external verification traffic and maintains its own read-model projected from
`diasoft-registry` events.

This service is intentionally read-oriented:

- it does not own source-of-truth diploma data
- it does not process raw diploma imports
- it does not implement university backoffice workflows

## Runtime components

The repository builds three binaries.

### `cmd/diasoft-gateway`

Public HTTP service responsible for:

- `POST /api/v1/public/verify`
- `GET /api/v1/public/verify/{verificationToken}`
- `GET /api/v1/public/share-links/{shareToken}`
- `GET /v/{verificationToken}`
- `GET /s/{shareToken}`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

Dependencies:

- PostgreSQL for read-model queries and public verification audit
- Redis for verification cache and fixed-window rate limiting

### `cmd/diasoft-gateway-consumer`

Background worker responsible for:

- consuming Kafka lifecycle events
- projecting events into gateway PostgreSQL tables
- retrying transient projection failures before DLQ publication
- invalidating Redis verification cache
- publishing poison messages into DLQ
- exposing `/metrics`, `/healthz`, `/readyz` on a dedicated listener

Dependencies:

- Kafka for source events and DLQ publishing
- PostgreSQL for read-model projection state
- Redis for cache invalidation

### `cmd/diasoft-gateway-dlq-replayer`

Background worker responsible for:

- consuming `gateway.dlq.v1`
- decoding stored DLQ envelopes
- re-publishing `source_value` back into `source_topic`
- filtering replay by source topic, event type, failure stage, or event id
- supporting dry-run mode and bounded replay batches
- attaching `x-dlq-*` headers for replay traceability
- exposing `/metrics`, `/healthz`, `/readyz` on a dedicated listener

Dependencies:

- Kafka for DLQ consumption and source-topic production

## Layering

- `internal/domain`
  - diploma and share-link domain objects
- `internal/application/port`
  - repository and infrastructure contracts
- `internal/application/usecase`
  - verification, share-link, audit, and projection orchestration
- `internal/infrastructure/postgres`
  - DB pool bootstrap
- `internal/infrastructure/storage`
  - PostgreSQL stores
- `internal/infrastructure/redis`
  - Redis client, verification cache, rate limiter
- `internal/infrastructure/kafka`
  - Kafka consumer and DLQ writer
- `internal/transport/handler`
  - HTTP handlers for public flows
- `internal/transport/middleware`
  - rate limiting, trusted-proxy client IP extraction, and security headers
- `internal/observability/metrics`
  - Prometheus registry and instrumentation
- `internal/observability/tracing`
  - OpenTelemetry tracer provider bootstrap and HTTP instrumentation wrapper

## Data flow

### Verification path

1. Client calls `diasoft-gateway` by diploma number or verification token.
2. Gateway checks Redis verification cache.
3. On cache miss, gateway reads projected data from PostgreSQL.
4. Gateway returns masked verification result.
5. Gateway writes public verification audit into PostgreSQL.
6. Gateway stores the verification result in Redis cache.

Current live behavior note:

- this path is still a projected read-model lookup
- it does not validate a university digital signature yet
- it is the current pragmatic runtime implementation, not the final trust model

### Target verification path

The selected production-shaped verification model is:

- signed QR payload for authenticity
- online status lookup for current diploma state

Target steps:

1. Client scans QR or opens a verification link.
2. Gateway extracts diploma reference payload and signature metadata.
3. Gateway validates the signature against the university public key identified by `kid`.
4. Gateway performs one indexed status lookup by stable diploma identifier.
5. Gateway returns `valid`, `revoked`, `expired`, or `not_found`.

This avoids any verification strategy that resembles a full-table scan even at multi-million diploma scale.

### QR path

Current live QR path:

1. QR stores only `https://verify.<domain>/v/{verificationToken}`.
2. User scans QR from a mobile phone.
3. Browser opens the URL.
4. Gateway resolves the verification token and renders HTML.

Target QR path:

1. QR stores a signed diploma reference payload or a URL carrying that payload.
2. Payload identifies the diploma and signing key version.
3. Gateway verifies the signature before checking status.
4. Gateway resolves current status with one indexed lookup.

### Share-link path

1. Student share-link token is generated upstream in `diasoft-registry`.
2. User opens `GET /s/{shareToken}`.
3. Gateway resolves token state from PostgreSQL.
4. Gateway returns HTML result or expiration state.

### Projection path

1. `diasoft-registry` writes business events into outbox.
2. `registry-outbox-publisher` publishes events into Kafka.
3. `diasoft-gateway-consumer` consumes lifecycle events.
4. Consumer checks `processed_events` for idempotency.
5. Consumer upserts read-model rows in PostgreSQL.
6. Consumer invalidates Redis cache keys affected by the event.
7. On decode or projection failure, consumer publishes the original message into `gateway.dlq.v1`.

### DLQ replay path

1. Operator starts `diasoft-gateway-dlq-replayer`.
2. Replayer consumes messages from `gateway.dlq.v1`.
3. Replayer validates the DLQ envelope and extracts `source_topic`, `source_key`, and `source_value`.
4. Replayer republishes the original payload back into the source topic.
5. Replayer can filter, dry-run, or limit replay count based on runtime config.
6. Replayer commits the DLQ offset only after successful replay or explicit skip of malformed or filtered DLQ payload.

## Storage model

### PostgreSQL

Gateway owns these tables:

- `verification_records`
- `share_link_records`
- `verification_audit`
- `processed_events`

Responsibilities:

- read-model for public verification
- share-link resolution
- audit trail for public requests
- idempotent event processing marker

### Redis

Used for:

- verification response cache
- cache invalidation targets
- fixed-window public rate limiting

### Kafka

Expected topics:

- `diploma.lifecycle.v1`
- `sharelink.lifecycle.v1`
- `gateway.dlq.v1`

## Operational characteristics

### Health and readiness

- API exposes `GET /healthz` and `GET /readyz`
- consumer exposes `GET /healthz` and `GET /readyz` on metrics listener
- API readiness depends on PostgreSQL and Redis connectivity
- consumer readiness depends on Kafka, PostgreSQL, and Redis connectivity
- DLQ replayer readiness depends on Kafka connectivity

### Metrics

Prometheus metrics currently cover:

- verification cache hit/miss/error
- rate-limit allowed/blocked/error
- Kafka processed/processed_after_retry/retry/failed/dlq_published/dlq_failed
- Kafka replayed/replay_failed/replay_skipped/replay_filtered/replay_dry_run
- Kafka event age at projection time

### Tracing

OpenTelemetry tracing is implemented now:

- `cmd/diasoft-gateway` wraps the public HTTP handler with `otelhttp`
- health and metrics endpoints are excluded from HTTP tracing
- `cmd/diasoft-gateway-consumer` creates spans around Kafka record processing
- `cmd/diasoft-gateway-dlq-replayer` creates spans around DLQ replay decisions
- Kafka trace context is extracted from record headers and injected into DLQ and replayed source records
- API access logs include status code, bytes written, trace id, and span id
- tracing is enabled through runtime config and exports over OTLP/HTTP

### Reliability patterns

- PostgreSQL remains the authoritative store for gateway read state
- Redis is treated as disposable cache
- Kafka offsets are committed manually
- `processed_events` protects projection from duplicate handling
- projection failures can be retried with bounded exponential backoff before DLQ handoff
- DLQ protects the consumer from poison-message loops
- forwarded client IP headers are ignored unless the immediate peer is in `trusted_proxies`
- public JSON endpoints reject oversized bodies and unknown fields

### Integration test status

The repository now contains Docker-based integration tests for:

- PostgreSQL-backed verification and audit paths
- Redis cache and rate limiting
- Kafka consumer projection into PostgreSQL
- Kafka DLQ publishing on projection failures
- Kafka DLQ replay back into source topics

Execution still depends on a working Docker host in local or CI environment.

## Current gaps

The architecture is still missing these production hardening pieces:

- deployment policy for operators in `platform-infra`
- trace backend provisioning and environment-specific OTLP wiring in `platform-infra`
- dashboards and alert rules as code
- deployment/scaling manifests in `platform-infra`
- richer HTML templates for public pages

Operational replay steps are documented in `docs/dlq-replay.md`.

## Boundary with other repos

This repository should stay focused on:

- public verification
- read-model projection
- cache/rate limiting
- public audit

This repository should not absorb responsibilities from:

- `diasoft-registry`
  - source-of-truth writes
  - import processing
  - revoke workflow ownership
- `diasoft-web`
  - role-based frontend logic
- `platform-infra`
  - Terraform, Helm, ArgoCD, KEDA, cluster policy
