#!/bin/sh
set -eu

script_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
root=${MODARY_DOCS_ROOT:-$script_root}
cd "$root"

status=0

fail() {
	printf '%s\n' "$1" >&2
	status=1
}

require_file() {
	file=$1
	if test ! -f "$file" || test -L "$file" || test ! -s "$file"; then
		fail "required canonical document must be a non-empty regular file: $file"
	fi
}

require_literal() {
	file=$1
	literal=$2
	if test -f "$file" && ! grep -Fq -- "$literal" "$file"; then
		fail "required canonical statement is missing from $file: $literal"
	fi
}

require_line() {
	file=$1
	line=$2
	if test ! -f "$file"; then
		return
	fi
	count=$(grep -Fxc -- "$line" "$file" || true)
	if test "$count" -ne 1; then
		fail "$file must contain exactly one $line line"
	fi
}

required='README.md
LICENSE
NOTICE
SECURITY.md
docs/index.md
docs/framework-f0.md
docs/f0-known-limitations.md
docs/f0-acceptance-report.md
docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md
docs/adr/ADR-002-governed-action-transaction.md
docs/adr/ADR-003-postgresql-and-module-migrations.md
docs/adr/ADR-004-consumer-owned-surfaces.md
docs/concepts/persistence-and-tasks.md
docs/how-to/run-background-tasks.md
docs/operations/deployment.md
docs/operations/security.md
docs/operations/postgresql-backup-restore.md
docs/reference/packages.md
docs/reference/support-matrix.md
docs/zh-CN/index.md
docs/zh-CN/getting-started/quickstart.md
docs/zh-CN/getting-started/first-application.md
docs/zh-CN/concepts/persistence-and-tasks.md
docs/zh-CN/how-to/run-background-tasks.md
.ai-platform/memory/constitution.md
.ai-platform/docs/product-design.md
.ai-platform/docs/technology-decision-record.md
.ai-platform/docs/tasks.md
.ai-platform/docs/release-report.md
.ai-platform/specs/005-postgres-task-runtime/spec.md
.ai-platform/specs/005-postgres-task-runtime/plan.md
.ai-platform/specs/005-postgres-task-runtime/analysis.md
.ai-platform/specs/005-postgres-task-runtime/tasks.md
.ai-platform/specs/005-postgres-task-runtime/checklists/requirements.md
.ai-platform/specs/005-postgres-task-runtime/packets/T024.yaml
.ai-platform/specs/005-postgres-task-runtime/packets/T025.yaml
.ai-platform/specs/005-postgres-task-runtime/packets/T026.yaml
.ai-platform/specs/006-postgres-alpha-release/spec.md
.ai-platform/specs/006-postgres-alpha-release/plan.md
.ai-platform/specs/006-postgres-alpha-release/analysis.md
.ai-platform/specs/006-postgres-alpha-release/tasks.md
.ai-platform/specs/006-postgres-alpha-release/checklists/requirements.md
.ai-platform/specs/006-postgres-alpha-release/packets/T027.yaml
.ai-platform/evidence/T024/summary.md
.ai-platform/evidence/T024/diff.patch
.ai-platform/evidence/T024/test-results.md
.ai-platform/evidence/T025/summary.md
.ai-platform/evidence/T025/diff.patch
.ai-platform/evidence/T025/test-results.md
.ai-platform/evidence/T026/summary.md
.ai-platform/evidence/T026/diff.patch
.ai-platform/evidence/T026/test-results.md
.ai-platform/evidence/T026/review-1.md
.ai-platform/evidence/T026/review-2.md
.ai-platform/evidence/T026/review-3.md
.ai-platform/evidence/T026/review-4.md
.ai-platform/evidence/T027/summary.md
.ai-platform/evidence/T027/diff.patch
.ai-platform/evidence/T027/test-results.md
.ai-platform/evidence/T027/review.md
.ai-platform/evidence/T027/release-notes.md'

for file in $required; do
	require_file "$file"
done

work_graph=.ai-platform/specs/005-postgres-task-runtime/tasks.md
current_graph=.ai-platform/docs/tasks.md
t027_work_status=$(awk '
	index($0, "## T027:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/006-postgres-alpha-release/tasks.md)
t027_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T027 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t027_work_status" != "$t027_current_status"; then
	fail "T027 must have one consistent current state: $t027_work_status / $t027_current_status"
fi
case "$t027_work_status" in
	In_Progress)
		require_line .ai-platform/evidence/T027/summary.md '- Status: In_Progress'
		require_line .ai-platform/evidence/T027/test-results.md '- Result: In_Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T027/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T027/test-results.md '- Result: Passed'
		;;
	*) fail "T027 has an invalid release state: $t027_work_status" ;;
