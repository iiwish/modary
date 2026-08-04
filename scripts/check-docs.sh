#!/bin/sh
set -eu

script_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
configured_root=${MODARY_DOCS_ROOT:-$script_root}
root=$(CDPATH= cd -- "$configured_root" && pwd -P)
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
docs/adr/ADR-005-create-only-profiles.md
docs/getting-started/choose-profile.md
docs/getting-started/admin-profile.md
docs/getting-started/governed-profile.md
docs/concepts/components-and-profiles.md
docs/concepts/persistence-and-tasks.md
docs/guides/rulary-bootstrap.md
docs/how-to/run-background-tasks.md
docs/operations/deployment.md
docs/operations/security.md
docs/operations/postgresql-backup-restore.md
docs/reference/packages.md
docs/reference/project-manifest.md
docs/reference/support-matrix.md
docs/releases/release-process.md
docs/releases/upgrade-guide.md
docs/releases/versioning.md
docs/zh-CN/index.md
docs/zh-CN/getting-started/choose-profile.md
docs/zh-CN/getting-started/quickstart.md
docs/zh-CN/getting-started/first-application.md
docs/zh-CN/getting-started/admin-profile.md
docs/zh-CN/getting-started/governed-profile.md
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
.ai-platform/specs/007-component-framework-refoundation/spec.md
.ai-platform/specs/007-component-framework-refoundation/plan.md
.ai-platform/specs/007-component-framework-refoundation/research.md
.ai-platform/specs/007-component-framework-refoundation/analysis.md
.ai-platform/specs/007-component-framework-refoundation/tasks.md
.ai-platform/specs/007-component-framework-refoundation/checklists/requirements.md
.ai-platform/specs/007-component-framework-refoundation/packets/T028.yaml
.ai-platform/specs/007-component-framework-refoundation/packets/T029.yaml
.ai-platform/specs/007-component-framework-refoundation/packets/T030.yaml
.ai-platform/specs/007-component-framework-refoundation/packets/T031.yaml
.ai-platform/specs/007-component-framework-refoundation/packets/T032.yaml
.ai-platform/specs/007-component-framework-refoundation/packets/T033.yaml
.ai-platform/specs/007-component-framework-refoundation/packets/T034.yaml
.ai-platform/specs/008-react-admin-starter/spec.md
.ai-platform/specs/008-react-admin-starter/plan.md
.ai-platform/specs/008-react-admin-starter/analysis.md
.ai-platform/specs/008-react-admin-starter/tasks.md
.ai-platform/specs/008-react-admin-starter/checklists/requirements.md
.ai-platform/specs/008-react-admin-starter/packets/T035.yaml
.ai-platform/specs/008-react-admin-starter/packets/T036.yaml
.ai-platform/specs/008-react-admin-starter/packets/T037.yaml
.ai-platform/specs/009-component-boundary-closure/spec.md
.ai-platform/specs/009-component-boundary-closure/plan.md
.ai-platform/specs/009-component-boundary-closure/analysis.md
.ai-platform/specs/009-component-boundary-closure/tasks.md
.ai-platform/specs/009-component-boundary-closure/checklists/requirements.md
.ai-platform/specs/009-component-boundary-closure/packets/T038.yaml
.ai-platform/specs/009-component-boundary-closure/packets/T039.yaml
.ai-platform/specs/009-component-boundary-closure/packets/T040.yaml
.ai-platform/specs/009-component-boundary-closure/packets/T041.yaml
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
.ai-platform/evidence/T027/release-notes.md
.ai-platform/evidence/T028/summary.md
.ai-platform/evidence/T028/diff.patch
.ai-platform/evidence/T028/test-results.md
.ai-platform/evidence/T028/review.md
.ai-platform/evidence/T029/summary.md
.ai-platform/evidence/T029/diff.patch
.ai-platform/evidence/T029/test-results.md
.ai-platform/evidence/T029/review.md
.ai-platform/evidence/T030/summary.md
.ai-platform/evidence/T030/diff.patch
.ai-platform/evidence/T030/test-results.md
.ai-platform/evidence/T030/review.md
.ai-platform/evidence/T031/summary.md
.ai-platform/evidence/T031/diff.patch
.ai-platform/evidence/T031/test-results.md
.ai-platform/evidence/T031/review.md
.ai-platform/evidence/T032/summary.md
.ai-platform/evidence/T032/diff.patch
.ai-platform/evidence/T032/test-results.md
.ai-platform/evidence/T032/review.md
.ai-platform/evidence/T033/summary.md
.ai-platform/evidence/T033/diff.patch
.ai-platform/evidence/T033/test-results.md
.ai-platform/evidence/T033/review.md
.ai-platform/evidence/T034/summary.md
.ai-platform/evidence/T034/diff.patch
.ai-platform/evidence/T034/test-results.md
.ai-platform/evidence/T034/review-product.md
.ai-platform/evidence/T034/review-engineering.md
.ai-platform/evidence/T034/external-acceptance.md
.ai-platform/evidence/T035/summary.md
.ai-platform/evidence/T035/diff.patch
.ai-platform/evidence/T035/test-results.md
.ai-platform/evidence/T035/review.md
.ai-platform/evidence/T036/summary.md
.ai-platform/evidence/T036/diff.patch
.ai-platform/evidence/T036/test-results.md
.ai-platform/evidence/T036/review.md
.ai-platform/evidence/T037/summary.md
.ai-platform/evidence/T037/diff.patch
.ai-platform/evidence/T037/test-results.md
.ai-platform/evidence/T037/review.md
.ai-platform/evidence/T037/external-acceptance.md
.ai-platform/evidence/T038/summary.md
.ai-platform/evidence/T038/diff.patch
.ai-platform/evidence/T038/test-results.md
.ai-platform/evidence/T038/review.md
.ai-platform/evidence/T039/summary.md
.ai-platform/evidence/T039/diff.patch
.ai-platform/evidence/T039/test-results.md
.ai-platform/evidence/T039/review.md
.ai-platform/evidence/T040/summary.md
.ai-platform/evidence/T040/diff.patch
.ai-platform/evidence/T040/test-results.md
.ai-platform/evidence/T040/review.md
.ai-platform/evidence/T041/summary.md
.ai-platform/evidence/T041/diff.patch
.ai-platform/evidence/T041/test-results.md
.ai-platform/evidence/T041/review.md'

