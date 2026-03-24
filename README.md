# go-service-sample

`go-service-sample` is a small Go HTTP service built as a production-oriented reference project.
It intentionally keeps the business domain simple and focuses instead on service lifecycle concerns:
readiness vs liveness, graceful shutdown, bounded background workers, request-scoped logging, Docker packaging, and basic operational safety.

## What This Repository Demonstrates

- Thin `main`, explicit composition root in `App`
- `healthz` and `readyz` with optional external dependency checks
- Graceful shutdown with readiness flip, optional drain window, and bounded worker stop
- Request ID propagation, structured request logging, and panic recovery
- Small JSON API with input validation and consistent error responses
- Multi-stage Docker build with separate dev and distroless production targets
- Automated verification with Go tests and a production-image smoke test

## Endpoints

- `GET /ping`
- `GET /healthz`
- `GET /readyz`
- `GET /debug/sleep?d=300ms&flush=1`
- `POST /api/v1/echo`
- `GET /api/v1/users`
- `POST /api/v1/users`
- `GET /api/v1/users/{id}`
- `DELETE /api/v1/users/{id}`

## Run Locally

### Go

```bash
go run ./cmd/go-service-sample
```

### Docker Compose

```bash
cp .env.example .env
docker compose up --build
docker compose logs -f app
```

Quick checks:

```bash
curl -fsS http://localhost:8081/healthz; echo
curl -i http://localhost:8081/readyz
curl -i -X POST http://localhost:8081/api/v1/echo \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello"}'
```

## Verification

Run the test suite:

```bash
go test ./...
```

Run the production-image smoke test:

```bash
bash ./scripts/smoke_runtime_prod.sh
```

The smoke test builds the distroless runtime image, starts a container, verifies `healthz` and `readyz`, sends `SIGTERM`, and checks that the service exits cleanly.

## Configuration

Key environment variables:

- `HTTP_HOST`
- `HTTP_PORT` or `PORT`
- `LOG_LEVEL`
- `MAX_BODY_BYTES`
- `SHUTDOWN_TIMEOUT`
- `DRAIN_WINDOW`
- `WORKER_ENABLED`
- `WORKER_INTERVAL`
- `EXTERNAL_BASE_URL`
- `EXTERNAL_TIMEOUT`
- `EXTERNAL_SOFT`
- `READY_TIMEOUT`

Flags are also available and override environment variables.
For local Docker Compose usage, start from `.env.example`.

## Project Layout

- `cmd/go-service-sample`: process entrypoint
- `internal/app`: service wiring, lifecycle, shutdown, worker management
- `internal/config`: configuration loading and validation
- `internal/httpapi`: HTTP handlers, middleware, and JSON response helpers
- `internal/users`: user model and in-memory store
- `internal/logging`: small logger wrapper

## Design Notes

- `healthz` is a liveness signal only. It should stay cheap and local to the process.
- `readyz` reflects whether the instance is safe to receive new traffic.
- On shutdown, the service marks itself not ready first, optionally waits through a drain window, stops workers, and then shuts down the HTTP server.
- The user API intentionally uses an in-memory store to keep the example focused on service behavior rather than persistence.
- `docker-compose.yml` is for local development. The `runtime-prod` image is distroless and is meant to be probed by the deployment platform rather than via in-container tools.

## Repository Scope

This is a showcase backend service, not a complete product.
Its purpose is to present implementation choices and operational patterns clearly, with enough code and tests to be reviewed as an engineering sample.
