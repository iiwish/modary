#!/bin/sh
set -eu

version=${1-}
root=${2-}
if test -z "$root"; then
	root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
fi

for tool in grep awk mktemp cp mkdir chmod rm cat; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'remote consumer gate requires %s\n' "$tool" >&2
		exit 2
	fi
done
if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$'; then
	printf 'remote consumer version must be one v-prefixed semantic prerelease: %s\n' "$version" >&2
	exit 2
fi
prerelease=${version#*-}
if ! printf '%s\n' "$prerelease" | awk -F. '{
	for (i = 1; i <= NF; i++) {
		if ($i ~ /^[0-9]+$/ && length($i) > 1 && substr($i, 1, 1) == "0") exit 1
	}
}'; then
	printf 'remote consumer version has a non-semantic numeric prerelease identifier: %s\n' "$version" >&2
	exit 2
fi

consumer=$root/examples/counter
if test ! -d "$consumer" || test -L "$consumer" ||
	test ! -f "$consumer/go.mod" || test -L "$consumer/go.mod"; then
	printf 'remote consumer source is invalid: %s\n' "$consumer" >&2
	exit 1
fi
GO=${GO-go}
temporary=$(mktemp -d /tmp/modary-remote-consumer.XXXXXX)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
no_node=$temporary/no-node
mkdir "$no_node"
for tool in node npm npx pnpm yarn corepack bun; do
	printf '#!/bin/sh\nprintf "unexpected Node-family tool invocation: %%s\\n" "$0" >&2\nexit 97\n' >"$no_node/$tool"
	chmod 0700 "$no_node/$tool"
done
PATH=$no_node:$PATH
export PATH
cp -R "$consumer" "$temporary/consumer"
cd "$temporary/consumer"

export GO111MODULE=on
export GOTOOLCHAIN=local
export GOENV=off
export GOWORK=off
GOFLAGS=
export GOFLAGS

for module_path in \
	github.com/iiwish/modary \
	github.com/iiwish/modary/components/postgres \
	github.com/iiwish/modary/components/governedpostgres \
	github.com/iiwish/modary/components/oidc \
	github.com/iiwish/modary/components/otel; do
	"$GO" mod edit -dropreplace="$module_path"
	"$GO" mod edit -require="$module_path@$version"
done
mkdir -p releaseprobe
cat >releaseprobe/releaseprobe.go <<'EOF'
package releaseprobe

import (
	_ "github.com/iiwish/modary/components/oidc"
	_ "github.com/iiwish/modary/components/otel"
)
EOF
"$GO" mod tidy

for module_path in \
	github.com/iiwish/modary \
	github.com/iiwish/modary/components/postgres \
	github.com/iiwish/modary/components/governedpostgres \
	github.com/iiwish/modary/components/oidc \
	github.com/iiwish/modary/components/otel; do
	resolved_version=$("$GO" list -m -f '{{.Version}}' "$module_path")
	if test "$resolved_version" != "$version"; then
		printf 'remote consumer resolved %s %s, want %s\n' "$module_path" "$resolved_version" "$version" >&2
		exit 1
	fi
	replacement=$("$GO" list -m -f '{{if .Replace}}{{.Replace.Path}}{{end}}' "$module_path")
	if test -n "$replacement"; then
		printf 'remote consumer resolved through a replacement: module=%s path=%s\n' "$module_path" "$replacement" >&2
		exit 1
	fi
done

"$GO" run ./tools/modary verify
"$GO" run ./tools/modary generate --check
"$GO" run ./tools/modary check
MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1 "$GO" test -count=1 ./...
"$GO" build ./...
"$GO" run ./cmd/counter-console version

printf 'remote consumer passed: modules=5 version=%s\n' "$version"
