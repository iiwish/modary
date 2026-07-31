#!/bin/sh
set -u

status=0

# Inspect the complete current source tree, not only unstaged differences. This
# keeps the gate meaningful in a fresh CI checkout and after files are staged.
if ! git ls-files --cached --others --exclude-standard -z -- . | xargs -0 sh -c '
	for file do
		if test -L "$file" || ! test -f "$file"; then
			if git ls-files --error-unmatch -- "$file" >/dev/null 2>&1; then
				continue
			fi
			printf "unsupported untracked path type: %s\n" "$file" >&2
			exit 1
		fi
		case "$file" in
			.ai-platform/evidence/*.patch) continue ;;
		esac
		git diff --no-index --check -- /dev/null "$file"
		result=$?
		if test "$result" -gt 1; then
			exit 1
		fi
	done
' sh
then
	status=1
fi

# Git does not report untracked FIFOs, sockets, or devices. Enumerate only
# non-directory, non-regular nodes without following symlinks, then apply Git's
# tracked and ignore rules before rejecting them.
if ! find . -path ./.git -prune -o ! -type d ! -type f -print0 | xargs -0 sh -c '
	for pathname do
		file=${pathname#./}
		if git ls-files --error-unmatch -- "$file" >/dev/null 2>&1; then
			continue
		fi
		if git check-ignore -q -- "$file"; then
			continue
		fi
		printf "unsupported untracked path type: %s\n" "$file" >&2
		exit 1
	done
' sh
then
	status=1
fi

exit "$status"
