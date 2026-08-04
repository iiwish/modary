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
	"docs/adr/ADR-005-create-only-profiles.md",
	"docs/getting-started/choose-profile.md", "docs/getting-started/admin-profile.md", "docs/getting-started/oidc-admin.md", "docs/getting-started/governed-profile.md",
	"docs/concepts/components-and-profiles.md", "docs/concepts/persistence-and-tasks.md",
	"docs/guides/rulary-bootstrap.md", "docs/how-to/run-background-tasks.md",
	"docs/operations/deployment.md", "docs/operations/observability.md", "docs/operations/security.md", "docs/operations/postgresql-backup-restore.md",
	"docs/reference/packages.md", "docs/reference/project-manifest.md", "docs/reference/support-matrix.md",
	"docs/releases/release-process.md", "docs/releases/upgrade-guide.md", "docs/releases/versioning.md",
	"docs/zh-CN/index.md", "docs/zh-CN/getting-started/choose-profile.md",
	"docs/zh-CN/getting-started/quickstart.md", "docs/zh-CN/getting-started/first-application.md",
	"docs/zh-CN/getting-started/admin-profile.md", "docs/zh-CN/getting-started/oidc-admin.md", "docs/zh-CN/getting-started/governed-profile.md",
	"docs/zh-CN/concepts/persistence-and-tasks.md", "docs/zh-CN/how-to/run-background-tasks.md", "docs/zh-CN/operations/deployment.md", "docs/zh-CN/operations/observability.md",
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
	".ai-platform/specs/007-component-framework-refoundation/spec.md",
	".ai-platform/specs/007-component-framework-refoundation/plan.md",
	".ai-platform/specs/007-component-framework-refoundation/research.md",
	".ai-platform/specs/007-component-framework-refoundation/analysis.md",
	".ai-platform/specs/007-component-framework-refoundation/tasks.md",
	".ai-platform/specs/007-component-framework-refoundation/checklists/requirements.md",
	".ai-platform/specs/007-component-framework-refoundation/packets/T028.yaml",
	".ai-platform/specs/007-component-framework-refoundation/packets/T029.yaml",
	".ai-platform/specs/007-component-framework-refoundation/packets/T030.yaml",
	".ai-platform/specs/007-component-framework-refoundation/packets/T031.yaml",
	".ai-platform/specs/007-component-framework-refoundation/packets/T032.yaml",
	".ai-platform/specs/007-component-framework-refoundation/packets/T033.yaml",
	".ai-platform/specs/007-component-framework-refoundation/packets/T034.yaml",
	".ai-platform/specs/008-react-admin-starter/spec.md",
	".ai-platform/specs/008-react-admin-starter/plan.md",
	".ai-platform/specs/008-react-admin-starter/analysis.md",
	".ai-platform/specs/008-react-admin-starter/tasks.md",
	".ai-platform/specs/008-react-admin-starter/checklists/requirements.md",
	".ai-platform/specs/008-react-admin-starter/packets/T035.yaml",
	".ai-platform/specs/008-react-admin-starter/packets/T036.yaml",
	".ai-platform/specs/008-react-admin-starter/packets/T037.yaml",
	".ai-platform/specs/009-component-boundary-closure/spec.md",
	".ai-platform/specs/009-component-boundary-closure/plan.md",
	".ai-platform/specs/009-component-boundary-closure/analysis.md",
	".ai-platform/specs/009-component-boundary-closure/tasks.md",
	".ai-platform/specs/009-component-boundary-closure/checklists/requirements.md",
	".ai-platform/specs/009-component-boundary-closure/packets/T038.yaml",
	".ai-platform/specs/009-component-boundary-closure/packets/T039.yaml",
	".ai-platform/specs/009-component-boundary-closure/packets/T040.yaml",
	".ai-platform/specs/009-component-boundary-closure/packets/T041.yaml",
	".ai-platform/specs/010-production-foundation/spec.md",
	".ai-platform/specs/010-production-foundation/plan.md",
	".ai-platform/specs/010-production-foundation/analysis.md",
	".ai-platform/specs/010-production-foundation/tasks.md",
	".ai-platform/specs/010-production-foundation/checklists/requirements.md",
	".ai-platform/specs/010-production-foundation/packets/T042.yaml",
	".ai-platform/specs/010-production-foundation/packets/T043.yaml",
	".ai-platform/specs/010-production-foundation/packets/T044.yaml",
	".ai-platform/specs/010-production-foundation/packets/T045.yaml",
	".ai-platform/specs/010-production-foundation/packets/T046.yaml",
	".ai-platform/specs/010-production-foundation/packets/T047.yaml",
	".ai-platform/specs/010-production-foundation/packets/T048.yaml",
	".ai-platform/evidence/T024/summary.md", ".ai-platform/evidence/T024/diff.patch", ".ai-platform/evidence/T024/test-results.md",
	".ai-platform/evidence/T025/summary.md", ".ai-platform/evidence/T025/diff.patch", ".ai-platform/evidence/T025/test-results.md",
	".ai-platform/evidence/T026/summary.md", ".ai-platform/evidence/T026/diff.patch", ".ai-platform/evidence/T026/test-results.md",
	".ai-platform/evidence/T026/review-1.md", ".ai-platform/evidence/T026/review-2.md",
	".ai-platform/evidence/T026/review-3.md", ".ai-platform/evidence/T026/review-4.md",
	".ai-platform/evidence/T027/summary.md", ".ai-platform/evidence/T027/diff.patch",
	".ai-platform/evidence/T027/test-results.md", ".ai-platform/evidence/T027/review.md",
	".ai-platform/evidence/T027/release-notes.md",
	".ai-platform/evidence/T028/summary.md", ".ai-platform/evidence/T028/diff.patch",
	".ai-platform/evidence/T028/test-results.md", ".ai-platform/evidence/T028/review.md",
	".ai-platform/evidence/T029/summary.md", ".ai-platform/evidence/T029/diff.patch",
	".ai-platform/evidence/T029/test-results.md", ".ai-platform/evidence/T029/review.md",
	".ai-platform/evidence/T030/summary.md", ".ai-platform/evidence/T030/diff.patch",
	".ai-platform/evidence/T030/test-results.md", ".ai-platform/evidence/T030/review.md",
	".ai-platform/evidence/T031/summary.md", ".ai-platform/evidence/T031/diff.patch",
	".ai-platform/evidence/T031/test-results.md", ".ai-platform/evidence/T031/review.md",
	".ai-platform/evidence/T032/summary.md", ".ai-platform/evidence/T032/diff.patch",
	".ai-platform/evidence/T032/test-results.md", ".ai-platform/evidence/T032/review.md",
	".ai-platform/evidence/T033/summary.md", ".ai-platform/evidence/T033/diff.patch",
	".ai-platform/evidence/T033/test-results.md", ".ai-platform/evidence/T033/review.md",
	".ai-platform/evidence/T034/summary.md", ".ai-platform/evidence/T034/diff.patch",
	".ai-platform/evidence/T034/test-results.md", ".ai-platform/evidence/T034/review-product.md",
	".ai-platform/evidence/T034/review-engineering.md", ".ai-platform/evidence/T034/external-acceptance.md",
	".ai-platform/evidence/T035/summary.md", ".ai-platform/evidence/T035/diff.patch",
	".ai-platform/evidence/T035/test-results.md", ".ai-platform/evidence/T035/review.md",
	".ai-platform/evidence/T036/summary.md", ".ai-platform/evidence/T036/diff.patch",
	".ai-platform/evidence/T036/test-results.md", ".ai-platform/evidence/T036/review.md",
	".ai-platform/evidence/T037/summary.md", ".ai-platform/evidence/T037/diff.patch",
	".ai-platform/evidence/T037/test-results.md", ".ai-platform/evidence/T037/review.md",
	".ai-platform/evidence/T037/external-acceptance.md",
	".ai-platform/evidence/T038/summary.md", ".ai-platform/evidence/T038/diff.patch",
	".ai-platform/evidence/T038/test-results.md", ".ai-platform/evidence/T038/review.md",
	".ai-platform/evidence/T039/summary.md", ".ai-platform/evidence/T039/diff.patch",
	".ai-platform/evidence/T039/test-results.md", ".ai-platform/evidence/T039/review.md",
	".ai-platform/evidence/T040/summary.md", ".ai-platform/evidence/T040/diff.patch",
	".ai-platform/evidence/T040/test-results.md", ".ai-platform/evidence/T040/review.md",
	".ai-platform/evidence/T041/summary.md", ".ai-platform/evidence/T041/diff.patch",
	".ai-platform/evidence/T041/test-results.md", ".ai-platform/evidence/T041/review.md",
	".ai-platform/evidence/T042/summary.md", ".ai-platform/evidence/T042/diff.patch",
	".ai-platform/evidence/T042/test-results.md", ".ai-platform/evidence/T042/review.md",
	".ai-platform/evidence/T043/summary.md", ".ai-platform/evidence/T043/diff.patch",
	".ai-platform/evidence/T043/test-results.md", ".ai-platform/evidence/T043/review.md",
	".ai-platform/evidence/T044/summary.md", ".ai-platform/evidence/T044/diff.patch",
	".ai-platform/evidence/T044/test-results.md", ".ai-platform/evidence/T044/review.md",
	".ai-platform/evidence/T045/summary.md", ".ai-platform/evidence/T045/diff.patch",
	".ai-platform/evidence/T045/test-results.md", ".ai-platform/evidence/T045/review.md",
	".ai-platform/evidence/T046/summary.md", ".ai-platform/evidence/T046/diff.patch",
	".ai-platform/evidence/T046/test-results.md", ".ai-platform/evidence/T046/review.md",
	".ai-platform/evidence/T047/summary.md", ".ai-platform/evidence/T047/diff.patch",
	".ai-platform/evidence/T047/test-results.md", ".ai-platform/evidence/T047/review.md",
	"starter/templates/admin/README.md.tmpl",
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

