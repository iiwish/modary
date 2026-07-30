#!/usr/bin/env zsh
set -euo pipefail

image=${1:-modary-f0:arm64}
platform=${2:-linux/arm64}
port=${PORT:-18084}
name="modary-f0-smoke-$$"
password=${MODARY_DEMO_PASSWORD:-smoke-password}
agent_token=${MODARY_AGENT_TOKEN:-smoke-agent-token}

cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker run --detach --name "$name" --platform "$platform" \
  --cpus 2 --memory 2g --pids-limit 128 \
  --publish "$port:8080" \
  --env MODARY_DEMO_PASSWORD="$password" \
  --env MODARY_AGENT_TOKEN="$agent_token" \
  "$image" >/dev/null

ready=false
for attempt in {1..200}; do
  if curl --silent --fail "http://127.0.0.1:$port/healthz" >/dev/null; then
    ready=true
    break
  fi
  sleep 0.01
done
if [[ "$ready" != true ]]; then
  docker logs "$name"
  print -u2 "release image did not become ready"
  exit 1
fi

MODARY_DEMO_PASSWORD="$password" ./scripts/f0-smoke.sh "http://127.0.0.1:$port"
