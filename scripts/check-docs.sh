#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

status=0
literal_checks=$(mktemp "${TMPDIR:-/tmp}/modary-doc-literals.XXXXXX")
metadata_cache=$(mktemp "${TMPDIR:-/tmp}/modary-doc-metadata.XXXXXX")
task_state=$(mktemp "${TMPDIR:-/tmp}/modary-doc-task-state.XXXXXX")
literal_separator=$(printf '\037')
literal_group=0
exec 3>"$literal_checks"
literal_checks_open=1
trap 'rm -f "$literal_checks" "$metadata_cache" "$task_state"' 0 HUP INT TERM

fail() {
	printf '%s\n' "$1" >&2
	status=1
}

queue_literal_check() {
	_check_kind=$1
	_check_file=$2
	_check_literal=$3
	case "$_check_file$_check_literal" in
		*"$literal_separator"* | *'
'*)
			fail "canonical literal checks must be single-line text without the record separator: $_check_file"
			return
			;;
	esac
	printf '%s%s%s%s%s\n' \
		"$_check_kind" "$literal_separator" "$_check_file" "$literal_separator" "$_check_literal" \
		>&3
}

queue_literal_group() {
	_check_file=$1
	_check_group=$2
	_check_description=$3
	case "$_check_file$_check_description" in
		*"$literal_separator"* | *'
'*)
			fail "canonical literal checks must be single-line text without the record separator: $_check_file"
			return
			;;
	esac
	printf '%s%s%s%s%s%s%s\n' \
		O "$literal_separator" "$_check_file" "$literal_separator" "$_check_group" \
		"$literal_separator" "$_check_description" >&3
}

require_regular_nonempty() {
	_check_file=$1
	if test ! -f "$_check_file" || test -L "$_check_file" || test ! -s "$_check_file"; then
		fail "required canonical document must be a non-empty regular file: $_check_file"
	fi
}

require_literal() {
	_check_file=$1
	_check_literal=$2
	if test -f "$_check_file"; then
		queue_literal_check R "$_check_file" "$_check_literal"
	fi
}

require_one_of_literals() {
	_check_file=$1
	shift
	if test -f "$_check_file"; then
		literal_group=$((literal_group + 1))
		queue_literal_group "$_check_file" "$literal_group" "$*"
		for _check_literal in "$@"; do
			queue_literal_check C "$literal_group" "$_check_literal"
		done
	fi
}

forbid_literal() {
	_check_file=$1
	_check_literal=$2
	if test -f "$_check_file"; then
		queue_literal_check F "$_check_file" "$_check_literal"
	fi
}

flush_literal_checks() {
	if test "$literal_checks_open" -eq 1; then
		exec 3>&-
		literal_checks_open=0
	fi
	if ! awk -v separator="$literal_separator" '
		BEGIN {
			FS = separator
		}
		function file_contents(path, line, result, read_status) {
			if (!(path in loaded)) {
				result = ""
				while ((read_status = getline line < path) > 0) {
					result = result line "\n"
				}
				close(path)
				loaded[path] = 1
				contents[path] = result
			}
			return contents[path]
		}
		$1 == "R" || $1 == "F" {
			check_count++
			check_kind[check_count] = $1
			check_file[check_count] = $2
			check_literal[check_count] = $3
			next
		}
		$1 == "O" {
			group_count++
			group = $3
			group_file[group] = $2
			group_description[group] = $4
			group_order[group_count] = group
			next
		}
		$1 == "C" {
			group = $2
			if (index(file_contents(group_file[group]), $3) != 0) {
				group_found[group] = 1
			}
		}
		END {
			failed = 0
			for (index_number = 1; index_number <= check_count; index_number++) {
				text = file_contents(check_file[index_number])
				if (check_kind[index_number] == "R" &&
						index(text, check_literal[index_number]) == 0) {
					print "required canonical statement is missing from " \
						check_file[index_number] ": " check_literal[index_number] > "/dev/stderr"
					failed = 1
				} else if (check_kind[index_number] == "F" &&
						index(text, check_literal[index_number]) != 0) {
					print "obsolete canonical statement remains in " \
						check_file[index_number] ": " check_literal[index_number] > "/dev/stderr"
					failed = 1
				}
			}
			for (index_number = 1; index_number <= group_count; index_number++) {
				group = group_order[index_number]
				if (!group_found[group]) {
					print "one required canonical statement is missing from " \
						group_file[group] ": " group_description[group] > "/dev/stderr"
					failed = 1
				}
			}
			exit failed
		}
	' "$literal_checks"; then
		status=1
	fi
}

metadata_lookup() {
	_metadata_lookup_file=$1
	_metadata_lookup_key=$2
	metadata_count=0
	metadata_result=
	while IFS="$literal_separator" read -r _metadata_record_file _metadata_record_key _metadata_record_value; do
		if test "$_metadata_record_file" = "$_metadata_lookup_file" &&
			test "$_metadata_record_key" = "$_metadata_lookup_key"; then
			metadata_count=$((metadata_count + 1))
			if test "$metadata_count" -eq 1; then
				metadata_result=$_metadata_record_value
			fi
		fi
	done <"$metadata_cache"
}

require_metadata() {
	_field_file=$1
	_field_key=$2
	_field_expected=$3
	metadata_lookup "$_field_file" "$_field_key"
	if test "$metadata_count" -ne 1 || test "$metadata_result" != "$_field_expected"; then
		fail "$_field_file must contain exactly one - $_field_key: $_field_expected field"
	fi
}

require_nonempty_metadata() {
	_field_file=$1
	_field_key=$2
	metadata_lookup "$_field_file" "$_field_key"
	if test "$metadata_count" -ne 1 || test -z "$metadata_result"; then
		fail "$_field_file must contain exactly one non-empty - $_field_key field"
	fi
}

