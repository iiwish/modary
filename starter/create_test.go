package starter_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/starter"
)

func TestCreateAPIProfileIsDeterministicAndBuildsOutsideWorkspace(t *testing.T) {
	first := filepath.Join(t.TempDir(), "sample-api")
	second := filepath.Join(t.TempDir(), "sample-api")
	if err := os.Mkdir(first, 0o755); err != nil {
		t.Fatal(err)
	}
	options := starter.CreateOptions{
		Destination:   first,
		ModulePath:    "example.com/acme/sample-api",
		Name:          "Sample API",
		Profile:       starter.ProfileAPI,
		ModaryVersion: "v0.1.0-alpha.3",
		ModaryReplace: repositoryRoot(t),
	}
	result, err := starter.Create(context.Background(), options)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Profile != starter.ProfileAPI || result.Destination != first || len(result.Files) < 6 || !sort.StringsAreSorted(result.Files) {
		t.Fatalf("Create() result = %#v", result)
	}
	assertGeneratedGoVersion(t, first)

	secondOptions := options
	secondOptions.Destination = second
	if _, err := starter.Create(context.Background(), secondOptions); err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	firstSnapshot := projectSnapshot(t, first)
	secondSnapshot := projectSnapshot(t, second)
	if len(firstSnapshot) != len(secondSnapshot) {
		t.Fatalf("file counts differ: %d / %d", len(firstSnapshot), len(secondSnapshot))
	}
	for name, firstHash := range firstSnapshot {
		if secondSnapshot[name] != firstHash {
			t.Errorf("nondeterministic file %s", name)
		}
	}

	for name := range firstSnapshot {
		if !strings.HasSuffix(name, ".go") && name != "go.mod" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(first, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"components/postgres", "components/governedpostgres", "riverqueue",
			"github.com/iiwish/modary/action", "github.com/iiwish/modary/audit",
			"github.com/iiwish/modary/authz", "github.com/iiwish/modary/identity",
			"github.com/iiwish/modary/task", "transport/httpapi.NewMCP", "DATABASE_URL",
		} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Errorf("%s contains unselected dependency %q", name, forbidden)
			}
		}
	}

	runGo(t, first, "mod", "tidy")
	dependencies := runGoOutput(t, first, "list", "-deps", "./...")
	for _, forbidden := range []string{
		"github.com/iiwish/modary/components/governedpostgres",
		"github.com/iiwish/modary/components/postgres/localidentity",
		"github.com/iiwish/modary/components/postgres/rbac",
		"github.com/iiwish/modary/components/postgres/sqlaudit",
		"github.com/riverqueue/river",
	} {
		if strings.Contains(dependencies, forbidden) {
			t.Errorf("API package graph contains unselected dependency %q", forbidden)
		}
	}
	modules := runGoOutput(t, first, "list", "-m", "all")
	for _, forbidden := range []string{
		"github.com/jackc/pgx/",
		"github.com/riverqueue/river",
	} {
		if strings.Contains(modules, forbidden) {
			t.Errorf("API module graph contains unselected dependency %q", forbidden)
		}
	}
	runGo(t, first, "test", "./...")
	runGo(t, first, "build", "./cmd/sample-api")
	runGeneratedAPI(t, first, "sample-api")
}

