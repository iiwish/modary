#!/bin/sh
set -eu

root=${1-}
if test -z "$root"; then
	root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
fi
if test ! -d "$root"; then
	printf 'documentation root is not a directory: %s\n' "$root" >&2
	exit 2
fi
cd "$root"

for tool in find rg sort dirname sed; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'documentation link check requires %s\n' "$tool" >&2
		exit 2
	fi
done

status=0
fail() {
	printf '%s\n' "$1" >&2
	status=1
}

required='README.md
CHANGELOG.md
CONTRIBUTING.md
SECURITY.md
docs/index.md
docs/getting-started/installation.md
docs/getting-started/first-application.md
docs/getting-started/quickstart.md
docs/getting-started/project-layout.md
docs/concepts/consumer-boundary.md
docs/concepts/modules-and-capabilities.md
docs/concepts/governed-actions.md
docs/how-to/add-module.md
docs/how-to/expose-action.md
docs/how-to/test-application.md
docs/how-to/troubleshooting.md
docs/reference/packages.md
docs/reference/support-matrix.md
docs/reference/project-manifest.md
docs/operations/deployment.md
docs/operations/security.md
docs/operations/sqlite-backup-restore.md
docs/releases/versioning.md
docs/releases/release-process.md
docs/releases/upgrade-guide.md'

for file in $required; do
	if test ! -f "$file" || test -L "$file" || test ! -s "$file"; then
		fail "required user document must be a non-empty regular file: $file"
	fi
done

markdown_files="README.md CHANGELOG.md CONTRIBUTING.md SECURITY.md"
if docs_files=$(find docs -type f -name '*.md' -print | sort); then
	markdown_files="$markdown_files $docs_files"
else
	fail 'cannot inventory user documentation'
	docs_files=
fi

for file in $markdown_files; do
	case "$file" in
		*[!A-Za-z0-9_./-]*)
			fail "documentation path contains unsupported characters: $file"
			continue
			;;
	esac
	if test ! -f "$file"; then
		continue
	fi
	if rg -q -F 'testdata/external-consumer' "$file"; then
		fail "retired public example path in user documentation: $file"
	fi
	if ! first_line=$(sed -n '1p' "$file"); then
		fail "cannot read user document: $file"
		continue
	fi
	case "$first_line" in
		'# '*) ;;
		*) fail "user document must start with one H1: $file" ;;
	esac

	link_status=0
	links=$(rg -o '\]\([^)]*\)' "$file" 2>/dev/null) || link_status=$?
	link_status=${link_status-0}
	if test "$link_status" -gt 1; then
		fail "cannot scan Markdown links in $file"
		continue
	fi
	if test -z "$links"; then
		continue
	fi
	while IFS= read -r record; do
		target=${record#](}
		target=${target%)}
		case "$target" in
			'' | \#* | http://* | https://* | mailto:* | app://*) continue ;;
			/*) continue ;;
		esac
		case "$target" in
			*' '*)
				fail "local Markdown links must not contain a title or unescaped space: $file -> $target"
				continue
				;;
		esac
		target=${target%%#*}
		target=${target%%\?*}
		test -n "$target" || continue
		base=$(dirname -- "$file")
		if test ! -e "$base/$target"; then
			fail "broken local Markdown link: $file -> $target"
		fi
	done <<EOF
$links
EOF
done

if test -f docs/index.md; then
	for file in $docs_files; do
		test "$file" = docs/index.md && continue
		target=${file#docs/}
		if ! rg -q -F "]($target)" docs/index.md; then
			fail "$file is not listed in docs/index.md"
		fi
	done
	for file in CHANGELOG.md CONTRIBUTING.md SECURITY.md; do
		if ! rg -q -F "](../$file)" docs/index.md; then
			fail "$file is not listed in docs/index.md"
		fi
	done
fi

exit "$status"
