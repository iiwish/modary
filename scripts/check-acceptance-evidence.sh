#!/bin/sh
set -eu

script_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
root=${1:-$script_root}
root=$(CDPATH= cd -- "$root" && pwd -P)
summary=$root/.ai-platform/evidence/T047/summary.md
release_summary=$root/.ai-platform/evidence/T048/summary.md
temporary=

cleanup() {
	test -z "$temporary" || rm -rf "$temporary"
}
trap cleanup EXIT HUP INT TERM

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
if test "$actual" = "$expected"; then
	exit 0
fi

# A completed release record is intentionally outside the accepted candidate.
# Recheck that immutable tree only when both its annotated tag and commit agree.
if test -f "$release_summary" && test ! -L "$release_summary" &&
	test "$(grep -c '^- Status: Completed$' "$release_summary" || true)" -eq 1; then
	candidate=$(sed -n 's/^- Candidate commit: `\([0-9a-f][0-9a-f]*\)`$/\1/p' "$release_summary")
	candidate_tag=$(sed -n 's/^- Candidate tag: `\(v[^`]*\)`$/\1/p' "$release_summary")
	if ! printf '%s\n' "$candidate" | grep -Eq '^[0-9a-f]{40}([0-9a-f]{24})?$'; then
		candidate=
	fi
	if ! printf '%s\n' "$candidate_tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$'; then
		candidate_tag=
	fi
	if test -n "$candidate" && test -n "$candidate_tag"; then
		tag_type=$(git -C "$root" cat-file -t "refs/tags/$candidate_tag" 2>/dev/null || true)
		tagged_candidate=$(git -C "$root" rev-parse -q --verify "refs/tags/$candidate_tag^{commit}" 2>/dev/null || true)
		if test "$tag_type" = tag && test "$tagged_candidate" = "$candidate"; then
			temporary=$(mktemp -d /tmp/modary-accepted-tree.XXXXXX)
			git -C "$root" archive "$candidate" | tar -xf - -C "$temporary"
			git -C "$temporary" init --quiet
			git -C "$temporary" add --all
			candidate_actual=$($script_root/scripts/acceptance-source-digest.sh "$temporary")
			if test "$candidate_actual" = "$expected"; then
				exit 0
			fi
		fi
	fi
fi

printf 'T047 acceptance evidence is stale: recorded %s, current %s\n' "$expected" "$actual" >&2
exit 1