require_sha256_metadata() {
	_field_file=$1
	_field_key=$2
	require_nonempty_metadata "$_field_file" "$_field_key"
	metadata_lookup "$_field_file" "$_field_key"
	_digest=${metadata_result#sha256:}
	if test "$metadata_result" = "$_digest" || test "${#_digest}" -ne 64; then
		fail "$_field_file - $_field_key must be one lowercase sha256 digest"
		return
	fi
	case "$_digest" in
		*[!0-9a-f]*)
			fail "$_field_file - $_field_key must be one lowercase sha256 digest"
			;;
	esac
}

require_git_commit_metadata() {
	_field_file=$1
	_field_key=$2
	require_nonempty_metadata "$_field_file" "$_field_key"
	metadata_lookup "$_field_file" "$_field_key"
	_commit=$metadata_result
	if test "${#_commit}" -ne 40; then
		fail "$_field_file - $_field_key must be one lowercase 40-character Git commit"
		return
	fi
	case "$_commit" in
		*[!0-9a-f]*)
			fail "$_field_file - $_field_key must be one lowercase 40-character Git commit"
			return
			;;
	esac
	if ! git cat-file -e "$_commit^{commit}" 2>/dev/null; then
		fail "$_field_file - $_field_key does not identify an available Git commit"
		return
	fi
	if ! git merge-base --is-ancestor "$_commit" HEAD 2>/dev/null; then
		fail "$_field_file - $_field_key must be an ancestor of the current source"
		return
	fi
}

