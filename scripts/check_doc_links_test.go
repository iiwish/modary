package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var requiredUserDocs = []string{
	"README.md",
	"CHANGELOG.md",
	"CONTRIBUTING.md",
	"SECURITY.md",
	"docs/index.md",
	"docs/getting-started/installation.md",
	"docs/getting-started/first-application.md",
	"docs/getting-started/quickstart.md",
	"docs/getting-started/project-layout.md",
	"docs/concepts/consumer-boundary.md",
	"docs/concepts/modules-and-capabilities.md",
	"docs/concepts/governed-actions.md",
	"docs/how-to/add-module.md",
	"docs/how-to/expose-action.md",
	"docs/how-to/test-application.md",
	"docs/how-to/troubleshooting.md",
	"docs/reference/packages.md",
	"docs/reference/support-matrix.md",
	"docs/reference/project-manifest.md",
	"docs/operations/deployment.md",
	"docs/operations/security.md",
	"docs/operations/sqlite-backup-restore.md",
	"docs/releases/versioning.md",
	"docs/releases/release-process.md",
	"docs/releases/upgrade-guide.md",
}

func TestCheckDocLinksAcceptsCompleteNavigation(t *testing.T) {
	repository := newUserDocsFixture(t)
	if output, err := runDocLinksCheck(t, repository); err != nil {
		t.Fatalf("complete user documentation failed: %v\n%s", err, output)
	}
}

func TestCheckDocLinksRejectsMissingRequiredDocument(t *testing.T) {
	repository := newUserDocsFixture(t)
	if err := os.Remove(filepath.Join(repository, "docs", "operations", "security.md")); err != nil {
		t.Fatal(err)
	}
	output, err := runDocLinksCheck(t, repository)
	if err == nil || !strings.Contains(output, "required user document") {
		t.Fatalf("missing document check = %v, output=%q", err, output)
	}
}

func TestCheckDocLinksRejectsBrokenLocalLink(t *testing.T) {
	repository := newUserDocsFixture(t)
	appendDocsFixture(t, filepath.Join(repository, "docs", "index.md"), "\n[Missing](operations/missing.md)\n")
	output, err := runDocLinksCheck(t, repository)
	if err == nil || !strings.Contains(output, "broken local Markdown link") {
		t.Fatalf("broken link check = %v, output=%q", err, output)
	}
}

func TestCheckDocLinksRejectsUnlistedDocument(t *testing.T) {
	repository := newUserDocsFixture(t)
	writeDocsFixtureFile(t, filepath.Join(repository, "docs", "concepts", "orphan.md"), "# Orphan\n")
	output, err := runDocLinksCheck(t, repository)
	if err == nil || !strings.Contains(output, "not listed in docs/index.md") {
		t.Fatalf("orphan check = %v, output=%q", err, output)
	}
}

func TestCheckDocLinksRejectsRetiredPublicExamplePath(t *testing.T) {
	repository := newUserDocsFixture(t)
	appendDocsFixture(t, filepath.Join(repository, "README.md"),
		"\nRetired path: `testdata/external-consumer`\n")
	output, err := runDocLinksCheck(t, repository)
	if err == nil || !strings.Contains(output, "retired public example path") {
		t.Fatalf("retired path check = %v, output=%q", err, output)
	}
}

func newUserDocsFixture(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	for _, relative := range requiredUserDocs {
		writeDocsFixtureFile(t, filepath.Join(repository, relative), "# Document\n")
	}
	appendDocsFixture(t, filepath.Join(repository, "README.md"), "\n[Documentation](docs/index.md)\n")
	var navigation strings.Builder
	for _, relative := range requiredUserDocs {
		if relative == "docs/index.md" || relative == "README.md" {
			continue
		}
		target := relative
		if strings.HasPrefix(target, "docs/") {
			target = strings.TrimPrefix(target, "docs/")
		} else {
			target = "../" + target
		}
		navigation.WriteString("\n[Document](" + target + ")")
	}
	appendDocsFixture(t, filepath.Join(repository, "docs", "index.md"), navigation.String()+"\n")
	return repository
}

func runDocLinksCheck(t *testing.T, repository string) (string, error) {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	checker := filepath.Join(filepath.Dir(workingDirectory), "scripts", "check-doc-links.sh")
	command := exec.Command("sh", checker, repository)
	output, runErr := command.CombinedOutput()
	return string(output), runErr
}
