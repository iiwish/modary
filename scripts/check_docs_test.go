package scripts_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var docsFixtureFiles = []string{
	"README.md",
	"LICENSE",
	"NOTICE",
	"docs/framework-f0.md",
	"docs/f0-known-limitations.md",
	"docs/f0-acceptance-report.md",
	"docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md",
	"docs/adr/ADR-002-governed-action-transaction.md",
	"docs/adr/ADR-003-sqlite-and-module-migrations.md",
	"docs/adr/ADR-004-consumer-owned-surfaces.md",
	".ai-platform/docs/product-design.md",
	".ai-platform/docs/technology-decision-record.md",
	".ai-platform/docs/tasks.md",
	".ai-platform/docs/release-report.md",
	".ai-platform/memory/constitution.md",
	".ai-platform/evidence/T013/summary.md",
	".ai-platform/evidence/T015/summary.md",
	".ai-platform/specs/002-framework-decoupling/spec.md",
	".ai-platform/specs/002-framework-decoupling/plan.md",
	".ai-platform/specs/002-framework-decoupling/analysis.md",
	".ai-platform/specs/002-framework-decoupling/tasks.md",
	".ai-platform/specs/002-framework-decoupling/checklists/requirements.md",
	".ai-platform/specs/002-framework-decoupling/packets/T010.yaml",
	".ai-platform/specs/002-framework-decoupling/packets/T011.yaml",
	".ai-platform/specs/002-framework-decoupling/packets/T012.yaml",
	".ai-platform/specs/002-framework-decoupling/packets/T013.yaml",
	".ai-platform/specs/002-framework-decoupling/packets/T014.yaml",
	".ai-platform/specs/002-framework-decoupling/packets/T015.yaml",
	".ai-platform/specs/002-framework-decoupling/packets/T016.yaml",
	"examples/counter/README.md",
	"scripts/check-docs.sh",
	"scripts/review-source-state.sh",
	"scripts/source-state.sh",
}

func TestCheckDocsAllowsConsistentInProgressStateButNotFinalMode(t *testing.T) {
	repository := newDocsFixture(t, false)
	if output, err := runDocsCheck(t, repository, false); err != nil {
		t.Fatalf("consistent in-progress docs failed: %v\n%s", err, output)
	}

	output, err := runDocsCheck(t, repository, true)
	if err == nil || !strings.Contains(output, "final documentation mode requires T016 Completed") {
		t.Fatalf("final check = %v, output=%q", err, output)
	}
}

func TestCheckDocsFinalModeAcceptsCompleteFixture(t *testing.T) {
	repository := newLiveDocsFixture(t, true)
	if output, err := runDocsCheck(t, repository, false); err != nil {
		t.Fatalf("completed state did not activate final checks: %v\n%s", err, output)
	}
	if output, err := runDocsCheck(t, repository, true); err != nil {
		t.Fatalf("complete final docs failed: %v\n%s", err, output)
	}
}

func TestCheckDocsCompletedStateCannotBypassFinalEvidence(t *testing.T) {
	repository := newDocsFixture(t, true)
	if err := os.Remove(filepath.Join(repository, ".ai-platform/evidence/T016/review-2.md")); err != nil {
		t.Fatal(err)
	}
	output, err := runDocsCheck(t, repository, false)
	if err == nil || !strings.Contains(output, "must be a non-empty regular file") {
		t.Fatalf("completed default check = %v, output=%q", err, output)
	}
}

func TestCheckDocsTreatsCompletedF0EvidenceAsHistorical(t *testing.T) {
	repository := newLiveDocsFixture(t, true)
	appendDocsFixture(t, filepath.Join(repository, "README.md"), "\nrelease-readiness documentation\n")

	if output, err := runDocsCheck(t, repository, false); err != nil {
		t.Fatalf("post-F0 documentation invalidated historical evidence: %v\n%s", err, output)
	}
}

