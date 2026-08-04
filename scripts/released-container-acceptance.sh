#!/bin/sh
set -eu

version=${1:-}
mode=${MODARY_CONTAINER_ACCEPTANCE_MODE:-released}
case "$version" in
	v[0-9]*.[0-9]*.[0-9]*-*) ;;
	*) printf '%s\n' 'usage: released-container-acceptance.sh vX.Y.Z-prerelease' >&2; exit 2 ;;
esac
case "$mode" in
	released | source) ;;
	*) printf '%s\n' 'MODARY_CONTAINER_ACCEPTANCE_MODE must be released or source' >&2; exit 2 ;;
esac
for tool in awk curl docker git grep mkdir mktemp mv rm sed sleep tar; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'container acceptance requires %s\n' "$tool" >&2
		exit 2
	fi
done
test -n "${MODARY_TEST_DATABASE_URL:-}" || {
	printf '%s\n' 'MODARY_TEST_DATABASE_URL is required' >&2
	exit 2
}

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
go_command=${GO:-go}
temporary=$(mktemp -d "${TMPDIR:-/tmp}/modary-container.XXXXXX")
suffix=$$
containers=""
images=""

cleanup() {
	for container in $containers; do
		docker stop "$container" >/dev/null 2>&1 || true
		docker container rm "$container" >/dev/null 2>&1 || true
	done
	for image in $images; do
		docker image rm "$image" >/dev/null 2>&1 || true
	done
	rm -rf "$temporary"
}
trap cleanup EXIT HUP INT TERM

generate() {
	id=$1
	profile=$2
	(
		cd "$root"
		GOWORK=off "$go_command" run ./cmd/modary new "$temporary/$id" \
			--profile "$profile" --module "example.com/modary-release/$id" --name "$id"
	)
	if test "$mode" = source; then
		mkdir "$temporary/$id/_modary"
		tar -C "$root" --exclude=.git --exclude='*/node_modules' --exclude='*/dist' -cf - . |
			tar -C "$temporary/$id/_modary" -xf -
		awk '{ print; if ($0 == "COPY go.mod go.sum ./") print "COPY _modary ./_modary" }' \
			"$temporary/$id/Dockerfile" >"$temporary/$id/Dockerfile.source"
		mv "$temporary/$id/Dockerfile.source" "$temporary/$id/Dockerfile"
		if test -n "${MODARY_DOCKERFILE_FRONTEND:-}"; then
			sed "s|^# syntax=.*|# syntax=${MODARY_DOCKERFILE_FRONTEND}|" \
				"$temporary/$id/Dockerfile" >"$temporary/$id/Dockerfile.source"
			mv "$temporary/$id/Dockerfile.source" "$temporary/$id/Dockerfile"
		fi
		if test -n "${MODARY_GO_IMAGE_REPOSITORY:-}"; then
			sed "s|^FROM golang:|FROM ${MODARY_GO_IMAGE_REPOSITORY}:|" \
				"$temporary/$id/Dockerfile" >"$temporary/$id/Dockerfile.source"
			mv "$temporary/$id/Dockerfile.source" "$temporary/$id/Dockerfile"
		fi
		(
			cd "$temporary/$id"
			"$go_command" mod edit -replace=github.com/iiwish/modary=./_modary
			case "$profile" in
				admin)
					"$go_command" mod edit -replace=github.com/iiwish/modary/components/postgres=./_modary/components/postgres
					;;
				governed)
					"$go_command" mod edit -replace=github.com/iiwish/modary/components/postgres=./_modary/components/postgres
					"$go_command" mod edit -replace=github.com/iiwish/modary/components/governedpostgres=./_modary/components/governedpostgres
					;;
			esac
		)
	else
		grep -Fq "github.com/iiwish/modary $version" "$temporary/$id/go.mod"
	fi
	(
		cd "$temporary/$id"
		GOWORK=off "$go_command" mod tidy
	)
}

build_image() {
	id=$1
	image=modary-release-$id:$suffix
	attempt=1
	pull_flag=
	if test "$mode" = released; then
		pull_flag=--pull
	fi
	while ! docker build $pull_flag \
		--build-arg VERSION="$version" \
		--build-arg REVISION="$(git -C "$root" rev-parse HEAD)" \
		--build-arg CREATED="2026-08-04T00:00:00Z" \
		-t "$image" "$temporary/$id" >/dev/null; do
		if test "$attempt" -ge 3; then
			return 1
		fi
		attempt=$((attempt + 1))
		sleep 3
	done
	images="$images $image"
	user=$(docker image inspect "$image" --format '{{.Config.User}}')
	test "$user" = 65532:65532
	label=$(docker image inspect "$image" --format '{{index .Config.Labels "org.opencontainers.image.version"}}')
	test "$label" = "$version"
	built_image=$image
}

wait_ready() {
	url=$1
	container=$2
	attempt=0
	until curl -fsS "$url/readyz" >/dev/null; do
		attempt=$((attempt + 1))
		if test "$attempt" -ge 120; then
			docker logs "$container" >&2
			exit 1
		fi
		sleep 1
	done
	curl -fsS "$url/livez" >/dev/null
}

stop_cleanly() {
	container=$1
	docker stop --signal TERM --time 15 "$container" >/dev/null
	exit_code=$(docker container inspect "$container" --format '{{.State.ExitCode}}')
	test "$exit_code" = 0
}

