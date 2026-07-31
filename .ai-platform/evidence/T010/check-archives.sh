#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
archive_root=${MODARY_ARCHIVE_ROOT:-/Users/iiwish/self/rulary/prototype}
source_dir=${MODARY_SOURCE_ARCHIVE_DIR:-$archive_root/modary-integrated-f0}
source_manifest=${MODARY_SOURCE_ARCHIVE_MANIFEST:-$archive_root/modary-integrated-f0.sha256}
governance_dir=${MODARY_GOVERNANCE_ARCHIVE_DIR:-$archive_root/modary-integrated-f0-governance}
governance_manifest=${MODARY_GOVERNANCE_ARCHIVE_MANIFEST:-$archive_root/modary-integrated-f0-governance.sha256}

source_inventory=$root/.ai-platform/evidence/T010/source-archive.inventory
governance_inventory=$root/.ai-platform/evidence/T010/governance-archive.inventory

source_manifest_digest=143d934c29693fe799f230fd04c1647b1f9ff41ac8b292fd7ca965f7e83d43fa
governance_manifest_digest=68593d9ad930e4e5c12fef8d6fb0abe55ac11e10a423563d43c96233fd586745
source_inventory_digest=119c2280023a59bcb2ac548ed9fba4ffcad0860a13991bea6bca228a1a6e9c38
governance_inventory_digest=fa15a300998571ba703f04e49294339b86212af2caf03ff183153293ad0086b0

fail() {
	printf 'archive check failed: %s\n' "$1" >&2
	exit 1
}

require_file() {
	test -f "$1" || fail "missing file: $1"
}

digest() {
	shasum -a 256 "$1" | cut -d ' ' -f 1
}

file_mode() {
	mode=$(stat -f '%Lp' "$1" 2>/dev/null || printf '')
	case "$mode" in
		'' | *[!0-7]*) mode=$(stat -c '%a' "$1" 2>/dev/null || printf '') ;;
	esac
	case "$mode" in
		'' | *[!0-7]*) fail "cannot determine file mode: $1" ;;
	esac
	printf '%s\n' "$mode"
}

verify_inventory() {
	base=$1
	inventory=$2
	label=$3

	test -d "$base" || fail "missing $label archive: $base"
	require_file "$inventory"

	awk '
		{
			path = $3
			for (i = 4; i <= NF; i++) path = path " " $i
			if (seen[path]++) {
				print "duplicate inventory path: " path > "/dev/stderr"
				exit 1
			}
		}
	' "$inventory" || fail "$label inventory has duplicate paths"

	special_entries=$(find "$base" -mindepth 1 ! -type d ! -type f -print)
	test -z "$special_entries" ||
		fail "$label archive contains symbolic or special entries: $special_entries"

	unexpected_directories=$(
		find "$base" -mindepth 1 -type d -print | while IFS= read -r directory; do
			relative_directory=${directory#"$base"/}
			awk -v prefix="$relative_directory/" '
				{
					path = $3
					for (i = 4; i <= NF; i++) path = path " " $i
					if (index(path, prefix) == 1) found = 1
				}
				END { exit found ? 0 : 1 }
			' "$inventory" || printf '%s\n' "$relative_directory"
		done
	)
	test -z "$unexpected_directories" ||
		fail "$label archive contains uncontracted directories: $unexpected_directories"

	expected_count=$(wc -l < "$inventory" | tr -d ' ')
	actual_count=$(find "$base" -type f ! -name .DS_Store -print | wc -l | tr -d ' ')
	test "$actual_count" = "$expected_count" ||
		fail "$label exact set differs: expected $expected_count files, found $actual_count"

	while IFS=' ' read -r expected_mode expected_digest relative_path; do
		test -n "$relative_path" || fail "$label inventory contains an empty path"
		file=$base/$relative_path
		require_file "$file"
		test ! -L "$file" || fail "$label inventory path is symbolic: $relative_path"
		actual_mode=$(file_mode "$file")
		actual_digest=$(digest "$file")
		test "$actual_mode" = "$expected_mode" ||
			fail "$label mode differs for $relative_path"
		test "$actual_digest" = "$expected_digest" ||
			fail "$label digest differs for $relative_path"
	done < "$inventory"

	printf '%s: %s files, exact set/mode/hash pass\n' "$label" "$actual_count"
}

require_file "$source_manifest"
require_file "$governance_manifest"
test "$(digest "$source_manifest")" = "$source_manifest_digest" ||
	fail 'source manifest anchor differs'
test "$(digest "$governance_manifest")" = "$governance_manifest_digest" ||
	fail 'governance manifest anchor differs'
test "$(digest "$source_inventory")" = "$source_inventory_digest" ||
	fail 'source inventory anchor differs'
test "$(digest "$governance_inventory")" = "$governance_inventory_digest" ||
	fail 'governance inventory anchor differs'

(cd "$source_dir" && shasum -a 256 -c "$source_manifest" >/dev/null) ||
	fail 'source manifest verification failed'
(cd "$governance_dir" && shasum -a 256 -c "$governance_manifest" >/dev/null) ||
	fail 'governance manifest verification failed'

verify_inventory "$source_dir" "$source_inventory" source
verify_inventory "$governance_dir" "$governance_inventory" governance
printf '%s\n' 'archive manifests and all four evidence anchors pass'