esac

for task in T024 T025 T026; do
	work_status=$(awk -v heading="## $task:" '
		index($0, heading) == 1 { active=1; next }
		/^## / { active=0 }
		active && /^Status: / { print substr($0, 9); exit }
	' "$work_graph")
	current_status=$(awk -F'|' -v task="$task" '
		function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
		$0 ~ "^\\| " task " \\|" { count++; value=trim($3) }
		END { if (count == 1) print value }
	' "$current_graph")
	if test "$work_status" != Completed || test "$current_status" != Completed; then
		fail "$task must be Completed in the 005 work graph and current task table: $work_status / $current_status"
	fi
	for evidence in summary.md diff.patch test-results.md; do
		require_file ".ai-platform/evidence/$task/$evidence"
	done
	require_line ".ai-platform/evidence/$task/summary.md" '- Status: Completed'
	require_line ".ai-platform/evidence/$task/test-results.md" '- Result: Passed'
done

for review in .ai-platform/evidence/T026/review-3.md .ai-platform/evidence/T026/review-4.md; do
	require_line "$review" '- Verdict: Pass'
	for severity in P0 P1 P2; do
		require_line "$review" "- $severity: 0"
	done
done

require_line docs/f0-acceptance-report.md '- Status: Accepted'
require_line docs/f0-acceptance-report.md '- Distribution status: Released'
require_line docs/f0-acceptance-report.md '- Version tag: v0.1.0-alpha.3'
require_line .ai-platform/docs/release-report.md '- Report version: 2.2'
require_line .ai-platform/docs/release-report.md '- Status: Remote_verified'
require_line .ai-platform/docs/release-report.md '- Technical F0 acceptance: Accepted'
require_line .ai-platform/docs/release-report.md '- Engineering readiness: Accepted'
require_line .ai-platform/docs/release-report.md '- Distribution status: Released'
require_line .ai-platform/docs/release-report.md '- Version tag: v0.1.0-alpha.3'
require_line .ai-platform/docs/release-report.md '- Remote consumer verification: Passed'
require_line .ai-platform/docs/release-report.md '- Owner-selected redistribution license: Apache-2.0'
require_line .ai-platform/docs/release-report.md '- Target version: v0.1.0-alpha.3'

require_literal LICENSE 'Apache License'
require_literal NOTICE 'Modary'
require_literal README.md 'Every state-changing business path converges on `action.Runtime`'
require_literal README.md '`v0.1.0-alpha.3` is the PostgreSQL and durable-task pre-v1 Alpha release'
require_literal docs/framework-f0.md '`module.CapabilityTasks`'
require_literal docs/framework-f0.md 'same PostgreSQL transaction'
require_literal docs/concepts/persistence-and-tasks.md 'at least once'
require_literal docs/concepts/persistence-and-tasks.md 'idempotent'
require_literal docs/how-to/run-background-tasks.md 'RetryDelays'
require_literal docs/operations/security.md 'Handler error text'
require_literal docs/reference/support-matrix.md 'PostgreSQL 17'
require_literal docs/zh-CN/concepts/persistence-and-tasks.md 'at-least-once'
require_literal docs/zh-CN/how-to/run-background-tasks.md 'RetryDelays'
require_literal .ai-platform/memory/constitution.md 'distinct owned application and River schemas'
require_literal .ai-platform/docs/product-design.md '`v0.1.0-alpha.3` pre-v1 release contract'

legacy_paths='README.md SECURITY.md docs .ai-platform/docs .ai-platform/memory examples/counter'
if legacy=$(rg -n -i 'sqlite|modernc|databasepath|sqlitetest' $legacy_paths \
	--glob '!CHANGELOG.md' 2>/dev/null); then
	printf '%s\n' "$legacy" >&2
	fail 'active source-facing documents retain an embedded-database compatibility surface'
fi

final_mode=${MODARY_DOCS_FINAL:-0}
case "$final_mode" in
	0 | 1) ;;
	*) fail 'MODARY_DOCS_FINAL must be 0 or 1' ;;
esac

exit "$status"
