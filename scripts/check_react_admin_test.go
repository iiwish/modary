package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var reactAdminFixtureFiles = []string{
	"starter/templates/admin/web/package.json",
	"starter/templates/admin/web/pnpm-lock.yaml",
	"starter/templates/admin/web/src/main.tsx",
	"starter/templates/admin/web/src/App.tsx",
	"starter/templates/admin/web/src/modules/index.ts",
	"starter/templates/admin/internal/web/dist/index.html",
}

func TestReactAdminCheckAcceptsCurrentSource(t *testing.T) {
	repository := reactAdminFixture(t)
	if output, err := runReactAdminCheck(t, repository); err != nil {
		t.Fatalf("check-react-admin failed: %v\n%s", err, output)
	}
}

func TestReactAdminCheckRejectsVueDependency(t *testing.T) {
	repository := reactAdminFixture(t)
	path := filepath.Join(repository, "starter/templates/admin/web/package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\n\"vue\": \"3.5.0\"\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runReactAdminCheck(t, repository)
	if err == nil || !strings.Contains(output, "dependency residue") {
		t.Fatalf("check-react-admin = %v, output=%q", err, output)
	}
}

func TestReactAdminCheckRejectsStaleCanonicalDocumentation(t *testing.T) {
	repository := reactAdminFixture(t)
	path := filepath.Join(repository, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\nThe Admin uses Vue 3.\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := runReactAdminCheck(t, repository)
	if err == nil || !strings.Contains(output, "canonical documentation") {
		t.Fatalf("check-react-admin = %v, output=%q", err, output)
	}
}

func reactAdminFixture(t *testing.T) string {
	t.Helper()
	repository := currentDocsFixture(t)
	root := repositoryRoot(t)
	files := append([]string(nil), reactAdminFixtureFiles...)
	for _, pattern := range []string{"starter/templates/admin/internal/web/dist/assets/app-*.js", "starter/templates/admin/internal/web/dist/assets/app-*.css"} {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil || len(matches) != 1 {
			t.Fatalf("glob %s: matches=%v error=%v", pattern, matches, err)
		}
		name, err := filepath.Rel(root, matches[0])
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, name)
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		target := filepath.Join(repository, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repository
}

func runReactAdminCheck(t *testing.T, repository string) (string, error) {
	t.Helper()
	command := exec.Command("sh", filepath.Join(repositoryRoot(t), "scripts/check-react-admin.sh"))
	command.Env = append(os.Environ(), "MODARY_REACT_ADMIN_ROOT="+repository)
	output, err := command.CombinedOutput()
	return string(output), err
}