func TestCheckDocsAllowsAnewDeliveryAfterF0Closure(t *testing.T) {
	repository := newLiveDocsFixture(t, true)
	appendDocsFixture(t, filepath.Join(repository, ".ai-platform", "docs", "tasks.md"),
		"\n## T020: Later Delivery\n\nStatus: In_Progress\n")
	appendDocsFixture(t, filepath.Join(repository, ".ai-platform", "docs", "release-report.md"),
		"\n- Engineering readiness: In_Progress\n")

	if output, err := runDocsCheck(t, repository, false); err != nil {
		t.Fatalf("new delivery was mistaken for unfinished T016 closure: %v\n%s", err, output)
	}
}

func TestCheckDocsFinalModeFailsClosed(t *testing.T) {
	// Each case launches the real POSIX gate. Bound process fan-out so a loaded
	// builder measures the checker rather than scheduler and filesystem thrash.
	parallelLimit := make(chan struct{}, 4)
	tests := []struct {
		name            string
		mutate          func(*testing.T, string)
		want            string
		liveReviewState bool
	}{
		{
			name: "missing evidence",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				if err := os.Remove(filepath.Join(repository, ".ai-platform/evidence/T016/test-results.md")); err != nil {
					t.Fatal(err)
				}
			},
			want: "must be a non-empty regular file",
		},
		{
			name: "empty patch",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				writeDocsFixtureFile(t, filepath.Join(repository, ".ai-platform/evidence/T016/diff.patch"), "")
			},
			want: "must be a non-empty regular file",
		},
		{
			name: "incomplete summary",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T016/summary.md"), "- Status: Completed", "- Status: Incomplete")
			},
			want: "- Status: Completed field",
		},
		{
			name: "failed test result",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T016/test-results.md"), "- Result: Passed", "- Result: Failed")
			},
			want: "- Result: Passed field",
		},
		{
			name: "failed review verdict",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T016/review-1.md"), "- Verdict: Pass", "- Verdict: Fail")
			},
			want: "- Verdict: Pass field",
		},
		{
			name: "nonzero review finding",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T016/review-1.md"), "- P1: 0", "- P1: 1")
			},
			want: "zero P1 result",
		},
		{
			name: "ambiguous review finding",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T016/review-1.md"), "- P1: unresolved\n")
			},
			want: "zero P1 result",
		},
		{
			name: "review tree mismatch",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				review := filepath.Join(repository, ".ai-platform/evidence/T016/review-1.md")
				frozenTree := docsFixtureMetadata(t, filepath.Join(repository, ".ai-platform/evidence/T016/summary.md"), "Frozen tree")
				replaceDocsFixture(t, review, frozenTree, "sha256:"+strings.Repeat("1", 64))
			},
			want: "must reference the reviewed frozen tree",
		},
		{
			name: "evidence patch is not frozen source state",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T016/diff.patch"), "self-reported but unverified\n")
			},
			want:            "differs from the accepted F0 commit",
			liveReviewState: true,
		},
		{
			name: "duplicate reviewer",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				review := filepath.Join(repository, ".ai-platform/evidence/T016/review-2.md")
				replaceDocsFixture(t, review, "- Reviewer: reviewer-two", "- Reviewer: reviewer-one")
			},
			want: "two different independent reviewers",
		},
		{
			name: "review predates freeze",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				review := filepath.Join(repository, ".ai-platform/evidence/T016/review-1.md")
				replaceDocsFixture(t, review, "- Started at: 2026-07-31T12:00:01Z", "- Started at: 2026-07-31T11:59:59Z")
			},
			want: "start at or after the implementation freeze",
		},
		{
			name: "tests predate freeze",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				results := filepath.Join(repository, ".ai-platform/evidence/T016/test-results.md")
				replaceDocsFixture(t, results, "- Completed at: 2026-07-31T12:10:00Z", "- Completed at: 2026-07-31T11:59:59Z")
			},
			want: "must complete at or after the implementation freeze",
		},
		{
			name: "impossible UTC timestamp",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				results := filepath.Join(repository, ".ai-platform/evidence/T016/test-results.md")
				replaceDocsFixture(t, results, "- Completed at: 2026-07-31T12:10:00Z", "- Completed at: 2026-99-31T12:10:00Z")
			},
			want: "must be a real second-precision UTC date and time",
		},
		{
			name: "task graph disagreement",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/docs/tasks.md"), "| T016 | Completed |", "| T016 | In_Progress |")
			},
			want: "state differs between canonical task graphs",
		},
		{
			name: "missing release boundary",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/docs/release-report.md"), "- Version tag: None", "- Version tag: v0.0.0")
			},
			want: "- Version tag: None",
		},
		{
			name: "ambiguous report version",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/docs/release-report.md"), "- Report version: 1.0", "- Version: 1.0")
			},
			want: "- Report version: 1.0 field",
		},
		{
			name: "unfinished closure marker",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T016/summary.md"), "\nPending\n")
			},
			want: "must not retain Pending or In_Progress",
		},
		{
			name: "nonstrict validator",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				packet := filepath.Join(repository, ".ai-platform/specs/002-framework-decoupling/packets/T014.yaml")
				replaceDocsFixture(t, packet, "--task-id T014 --strict", "--task-id T014")
			},
			want: "--task-id T014 --strict",
		},
		{
			name: "nonexistent repository instructions",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/specs/002-framework-decoupling/packets/T015.yaml"), "\nlocal_instructions: [AGENTS.md]\n")
			},
			want: "AGENTS.md",
		},
		{
			name: "generic POSIX mode privacy",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "effective-UID-owned with exact mode `0700`", "private on every POSIX system with mode `0700`")
			},
			want: "private on every POSIX system",
		},
		{
			name: "unsupported platform Build fallback",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "Every other platform,\nincluding other Unix variants and Windows, fails Build.\nF0 has no validated ACL policy there.", "Every platform runs Build with mode-only checks.")
			},
			want: "Every other platform",
		},
		{
			name: "unsupported native runtime overclaim",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "F0 claims no native Build, ACL, or rename runtime validation for them.", "F0 claims native Build, ACL, and rename runtime validation for them.")
			},
			want: "no native Build",
		},
		{
			name: "Darwin inherited ACL fallback",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/adr/ADR-004-consumer-owned-surfaces.md")
				replaceDocsFixture(t, file, "Darwin also rejects\nany extended ACL at every retained level.", "Darwin accepts inherited access lists at every retained level.")
			},
			want: "accepts inherited access lists at every retained level",
		},
		{
			name: "project owned TMPDIR",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "rejects it when it is inside\nor resolves through a symlink into the project", "accepts it inside\nor through a symlink into the project")
			},
			want: "accepts it inside",
		},
		{
			name: "ambient toolchain selection",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, ".ai-platform/evidence/T013/summary.md")
				replaceDocsFixture(t, file, "`GOTOOLCHAIN=local`", "`GOTOOLCHAIN=auto`")
			},
			want: "`GOTOOLCHAIN=local`",
		},
		{
			name: "mutable module build",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, ".ai-platform/evidence/T015/summary.md")
				replaceDocsFixture(t, file, "`-mod=readonly`", "`-mod=mod`")
			},
			want: "`-mod=readonly`",
		},
		{
			name: "missing GOTMPDIR pin",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "`TMPDIR` and `GOTMPDIR`", "`TMPDIR` alone")
				replaceDocsFixture(t, file, "An ambient `GOTMPDIR` cannot override that parent.", "The canonical parent remains fixed.")
			},
			want: "`GOTMPDIR`",
		},
		{
			name: "inherited temporary environment duplicates",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "removes every inherited\ncase-variant", "keeps inherited duplicate\ncase-variant")
			},
			want: "removes every inherited",
		},
		{
			name: "ambient GOTMPDIR override",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, "docs/framework-f0.md"), "\nAn ambient `GOTMPDIR` may override that parent.\n")
			},
			want: "ambient `GOTMPDIR` may override that parent",
		},
		{
			name: "mismatched child temporary directories",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T013/summary.md"), "\n`TMPDIR` and `GOTMPDIR` may point to different directories\n")
			},
			want: "`TMPDIR` and `GOTMPDIR` may point to different directories",
		},
		{
			name: "temporary environment regression omission",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, ".ai-platform/specs/002-framework-decoupling/plan.md")
				replaceDocsFixture(t, file, "A fake Go regression receives a symlink alias through ambient `TMPDIR` and a\n  malicious project-path `GOTMPDIR`, and observes the same canonical staging\n  parent in both child variables.", "A fake Go regression observes already-canonical temporary variables.")
			},
			want: "both child variables",
		},
		{
			name: "trusted inputs sandbox overclaim",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "consumer source remain trusted inputs; Build is not a sandbox.", "consumer source are fully sandboxed.")
			},
			want: "fully sandboxed",
		},
		{
			name: "daemonized descendant containment overclaim",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "A trusted descendant that daemonizes or enters\nanother process group can escape cleanup", "all descendants are contained")
			},
			want: "all descendants are contained",
		},
		{
			name: "process group cleanup omission",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "Build kills residual same-group\ndescendants", "Build only observes same-group\ndescendants")
			},
			want: "kills residual same-group",
		},
		{
			name: "TMPDIR ancestor chain omission",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "that directory and every ancestor through `/`", "only that directory's immediate parent")
			},
			want: "every ancestor through `/`",
		},
		{
			name: "unsafe writable TMPDIR ancestry",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "group- or other-writable only when root-owned and sticky", "group- or other-writable whenever sticky regardless of owner")
			},
			want: "sticky regardless of owner",
		},
		{
			name: "compiler leader reaped before cleanup",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "observes the group leader without reaping it", "observes and reaps it before cleanup")
			},
			want: "without reaping",
		},
		{
			name: "WaitDelay misclassifies successful Build",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, "docs/framework-f0.md"), "\nexec.ErrWaitDelay fails an otherwise successful Build\n")
			},
			want: "exec.ErrWaitDelay fails an otherwise successful Build",
		},
		{
			name: "unsupported token path filesystem access",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, ".ai-platform/specs/002-framework-decoupling/plan.md")
				replaceDocsFixture(t, file, "before any filesystem access", "after filesystem access")
			},
			want: "before any filesystem access",
		},
		{
			name: "generic POSIX token support",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T015/summary.md"), "\nPOSIX token files are supported\n")
			},
			want: "obsolete canonical statement remains",
		},
		{
			name: "caller error method omission",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "`Error`, `Is`, `As`, or `Unwrap`", "`Is`, `As`, or `Unwrap`")
			},
			want: "`Error`, `Is`, `As`, or `Unwrap`",
		},
		{
			name: "internal error helper requirement",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T015/summary.md"), "\nexternal consumer must import an internal helper\n")
			},
			want: "external consumer must import an internal helper",
		},
		{
			name: "blocked appcmd writer interruption claim",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/f0-known-limitations.md")
				replaceDocsFixture(t, file, "cannot interrupt a blocked writer", "can interrupt a blocked writer")
			},
			want: "can interrupt a blocked writer",
		},
		{
			name: "SQLite accepts arbitrary sticky ancestor",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, "docs/adr/ADR-003-sqlite-and-module-migrations.md"), "\nany sticky writable ancestor is accepted\n")
			},
			want: "any sticky writable ancestor is accepted",
		},
		{
			name: "SQLite accepts foreign read-only ancestor",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, "docs/adr/ADR-003-sqlite-and-module-migrations.md"), "\nforeign-owned read-only ancestors are accepted\n")
			},
			want: "foreign-owned read-only ancestors are accepted",
		},
		{
			name: "stale generic POSIX staging claim",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T013/summary.md"), "\nPOSIX compiler staging requires owner-only mode `0700`\n")
			},
			want: "obsolete canonical statement remains",
		},
		{
			name: "pathname keyed lock claim",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "not by pathname spelling", "by pathname spelling")
			},
			want: "not by pathname spelling",
		},
		{
			name: "interruptible caller writer claim",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/f0-known-limitations.md")
				replaceDocsFixture(t, file, "Caller-supplied `io.Writer.Write` must return", "Caller-supplied `io.Writer.Write` can be interrupted")
			},
			want: "Caller-supplied `io.Writer.Write` must return",
		},
		{
			name: "Action JSON node budget drift",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "65,536 JSON value nodes", "65,535 JSON value nodes")
			},
			want: "65,536 JSON value nodes",
		},
		{
			name: "Action JSON exact-boundary acceptance omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, ".ai-platform/specs/002-framework-decoupling/spec.md")
				replaceDocsFixture(t, file, "Exact-boundary tests accept each Action JSON limit exactly", "Boundary tests cover Action JSON limits")
			},
			want: "Exact-boundary tests accept each Action JSON limit exactly",
		},
		{
			name: "Action JSON envelope budget merged",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "independent byte budgets", "one shared Action byte budget")
			},
			want: "independent byte budgets",
		},
		{
			name: "Action JSON envelope default reduced",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/f0-known-limitations.md")
				replaceDocsFixture(t, file, "2 MiB defaults", "1 MiB defaults")
			},
			want: "2 MiB defaults",
		},
		{
			name: "schema node budget drift",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "2,048 schema nodes", "2,047 schema nodes")
			},
			want: "2,048",
		},
		{
			name: "schema graph contract omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "one immutable executable SchemaGraph", "one schema compilation model")
			},
			want: "SchemaGraph",
		},
		{
			name: "offline pinned metaschema contract omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "validated offline against an embedded Draft 7 metaschema", "validated against a dependency schema")
			},
			want: "Draft 7 metaschema",
		},
		{
			name: "exact-number rewriting reintroduced",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, "docs/framework-f0.md"), "\nMCP uses exact-number rewriting before compilation.\n")
			},
			want: "obsolete canonical statement remains",
		},
		{
			name: "schema evaluation frame budget drift",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "4,096 active evaluation frames", "4,095 active evaluation frames")
			},
			want: "4,096 active evaluation frames",
		},
		{
			name: "official schema corpus accounting omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "223 cases and 856 tests", "representative cases and tests")
			},
			want: "223",
		},
		{
			name: "schema flag-only contract omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "Validation is flag-only", "Validation returns dependency diagnostics")
			},
			want: "flag-only",
		},
		{
			name: "MCP schema wrapper budget drift",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "Its fixed wrapper adds exactly\n128 schema nodes", "Its fixed wrapper adds exactly\n129 schema nodes")
			},
			want: "128",
		},
		{
			name: "protocol missing-input contract omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "A missing member is a protocol\nvalidation failure", "A missing member is accepted by the protocol")
			},
			want: "A missing member is a protocol",
		},
		{
			name: "open capability contract omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/framework-f0.md")
				replaceDocsFixture(t, file, "`module.Capability` is an open named string type", "Capabilities are closed framework strings")
			},
			want: "`module.Capability`",
		},
		{
			name: "Host startup reference ownership omitted",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, "docs/f0-known-limitations.md")
				replaceDocsFixture(t, file, "Start callbacks, handler factories,\n    and migration filesystem references", "startup implementation values")
			},
			want: "Start callbacks",
		},
		{
			name: "stale root confinement evidence",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				appendDocsFixture(t, filepath.Join(repository, ".ai-platform/evidence/T013/summary.md"), "\nroot-confined build\n")
			},
			want: "obsolete canonical statement remains",
		},
		{
			name: "stale Go-only evidence",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				file := filepath.Join(repository, ".ai-platform/evidence/T015/summary.md")
				replaceDocsFixture(t, file, "Node-free Go framework Make and CI", "Go-only Make and CI")
			},
			want: "Node-free Go framework Make and CI gates",
		},
	}

	if runtime.GOOS != "windows" {
		tests = append(tests, struct {
			name            string
			mutate          func(*testing.T, string)
			want            string
			liveReviewState bool
		}{
			name: "symlink evidence",
			mutate: func(t *testing.T, repository string) {
				t.Helper()
				name := filepath.Join(repository, ".ai-platform/evidence/T016/review-2.md")
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("review-1.md", name); err != nil {
					t.Fatal(err)
				}
			},
			want: "must be a non-empty regular file",
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parallelLimit <- struct{}{}
			defer func() { <-parallelLimit }()
			repository := newDocsFixtureMode(t, true, test.liveReviewState)
			test.mutate(t, repository)
			output, err := runDocsCheck(t, repository, true)
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("check = %v, output=%q, want %q", err, output, test.want)
			}
		})
	}
}

