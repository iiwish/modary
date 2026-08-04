#!/bin/sh
set -eu

root=${1-}
if test -z "$root"; then
	root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
fi
root=$(CDPATH= cd -- "$root" && pwd -P)
cd "$root"

if test "$(git rev-parse --show-toplevel 2>/dev/null || true)" != "$root"; then
	printf 'acceptance digest root must be the canonical Git worktree root: %s\n' "$root" >&2
	exit 2
fi

paths=$(mktemp /tmp/modary-acceptance-paths.XXXXXX)
objects=$(mktemp /tmp/modary-acceptance-objects.XXXXXX)
symlinks=$(mktemp /tmp/modary-acceptance-symlinks.XXXXXX)
manifest=$(mktemp /tmp/modary-acceptance-manifest.XXXXXX)
trap 'rm -f "$paths" "$objects" "$symlinks" "$manifest"' EXIT HUP INT TERM

git -c core.quotePath=false ls-files --cached --others --exclude-standard | LC_ALL=C sort | while IFS= read -r path; do
	case "$path" in
		.ai-platform/evidence/T047/*) continue ;;
	esac
	if test -L "$path"; then
		printf '%s\n' "$path" >>"$symlinks"
	elif test -f "$path"; then
		printf '%s\n' "$path" >>"$paths"
	fi
done

git hash-object --stdin-paths <"$paths" >"$objects"
paste "$objects" "$paths" >"$manifest"
while IFS= read -r path; do
	if test -x "$path"; then
		printf 'executable\t%s\n' "$path" >>"$manifest"
	fi
done <"$paths"
while IFS= read -r path; do
	target=$(readlink "$path")
	object=$(printf '%s' "$target" | git hash-object --stdin)
	printf 'symlink\t%s\t%s\n' "$object" "$path" >>"$manifest"
done <"$symlinks"

printf 'git-hash:%s\n' "$(git hash-object --stdin <"$manifest")"