func TestCheckDocsRejectsUnresolvedCurrentClosureFinding(t *testing.T) {
	repository := currentDocsFixture(t)
	path := filepath.Join(repository, ".ai-platform/evidence/T047/review.md")
	replaceCurrentDocs(t, path, "- P1: 0", "- P1: 1")
	output, err := runCurrentDocsCheck(t, repository)
	if err == nil || !strings.Contains(output, "- P1: 0") {
		t.Fatalf("check-docs = %v, output=%q", err, output)
	}
}

func TestAcceptanceEvidenceDigestRejectsSourceDrift(t *testing.T) {
	repository := t.TempDir()
	writeDocsFixtureFile(t, filepath.Join(repository, "implementation.txt"), "accepted implementation\n")
	writeDocsFixtureFile(t, filepath.Join(repository, ".ai-platform", "evidence", "T047", "summary.md"),
		"# T047\n\n- Source digest: git-hash:0000000000000000000000000000000000000000\n")
	runGitFixture(t, repository, "init", "--quiet")
	digestCommand := exec.Command(filepath.Join(repositoryRoot(t), "scripts", "acceptance-source-digest.sh"), repository)
	digestOutput, err := digestCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("acceptance digest failed: %v\n%s", err, digestOutput)
	}
	digest := strings.TrimSpace(string(digestOutput))
	replaceCurrentDocs(t, filepath.Join(repository, ".ai-platform", "evidence", "T047", "summary.md"),
		"git-hash:0000000000000000000000000000000000000000", digest)

	check := func() (string, error) {
		command := exec.Command(filepath.Join(repositoryRoot(t), "scripts", "check-acceptance-evidence.sh"), repository)
		output, checkErr := command.CombinedOutput()
		return string(output), checkErr
	}
	if output, checkErr := check(); checkErr != nil {
		t.Fatalf("current acceptance evidence failed: %v\n%s", checkErr, output)
	}
	writeDocsFixtureFile(t, filepath.Join(repository, "implementation.txt"), "drifted implementation\n")
	if output, checkErr := check(); checkErr == nil || !strings.Contains(output, "acceptance evidence is stale") {
		t.Fatalf("stale acceptance evidence = %v, output=%q", checkErr, output)
	}
	writeDocsFixtureFile(t, filepath.Join(repository, "implementation.txt"), "accepted implementation\n")
	if err := os.Chmod(filepath.Join(repository, "implementation.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if output, checkErr := check(); checkErr == nil || !strings.Contains(output, "acceptance evidence is stale") {
		t.Fatalf("executable-mode acceptance drift = %v, output=%q", checkErr, output)
	}
}

func TestCheckDocsCannotBypassAcceptanceDigestWithRelativeRoot(t *testing.T) {
	repository := currentDocsFixture(t)
	for _, name := range []string{
		"check-docs.sh",
		"check-acceptance-evidence.sh",
		"acceptance-source-digest.sh",
	} {
		copyDocsFixtureScript(t, repository, name)
	}
	runGitFixture(t, repository, "init", "--quiet")
	refreshAcceptanceDigest(t, repository)
	appendDocsFixture(t, filepath.Join(repository, "README.md"), "\nacceptance source drift\n")

	command := exec.Command("sh", "./scripts/check-docs.sh")
	command.Dir = repository
	command.Env = append(os.Environ(), "MODARY_DOCS_ROOT=.", "MODARY_DOCS_FINAL=1")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "acceptance evidence is stale") {
		t.Fatalf("relative-root acceptance check = %v, output=%q", err, output)
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

func copyDocsFixtureScript(t *testing.T, repository, name string) {
	t.Helper()
	source := filepath.Join(repositoryRoot(t), "scripts", name)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(repository, "scripts", name)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		t.Fatal(err)
	}
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