assert_runtime_contents() {
	container=$1
	contents="$temporary/$container.contents"
	docker export "$container" | tar -tf - >"$contents"
	grep -Fxq 'application' "$contents"
	if grep -Eq '(^|/)(\.git|_modary|node_modules)(/|$)|(^|/)(go\.mod|go\.sum|package\.json|pnpm-lock\.yaml)$|\.go$|(^|/)node$' "$contents"; then
		printf 'runtime image contains build-only content:\n' >&2
		grep -E '(^|/)(\.git|_modary|node_modules)(/|$)|(^|/)(go\.mod|go\.sum|package\.json|pnpm-lock\.yaml)$|\.go$|(^|/)node$' "$contents" >&2
		exit 1
	fi
}

DATABASE_URL=$(printf '%s' "$MODARY_TEST_DATABASE_URL" | sed 's/127\.0\.0\.1/host.docker.internal/g; s/localhost/host.docker.internal/g')
export DATABASE_URL

generate container-api api
build_image container-api
api_image=$built_image
api_container=modary-release-api-$suffix
containers="$containers $api_container"
docker run -d --name "$api_container" --read-only --tmpfs /tmp --cap-drop ALL \
	--security-opt no-new-privileges --add-host host.docker.internal:host-gateway \
	-p 18081:8080 -e MODARY_HTTP_ADDR=0.0.0.0:8080 "$api_image" >/dev/null
wait_ready http://127.0.0.1:18081 "$api_container"
stop_cleanly "$api_container"
assert_runtime_contents "$api_container"
containers="$containers modary-release-node-$suffix"
if docker run --name modary-release-node-$suffix --entrypoint /usr/bin/node "$api_image" --version >/dev/null 2>&1; then
	printf '%s\n' 'runtime image unexpectedly contains Node.js' >&2
	exit 1
fi

generate container-admin admin
build_image container-admin
admin_image=$built_image
(
	cd "$temporary/container-admin"
	POSTGRES_PASSWORD=acceptance-password MODARY_ADMIN_PASSWORD=development-password docker compose config -q
)
MODARY_DATABASE_SCHEMA=modary_container_admin_$suffix
MODARY_ADMIN_USERNAME=admin
MODARY_ADMIN_PASSWORD=development-password
MODARY_ALLOW_INSECURE_COOKIE=true
export MODARY_DATABASE_SCHEMA MODARY_ADMIN_USERNAME MODARY_ADMIN_PASSWORD MODARY_ALLOW_INSECURE_COOKIE
admin_migrate=modary-release-admin-migrate-$suffix
containers="$containers $admin_migrate"
docker run --name "$admin_migrate" --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges \
	--add-host host.docker.internal:host-gateway -e DATABASE_URL -e MODARY_DATABASE_SCHEMA \
	"$admin_image" migrate >/dev/null
admin_container=modary-release-admin-$suffix
containers="$containers $admin_container"
docker run -d --name "$admin_container" --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges \
	--add-host host.docker.internal:host-gateway -p 18082:8080 -e DATABASE_URL -e MODARY_DATABASE_SCHEMA \
	-e MODARY_ADMIN_USERNAME -e MODARY_ADMIN_PASSWORD -e MODARY_ALLOW_INSECURE_COOKIE \
	-e MODARY_HTTP_ADDR=0.0.0.0:8080 "$admin_image" >/dev/null
wait_ready http://127.0.0.1:18082 "$admin_container"
curl -fsS http://127.0.0.1:18082/ | grep -Fq '<div id="root"></div>'
stop_cleanly "$admin_container"
assert_runtime_contents "$admin_container"

generate container-governed governed
build_image container-governed
governed_image=$built_image
(
	cd "$temporary/container-governed"
	POSTGRES_PASSWORD=acceptance-password MODARY_OPERATOR_PASSWORD=development-password \
		MODARY_OPERATOR_TOKEN=0123456789abcdef0123456789abcdef docker compose config -q
)
MODARY_APPLICATION_SCHEMA=modary_container_governed_$suffix
MODARY_QUEUE_SCHEMA=modary_container_queue_$suffix
MODARY_OPERATOR_USERNAME=operator
MODARY_OPERATOR_PASSWORD=development-password
MODARY_OPERATOR_TOKEN=0123456789abcdef0123456789abcdef
export MODARY_APPLICATION_SCHEMA MODARY_QUEUE_SCHEMA MODARY_OPERATOR_USERNAME MODARY_OPERATOR_PASSWORD MODARY_OPERATOR_TOKEN
governed_migrate=modary-release-governed-migrate-$suffix
containers="$containers $governed_migrate"
docker run --name "$governed_migrate" --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges \
	--add-host host.docker.internal:host-gateway -e DATABASE_URL -e MODARY_APPLICATION_SCHEMA -e MODARY_QUEUE_SCHEMA \
	"$governed_image" migrate >/dev/null
governed_container=modary-release-governed-$suffix
containers="$containers $governed_container"
docker run -d --name "$governed_container" --read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges \
	--add-host host.docker.internal:host-gateway -p 18083:8080 -e DATABASE_URL -e MODARY_APPLICATION_SCHEMA -e MODARY_QUEUE_SCHEMA \
	-e MODARY_OPERATOR_USERNAME -e MODARY_OPERATOR_PASSWORD -e MODARY_OPERATOR_TOKEN \
	-e MODARY_HTTP_ADDR=0.0.0.0:8080 "$governed_image" >/dev/null
wait_ready http://127.0.0.1:18083 "$governed_container"
stop_cleanly "$governed_container"
assert_runtime_contents "$governed_container"

printf '%s\n' "Container acceptance passed for $version in $mode mode."