for file in $required; do
	require_file "$file"
done

work_graph=.ai-platform/specs/005-postgres-task-runtime/tasks.md
current_graph=.ai-platform/docs/tasks.md
component_work_graph=.ai-platform/specs/009-component-boundary-closure/tasks.md
for task in T038 T039 T040 T041; do
	work_status=$(awk -v heading="## $task:" '
		index($0, heading) == 1 { active=1; next }
		/^## / { active=0 }
		active && /^Status: / { print substr($0, 9); exit }
	' "$component_work_graph")
	current_status=$(awk -F'|' -v task="$task" '
		function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
		$0 ~ "^\\| " task " \\|" { count++; value=trim($3) }
		END { if (count == 1) print value }
	' "$current_graph")
	if test "$work_status" != "$current_status"; then
		fail "$task must have one consistent current state: $work_status / $current_status"
	fi
	case "$work_status" in
		Completed)
			require_line ".ai-platform/evidence/$task/summary.md" '- Status: Completed'
			require_line ".ai-platform/evidence/$task/test-results.md" '- Result: Passed'
			require_line ".ai-platform/evidence/$task/review.md" '- Verdict: Pass'
			if test "$task" = T041; then
				for severity in P0 P1 P2; do
					require_line ".ai-platform/evidence/T041/review.md" "- $severity: 0"
				done
			fi
			;;
		*) fail "$task has an invalid delivery state: $work_status" ;;
	esac
done

t028_work_status=$(awk '
	index($0, "## T028:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/007-component-framework-refoundation/tasks.md)
t028_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T028 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t028_work_status" != "$t028_current_status"; then
	fail "T028 must have one consistent current state: $t028_work_status / $t028_current_status"
fi
case "$t028_work_status" in
	In\ Progress)
		require_line .ai-platform/evidence/T028/summary.md '- Status: In Progress'
		require_line .ai-platform/evidence/T028/test-results.md '- Result: In Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T028/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T028/test-results.md '- Result: Passed'
		require_line .ai-platform/evidence/T028/review.md '- Verdict: Pass'
		;;
	*) fail "T028 has an invalid delivery state: $t028_work_status" ;;
esac

t029_work_status=$(awk '
	index($0, "## T029:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/007-component-framework-refoundation/tasks.md)
t029_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T029 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t029_work_status" != "$t029_current_status"; then
	fail "T029 must have one consistent current state: $t029_work_status / $t029_current_status"
fi
case "$t029_work_status" in
	In\ Progress)
		require_line .ai-platform/evidence/T029/summary.md '- Status: In Progress'
		require_line .ai-platform/evidence/T029/test-results.md '- Result: In Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T029/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T029/test-results.md '- Result: Passed'
		require_line .ai-platform/evidence/T029/review.md '- Verdict: Pass'
		;;
	*) fail "T029 has an invalid delivery state: $t029_work_status" ;;
esac

t030_work_status=$(awk '
	index($0, "## T030:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/007-component-framework-refoundation/tasks.md)
t030_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T030 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t030_work_status" != "$t030_current_status"; then
	fail "T030 must have one consistent current state: $t030_work_status / $t030_current_status"
fi
case "$t030_work_status" in
	In\ Progress)
		require_line .ai-platform/evidence/T030/summary.md '- Status: In Progress'
		require_line .ai-platform/evidence/T030/test-results.md '- Result: In Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T030/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T030/test-results.md '- Result: Passed'
		require_line .ai-platform/evidence/T030/review.md '- Verdict: Pass'
		;;
	*) fail "T030 has an invalid delivery state: $t030_work_status" ;;
