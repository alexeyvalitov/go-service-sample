#!/usr/bin/env bash
set -euo pipefail

# Smoke test for the production-like image target (distroless).
# What it verifies:
# - image builds
# - container starts
# - /healthz and /readyz are reachable from the host
# - SIGTERM triggers graceful shutdown (container exits)

IMAGE="${IMAGE:-go-service-sample:prod}"
NAME="${NAME:-go-service-sample-smoke-prod}"
PORT="${PORT:-8081}"

cleanup() {
  docker rm -f "${NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> build (${IMAGE})"
docker build --target runtime-prod -t "${IMAGE}" .

echo "==> run (${NAME})"
CID="$(docker run -d --name "${NAME}" -p "${PORT}:8081" "${IMAGE}")"
echo "container_id=${CID}"

echo "==> wait for /healthz"
deadline=$((SECONDS + 10))
until curl -fsS "http://localhost:${PORT}/healthz" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "healthz did not become ready in time"
    docker logs "${NAME}" || true
    exit 1
  fi
  sleep 0.1
done

echo "==> check /readyz (expect 200 or 503 depending on config)"
curl -s -o /dev/null -w "readyz_http=%{http_code}\n" "http://localhost:${PORT}/readyz"

echo "==> send SIGTERM"
docker kill --signal=SIGTERM "${NAME}" >/dev/null

echo "==> wait for exit"
deadline=$((SECONDS + 10))
while docker ps -q --no-trunc | grep -q "${CID}"; do
  if (( SECONDS >= deadline )); then
    echo "container did not exit in time"
    docker logs "${NAME}" || true
    exit 1
  fi
  sleep 0.1
done

echo "OK"


