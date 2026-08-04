package consumer_test

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const copiedOutEnvironment = "MODARY_EXTERNAL_CONSUMER_COPIED_OUT"

func TestCopiedOutConsumerGate(t *testing.T) {
	if os.Getenv(copiedOutEnvironment) == "1" {
		t.Skip("outer copied-out gate owns recursive isolation")
	}
	source := consumerRoot(t)
	framework := filepath.Clean(filepath.Join(source, "..", ".."))
	copied := filepath.Join(t.TempDir(), "counter-consumer")
	copyConsumerSource(t, source, copied)
	rewriteLocalReplace(t, filepath.Join(copied, "go.mod"), framework)
	assertOutsideFramework(t, copied, framework)
	fakeBin := installFailingNodeShims(t)

	environment := isolatedEnvironment(fakeBin)
	runExternalCommand(t, copied, environment, "go", "mod", "tidy", "-diff")
	runExternalCommand(t, copied, environment, "go", "test", "-count=1", "./...")
	runExternalCommand(t, copied, environment, "go", "vet", "./...")
	runExternalCommand(t, copied, environment, "go", "run", "./tools/modary", "verify")
	runExternalCommand(t, copied, environment, "go", "run", "./tools/modary", "generate", "--check")
	runExternalCommand(t, copied, environment, "go", "run", "./tools/modary", "check")
	runExternalCommand(t, copied, environment, "go", "run", "./tools/modary", "build")
	toolBinary := filepath.Join(t.TempDir(), "modary-project-tool")
	runExternalCommand(t, copied, environment, "go", "build", "-o", toolBinary, "./tools/modary")
	hostileGoEnvironment := replaceEnvironment(environment,
		"GOFLAGS=-toolexec=node",
		"GOENV="+filepath.Join(t.TempDir(), "untrusted-goenv"),
		"GOWORK="+filepath.Join(t.TempDir(), "untrusted.go.work"),
		"GO111MODULE=off",
		"GOTOOLCHAIN=untrusted+auto",
		"GOROOT="+filepath.Join(t.TempDir(), "untrusted-goroot"),
	)
	runExternalCommand(t, copied, hostileGoEnvironment, toolBinary, "build")
	runExternalCommand(t, copied, environment, filepath.Join(copied, "dist", "counter-console"), "version")
}

func replaceEnvironment(environment []string, assignments ...string) []string {
	replacements := make(map[string]string, len(assignments))
	for _, assignment := range assignments {
		name, _, _ := strings.Cut(assignment, "=")
		replacements[strings.ToUpper(name)] = assignment
	}
	result := make([]string, 0, len(environment)+len(assignments))
	for _, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		if _, replaced := replacements[strings.ToUpper(name)]; !replaced {
			result = append(result, value)
		}
	}
	for _, assignment := range assignments {
		result = append(result, assignment)
	}
	return result
}

