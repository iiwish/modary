#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
go_command=${GO:-go}
dex_image=${MODARY_DEX_IMAGE:-ghcr.io/dexidp/dex:v2.45.1}
collector_image=${MODARY_OTEL_COLLECTOR_IMAGE:-otel/opentelemetry-collector:0.157.0}
dex_name=modary-dex-$$
collector_name=modary-otel-$$

cleanup() {
	docker stop "$dex_name" "$collector_name" >/dev/null 2>&1 || true
	docker container rm "$dex_name" "$collector_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

docker run -d --name "$dex_name" -p 5556:5556 \
	-v "$root/scripts/fixtures/dex/config.yaml:/etc/dex/config.yaml:ro" \
	"$dex_image" dex serve /etc/dex/config.yaml >/dev/null
docker run -d --name "$collector_name" -p 4318:4318 \
	-v "$root/scripts/fixtures/otel-collector/config.yaml:/etc/otelcol/config.yaml:ro" \
	"$collector_image" --config=/etc/otelcol/config.yaml >/dev/null

attempt=0
until curl -fsS http://127.0.0.1:5556/dex/.well-known/openid-configuration >/dev/null; do
	attempt=$((attempt + 1))
	if test "$attempt" -ge 60; then
		docker logs "$dex_name" >&2
		exit 1
	fi
	sleep 1
done
attempt=0
until curl -sS http://127.0.0.1:4318/ >/dev/null; do
	attempt=$((attempt + 1))
	if test "$attempt" -ge 60; then
		docker logs "$collector_name" >&2
		exit 1
	fi
	sleep 1
done

(
	cd "$root/components/oidc"
	MODARY_TEST_DEX_ISSUER_URL=http://127.0.0.1:5556/dex \
		GOWORK=off "$go_command" test -count=1 -race -run '^TestDisposableDexAuthorizationCodeFlow$' ./...
)
(
	cd "$root/components/otel"
	MODARY_TEST_OTEL_ENDPOINT=http://127.0.0.1:4318 \
		GOWORK=off "$go_command" test -count=1 -race -run '^TestDisposableOTLPCollectorExport$' ./...
)

collector_logs=$(docker logs "$collector_name" 2>&1)
printf '%s\n' "$collector_logs" | grep -Fq 'modary-collector-acceptance'
printf '%s\n' "$collector_logs" | grep -Fq '/collector-acceptance/{id}'
printf '%s\n' "$collector_logs" | grep -Fq 'task.handle'
if printf '%s\n' "$collector_logs" | grep -Eq 'private-value|token=private'; then
	printf '%s\n' 'Collector output contains a forbidden raw path or query value' >&2
	exit 1
fi

printf '%s\n' 'Disposable Dex and OTLP Collector acceptance passed.'
