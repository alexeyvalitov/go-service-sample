# syntax=docker/dockerfile:1

# One Dockerfile, two runtime targets:
# - runtime-dev  (Alpine, demo-friendly, optional in-image HEALTHCHECK)
# - runtime-prod (distroless, minimal surface, probes done by the platform)
#
# Version pinning:
# Bad:  FROM golang:latest
# Meh:  FROM golang:1.25               (moving tag: changes over time)
# Good: FROM golang:1.25.7-<variant>   (repeatable, easy to bump intentionally)
#
# Pro tip (prod): pin base images by digest in CI (FROM image@sha256:...) for byte-for-byte rebuild stability.
ARG GO_VERSION=1.25.7

# BuildKit provides automatic build args like $BUILDPLATFORM (where the build runs).
# Using it lets you do multi-arch builds cleanly (e.g. build on amd64 for linux/arm64).
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

# Make a static binary (portable across distros).
# If you later need CGO, switch CGO_ENABLED=1 and use a glibc runtime base.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
  -trimpath \
  -buildvcs=false \
  -ldflags="-s -w" \
  -o /out/go-service-sample ./cmd/go-service-sample

FROM alpine:3.20 AS runtime-dev

# Runtime: keep it small.
# - We keep CA certs because the app may do outbound HTTPS (e.g. external /readyz deps).
# - We intentionally do NOT install curl: smaller image + smaller attack surface.
RUN apk add --no-cache ca-certificates \
  && addgroup -S app \
  && adduser -S -G app app

WORKDIR /app
COPY --from=build /out/go-service-sample /app/go-service-sample

USER app

EXPOSE 8081

# Defaults: can be overridden by env/flags.
ENV HTTP_PORT=8081
ENV LOG_LEVEL=info

# Healthcheck is intentionally liveness-only (no external deps).
HEALTHCHECK --interval=5s --timeout=2s --start-period=2s --retries=10 \
  CMD sh -c 'wget -q -T 1 -O - "http://127.0.0.1:${HTTP_PORT:-8081}/healthz" >/dev/null || exit 1'

ENTRYPOINT ["/app/go-service-sample"]


# Production runtime: minimal surface (no shell/curl/apk). Prefer platform probes (Kubernetes/Compose).
FROM gcr.io/distroless/static-debian12:nonroot AS runtime-prod

WORKDIR /app
COPY --from=build /out/go-service-sample /app/go-service-sample

EXPOSE 8081

ENV HTTP_PORT=8081
ENV LOG_LEVEL=info

ENTRYPOINT ["/app/go-service-sample"]