func TestCreateAdminProfileBuildsAndRunsScopedCRUDWithoutRiver(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "sample-admin")
	result, err := starter.Create(context.Background(), starter.CreateOptions{
		Destination: destination, ModulePath: "example.com/acme/sample-admin", Name: "Sample Admin",
		Profile: starter.ProfileAdmin, ModaryVersion: "v0.1.0-alpha.3", ModaryReplace: repositoryRoot(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile != starter.ProfileAdmin || len(result.Files) < 9 || !sort.StringsAreSorted(result.Files) {
		t.Fatalf("Create(Admin)=%#v", result)
	}
	assertGeneratedGoVersion(t, destination)
	for _, name := range []string{
		"web/package.json",
		"web/pnpm-lock.yaml",
		"web/src/App.tsx",
		"web/src/main.tsx",
		"web/src/modules/index.ts",
		"internal/web/dist/index.html",
	} {
		info, statErr := os.Stat(filepath.Join(destination, name))
		if statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Errorf("generated Admin asset %s: info=%v error=%v", name, info, statErr)
		}
	}
	for _, pattern := range []string{"internal/web/dist/assets/app-*.css", "internal/web/dist/assets/app-*.js"} {
		matches, globErr := filepath.Glob(filepath.Join(destination, pattern))
		if globErr != nil || len(matches) != 1 {
			t.Errorf("generated Admin asset pattern %s: matches=%v error=%v", pattern, matches, globErr)
			continue
		}
		info, statErr := os.Stat(matches[0])
		if statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Errorf("generated Admin asset %s: info=%v error=%v", matches[0], info, statErr)
		}
	}
	packageManifest, err := os.ReadFile(filepath.Join(destination, "web/package.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"\"react\"", "\"react-dom\"", "\"react-router\"", "\"@vitejs/plugin-react\""} {
		if !bytes.Contains(packageManifest, []byte(required)) {
			t.Errorf("Admin frontend package manifest is missing %s", required)
		}
	}
	for _, forbidden := range []string{"\"vue\"", "pinia", "plugin-vue", "vue-tsc", "@vue/"} {
		if bytes.Contains(bytes.ToLower(packageManifest), []byte(forbidden)) {
			t.Errorf("Admin frontend package manifest retains Vue dependency %q", forbidden)
		}
	}
	if bytes.Contains(packageManifest, []byte("build:variants")) {
		t.Fatal("generated Admin package manifest retains the repository-only variant build command")
	}
	viteConfig, err := os.ReadFile(filepath.Join(destination, "web/vite.config.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"VITE_ADMIN_SELECTION", "scripts/selections", "modary-admin-selection"} {
		if bytes.Contains(viteConfig, []byte(forbidden)) {
			t.Errorf("generated Admin Vite config retains repository-only selector %q", forbidden)
		}
	}
	for _, internalPath := range []string{"web/scripts/build-variants.mjs", "web/scripts/selections"} {
		if _, statErr := os.Stat(filepath.Join(destination, internalPath)); !os.IsNotExist(statErr) {
			t.Errorf("generated Admin retains repository-only path %s: %v", internalPath, statErr)
		}
	}
	for name, required := range map[string][]string{
		"web/index.html":                {`<html lang="zh-CN">`, "<title>管理后台</title>"},
		"web/src/App.tsx":               {"应用暂时不可用", "暂无可用模块"},
		"internal/records/component.go": {`Label: "记录"`},
		"internal/app/application.go":   {`DisplayName: "管理员"`},
	} {
		source, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, value := range required {
			if !bytes.Contains(source, []byte(value)) {
				t.Errorf("generated Admin file %s is missing Chinese UI contract %q", name, value)
			}
		}
	}
	for name := range projectSnapshot(t, destination) {
		if strings.HasSuffix(name, ".vue") {
			t.Errorf("Admin frontend retains Vue source %s", name)
		}
	}
	registry, err := os.ReadFile(filepath.Join(destination, "web/src/modules/active.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(registry, []byte("recordsModule")) {
		t.Fatal("Admin registry does not select the records module")
	}
	for _, name := range []string{"internal/app/application.go", "internal/records/component.go"} {
		source, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Contains(source, []byte("module.CapabilitySessions")) {
			t.Errorf("%s does not declare its session dependency", name)
		}
	}
	for _, forbidden := range []string{"task", "audit", "action", "mcp", "marketplace"} {
		if bytes.Contains(bytes.ToLower(registry), []byte(forbidden)) {
			t.Errorf("Admin registry contains unselected module %q", forbidden)
		}
	}
	for name := range projectSnapshot(t, destination) {
		if !strings.HasSuffix(name, ".go") && name != "go.mod" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, forbidden := range []string{"github.com/iiwish/modary/components/governedpostgres\"", "riverqueue/river",
			"components/postgres/sqlaudit", "github.com/iiwish/modary/action", "github.com/iiwish/modary/audit",
			"github.com/iiwish/modary/task", "NewMCP", "/api/actions"} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Errorf("%s contains unselected governed dependency %q", name, forbidden)
			}
		}
	}
	runGo(t, destination, "mod", "tidy")
	dependencies := runGoOutput(t, destination, "list", "-deps", "./...")
	for _, forbidden := range []string{"github.com/riverqueue/river", "github.com/iiwish/modary/components/governedpostgres\n",
		"github.com/iiwish/modary/components/postgres/sqlaudit"} {
		if strings.Contains(dependencies, forbidden) {
			t.Errorf("Admin dependency graph contains %q", forbidden)
		}
	}
	modules := runGoOutput(t, destination, "list", "-m", "all")
	if strings.Contains(modules, "github.com/riverqueue/river") {
		t.Fatal("default Admin module graph contains River")
	}
	runGeneratedAdminProfileTests(t, destination, "modary_starter_default_test", "")
	runGo(t, destination, "build", "./cmd/sample-admin")
}

func TestCreateAdminOperationalComponentsAreExplicitAndBuild(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "operations-admin")
	result, err := starter.Create(context.Background(), starter.CreateOptions{
		Destination: destination, ModulePath: "example.com/acme/operations-admin", Name: "Operations Admin",
		Profile: starter.ProfileAdmin, Components: []starter.Component{starter.ComponentTasks, starter.ComponentAudit},
		ModaryVersion: "v0.1.0-alpha.3", ModaryReplace: repositoryRoot(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Components, []starter.Component{starter.ComponentAudit, starter.ComponentTasks}) {
		t.Fatalf("components = %v", result.Components)
	}
	for _, name := range []string{
		"internal/tasks/component.go", "internal/auditlog/component.go",
		"web/src/modules/tasks/TasksView.tsx", "web/src/modules/audit/AuditView.tsx",
	} {
		if info, statErr := os.Stat(filepath.Join(destination, name)); statErr != nil || !info.Mode().IsRegular() {
			t.Errorf("selected component file %s: info=%v error=%v", name, info, statErr)
		}
	}
	active, err := os.ReadFile(filepath.Join(destination, "web/src/modules/active.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{"recordsModule", "tasksModule", "auditModule"} {
		if !bytes.Contains(active, []byte(selected)) {
			t.Errorf("active Admin registry is missing %s", selected)
		}
	}
	for _, name := range []string{"internal/tasks/component.go", "internal/auditlog/component.go"} {
		source, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Contains(source, []byte("module.CapabilitySessions")) {
			t.Errorf("%s does not declare its session dependency", name)
		}
	}
	for name, label := range map[string]string{
		"internal/tasks/component.go":    `Label: "任务"`,
		"internal/auditlog/component.go": `Label: "审计日志"`,
	} {
		source, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Contains(source, []byte(label)) {
			t.Errorf("%s is missing Chinese Admin contribution label %q", name, label)
		}
	}
	runGo(t, destination, "mod", "tidy")
	modules := runGoOutput(t, destination, "list", "-m", "all")
	for _, required := range []string{"github.com/iiwish/modary/components/governedpostgres", "github.com/riverqueue/river"} {
		if !strings.Contains(modules, required) {
			t.Errorf("operational Admin module graph is missing %q", required)
		}
	}
	runGeneratedAdminProfileTests(t, destination, "modary_starter_operations_test", "modary_starter_operations_queue_test")
	runGo(t, destination, "build", "./cmd/operations-admin")
}

func TestCreateGovernedProfileBuildsAndConsumesTransactionalWork(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "sample-governed")
	result, err := starter.Create(context.Background(), starter.CreateOptions{
		Destination: destination, ModulePath: "example.com/acme/sample-governed", Name: "Sample Governed",
		Profile: starter.ProfileGoverned, ModaryVersion: "v0.1.0-alpha.3", ModaryReplace: repositoryRoot(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPhysicalPathOutside(t, destination, repositoryRoot(t))
	if result.Profile != starter.ProfileGoverned || len(result.Files) < 11 || !sort.StringsAreSorted(result.Files) {
		t.Fatalf("Create(Governed)=%#v", result)
	}
	assertGeneratedGoVersion(t, destination)
	for _, name := range []string{
		"cmd/sample-governed/main.go",
		"cmd/sample-governed-worker/main.go",
		"internal/project/project.go",
		"internal/limits/module.go",
		"internal/limits/worker.go",
		"internal/limits/migrations/postgres/0001_limits.sql",
	} {
		info, statErr := os.Stat(filepath.Join(destination, name))
		if statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Errorf("generated Governed file %s: info=%v error=%v", name, info, statErr)
		}
	}
	projectSource, err := os.ReadFile(filepath.Join(destination, "internal/project/project.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range []string{
		"components/governedpostgres", "components/postgres/localidentity",
		"components/postgres/rbac", "components/postgres/sqlaudit",
		"transport/httpapi", "NewMCP", "limits.Module",
	} {
		if !bytes.Contains(projectSource, []byte(selected)) {
			t.Errorf("Governed composition does not visibly select %q", selected)
		}
	}
	for _, forbidden := range []string{"components/postgres\"", "transport/sessionhttp", "internal/web", "records.Registration"} {
		if bytes.Contains(projectSource, []byte(forbidden)) {
			t.Errorf("Governed composition contains unselected Admin surface %q", forbidden)
		}
	}

	runGo(t, destination, "mod", "tidy")
	dependencies := runGoOutput(t, destination, "list", "-deps", "./...")
	for _, required := range []string{
		"github.com/iiwish/modary/components/governedpostgres\n",
		"github.com/iiwish/modary/components/postgres/sqlaudit\n",
		"github.com/riverqueue/river\n",
	} {
		if !strings.Contains(dependencies, required) {
			t.Errorf("Governed dependency graph is missing %q", strings.TrimSpace(required))
		}
	}
	runGeneratedGovernedProfileTests(t, destination)
	runGo(t, destination, "build", "./cmd/sample-governed", "./cmd/sample-governed-worker")
}

func TestCreateNeverOverwritesOrPatchesDestination(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "owned-api")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(destination, "owned.txt")
	if err := os.WriteFile(owned, []byte("consumer-owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	options := starter.CreateOptions{
		Destination: destination,
		ModulePath:  "example.com/owned-api",
		Name:        "Owned API",
		Profile:     starter.ProfileAPI,
	}
	if _, err := starter.Create(context.Background(), options); !errors.Is(err, starter.ErrDestinationNotEmpty) {
		t.Fatalf("Create(non-empty) error = %v", err)
	}
	data, err := os.ReadFile(owned)
	if err != nil || string(data) != "consumer-owned\n" {
		t.Fatalf("consumer file changed: %q, %v", data, err)
	}

	if err := os.Remove(owned); err != nil {
		t.Fatal(err)
	}
	options.ModaryVersion = "v0.1.0-alpha.3"
	options.ModaryReplace = repositoryRoot(t)
	if _, err := starter.Create(context.Background(), options); err != nil {
		t.Fatalf("Create(empty) error = %v", err)
	}
	before := projectSnapshot(t, destination)
	if _, err := starter.Create(context.Background(), options); !errors.Is(err, starter.ErrDestinationNotEmpty) {
		t.Fatalf("repeat Create() error = %v", err)
	}
	after := projectSnapshot(t, destination)
	for name, hash := range before {
		if after[name] != hash {
			t.Errorf("repeat creation changed %s", name)
		}
	}
}

func TestCreateRejectsUnsafeOrInvalidInputsBeforeWrites(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked-api")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		options starter.CreateOptions
		want    error
	}{
		{name: "nil context", options: validCreateOptions(filepath.Join(root, "nil-context")), want: starter.ErrContextRequired},
		{name: "missing destination", options: validCreateOptions(""), want: starter.ErrInvalidOptions},
		{name: "invalid destination id", options: validCreateOptions(filepath.Join(root, "Invalid Name")), want: starter.ErrInvalidOptions},
		{name: "invalid module", options: withModule(validCreateOptions(filepath.Join(root, "bad-module")), "../bad"), want: starter.ErrInvalidOptions},
		{name: "vendor module segment", options: withModule(validCreateOptions(filepath.Join(root, "vendor-module")), "example.com/acme/vendor/service"), want: starter.ErrInvalidOptions},
		{name: "default vendor module", options: withModule(validCreateOptions(filepath.Join(root, "vendor")), ""), want: starter.ErrInvalidOptions},
		{name: "unknown profile", options: withProfile(validCreateOptions(filepath.Join(root, "unknown-api")), starter.Profile("unknown")), want: starter.ErrInvalidOptions},
		{name: "component on API", options: withComponents(validCreateOptions(filepath.Join(root, "api-component")), starter.ComponentTasks), want: starter.ErrInvalidOptions},
		{name: "unknown component", options: withComponents(withProfile(validCreateOptions(filepath.Join(root, "unknown-component")), starter.ProfileAdmin), starter.Component("unknown")), want: starter.ErrInvalidOptions},
		{name: "duplicate component", options: withComponents(withProfile(validCreateOptions(filepath.Join(root, "duplicate-component")), starter.ProfileAdmin), starter.ComponentAudit, starter.ComponentAudit), want: starter.ErrInvalidOptions},
		{name: "symlink destination", options: validCreateOptions(link), want: starter.ErrUnsafeDestination},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if test.name == "nil context" {
				ctx = nil
			}
			if _, err := starter.Create(ctx, test.options); !errors.Is(err, test.want) {
				t.Fatalf("Create() error = %v, want %v", err, test.want)
			}
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	destination := filepath.Join(root, "canceled-api")
	if _, err := starter.Create(canceled, validCreateOptions(destination)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Create(canceled) error = %v", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("canceled creation destination error = %v", err)
	}
}

func validCreateOptions(destination string) starter.CreateOptions {
	return starter.CreateOptions{
		Destination:   destination,
		ModulePath:    "example.com/acme/sample-api",
		Name:          "Sample API",
		Profile:       starter.ProfileAPI,
		ModaryVersion: "v0.1.0-alpha.3",
	}
}

func withModule(options starter.CreateOptions, value string) starter.CreateOptions {
	options.ModulePath = value
	return options
}

func withProfile(options starter.CreateOptions, value starter.Profile) starter.CreateOptions {
	options.Profile = value
	return options
}

func withComponents(options starter.CreateOptions, values ...starter.Component) starter.CreateOptions {
	options.Components = values
	return options
}

func projectSnapshot(t *testing.T, root string) map[string][sha256.Size]byte {
	t.Helper()
	result := make(map[string][sha256.Size]byte)
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertGeneratedGoVersion(t *testing.T, destination string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(destination, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("\ngo 1.26.5\n")) {
		t.Fatalf("generated go.mod does not require the security-patched Go baseline:\n%s", data)
	}
}

func runGo(t *testing.T, directory string, args ...string) {
	t.Helper()
	_ = runGoOutput(t, directory, args...)
}

func runGoOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command(acceptanceTool("MODARY_ACCEPTANCE_GO", "go"), args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func runGoEnv(t *testing.T, directory string, environment []string, args ...string) {
	t.Helper()
	command := exec.Command(acceptanceTool("MODARY_ACCEPTANCE_GO", "go"), args...)
	command.Dir = directory
	command.Env = append(append(os.Environ(), "GOWORK=off", "GOFLAGS="), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func runGeneratedAdminProfileTests(t *testing.T, directory, schema, queueSchema string) {
	t.Helper()
	databaseURL := os.Getenv("MODARY_TEST_DATABASE_URL")
	if databaseURL == "" {
		runGoEnv(t, directory, []string{"DATABASE_URL="}, "test", "./...")
		return
	}
	environment := []string{"DATABASE_URL=" + databaseURL, "MODARY_DATABASE_SCHEMA=" + schema}
	if queueSchema != "" {
		environment = append(environment, "MODARY_QUEUE_SCHEMA="+queueSchema)
	}
	runGoEnv(t, directory, environment, "test", "-count=1", "./...")
}

func runGeneratedGovernedProfileTests(t *testing.T, directory string) {
	t.Helper()
	databaseURL := os.Getenv("MODARY_TEST_DATABASE_URL")
	if databaseURL == "" {
		runGoEnv(t, directory, []string{"DATABASE_URL="}, "test", "-count=1", "./...")
		return
	}
	if strings.TrimSpace(databaseURL) != databaseURL {
		t.Fatal("MODARY_TEST_DATABASE_URL must not contain surrounding whitespace")
	}
	suffix := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
	environment := []string{
		"DATABASE_URL=" + databaseURL,
		"MODARY_APPLICATION_SCHEMA=modary_gov_a_" + suffix,
		"MODARY_QUEUE_SCHEMA=modary_gov_q_" + suffix,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	runRequiredGeneratedProfileTest(t, ctx, directory, environment,
		acceptanceTool("MODARY_ACCEPTANCE_GO", "go"),
		"TestGovernedProfileCommitsAndConsumesDurableWork", "Governed")
}

func runGeneratedAPI(t *testing.T, directory, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("graceful process signal acceptance is Unix-only")
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(directory, name))
	command.Dir = directory
	command.Env = append(os.Environ(), "MODARY_HTTP_ADDR="+address)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			<-wait
		}
	})

	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for {
		request, requestErr := http.NewRequest(http.MethodGet, "http://"+address+"/healthz", nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		request.Header.Set("Accept", "application/json")
		response, requestErr := client.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("generated API did not become ready: %v\n%s", requestErr, output.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("generated API shutdown: %v\n%s", err, output.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("generated API did not stop after interrupt\n%s", output.String())
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate starter test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}