func copyConsumerSource(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(destination, 0o755)
		}
		if entry.IsDir() && (entry.Name() == "dist" || entry.Name() == "data") {
			return filepath.SkipDir
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("external consumer source contains unsupported file %s", relative)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if entry.Type().Perm()&0o111 != 0 {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func rewriteLocalReplace(t *testing.T, goMod, framework string) {
	t.Helper()
	data, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	replacements := []struct {
		module      string
		development string
		directory   string
	}{
		{module: "github.com/iiwish/modary", development: "../..", directory: framework},
		{module: "github.com/iiwish/modary/components/governedpostgres", development: "../../components/governedpostgres", directory: filepath.Join(framework, "components", "governedpostgres")},
		{module: "github.com/iiwish/modary/components/postgres", development: "../../components/postgres", directory: filepath.Join(framework, "components", "postgres")},
	}
	rewritten := string(data)
	for _, replacement := range replacements {
		development := "replace " + replacement.module + " => " + replacement.development
		if strings.Count(rewritten, development) != 1 {
			t.Fatalf("development go.mod must contain exactly one %q binding", development)
		}
		rewritten = strings.Replace(rewritten, development,
			"replace "+replacement.module+" => "+strconv.Quote(filepath.ToSlash(replacement.directory)), 1)
	}
	if err := os.WriteFile(goMod, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRewriteLocalReplaceQuotesPathsWithSpaces(t *testing.T) {
	goMod := filepath.Join(t.TempDir(), "go.mod")
	data := "module example.com/copied-consumer\n\ngo 1.26.0\n\n" +
		"replace github.com/iiwish/modary => ../..\n\n" +
		"replace github.com/iiwish/modary/components/governedpostgres => ../../components/governedpostgres\n\n" +
		"replace github.com/iiwish/modary/components/postgres => ../../components/postgres\n"
	if err := os.WriteFile(goMod, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	framework := filepath.Join(t.TempDir(), "framework with spaces")
	rewriteLocalReplace(t, goMod, framework)
	rewritten, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	want := "replace github.com/iiwish/modary => " + strconv.Quote(filepath.ToSlash(framework))
	if !strings.Contains(string(rewritten), want) {
		t.Fatalf("rewritten go.mod = %s, want %q", rewritten, want)
	}
	command := exec.Command("go", "mod", "edit", "-json")
	command.Dir = filepath.Dir(goMod)
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("quoted replacement is not valid go.mod syntax: %v\n%s", err, output)
	}
}

func assertOutsideFramework(t *testing.T, copied, framework string) {
	t.Helper()
	if err := requirePhysicalPathOutside(copied, framework); err != nil {
		t.Fatal(err)
	}
}

func requirePhysicalPathOutside(copied, framework string) error {
	copiedPhysical, err := physicalPath(copied)
	if err != nil {
		return fmt.Errorf("resolve copied consumer physical path: %w", err)
	}
	frameworkPhysical, err := physicalPath(framework)
	if err != nil {
		return fmt.Errorf("resolve framework physical path: %w", err)
	}
	inside, err := pathWithin(frameworkPhysical, copiedPhysical)
	if err != nil {
		return fmt.Errorf("compare copied consumer and framework physical paths: %w", err)
	}
	if inside {
		return fmt.Errorf(
			"copied consumer physical path %s remains inside framework checkout %s",
			copiedPhysical,
			frameworkPhysical,
		)
	}
	return nil
}

func physicalPath(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func pathWithin(root, candidate string) (bool, error) {
	if !strings.EqualFold(filepath.VolumeName(root), filepath.VolumeName(candidate)) {
		return false, nil
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, err
	}
	return relative == "." ||
		(relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))), nil
}

func TestPhysicalOutsideCheckRejectsSymlinkIntoFramework(t *testing.T) {
	root := t.TempDir()
	framework := filepath.Join(root, "framework")
	consumer := filepath.Join(framework, "consumer")
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(root, "external")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(external, "consumer")
	if err := os.Symlink(consumer, alias); err != nil {
		t.Skipf("directory symlinks are unavailable on this platform: %v", err)
	}

	if err := requirePhysicalPathOutside(alias, framework); err == nil {
		t.Fatal("lexically external symlink resolving inside the framework was accepted")
	}
}

func installFailingNodeShims(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"node", "npm", "pnpm"} {
		path := filepath.Join(directory, name)
		script := "#!/bin/sh\nprintf '%s must not be invoked\\n' " + name + " >&2\nexit 97\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func isolatedEnvironment(fakeBin string) []string {
	result := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch strings.ToUpper(name) {
		case "GOWORK", "GOFLAGS", "GOENV", "GO111MODULE", "GOTOOLCHAIN", "GOROOT", copiedOutEnvironment, "PATH":
			continue
		}
		result = append(result, value)
	}
	return append(
		result,
		"GOWORK=off",
		"GOFLAGS=",
		"GOENV=off",
		"GO111MODULE=on",
		"GOTOOLCHAIN=local",
		copiedOutEnvironment+"=1",
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
}

func runExternalCommand(
	t *testing.T,
	directory string,
	environment []string,
	name string,
	args ...string,
) {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = directory
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	t.Logf("%s %s\n%s", name, strings.Join(args, " "), output)
}

func TestDevelopmentReplaceIsRelativeAndGOWORKIndependent(t *testing.T) {
	if os.Getenv(copiedOutEnvironment) == "1" {
		t.Skip("copied-out gate intentionally rewrites the development binding")
	}
	data, err := os.ReadFile(filepath.Join(consumerRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	const replacement = "replace github.com/iiwish/modary => ../.."
	if strings.Count(string(data), replacement) != 1 {
		t.Fatalf("go.mod local source binding is not canonical:\n%s", data)
	}
	if runtime.GOOS == "windows" && strings.Contains(string(data), `:\\`) {
		t.Fatal("go.mod contains a machine-specific Windows path")
	}
}
