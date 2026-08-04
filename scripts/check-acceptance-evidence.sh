#!/bin/sh
set -eu

script_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
root=${1:-$script_root}
root=$(CDPATH= cd -- "$root" && pwd -P)
summary=$root/.ai-platform/evidence/T047/summary.md

if test ! -f "$summary" || test -L "$summary"; then
	printf 'current T047 acceptance summary must be a regular file: %s\n' "$summary" >&2
	exit 1
fi

prefix='- Source digest: '
count=$(grep -c "^$prefix" "$summary" || true)
if test "$count" -ne 1; then
	printf 'T047 summary must contain exactly one source digest line\n' >&2
	exit 1
fi
expected=$(sed -n "s/^$prefix//p" "$summary")
if ! printf '%s\n' "$expected" | grep -Eq '^git-hash:[0-9a-f]{40,64}$'; then
	printf 'T047 source digest has an invalid format: %s\n' "$expected" >&2
	exit 1
fi
actual=$($script_root/scripts/acceptance-source-digest.sh "$root")
if test "$actual" != "$expected"; then
	printf 'T047 acceptance evidence is stale: recorded %s, current %s\n' "$expected" "$actual" >&2
	exit 1
fi