valid_utc_timestamp() {
	_timestamp=$1
	case "$_timestamp" in
		[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z) ;;
		*) return 1 ;;
	esac
	_year=${_timestamp%%-*}
	_timestamp_rest=${_timestamp#*-}
	_month=${_timestamp_rest%%-*}
	_timestamp_rest=${_timestamp_rest#*-}
	_day=${_timestamp_rest%%T*}
	_timestamp_rest=${_timestamp_rest#*T}
	_hour=${_timestamp_rest%%:*}
	_timestamp_rest=${_timestamp_rest#*:}
	_minute=${_timestamp_rest%%:*}
	_timestamp_rest=${_timestamp_rest#*:}
	_second=${_timestamp_rest%Z}
	_month_number=${_month#0}
	_day_number=${_day#0}
	_hour_number=${_hour#0}
	_minute_number=${_minute#0}
	_second_number=${_second#0}
	test -n "$_month_number" || _month_number=0
	test -n "$_day_number" || _day_number=0
	test -n "$_hour_number" || _hour_number=0
	test -n "$_minute_number" || _minute_number=0
	test -n "$_second_number" || _second_number=0
	if test "$_month_number" -lt 1 || test "$_month_number" -gt 12 ||
		test "$_hour_number" -gt 23 || test "$_minute_number" -gt 59 ||
		test "$_second_number" -gt 59; then
		return 1
	fi
	_days=31
	case "$_month_number" in
		4 | 6 | 9 | 11) _days=30 ;;
		2)
			_year_number=$_year
			while test "${_year_number#0}" != "$_year_number"; do
				_year_number=${_year_number#0}
			done
			test -n "$_year_number" || _year_number=0
			if { test $((_year_number % 4)) -eq 0 && test $((_year_number % 100)) -ne 0; } ||
				test $((_year_number % 400)) -eq 0; then
				_days=29
			else
				_days=28
			fi
			;;
	esac
	test "$_day_number" -ge 1 && test "$_day_number" -le "$_days"
}

require_utc_metadata() {
	_field_file=$1
	_field_key=$2
	require_nonempty_metadata "$_field_file" "$_field_key"
	metadata_lookup "$_field_file" "$_field_key"
	case "$metadata_result" in
		[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z) ;;
		*)
			fail "$_field_file - $_field_key must use second-precision UTC"
			return
			;;
	esac
	if ! valid_utc_timestamp "$metadata_result"; then
		fail "$_field_file - $_field_key must be a real second-precision UTC date and time"
	fi
}

sha256_file() {
	_digest_file=$1
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$_digest_file" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$_digest_file" | awk '{ print $1 }'
	elif command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 "$_digest_file" | awk '{ print $NF }'
	else
		return 1
	fi
}

required='
README.md
LICENSE
NOTICE
docs/framework-f0.md
docs/f0-known-limitations.md
docs/f0-acceptance-report.md
docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md
docs/adr/ADR-002-governed-action-transaction.md
docs/adr/ADR-003-sqlite-and-module-migrations.md
docs/adr/ADR-004-consumer-owned-surfaces.md
.ai-platform/docs/product-design.md
.ai-platform/docs/technology-decision-record.md
.ai-platform/docs/tasks.md
.ai-platform/docs/release-report.md
.ai-platform/memory/constitution.md
.ai-platform/evidence/T013/summary.md
.ai-platform/evidence/T015/summary.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/analysis.md
.ai-platform/specs/002-framework-decoupling/tasks.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
.ai-platform/specs/002-framework-decoupling/packets/T010.yaml
.ai-platform/specs/002-framework-decoupling/packets/T011.yaml
.ai-platform/specs/002-framework-decoupling/packets/T012.yaml
.ai-platform/specs/002-framework-decoupling/packets/T013.yaml
.ai-platform/specs/002-framework-decoupling/packets/T014.yaml
.ai-platform/specs/002-framework-decoupling/packets/T015.yaml
.ai-platform/specs/002-framework-decoupling/packets/T016.yaml
'

for file in $required; do
	require_regular_nonempty "$file"
done

metadata_sources=
for file in $required \
	.ai-platform/evidence/T016/summary.md \
	.ai-platform/evidence/T016/test-results.md \
	.ai-platform/evidence/T016/review-1.md \
	.ai-platform/evidence/T016/review-2.md; do
	if test -f "$file"; then
		metadata_sources="$metadata_sources $file"
	fi
done
if test -n "$metadata_sources"; then
	if ! awk -v separator="$literal_separator" '
		index($0, separator) != 0 {
			print "canonical metadata must not contain the record separator: " FILENAME > "/dev/stderr"
			failed = 1
			next
		}
		index($0, "- ") == 1 {
			body = substr($0, 3)
			delimiter = index(body, ": ")
			if (delimiter != 0) {
				key = substr(body, 1, delimiter - 1)
				value = substr(body, delimiter + 2)
				print FILENAME separator key separator value
			}
		}
		END { exit failed }
	' $metadata_sources >"$metadata_cache"; then
		fail 'cannot build canonical metadata cache'
	fi
else
	: >"$metadata_cache"
fi

for link in docs/framework-f0.md docs/f0-known-limitations.md docs/f0-acceptance-report.md examples/counter; do
	if test ! -e "$link"; then
		fail "README local link target is missing: $link"
	fi
done

work_graph=.ai-platform/specs/002-framework-decoupling/tasks.md
current_graph=.ai-platform/docs/tasks.md

if ! awk -v work_graph="$work_graph" -v current_graph="$current_graph" -v state_file="$task_state" '
	function trim(value) {
		gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
		return value
	}
	NR == FNR {
		if ($0 ~ /^## T01[0-6]:/) {
			current_task = substr($0, 4, 4)
			section_count[current_task]++
			next
		}
		if ($0 ~ /^## /) {
			current_task = ""
			next
		}
		if (current_task != "" && $0 ~ /^Status: / && !(current_task in section_status)) {
			section_status[current_task] = substr($0, 9)
		}
		next
	}
	$0 ~ /^\| T01[0-6] \|/ {
		field_count = split($0, fields, "|")
		task = trim(fields[2])
		row_count[task]++
		if (!(task in table_status)) {
			table_status[task] = trim(fields[3])
		}
	}
	END {
		failed = 0
		for (number = 10; number <= 16; number++) {
			task = sprintf("T%03d", number)
			if (section_count[task] != 1) {
				print work_graph " must contain exactly one " task " section" > "/dev/stderr"
				failed = 1
			}
			if (row_count[task] != 1) {
				print current_graph " must contain exactly one " task " row" > "/dev/stderr"
				failed = 1
			}
			if (section_status[task] != table_status[task]) {
				print task " state differs between canonical task graphs: " \
					section_status[task] " / " table_status[task] > "/dev/stderr"
				failed = 1
			}
			if (task != "T016" && section_status[task] != "Completed") {
				print task " must remain Completed in both canonical task graphs" > "/dev/stderr"
				failed = 1
			}
		}
		print section_status["T016"] > state_file
		exit failed
	}
' "$work_graph" "$current_graph"; then
	status=1
fi

t016_status=
IFS= read -r t016_status <"$task_state" || true

for task in T010 T011 T012 T013 T014 T015 T016; do
	packet=".ai-platform/specs/002-framework-decoupling/packets/$task.yaml"
	validator="python3 /Users/iiwish/.codex/skills/ai-delivery-governor/scripts/validate_delivery_artifacts.py --root /Users/iiwish/self/modary --feature-id 002-framework-decoupling --task-id $task --strict"
	require_literal "$packet" "$validator"
	require_literal "$work_graph" "$validator"
done

metadata_lookup docs/f0-acceptance-report.md Status
acceptance_status=$metadata_result
metadata_lookup .ai-platform/docs/release-report.md 'Technical F0 acceptance'
release_status=$metadata_result

case "$t016_status" in
	In_Progress)
		expected_report_status=In_Progress
		;;
	Completed)
		expected_report_status=Accepted
		;;
	*)
		fail "T016 has unsupported canonical state: $t016_status"
		expected_report_status=
		;;
esac

if test -n "$expected_report_status" && test "$acceptance_status" != "$expected_report_status"; then
	fail "acceptance report status $acceptance_status conflicts with T016 state $t016_status"
fi
if test -n "$expected_report_status" && test "$release_status" != "$expected_report_status"; then
	fail "release report technical status $release_status conflicts with T016 state $t016_status"
fi
if test -n "$expected_report_status"; then
	require_metadata docs/f0-acceptance-report.md Status "$expected_report_status"
	require_metadata .ai-platform/docs/release-report.md Status "$expected_report_status"
	require_metadata .ai-platform/docs/release-report.md 'Technical F0 acceptance' "$expected_report_status"
fi

release_report=.ai-platform/docs/release-report.md
require_metadata "$release_report" 'Report version' 1.0
require_metadata "$release_report" 'Distribution status' Not_released
require_metadata "$release_report" 'Version tag' None
require_metadata "$release_report" 'Owner-selected redistribution license' Apache-2.0

release_readiness_graph=.ai-platform/specs/003-release-readiness/tasks.md
if test -f "$release_readiness_graph"; then
	for file in \
		.ai-platform/specs/003-release-readiness/spec.md \
		.ai-platform/specs/003-release-readiness/plan.md \
		.ai-platform/specs/003-release-readiness/analysis.md \
		.ai-platform/specs/003-release-readiness/checklists/requirements.md \
		.ai-platform/specs/003-release-readiness/packets/T017.yaml \
		.ai-platform/specs/003-release-readiness/packets/T018.yaml \
		.ai-platform/specs/003-release-readiness/packets/T019.yaml \
		.ai-platform/specs/003-release-readiness/packets/T020.yaml; do
		require_regular_nonempty "$file"
	done
	require_metadata "$release_report" 'Target version' v0.1.0-alpha.1
	require_metadata "$release_report" 'Canonical remote' https://github.com/iiwish/modary
	require_metadata "$release_report" 'Private security reporting channel' https://github.com/iiwish/modary/security/advisories/new
	require_metadata "$release_report" 'Remote consumer verification' Not_run

	t020_graph_status=$(awk '
		$0 == "## T020: Full Release Readiness Acceptance" { in_task=1; next }
		/^## / { in_task=0 }
		in_task && /^Status: / { print substr($0, 9); exit }
	' "$release_readiness_graph")
	t020_current_status=$(awk '
		$0 == "## T020: Current Alpha Release Readiness" { in_task=1; next }
		/^## / { in_task=0 }
		in_task && /^Status: / { print substr($0, 9); exit }
	' "$current_graph")
	if test -z "$t020_graph_status" || test "$t020_graph_status" != "$t020_current_status"; then
		fail "T020 state differs between release-readiness task graphs: $t020_graph_status / $t020_current_status"
	fi
	case "$t020_graph_status" in
		In_Progress) expected_engineering_readiness=In_Progress ;;
		Completed) expected_engineering_readiness=Accepted ;;
		*)
			fail "T020 has unsupported current state: $t020_graph_status"
			expected_engineering_readiness=
			;;
	esac
	if test -n "$expected_engineering_readiness"; then
		require_metadata "$release_report" 'Engineering readiness' "$expected_engineering_readiness"
	fi
fi

forbid_literal .ai-platform/specs/002-framework-decoupling/packets/T016.yaml 'check-archives.sh'
forbid_literal .ai-platform/specs/002-framework-decoupling/packets/T016.yaml 'a preservation archive checksum changes'
forbid_literal .ai-platform/specs/002-framework-decoupling/packets/T015.yaml 'AGENTS.md'
forbid_literal .ai-platform/specs/002-framework-decoupling/packets/T016.yaml 'AGENTS.md'

require_literal LICENSE 'Apache License'
require_literal LICENSE 'Version 2.0, January 2004'
require_literal NOTICE 'Modary'
require_literal NOTICE 'xeipuuv'
require_literal NOTICE 'MongoDB, Inc.'
require_literal docs/framework-f0.md 'Apache-2.0'
require_literal docs/f0-known-limitations.md 'Apache-2.0'
require_literal .ai-platform/docs/product-design.md 'Apache-2.0'
require_literal .ai-platform/specs/002-framework-decoupling/spec.md 'owner-selected redistribution license'
require_literal docs/framework-f0.md 'becomes a release claim only after'
require_literal docs/f0-known-limitations.md 'download is not part of F0 acceptance'
require_literal README.md 'Every state-changing business path converges on `action.Runtime`'
require_literal README.md 'Go 1.26 or newer is required'
require_literal README.md 'Node.js is not required'
require_literal docs/framework-f0.md 'migration declarations'
require_literal docs/framework-f0.md 'opaque public'
require_literal docs/framework-f0.md 'not a filesystem-wide or crash-atomic transaction'
require_literal docs/f0-known-limitations.md 'not a filesystem-wide or crash-atomic transaction'
require_literal docs/adr/ADR-004-consumer-owned-surfaces.md 'not claim filesystem-wide, crash, or cross-process atomicity'
require_literal docs/f0-known-limitations.md 'not a sandbox boundary'
require_literal docs/framework-f0.md 'not a sandbox boundary'

lifecycle_contract_docs='
docs/framework-f0.md
docs/f0-known-limitations.md
docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
'
for file in $lifecycle_contract_docs; do
	require_literal "$file" 'The HandlerFactory Resolver is valid only during the factory call'
	require_literal "$file" '`module.ErrInvalidResolver`'
	require_literal "$file" 'invocation-start order'
	require_literal "$file" 'timed-out cleanup callback may overlap'
	require_literal "$file" '`AuditFailure`'
	require_literal "$file" 'independent deadline context'
done

capability_contract_docs='
docs/framework-f0.md
docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
examples/counter/README.md
'
for file in $capability_contract_docs; do
	require_literal "$file" 'package-level typed key'
done

require_literal .ai-platform/specs/002-framework-decoupling/spec.md 'explicit public production package'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'privileged-internal allowlist'

json_boundary_docs='
docs/framework-f0.md
docs/f0-known-limitations.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
'
for file in $json_boundary_docs; do
	require_literal "$file" 'Action schema'
	require_literal "$file" 'Handler plan payload'
	require_literal "$file" 'Preview summary'
	require_literal "$file" 'Result data'
	require_literal "$file" 'persisted Action JSON value'
	require_literal "$file" '1 MiB'
	require_literal "$file" '(1,048,576)'
	require_literal "$file" '256 nested object or array containers'
	require_literal "$file" 'root container'
	require_literal "$file" '65,536 JSON value nodes'
	require_literal "$file" '4,096 source bytes'
	require_literal "$file" 'JSON number token'
	require_literal "$file" 'valid UTF-8'
	require_literal "$file" 'exactly one JSON value'
	require_literal "$file" 'duplicate object member names'
	require_literal "$file" 'HTTP and MCP request envelopes'
	require_literal "$file" 'independent byte budgets'
	require_literal "$file" '2 MiB defaults'
	require_literal "$file" 'maximum-size Action JSON document'
	require_literal "$file" 'required envelope fields'
	require_literal "$file" 'extracted Action document'
	require_literal "$file" 'per-document Action limits'
done
require_literal .ai-platform/specs/002-framework-decoupling/spec.md 'Exact-boundary tests accept each Action JSON limit exactly'
require_literal docs/f0-known-limitations.md 'increasing an envelope budget does not increase an'

platform_boundary_docs='
docs/framework-f0.md
docs/f0-known-limitations.md
docs/adr/ADR-004-consumer-owned-surfaces.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/tasks.md
.ai-platform/evidence/T013/summary.md
.ai-platform/evidence/T015/summary.md
'
for file in $platform_boundary_docs; do
	require_literal "$file" '`TMPDIR`'
	require_literal "$file" '`GOTMPDIR`'
	require_literal "$file" 'removes every inherited'
	require_literal "$file" 'case-variant'
	require_literal "$file" 'child environment'
	require_literal "$file" 'exactly once'
	require_literal "$file" 'same canonical staging parent'
	require_literal "$file" 'ambient `GOTMPDIR`'
	require_literal "$file" 'override that parent'
	require_literal "$file" 'every ancestor through `/`'
	require_literal "$file" 'descriptor'
	require_literal "$file" 'revalidat'
	require_literal "$file" 'root-owned and sticky'
	require_literal "$file" 'effective-UID-owned'
	require_literal "$file" 'exact mode `0700`'
	require_literal "$file" 'extended ACL'
	require_literal "$file" 'retained level'
	require_literal "$file" 'reject'
	require_literal "$file" 'Every other platform'
	require_literal "$file" 'no validated ACL policy'
	require_literal "$file" 'cross-compile-only'
	require_literal "$file" 'no native Build'
	require_literal "$file" 'rename runtime validation'
	require_literal "$file" '`GO111MODULE=on`'
	require_literal "$file" '`GOTOOLCHAIN=local`'
	require_literal "$file" '`GOENV=off`'
	require_literal "$file" '`GOWORK=off`'
	require_literal "$file" '`GOFLAGS`'
	require_literal "$file" '`-mod=readonly`'
	require_literal "$file" '`-buildvcs=false`'
	require_literal "$file" 'Other inherited environment'
	require_literal "$file" 'selected Go executable'
	require_literal "$file" 'consumer source'
	require_literal "$file" 'not a sandbox'
	require_literal "$file" '`waitid(WEXITED|WNOWAIT)`'
	require_literal "$file" 'leader'
	require_one_of_literals "$file" 'without reaping' 'does not reap'
	require_literal "$file" 'PID and PGID'
	require_literal "$file" 'zero'
	require_literal "$file" 'non-zero'
	require_literal "$file" 'kills residual same-group'
	require_literal "$file" '`Cmd.Wait`'
	require_literal "$file" '`exec.ErrWaitDelay`'
	require_literal "$file" 'pre-reap'
	require_literal "$file" 'context remains active'
	require_literal "$file" 'inherited-pipe'
	require_literal "$file" 'backstop'
	require_one_of_literals "$file" 'does not fail' 'not a Build failure'
	require_literal "$file" 'process-exit'
	require_literal "$file" 'cleanup errors'
	require_literal "$file" 'descendants'
	require_literal "$file" 'opaque public'
	require_literal "$file" 'error-chain'
	require_literal "$file" '`errors.Is`'
	require_literal "$file" '`errors.As`'
	require_literal "$file" '`Error`, `Is`, `As`, or `Unwrap`'
	require_literal "$file" 'internal helper'
	require_literal "$file" 'filesystem identity'
	require_literal "$file" 'not by pathname spelling'
	require_literal "$file" 'compiler cancellation and inherited output'
	require_literal "$file" 'Caller-supplied `io.Writer.Write` must return'
done

temporary_environment_test_docs='
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/tasks.md
.ai-platform/evidence/T013/summary.md
.ai-platform/evidence/T015/summary.md
'
for file in $temporary_environment_test_docs; do
	require_literal "$file" 'fake Go'
	require_literal "$file" 'symlink'
	require_literal "$file" '`GOTMPDIR`'
	require_literal "$file" 'both child variables'
done

token_boundary_docs='
docs/framework-f0.md
docs/f0-known-limitations.md
docs/adr/ADR-004-consumer-owned-surfaces.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/tasks.md
.ai-platform/evidence/T015/summary.md
'
for file in $token_boundary_docs; do
	require_literal "$file" '`--token-file <path>`'
	require_literal "$file" 'Linux'
	require_literal "$file" 'Darwin'
	require_literal "$file" 'regular'
	require_literal "$file" 'effective UID'
	require_literal "$file" '`0400`'
	require_literal "$file" '`0600`'
	require_literal "$file" 'retained open'
	require_literal "$file" 'extended ACL'
	require_literal "$file" 'token path'
	require_literal "$file" 'before any filesystem access'
	require_literal "$file" '`--token-file -`'
	require_literal "$file" 'standard input'
	require_literal "$file" '`appcmd.Options.Stdout`'
	require_literal "$file" '`Stderr`'
	require_literal "$file" 'trusted'
	require_literal "$file" 'cooperative'
	require_literal "$file" '`Write`'
	require_literal "$file" 'context cancellation'
	require_literal "$file" 'shutdown timeout'
	require_literal "$file" 'blocked writer'
done

sqlite_boundary_docs='
docs/framework-f0.md
docs/f0-known-limitations.md
docs/adr/ADR-003-sqlite-and-module-migrations.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/tasks.md
.ai-platform/evidence/T015/summary.md
'
for file in $sqlite_boundary_docs; do
	require_literal "$file" 'directory ancestor'
	require_literal "$file" 'effective UID or root'
	require_literal "$file" 'group- or other-writable ancestor'
	require_literal "$file" 'root-owned and sticky'
	require_literal "$file" 'final database directory'
	require_literal "$file" 'effective-UID-owned'
	require_literal "$file" 'non-writable by group or other users'
done

schema_boundary_docs='
docs/framework-f0.md
docs/f0-known-limitations.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
'
for file in $schema_boundary_docs; do
	require_one_of_literals "$file" 'JSON Schema Draft 7' 'Draft 7'
	require_literal "$file" 'boolean'
	require_literal "$file" 'SchemaGraph'
	require_literal "$file" 'local JSON Pointer'
	require_literal "$file" 'Draft 7 metaschema'
	require_literal "$file" 'offline'
	require_literal "$file" 'pinned'
	require_literal "$file" '`$id`'
	require_literal "$file" 'Go RE2'
	require_literal "$file" '2,048'
	require_literal "$file" 'schema nodes'
	require_literal "$file" '512'
	require_literal "$file" 'collection'
	require_literal "$file" '256'
	require_literal "$file" 'enum'
	require_literal "$file" 'values'
	require_literal "$file" '16 KiB'
	require_literal "$file" '4 KiB'
	require_literal "$file" '1,024'
	require_literal "$file" '64 Mi'
	require_literal "$file" 'cumulative'
	require_literal "$file" '4,096 mismatch events'
	require_literal "$file" '4,096 active evaluation frames'
	require_one_of_literals "$file" 'flag-only' 'Flag-only'
	require_literal "$file" 'diagnostic'
	require_literal "$file" 'tree'
	require_literal "$file" '`LIMIT_EXCEEDED`'
	require_literal "$file" 'MCP'
	require_literal "$file" '128'
	require_literal "$file" '1 Mi numeric wrapper allowance'
	require_literal "$file" 'compile-only JSON'
	require_literal "$file" 'value nodes'
	require_literal "$file" 'official Draft 7 mandatory corpus'
	require_literal "$file" '223'
	require_literal "$file" '856'
	require_literal "$file" '34'
	require_literal "$file" '71'
	forbid_literal "$file" 'exact-number rewriting'
	forbid_literal "$file" 'before and after exact-number'
done
require_literal docs/framework-f0.md 'one immutable executable SchemaGraph'
protocol_input_docs='
docs/framework-f0.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
'
for file in $protocol_input_docs; do
	require_literal "$file" '`input`'
	require_one_of_literals "$file" 'missing input' 'Missing input' 'missing member' 'Missing `input`' 'missing `input`' 'absence'
	require_one_of_literals "$file" 'explicit `null`' 'Explicit `null`' 'explicitly present `null`' 'Explicitly present `null`' 'Present `null`' 'preserving explicit `null`'
	require_literal "$file" 'Runtime'
	require_literal "$file" 'audit'
done
require_literal docs/framework-f0.md 'A missing member is a protocol'

capability_contract_docs='
docs/framework-f0.md
docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
'
for file in $capability_contract_docs; do
	require_literal "$file" '`module.Capability`'
	require_literal "$file" 'database'
	require_literal "$file" 'identity'
	require_literal "$file" 'authorization'
	require_literal "$file" 'audit'
	require_literal "$file" 'consumer'
	require_literal "$file" 'namespaced'
done

host_reference_docs='
docs/framework-f0.md
docs/f0-known-limitations.md
docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
'
for file in $host_reference_docs; do
	require_literal "$file" 'Start callbacks'
	require_one_of_literals "$file" 'handler factories' 'Handler factories'
	require_literal "$file" 'migration'
	require_literal "$file" 'filesystem'
	require_one_of_literals "$file" 'caller' 'consumer-owned' 'consumer retains'
done

host_availability_docs='
docs/framework-f0.md
docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md
.ai-platform/specs/002-framework-decoupling/spec.md
.ai-platform/specs/002-framework-decoupling/plan.md
.ai-platform/specs/002-framework-decoupling/checklists/requirements.md
'
for file in $host_availability_docs; do
	require_one_of_literals "$file" 'nil' 'Nil'
	require_literal "$file" 'zero'
	require_literal "$file" 'copied'
	require_literal "$file" 'unavailable'
	require_literal "$file" 'fail closed'
done

require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'canonical TMPDIR and every ancestor through / retain file descriptors and filesystem identities'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'every retained TMPDIR level is effective-UID- or root-owned; group/other-writable levels are accepted only when root-owned and sticky'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'child staging directory is effective-UID-owned with exact mode 0700'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'Darwin also rejects its extended ACL'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'Build removes every inherited case-variant TMPDIR and GOTMPDIR entry from the child environment, then sets both exactly once to the same canonical staging parent whose descriptor and ancestry are retained and revalidated'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'ambient GOTMPDIR cannot override that parent'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'a symlink alias supplied through ambient TMPDIR is canonicalized and a malicious project-path GOTMPDIR is discarded'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'every other platform, including other Unix variants and Windows, fails Build'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'no native Build, ACL, or rename runtime claim'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'GO111MODULE=on, GOTOOLCHAIN=local, GOENV=off, GOWORK=off, and empty GOFLAGS'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml '-mod=readonly and -buildvcs=false'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'remaining inherited environment, selected Go executable and toolchain, consumer source'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'waitid(WEXITED|WNOWAIT) observes but does not reap'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'kills residual same-group descendants after zero and non-zero exits, then calls Cmd.Wait'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'exec.ErrWaitDelay from Cmd.Wait is only a residual inherited-pipe close backstop and does not fail Build'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'writer, process-exit, cancellation, and cleanup errors still fail'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'daemonized or re-grouped trusted descendants'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'opaque public error-chain boundary'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'standard errors.Is/errors.As use bounded matching without calling caller-defined Error, Is, As, or Unwrap'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'filesystem identity captured when the Project is loaded, not by pathname spelling'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'compiler cancellation and inherited output pipes have a bounded wait'
require_literal .ai-platform/specs/002-framework-decoupling/packets/T013.yaml 'caller io.Writer.Write must return'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'canonical outside-project staging'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'Linux/Darwin ACL policy'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'unsupported-platform fail-closed Build'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'cross-compile-only platform claims'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'same-UID pathname boundaries'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'load-captured filesystem-identity locking'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'blocked caller `io.Writer.Write`'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'Go module/toolchain/work-file/flags isolation'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'child-environment deduplication'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'canonical `TMPDIR`/`GOTMPDIR` pinning'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'remaining trusted environment, toolchain, and consumer-source boundary'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'Linux/Darwin waitid-before-reap process-group cleanup'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'successful-Build `exec.ErrWaitDelay` residual-pipe handling'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'failure precedence'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'daemonized or re-grouped descendant limitation'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'Linux/Darwin CLI token-path policy'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'other-platform pre-filesystem rejection'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'File-backed SQLite ancestor ownership'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'root-owned sticky writable-directory exceptions'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'opaque public extension/dependency error chains'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'Declared custom Action errors'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'quoted or unquoted `temp.` schema access before invoking a backend'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'applied-history bounds'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'exact private completion and rollback outcome proof'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'outer rollback-only unit without savepoints'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md '`panic(nil)`'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'Every Action JSON document has independent limits'
require_literal .ai-platform/specs/002-framework-decoupling/checklists/requirements.md 'Public database row lifecycle'

require_literal .ai-platform/evidence/T013/summary.md 'verified `os.Root`'
require_literal .ai-platform/evidence/T013/summary.md 'Node-free build'
require_literal .ai-platform/evidence/T013/summary.md 'atomic only where the host'
require_literal .ai-platform/evidence/T013/summary.md '`command.Dir`'
require_literal .ai-platform/evidence/T013/summary.md 'same-UID replacement are not a sandbox boundary'
require_literal .ai-platform/evidence/T015/summary.md 'Node-free Go framework Make and CI gates'
require_literal .ai-platform/evidence/T015/summary.md 'verified `os.Root`'
require_literal .ai-platform/evidence/T015/summary.md 'atomic only where the host filesystem'
require_literal .ai-platform/evidence/T015/summary.md 'The Go subprocess `command.Dir`'
require_literal .ai-platform/evidence/T015/summary.md 'same-UID replacement are not a sandbox boundary'

forbid_literal .ai-platform/specs/002-framework-decoupling/spec.md 'Installer'
forbid_literal .ai-platform/specs/002-framework-decoupling/plan.md 'Action schemas, migrations, and configured outputs'
forbid_literal README.md 'Every state-changing path'
forbid_literal "$release_report" '- Version: 1.0'
forbid_literal .ai-platform/evidence/T013/summary.md 'root-confined'
forbid_literal .ai-platform/evidence/T013/summary.md 'Go-only'
forbid_literal .ai-platform/evidence/T013/summary.md 'atomically replaces each changed file'
forbid_literal .ai-platform/evidence/T013/summary.md 'atomically installs one regular executable'
forbid_literal .ai-platform/evidence/T015/summary.md 'Go-only Make and CI'
for file in $platform_boundary_docs .ai-platform/specs/002-framework-decoupling/packets/T013.yaml; do
	forbid_literal "$file" 'Non-Unix systems rely on OS temporary-directory ACL protections'
	forbid_literal "$file" 'POSIX compiler staging requires owner-only mode `0700`'
	forbid_literal "$file" 'POSIX compiler staging requires owner-only mode 0700'
	forbid_literal "$file" 'owner-only mode 0700 on POSIX'
	forbid_literal "$file" 'private on every POSIX system'
	forbid_literal "$file" 'Non-Unix Build fails closed until an ACL-aware implementation'
	forbid_literal "$file" 'F0 does not claim native Windows ACL, rename, or Build runtime validation'
	forbid_literal "$file" 'fully sandboxed'
	forbid_literal "$file" 'all descendants are contained'
	forbid_literal "$file" 'accepts inherited access lists at every retained level'
	forbid_literal "$file" 'Cancellation and `WaitDelay` cleanup kill residual descendants'
	forbid_literal "$file" 'sticky regardless of owner'
	forbid_literal "$file" 'accepts it inside'
	forbid_literal "$file" 'reaps it before cleanup'
	forbid_literal "$file" 'exec.ErrWaitDelay fails an otherwise successful Build'
	forbid_literal "$file" 'keeps inherited duplicate case-variant'
	forbid_literal "$file" 'ambient `GOTMPDIR` may override that parent'
	forbid_literal "$file" 'ambient GOTMPDIR may override that parent'
	forbid_literal "$file" '`TMPDIR` and `GOTMPDIR` may point to different directories'
	forbid_literal "$file" 'TMPDIR and GOTMPDIR may point to different directories'
	forbid_literal "$file" 'never dispatches caller-defined'
	forbid_literal "$file" 'external consumer must import an internal helper'
done
for file in $token_boundary_docs; do
	forbid_literal "$file" 'POSIX token files'
	forbid_literal "$file" 'supported POSIX systems'
	forbid_literal "$file" 'after filesystem access'
	forbid_literal "$file" 'unsupported token paths are inspected before rejection'
	forbid_literal "$file" 'can interrupt a blocked writer'
done
for file in $sqlite_boundary_docs; do
	forbid_literal "$file" 'any sticky writable ancestor is accepted'
	forbid_literal "$file" 'foreign-owned read-only ancestors are accepted'
done

flush_literal_checks

final_mode=${MODARY_DOCS_FINAL:-0}
case "$final_mode" in
	0 | 1) ;;
	*)
		fail 'MODARY_DOCS_FINAL must be 0 or 1'
		final_mode=1
		;;
esac
if test "$t016_status" = Completed; then
	final_mode=1
fi

if test "$final_mode" = 1; then
	if test "$t016_status" != Completed; then
		fail 'final documentation mode requires T016 Completed'
	fi
	if test "$acceptance_status" != Accepted; then
		fail 'final documentation mode requires technical acceptance Accepted'
	fi
	if test "$release_status" != Accepted; then
		fail 'final documentation mode requires release report technical acceptance Accepted'
	fi

	require_metadata docs/f0-acceptance-report.md 'Distribution status' Not_released
	require_metadata docs/f0-acceptance-report.md 'Version tag' None
	require_metadata docs/f0-acceptance-report.md 'Owner-selected redistribution license' None
	require_git_commit_metadata docs/f0-acceptance-report.md 'Accepted commit'
	metadata_lookup docs/f0-acceptance-report.md 'Accepted commit'
	t016_commit=$metadata_result
	if test -n "$t016_commit"; then
		if ! git diff --quiet "$t016_commit" -- .ai-platform/evidence/T016; then
			fail '.ai-platform/evidence/T016 differs from the accepted F0 commit'
		fi
		if test -n "$(git ls-files --others --exclude-standard -- .ai-platform/evidence/T016)"; then
			fail '.ai-platform/evidence/T016 contains files absent from the accepted F0 commit'
		fi
	fi

	evidence_dir=.ai-platform/evidence/T016
	evidence_complete=1
	for file in summary.md diff.patch test-results.md review-1.md review-2.md; do
		require_regular_nonempty "$evidence_dir/$file"
		if test ! -f "$evidence_dir/$file" || test -L "$evidence_dir/$file" || test ! -s "$evidence_dir/$file"; then
			evidence_complete=0
		fi
	done

	if test "$evidence_complete" -eq 1; then
		require_metadata "$evidence_dir/summary.md" Status Completed
		require_metadata "$evidence_dir/test-results.md" Result Passed
		require_sha256_metadata "$evidence_dir/summary.md" 'Frozen tree'
		require_utc_metadata "$evidence_dir/summary.md" 'Frozen at'
		require_sha256_metadata "$evidence_dir/test-results.md" 'Frozen tree'
		require_utc_metadata "$evidence_dir/test-results.md" 'Completed at'
		metadata_lookup "$evidence_dir/summary.md" 'Frozen tree'
		frozen_tree=$metadata_result
		metadata_lookup "$evidence_dir/summary.md" 'Frozen at'
		frozen_at=$metadata_result
		metadata_lookup "$evidence_dir/test-results.md" 'Completed at'
		tests_completed=$metadata_result
		if ! awk -v completed="$tests_completed" -v frozen="$frozen_at" 'BEGIN { exit !(completed >= frozen) }'; then
			fail "$evidence_dir/test-results.md must complete at or after the implementation freeze"
		fi
		metadata_lookup "$evidence_dir/test-results.md" 'Frozen tree'
		if test "$metadata_result" != "$frozen_tree"; then
			fail "$evidence_dir/test-results.md must reference the reviewed frozen tree"
		fi
		for review in "$evidence_dir/review-1.md" "$evidence_dir/review-2.md"; do
			require_metadata "$review" Verdict Pass
			require_nonempty_metadata "$review" Reviewer
			require_utc_metadata "$review" 'Started at'
			require_sha256_metadata "$review" 'Frozen tree'
			require_nonempty_metadata "$review" Scope
			require_nonempty_metadata "$review" Commands
			metadata_lookup "$review" 'Frozen tree'
			if test "$metadata_result" != "$frozen_tree"; then
				fail "$review must reference the reviewed frozen tree"
			fi
			metadata_lookup "$review" 'Started at'
			review_started=$metadata_result
			if ! awk -v review="$review_started" -v frozen="$frozen_at" 'BEGIN { exit !(review >= frozen) }'; then
				fail "$review must start at or after the implementation freeze"
			fi
			for severity in P0 P1 P2; do
				metadata_lookup "$review" "$severity"
				if test "$metadata_count" -ne 1 || test "$metadata_result" != 0; then
					fail "$review must contain exactly one zero $severity result"
				fi
			done
		done
		metadata_lookup "$evidence_dir/review-1.md" Reviewer
		reviewer_one=$metadata_result
		metadata_lookup "$evidence_dir/review-2.md" Reviewer
		reviewer_two=$metadata_result
		if test "$reviewer_one" = "$reviewer_two"; then
			fail 'T016 reviews must identify two different independent reviewers'
		fi
		if test "$status" -eq 0; then
			# T016 is an accepted historical freeze. Later release-readiness work
			# must not redefine that evidence as a digest of the current tree.
			# Verify the stored source-state artifact against its recorded digest;
			# Git history and the subsequent release acceptance preserve lineage.
			if review_digest=$(sha256_file "$evidence_dir/diff.patch"); then
				if test "$frozen_tree" != "sha256:$review_digest"; then
					fail "$evidence_dir/diff.patch does not match the stored T016 review source state digest"
				fi
			else
				fail 'no SHA-256 implementation is available for T016 review-state validation'
			fi
		fi
	fi

	closure_files="
		$work_graph
		docs/f0-acceptance-report.md
	"
	if test "$evidence_complete" -eq 1; then
		closure_files="$closure_files
			$evidence_dir/summary.md
			$evidence_dir/test-results.md
			$evidence_dir/review-1.md
			$evidence_dir/review-2.md
		"
	fi
	if grep -Ein '(^|[^[:alpha:]])(pending|in_progress|in progress)([^[:alpha:]]|$)' $closure_files >/dev/null; then
		fail 'final closure documents must not retain Pending or In_Progress state'
	fi
fi

exit "$status"
