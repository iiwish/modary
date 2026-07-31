#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$root"

if test "$#" -ne 0; then
	printf 'usage: %s\n' "$0" >&2
	exit 2
fi

printf '%s\t%s\n' 'head' "$(git rev-parse --verify HEAD)"
printf '%s\n' 'complete-source-snapshot'
sh ./scripts/review-source-state.sh --all