esac

t031_work_status=$(awk '
	index($0, "## T031:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/007-component-framework-refoundation/tasks.md)
t031_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T031 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t031_work_status" != "$t031_current_status"; then
	fail "T031 must have one consistent current state: $t031_work_status / $t031_current_status"
fi
case "$t031_work_status" in
	In\ Progress)
		require_line .ai-platform/evidence/T031/summary.md '- Status: In Progress'
		require_line .ai-platform/evidence/T031/test-results.md '- Result: In Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T031/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T031/test-results.md '- Result: Passed'
		require_line .ai-platform/evidence/T031/review.md '- Verdict: Pass'
		;;
	*) fail "T031 has an invalid delivery state: $t031_work_status" ;;
esac

t032_work_status=$(awk '
	index($0, "## T032:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/007-component-framework-refoundation/tasks.md)
t032_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T032 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t032_work_status" != "$t032_current_status"; then
	fail "T032 must have one consistent current state: $t032_work_status / $t032_current_status"
fi
case "$t032_work_status" in
	In\ Progress)
		require_line .ai-platform/evidence/T032/summary.md '- Status: In Progress'
		require_line .ai-platform/evidence/T032/test-results.md '- Result: In Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T032/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T032/test-results.md '- Result: Passed'
		require_line .ai-platform/evidence/T032/review.md '- Verdict: Pass'
		;;
	*) fail "T032 has an invalid delivery state: $t032_work_status" ;;
esac

t033_work_status=$(awk '
	index($0, "## T033:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/007-component-framework-refoundation/tasks.md)
t033_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T033 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t033_work_status" != "$t033_current_status"; then
	fail "T033 must have one consistent current state: $t033_work_status / $t033_current_status"
fi
case "$t033_work_status" in
	In\ Progress)
		require_line .ai-platform/evidence/T033/summary.md '- Status: In Progress'
		require_line .ai-platform/evidence/T033/test-results.md '- Result: In Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T033/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T033/test-results.md '- Result: Passed'
		require_line .ai-platform/evidence/T033/review.md '- Verdict: Pass'
		;;
	*) fail "T033 has an invalid delivery state: $t033_work_status" ;;
esac

t034_work_status=$(awk '
	index($0, "## T034:") == 1 { active=1; next }
	/^## / { active=0 }
	active && /^Status: / { print substr($0, 9); exit }
' .ai-platform/specs/007-component-framework-refoundation/tasks.md)
t034_current_status=$(awk -F'|' '
	function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
	$0 ~ "^\\| T034 \\|" { count++; value=trim($3) }
	END { if (count == 1) print value }
' "$current_graph")
if test "$t034_work_status" != "$t034_current_status"; then
	fail "T034 must have one consistent current state: $t034_work_status / $t034_current_status"
fi
case "$t034_work_status" in
	In\ Progress)
		require_line .ai-platform/evidence/T034/summary.md '- Status: In Progress'
		require_line .ai-platform/evidence/T034/test-results.md '- Result: In Progress'
		require_line .ai-platform/evidence/T034/external-acceptance.md '- Result: In Progress'
		;;
	Completed)
		require_line .ai-platform/evidence/T034/summary.md '- Status: Completed'
		require_line .ai-platform/evidence/T034/test-results.md '- Result: Passed'
		require_line .ai-platform/evidence/T034/review-product.md '- Verdict: Pass'
		require_line .ai-platform/evidence/T034/review-engineering.md '- Verdict: Pass'
		require_line .ai-platform/evidence/T034/external-acceptance.md '- Result: Passed'
		for review in .ai-platform/evidence/T034/review-product.md .ai-platform/evidence/T034/review-engineering.md; do
			for severity in P0 P1 P2; do
				require_line "$review" "- $severity: 0"
			done
		done
		;;
	*) fail "T034 has an invalid delivery state: $t034_work_status" ;;
esac