func newDocsFixture(t *testing.T, final bool) string {
	return newDocsFixtureMode(t, final, false)
}

func newLiveDocsFixture(t *testing.T, final bool) string {
	return newDocsFixtureMode(t, final, true)
}

func newDocsFixtureMode(t *testing.T, final, liveReviewState bool) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := filepath.Dir(workingDirectory)
	repository := t.TempDir()
	for _, relative := range docsFixtureFiles {
		content, readErr := os.ReadFile(filepath.Join(sourceRoot, relative))
		if readErr != nil {
			t.Fatalf("read fixture source %s: %v", relative, readErr)
		}
		writeDocsFixtureFile(t, filepath.Join(repository, relative), string(content))
	}
	if err := os.MkdirAll(filepath.Join(repository, "examples/counter"), 0o755); err != nil {
		t.Fatal(err)
	}
	normalizeDocsFixture(t,
		filepath.Join(repository, ".ai-platform/specs/002-framework-decoupling/tasks.md"),
		"## T016: Full Review And F0 Acceptance\n\nStatus: Completed",
		"## T016: Full Review And F0 Acceptance\n\nStatus: In_Progress",
	)
	normalizeDocsFixture(t,
		filepath.Join(repository, ".ai-platform/docs/tasks.md"),
		"| T016 | Completed |",
		"| T016 | In_Progress |",
	)
	normalizeDocsFixture(t,
		filepath.Join(repository, ".ai-platform/docs/tasks.md"),
		"## T016: Current Framework F0 Acceptance\n\nStatus: Completed",
		"## T016: Current Framework F0 Acceptance\n\nStatus: In_Progress",
	)
	writeDocsFixtureFile(t, filepath.Join(repository, "docs/f0-acceptance-report.md"), "# F0 Acceptance\n\n- Status: In_Progress\n")
	writeDocsFixtureFile(t, filepath.Join(repository, ".ai-platform/docs/release-report.md"), `# F0 Release

- Report version: 1.0
- Status: In_Progress
- Technical F0 acceptance: In_Progress
- Distribution status: Not_released
- Version tag: None
- Owner-selected redistribution license: Apache-2.0
`)

	if !final {
		return repository
	}

	replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/specs/002-framework-decoupling/tasks.md"), "Status: In_Progress", "Status: Completed")
	replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/docs/tasks.md"), "| T016 | In_Progress |", "| T016 | Completed |")
	replaceDocsFixture(t, filepath.Join(repository, ".ai-platform/docs/tasks.md"),
		"## T016: Current Framework F0 Acceptance\n\nStatus: In_Progress",
		"## T016: Current Framework F0 Acceptance\n\nStatus: Completed")

	writeDocsFixtureFile(t, filepath.Join(repository, "docs/f0-acceptance-report.md"), `# F0 Acceptance

- Status: Accepted
- Distribution status: Not_released
- Version tag: None
- Owner-selected redistribution license: None
`)
	writeDocsFixtureFile(t, filepath.Join(repository, ".ai-platform/docs/release-report.md"), `# F0 Release

- Report version: 1.0
- Status: Accepted
- Technical F0 acceptance: Accepted
- Distribution status: Not_released
- Version tag: None
- Owner-selected redistribution license: Apache-2.0
`)

	if !liveReviewState {
		writeDocsFixtureFile(t, filepath.Join(repository, "scripts/review-source-state.sh"), `#!/bin/sh
set -eu
cat .ai-platform/evidence/T016/diff.patch
`)
	}
	reviewState := []byte("canonical non-live fixture review state\n")
	if liveReviewState {
		initializeDocsFixtureRepository(t, repository)
		reviewState = captureDocsFixtureReviewState(t, repository)
	}
	sum := sha256.Sum256(reviewState)
	fixtureFrozenTree := "sha256:" + hex.EncodeToString(sum[:])

	evidence := filepath.Join(repository, ".ai-platform/evidence/T016")
	writeDocsFixtureFile(t, filepath.Join(evidence, "summary.md"), "# T016 Summary\n\n- Status: Completed\n- Frozen tree: "+fixtureFrozenTree+"\n- Frozen at: 2026-07-31T12:00:00Z\n")
	writeDocsFixtureFile(t, filepath.Join(evidence, "diff.patch"), string(reviewState))
	writeDocsFixtureFile(t, filepath.Join(evidence, "test-results.md"), "# T016 Tests\n\n- Result: Passed\n- Frozen tree: "+fixtureFrozenTree+"\n- Completed at: 2026-07-31T12:10:00Z\n")
	for index, review := range []string{"review-1.md", "review-2.md"} {
		reviewer := "reviewer-one"
		if index == 1 {
			reviewer = "reviewer-two"
		}
		writeDocsFixtureFile(t, filepath.Join(evidence, review), "# Review\n\n- Reviewer: "+reviewer+"\n- Started at: 2026-07-31T12:00:01Z\n- Frozen tree: "+fixtureFrozenTree+"\n- Scope: complete F0 architecture and implementation\n- Commands: focused tests and repository gates\n- Verdict: Pass\n- P0: 0\n- P1: 0\n- P2: 0\n")
	}
	acceptedCommit := commitDocsFixture(t, repository)
	appendDocsFixture(t, filepath.Join(repository, "docs/f0-acceptance-report.md"), "- Accepted commit: "+acceptedCommit+"\n")
	return repository
}

