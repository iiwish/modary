#!/bin/sh
set -eu

version=${1-}
mode=${2-candidate}
root=${3-}
if test -z "$root"; then
	root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
fi

for tool in git grep awk sed; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'release preflight requires %s\n' "$tool" >&2
		exit 2
	fi
done

if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$'; then
	printf 'release version must be one v-prefixed semantic prerelease: %s\n' "$version" >&2
	exit 2
fi
prerelease=${version#*-}
if ! printf '%s\n' "$prerelease" | awk -F. '{
	for (i = 1; i <= NF; i++) {
		if ($i ~ /^[0-9]+$/ && length($i) > 1 && substr($i, 1, 1) == "0") exit 1
	}
}'; then
	printf 'release version has a non-semantic numeric prerelease identifier: %s\n' "$version" >&2
	exit 2
fi
case "$mode" in
	candidate | tag) ;;
	*)
		printf 'release mode must be candidate or tag: %s\n' "$mode" >&2
		exit 2
		;;
esac
if test ! -d "$root"; then
	printf 'release root is not a directory: %s\n' "$root" >&2
	exit 2
fi
cd "$root"

if test ! -f go.mod || test -L go.mod; then
	printf '%s\n' 'release requires one regular go.mod' >&2
	exit 1
fi
module_path=$(awk '$1 == "module" { count++; path=$2 } END { if (count != 1) exit 1; print path }' go.mod) || {
	printf '%s\n' 'release go.mod must contain exactly one module declaration' >&2
	exit 1
}
if test "$module_path" != github.com/iiwish/modary; then
	printf 'release module path is %s, want github.com/iiwish/modary\n' "$module_path" >&2
	exit 1
fi
go_version=$(awk '$1 == "go" { count++; version=$2 } END { if (count != 1) exit 1; print version }' go.mod) || {
	printf '%s\n' 'release go.mod must contain exactly one Go version directive' >&2
	exit 1
}
if test "$go_version" != 1.26.5; then
	printf 'release Go baseline is %s, want security-patched 1.26.5\n' "$go_version" >&2
	exit 1
fi

validate_component_module() {
	component_directory=$1
	component_path=$2
	component_mod=$component_directory/go.mod
	if test ! -f "$component_mod" || test -L "$component_mod"; then
		printf 'release requires regular component module %s\n' "$component_mod" >&2
		exit 1
	fi
	actual_component_path=$(awk '$1 == "module" { count++; path=$2 } END { if (count != 1) exit 1; print path }' "$component_mod") || {
		printf 'component go.mod must contain exactly one module declaration: %s\n' "$component_mod" >&2
		exit 1
	}
	if test "$actual_component_path" != "$component_path"; then
		printf 'component module path is %s, want %s\n' "$actual_component_path" "$component_path" >&2
		exit 1
	fi
	component_go_version=$(awk '$1 == "go" { count++; version=$2 } END { if (count != 1) exit 1; print version }' "$component_mod") || {
		printf 'component go.mod must contain exactly one Go version: %s\n' "$component_mod" >&2
		exit 1
	}
	if test "$component_go_version" != "$go_version"; then
		printf 'component %s Go baseline is %s, want %s\n' "$component_path" "$component_go_version" "$go_version" >&2
		exit 1
	fi
	if ! awk -v wanted="$version" '($1 == "github.com/iiwish/modary" && $2 == wanted) || ($1 == "require" && $2 == "github.com/iiwish/modary" && $3 == wanted) { count++ } END { exit count == 1 ? 0 : 1 }' "$component_mod"; then
		printf 'component %s must require github.com/iiwish/modary %s exactly once\n' "$component_path" "$version" >&2
		exit 1
	fi
}

validate_component_module components/postgres github.com/iiwish/modary/components/postgres
validate_component_module components/governedpostgres github.com/iiwish/modary/components/governedpostgres
validate_component_module components/oidc github.com/iiwish/modary/components/oidc
validate_component_module components/otel github.com/iiwish/modary/components/otel

license=
for candidate in LICENSE LICENSE.txt; do
	if test -e "$candidate" || test -L "$candidate"; then
		if test -n "$license"; then
			printf '%s\n' 'release has ambiguous owner-selected redistribution license files' >&2
			exit 1
		fi
		license=$candidate
	fi
done
if test -z "$license" || test ! -f "$license" || test -L "$license" || test ! -s "$license"; then
	printf '%s\n' 'release requires one non-empty owner-selected redistribution license' >&2
	exit 1
