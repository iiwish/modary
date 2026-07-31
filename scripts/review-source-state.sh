#!/bin/sh
set -eu

case "${1-}" in
	--all)
		exclude_t016=0
		set -- .
		;;
	--exclude-t016-evidence)
		exclude_t016=1
		set -- . ':(exclude).ai-platform/evidence/T016/**'
		;;
	*)
		printf 'usage: %s --all|--exclude-t016-evidence\n' "$0" >&2
		exit 2
		;;
esac

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$root"

# Source-state capture must not depend on a caller-selected TMPDIR, which may
# itself be inside the repository and would make the snapshot self-referential.
temporary=$(mktemp -d /tmp/modary-source-state.XXXXXX)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

index=$temporary/index
objects=$temporary/objects
mkdir -p "$objects"

git_objects=$(git rev-parse --git-path objects)
case "$git_objects" in
	/*) ;;
	*) git_objects=$root/$git_objects ;;
esac

export GIT_INDEX_FILE=$index
export GIT_OBJECT_DIRECTORY=$objects
export GIT_ALTERNATE_OBJECT_DIRECTORIES=$git_objects

check_special_nodes() {
	if test "$exclude_t016" -eq 1; then
		find . \
			-path './.git' -prune -o \
			-path './.ai-platform/evidence/T016' -prune -o \
			! -type d ! -type f ! -type l -print0
	else
		find . \
			-path './.git' -prune -o \
			! -type d ! -type f ! -type l -print0
	fi
}

# Git discovery omits untracked FIFOs and sockets. Reject every non-ignored
# special node before Git can silently leave it outside the source state.
check_special_nodes |
	xargs -0 sh -c '
		for file do
			if git check-ignore -q -- "$file"; then
				continue
			fi
			printf "review source contains unsupported node: %s\n" "$file" >&2
			exit 1
		done
	' sh

# Refuse tracked or discoverable nodes that Git cannot represent as one regular
# source file or symlink. NUL-delimited discovery preserves every valid path.
git ls-files -z --cached --others --exclude-standard -- "$@" |
	xargs -0 sh -c '
		for file do
			if test ! -e "$file" && test ! -L "$file"; then
				continue
			fi
			if test -f "$file" || test -L "$file"; then
				continue
			fi
			printf "review source contains unsupported node: %s\n" "$file" >&2
			exit 1
		done
	' sh

git read-tree HEAD
git \
	-c core.autocrlf=false \
	-c core.fileMode=true \
	-c core.safecrlf=false \
	add -A -- "$@"

empty_tree=$(printf '' | git hash-object -w -t tree --stdin)
git \
	-c core.quotePath=true \
	-c diff.algorithm=myers \
	--no-pager diff \
	--cached \
	--binary \
	--full-index \
	--no-color \
	--no-ext-diff \
	--no-renames \
	--no-textconv \
	"$empty_tree" -- "$@"