react_work_graph=.ai-platform/specs/008-react-admin-starter/tasks.md
for task in T035 T036 T037; do
	work_status=$(awk -v heading="## $task:" '
		index($0, heading) == 1 { active=1; next }
		/^## / { active=0 }
		active && /^Status: / { print substr($0, 9); exit }
	' "$react_work_graph")
	current_status=$(awk -F'|' -v task="$task" '
		function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
		$0 ~ "^\\| " task " \\|" { count++; value=trim($3) }
		END { if (count == 1) print value }
	' "$current_graph")
	if test "$work_status" != "$current_status"; then
		fail "$task must have one consistent current state: $work_status / $current_status"
	fi
	case "$work_status" in
		Running)
			require_line ".ai-platform/evidence/$task/summary.md" '- Status: In Progress'
			require_line ".ai-platform/evidence/$task/test-results.md" '- Result: In Progress'
			if test "$task" = T037; then
				require_line .ai-platform/evidence/T037/external-acceptance.md '- Result: In Progress'
			fi
			;;
		Completed)
			require_line ".ai-platform/evidence/$task/summary.md" '- Status: Completed'
			require_line ".ai-platform/evidence/$task/test-results.md" '- Result: Passed'
			require_line ".ai-platform/evidence/$task/review.md" '- Verdict: Pass'
			if test "$task" = T037; then
				require_line .ai-platform/evidence/T037/external-acceptance.md '- Result: Passed'
			fi
			;;
		*) fail "$task has an invalid delivery state: $work_status" ;;
	esac
done

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
require_line docs/f0-acceptance-report.md '- Distribution status: Not released'
require_line docs/f0-acceptance-report.md '- Target version: v0.2.0-alpha.1'
require_line docs/f0-acceptance-report.md '- Frozen baseline tag: v0.1.0-alpha.3'
require_line .ai-platform/docs/release-report.md '- Report version: 3.0'
require_line .ai-platform/docs/release-report.md '- Status: Distribution_ready'
require_line .ai-platform/docs/release-report.md '- Technical F0 acceptance: Accepted'
require_line .ai-platform/docs/release-report.md '- Engineering readiness: Accepted'
require_line .ai-platform/docs/release-report.md '- Distribution status: Not_released'
require_line .ai-platform/docs/release-report.md '- Version tags: None'
require_line .ai-platform/docs/release-report.md '- Remote consumer verification: Not_run'
require_line .ai-platform/docs/release-report.md '- Owner-selected redistribution license: Apache-2.0'
require_line .ai-platform/docs/release-report.md '- Target version: v0.2.0-alpha.1'

require_literal LICENSE 'Apache License'
require_literal NOTICE 'Modary'
require_literal README.md '`v0.2.0-alpha.1` is the current component-framework release.'
require_literal README.md 'This path is optional. Ordinary Admin CRUD does not need Preview or River.'
require_literal docs/framework-f0.md 'Core defines composition and lifecycle. Components add capabilities.'
require_literal docs/framework-f0.md 'This path is optional. Ordinary Admin CRUD does not need Preview or River.'
require_literal docs/f0-known-limitations.md 'The optional Admin Audit log is a bounded, scope-bound metadata view.'
require_literal docs/f0-acceptance-report.md '.ai-platform/specs/009-component-boundary-closure/spec.md'
require_literal docs/f0-acceptance-report.md '.ai-platform/evidence/T041/'
require_literal docs/concepts/persistence-and-tasks.md 'at least once'
require_literal docs/concepts/persistence-and-tasks.md 'idempotent'
require_literal docs/how-to/run-background-tasks.md 'RetryDelays'
require_literal docs/how-to/run-background-tasks.md 'StateQueued'
require_literal docs/operations/security.md 'Handler error text'
require_literal docs/reference/packages.md 'Governed uses `governedpostgres`'
require_literal docs/reference/support-matrix.md '| PostgreSQL | 17 used by integration acceptance |'
require_literal docs/reference/support-matrix.md '| Go | 1.26.5 or newer |'
require_literal docs/zh-CN/concepts/persistence-and-tasks.md 'at least once'
require_literal docs/zh-CN/how-to/run-background-tasks.md 'RetryDelays'
require_literal docs/guides/rulary-bootstrap.md 'Rulary is a separate product repository.'
require_literal .ai-platform/memory/constitution.md 'The empty Core has no database'
require_literal .ai-platform/docs/product-design.md 'Start with a small Go application. Add only the components the product needs.'
require_literal .ai-platform/docs/product-design.md '`v0.1.0-alpha.3` is the immutable accepted PostgreSQL and Governed Action'

legacy_paths='README.md SECURITY.md docs .ai-platform/docs .ai-platform/memory examples/counter'
if legacy=$(rg -n -i 'use (the )?sqlite|sqlite adapter is (supported|available)|modernc|databasepath|sqlitetest' $legacy_paths \
	--glob '!CHANGELOG.md' 2>/dev/null); then
	printf '%s\n' "$legacy" >&2
	fail 'active source-facing documents retain an embedded-database compatibility surface'
fi

final_mode=${MODARY_DOCS_FINAL:-0}
case "$final_mode" in
	0 | 1) ;;
	*) fail 'MODARY_DOCS_FINAL must be 0 or 1' ;;
esac

if test "$root" = "$script_root" && ! "$script_root/scripts/check-acceptance-evidence.sh" "$root"; then
	status=1
fi

exit "$status"