fi

if test ! -f SECURITY.md || test -L SECURITY.md; then
	printf '%s\n' 'release requires a regular SECURITY.md' >&2
	exit 1
fi
security_channels=$(sed -n 's/^- Private reporting channel: //p' SECURITY.md)
security_count=$(printf '%s\n' "$security_channels" | awk 'NF { count++ } END { print count+0 }')
case "$security_channels" in
	https://* | mailto:*) security_channel_valid=1 ;;
	*) security_channel_valid=0 ;;
esac
if test "$security_count" -ne 1 || test "$security_channel_valid" -ne 1; then
	printf '%s\n' 'release requires exactly one concrete private security reporting channel' >&2
	exit 1
fi

if test ! -f CHANGELOG.md || test -L CHANGELOG.md; then
	printf '%s\n' 'release requires a regular CHANGELOG.md' >&2
	exit 1
fi
release_heading_count=$(grep -Ec "^## $version - (Unreleased|[0-9]{4}-[0-9]{2}-[0-9]{2})$" CHANGELOG.md || true)
if test "$release_heading_count" -ne 1; then
	printf 'CHANGELOG.md must contain exactly one release heading for %s\n' "$version" >&2
	exit 1
fi
if test "$mode" = tag && grep -q -F "## $version - Unreleased" CHANGELOG.md; then
	printf 'tag release changelog entry is still Unreleased: %s\n' "$version" >&2
	exit 1
fi

if test ! -f docs/f0-acceptance-report.md ||
	! grep -q -F -- '- Status: Accepted' docs/f0-acceptance-report.md; then
	printf '%s\n' 'release requires accepted F0 technical evidence' >&2
	exit 1
fi
if ! grep -q -F '.ai-platform/specs/010-production-foundation/spec.md' docs/f0-acceptance-report.md ||
	! grep -q -F '.ai-platform/evidence/T047/' docs/f0-acceptance-report.md; then
	printf '%s\n' 'release acceptance report does not cover the current production foundation' >&2
	exit 1
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1 ||
	test "$(git rev-parse --show-toplevel)" != "$(pwd -P)"; then
	printf '%s\n' 'release root must be the canonical Git worktree root' >&2
	exit 1
fi
origin=$(git remote get-url origin 2>/dev/null || true)
case "$origin" in
	https://github.com/iiwish/modary | https://github.com/iiwish/modary.git | git@github.com:iiwish/modary | git@github.com:iiwish/modary.git | ssh://git@github.com/iiwish/modary | ssh://git@github.com/iiwish/modary.git) ;;
	*)
		printf 'release requires the canonical origin for github.com/iiwish/modary, found %s\n' "${origin:-none}" >&2
		exit 1
		;;
esac

if test -n "$(git status --porcelain --untracked-files=all)"; then
	printf '%s\n' 'release requires a clean committed worktree' >&2
	exit 1
fi
script_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
"$script_root/scripts/check-acceptance-evidence.sh" "$root"
head=$(git rev-parse --verify HEAD)

validate_release_tag() {
	release_tag=$1
	release_label=$2
	if git rev-parse --verify --quiet "refs/tags/$release_tag" >/dev/null; then
		tag_commit=$(git rev-list -n 1 "$release_tag")
		if test "$tag_commit" != "$head"; then
			printf '%s %s points to %s, not candidate %s\n' "$release_label" "$release_tag" "$tag_commit" "$head" >&2
			exit 1
		fi
	fi
	if test "$mode" = tag; then
		if ! git rev-parse --verify --quiet "refs/tags/$release_tag" >/dev/null ||
			test "$(git cat-file -t "refs/tags/$release_tag" 2>/dev/null || true)" != tag; then
			printf 'tag mode requires one annotated exact %s %s at HEAD\n' "$release_label" "$release_tag" >&2
			exit 1
		fi
	fi
}

validate_release_tag "$version" 'release tag'
validate_release_tag "components/postgres/$version" 'component release tag'
validate_release_tag "components/governedpostgres/$version" 'component release tag'
validate_release_tag "components/oidc/$version" 'component release tag'
validate_release_tag "components/otel/$version" 'component release tag'

printf 'release preflight passed: version=%s mode=%s commit=%s modules=5 license=%s origin=%s\n' \
	"$version" "$mode" "$head" "$license" "$origin"
