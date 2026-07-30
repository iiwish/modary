#!/usr/bin/env zsh
set -euo pipefail
zmodload zsh/datetime

image=${1:-modary-f0:arm64}
platform=${2:-linux/arm64}
port=${PORT:-18082}
name="modary-f0-bench-$$"
password=${MODARY_DEMO_PASSWORD:-benchmark-password}
agent_token=${MODARY_AGENT_TOKEN:-benchmark-agent-token}

cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

started=$EPOCHREALTIME
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
  print -u2 "readiness did not succeed"
  exit 1
fi

readiness_ms=$(( (EPOCHREALTIME - started) * 1000 ))
health=$(curl --silent --fail "http://127.0.0.1:$port/healthz")
process_startup_ms=$(jq -er '.startup_ms | numbers' <<<"$health")
if (( process_startup_ms > 2000 )); then
  print -u2 "process readiness ${process_startup_ms}ms exceeds the 2000ms budget"
  exit 1
fi
sleep 1
memory=$(docker stats --no-stream --format '{{.MemUsage}}' "$name")
limits=$(docker inspect --format 'nano_cpus={{.HostConfig.NanoCpus}} memory_bytes={{.HostConfig.Memory}}' "$name")
memory_used=${memory%% / *}
case "$memory_used" in
  *KiB) memory_mib=$(( ${memory_used%KiB} / 1024.0 )) ;;
  *MiB) memory_mib=${memory_used%MiB} ;;
  *GiB) memory_mib=$(( ${memory_used%GiB} * 1024.0 )) ;;
  *) print -u2 "unsupported Docker memory value: $memory_used"; exit 1 ;;
esac
if (( memory_mib > 128 )); then
  print -u2 "idle memory ${memory_mib}MiB exceeds the 128MiB budget"
  exit 1
fi

printf 'image=%s platform=%s process_startup_ms=%s external_probe_ms=%.1f memory=%s %s\n' "$image" "$platform" "$process_startup_ms" "$readiness_ms" "$memory" "$limits"
printf 'health=%s\n' "$health"