func commitDocsFixture(t *testing.T, repository string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repository, ".git")); os.IsNotExist(err) {
		runDocsFixtureGit(t, repository, "init", "--quiet")
	}
	runDocsFixtureGit(t, repository, "add", "--all")
	runDocsFixtureGit(t, repository, "-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid",
		"commit", "--quiet", "-m", "accepted evidence")
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve fixture commit: %v\n%s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func runDocsFixtureGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func initializeDocsFixtureRepository(t *testing.T, repository string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	run("init", "--quiet")
	run("add", "--all")
	run("-c", "user.name=Modary Test", "-c", "user.email=modary@example.invalid",
		"commit", "--quiet", "-m", "fixture")
}

func captureDocsFixtureReviewState(t *testing.T, repository string) []byte {
	t.Helper()
	checker := filepath.Join(repository, "scripts/review-source-state.sh")
	arguments := []string{"--exclude-t016-evidence"}
	command := exec.Command("sh", append([]string{checker}, arguments...)...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("capture fixture review state: %v\n%s", err, output)
	}
	return output
}

func docsFixtureMetadata(t *testing.T, name, key string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "- " + key + ": "
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("%s has no %s metadata", name, key)
	return ""
}

func runDocsCheck(t *testing.T, repository string, final bool) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", filepath.Join(repository, "scripts/check-docs.sh"))
	command.Dir = repository
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "MODARY_DOCS_FINAL=") {
			command.Env = append(command.Env, variable)
		}
	}
	mode := "0"
	if final {
		mode = "1"
	}
	command.Env = append(command.Env, "MODARY_DOCS_FINAL="+mode)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("docs check timed out: %v", ctx.Err())
	}
	return string(output), err
}

func replaceDocsFixture(t *testing.T, name, old, replacement string) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), old) != 1 {
		t.Fatalf("%s: expected exactly one %q", name, old)
	}
	writeDocsFixtureFile(t, name, strings.Replace(string(content), old, replacement, 1))
}

func normalizeDocsFixture(t *testing.T, name, alternate, canonical string) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	switch {
	case strings.Count(text, canonical) == 1 && strings.Count(text, alternate) == 0:
		return
	case strings.Count(text, canonical) == 0 && strings.Count(text, alternate) == 1:
		writeDocsFixtureFile(t, name, strings.Replace(text, alternate, canonical, 1))
	default:
		t.Fatalf("%s: expected exactly one of %q or %q", name, alternate, canonical)
	}
}

func appendDocsFixture(t *testing.T, name, suffix string) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	writeDocsFixtureFile(t, name, string(content)+suffix)
}

func writeDocsFixtureFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
