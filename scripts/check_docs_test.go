package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var currentDocsFiles = []string{
	"README.md", "LICENSE", "NOTICE", "SECURITY.md",
	"docs/index.md", "docs/framework-f0.md", "docs/f0-known-limitations.md", "docs/f0-acceptance-report.md",
	"docs/adr/ADR-001-explicit-composition-and-capability-lifecycle.md",
	"docs/adr/ADR-002-governed-action-transaction.md",
	"docs/adr/ADR-003-postgresql-and-module-migrations.md",
	"docs/adr/ADR-004-consumer-owned-surfaces.md",
	"docs/concepts/persistence-and-tasks.md", "docs/how-to/run-background-tasks.md",
	"docs/operations/deployment.md", "docs/operations/security.md", "docs/operations/postgresql-backup-restore.md",
	"docs/reference/packages.md", "docs/reference/support-matrix.md",
	"docs/zh-CN/index.md", "docs/zh-CN/getting-started/quickstart.md", "docs/zh-CN/getting-started/first-application.md",
	"docs/zh-CN/concepts/persistence-and-tasks.md", "docs/zh-CN/how-to/run-background-tasks.md",
	".ai-platform/memory/constitution.md", ".ai-platform/docs/product-design.md",
	".ai-platform/docs/technology-decision-record.md", ".ai-platform/docs/tasks.md", ".ai-platform/docs/release-report.md",
	".ai-platform/specs/005-postgres-task-runtime/spec.md", ".ai-platform/specs/005-postgres-task-runtime/plan.md",
	".ai-platform/specs/005-postgres-task-runtime/analysis.md", ".ai-platform/specs/005-postgres-task-runtime/tasks.md",
	".ai-platform/specs/005-postgres-task-runtime/checklists/requirements.md",
	".ai-platform/specs/005-postgres-task-runtime/packets/T024.yaml",
	".ai-platform/specs/005-postgres-task-runtime/packets/T025.yaml",
	".ai-platform/specs/005-postgres-task-runtime/packets/T026.yaml",
	".ai-platform/specs/006-postgres-alpha-release/spec.md", ".ai-platform/specs/006-postgres-alpha-release/plan.md",
	".ai-platform/specs/006-postgres-alpha-release/analysis.md", ".ai-platform/specs/006-postgres-alpha-release/tasks.md",
	".ai-platform/specs/006-postgres-alpha-release/checklists/requirements.md",
	".ai-platform/specs/006-postgres-alpha-release/packets/T027.yaml",
	".ai-platform/evidence/T024/summary.md", ".ai-platform/evidence/T024/diff.patch", ".ai-platform/evidence/T024/test-results.md",
	".ai-platform/evidence/T025/summary.md", ".ai-platform/evidence/T025/diff.patch", ".ai-platform/evidence/T025/test-results.md",
	".ai-platform/evidence/T026/summary.md", ".ai-platform/evidence/T026/diff.patch", ".ai-platform/evidence/T026/test-results.md",
	".ai-platform/evidence/T026/review-1.md", ".ai-platform/evidence/T026/review-2.md",
	".ai-platform/evidence/T026/review-3.md", ".ai-platform/evidence/T026/review-4.md",
	".ai-platform/evidence/T027/summary.md", ".ai-platform/evidence/T027/diff.patch",
	".ai-platform/evidence/T027/test-results.md", ".ai-platform/evidence/T027/review.md",
	".ai-platform/evidence/T027/release-notes.md",
}

func TestCheckDocsAcceptsCurrentCanonicalState(t *testing.T) {
	repository := currentDocsFixture(t)
	if output, err := runCurrentDocsCheck(t, repository); err != nil {
		t.Fatalf("check-docs failed: %v\n%s", err, output)
	}
}

func TestCheckDocsRejectsMissingCanonicalDocument(t *testing.T) {
	repository := currentDocsFixture(t)
	if err := os.Remove(filepath.Join(repository, "docs/concepts/persistence-and-tasks.md")); err != nil {
		t.Fatal(err)
	}
	output, err := runCurrentDocsCheck(t, repository)
	if err == nil || !strings.Contains(output, "non-empty regular file") {
		t.Fatalf("check-docs = %v, output=%q", err, output)
	}
}

func TestCheckDocsRejectsCurrentTaskStateDrift(t *testing.T) {
	repository := currentDocsFixture(t)
	path := filepath.Join(repository, ".ai-platform/docs/tasks.md")
	replaceCurrentDocs(t, path, "| T025 | Completed |", "| T025 | In_Progress |")
	output, err := runCurrentDocsCheck(t, repository)
	if err == nil || !strings.Contains(output, "T025 must be Completed") {
		t.Fatalf("check-docs = %v, output=%q", err, output)
	}
}

func TestCheckDocsRejectsLegacyStorageInActiveDocs(t *testing.T) {
	repository := currentDocsFixture(t)
	path := filepath.Join(repository, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\nUse the SQLite adapter.\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runCurrentDocsCheck(t, repository)
	if err == nil || !strings.Contains(output, "compatibility surface") {
		t.Fatalf("check-docs = %v, output=%q", err, output)
	}
}

func TestCheckDocsRejectsUnresolvedAcceptanceFinding(t *testing.T) {
	repository := currentDocsFixture(t)
	path := filepath.Join(repository, ".ai-platform/evidence/T026/review-4.md")
	replaceCurrentDocs(t, path, "- P1: 0", "- P1: 1")
	output, err := runCurrentDocsCheck(t, repository)
	if err == nil || !strings.Contains(output, "- P1: 0") {
		t.Fatalf("check-docs = %v, output=%q", err, output)
	}
}

func currentDocsFixture(t *testing.T) string {
	t.Helper()
	root := repositoryRoot(t)
	destination := t.TempDir()
	for _, name := range currentDocsFiles {
		source := filepath.Join(root, name)
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		target := filepath.Join(destination, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(destination, "examples/counter"), 0o755); err != nil {
		t.Fatal(err)
	}
	return destination
}

func runCurrentDocsCheck(t *testing.T, repository string) (string, error) {
	t.Helper()
	command := exec.Command("sh", filepath.Join(repositoryRoot(t), "scripts/check-docs.sh"))
	command.Env = append(os.Environ(), "MODARY_DOCS_ROOT="+repository, "MODARY_DOCS_FINAL=1")
	output, err := command.CombinedOutput()
	return string(output), err
}

func replaceCurrentDocs(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(text, old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeDocsFixtureFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendDocsFixture(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func replaceDocsFixture(t *testing.T, path, old, replacement string) {
	t.Helper()
	replaceCurrentDocs(t, path, old, replacement)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate check_docs_test.go")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}
