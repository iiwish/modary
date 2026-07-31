package action_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalModuleCannotImportFrameworkAssemblyPackages(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	for _, packagePath := range []string{
		"github.com/iiwish/modary/internal/actionpersistence",
		"github.com/iiwish/modary/internal/actionruntime",
		"github.com/iiwish/modary/internal/callbackcontract",
		"github.com/iiwish/modary/internal/databasecontrol",
		"github.com/iiwish/modary/internal/moduleassembly",
		"github.com/iiwish/modary/internal/runtimecontrol",
	} {
		t.Run(filepath.Base(packagePath), func(t *testing.T) {
			directory := t.TempDir()
			goMod := "module example.com/external-consumer\n\ngo 1.26\n\nrequire github.com/iiwish/modary v0.0.0\n\nreplace github.com/iiwish/modary => " + filepath.ToSlash(root) + "\n"
			program := "package boundary\n\nimport _ \"" + packagePath + "\"\n"
			if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(goMod), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "boundary.go"), []byte(program), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("go", "test", "-mod=mod", "./...")
			command.Dir = directory
			command.Env = append(os.Environ(), "GOWORK=off")
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("external module unexpectedly imported %s", packagePath)
			}
			if !strings.Contains(string(output), "use of internal package "+packagePath+" not allowed") {
				t.Fatalf("unexpected compilation failure:\n%s", output)
			}
		})
	}
}
