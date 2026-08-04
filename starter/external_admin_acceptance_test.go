package starter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/modary/starter"
)

const externalAdminAcceptanceEnvironment = "MODARY_EXTERNAL_ADMIN_ACCEPTANCE"

func TestCopiedOutAdminProfiles(t *testing.T) {
	if os.Getenv(externalAdminAcceptanceEnvironment) != "1" {
		t.Skip(externalAdminAcceptanceEnvironment + "=1 is required for the copied-out Admin gate")
	}
	databaseURL := os.Getenv("MODARY_TEST_DATABASE_URL")
	if databaseURL == "" || strings.TrimSpace(databaseURL) != databaseURL {
		t.Fatal("MODARY_TEST_DATABASE_URL must be a non-empty URL without surrounding whitespace")
	}

	root := repositoryRoot(t)
	goTool := acceptanceTool("MODARY_ACCEPTANCE_GO", "go")
	pnpmTool := acceptanceTool("MODARY_ACCEPTANCE_PNPM", "pnpm")
	profiles := []struct {
		name                       string
		id                         string
		components                 []starter.Component
		required                   []string
		forbidden                  []string
		useGeneratedSchemaDefaults bool
		verifyRuntimeDefaults      bool
		backendOnly                bool
	}{
		{
			name: "default-admin",
			id:   "default-admin",
			required: []string{
				"github.com/iiwish/modary/components/postgres",
			},
			forbidden: []string{
				"github.com/iiwish/modary/components/governedpostgres",
				"github.com/riverqueue/river",
			},
		},
		{
			name:       "operations-admin",
			id:         "operations-admin",
			components: []starter.Component{starter.ComponentTasks, starter.ComponentAudit},
			required: []string{
				"github.com/iiwish/modary/components/postgres",
				"github.com/iiwish/modary/components/governedpostgres",
				"github.com/riverqueue/river",
			},
		},
		{
			name:                       "long-project-schema-defaults",
			id:                         strings.Repeat("a", 63),
			components:                 []starter.Component{starter.ComponentTasks},
			required:                   []string{"github.com/iiwish/modary/components/governedpostgres", "github.com/riverqueue/river"},
			useGeneratedSchemaDefaults: true,
			verifyRuntimeDefaults:      true,
			backendOnly:                true,
		},
		{
			name:                       "reserved-public-schema-defaults",
			id:                         "public",
			required:                   []string{"github.com/iiwish/modary/components/postgres"},
			forbidden:                  []string{"github.com/iiwish/modary/components/governedpostgres", "github.com/riverqueue/river"},
			useGeneratedSchemaDefaults: true,
			verifyRuntimeDefaults:      true,
			backendOnly:                true,
		},
		{
			name:                       "reserved-pg-schema-defaults",
			id:                         "pg",
			components:                 []starter.Component{starter.ComponentTasks},
			required:                   []string{"github.com/iiwish/modary/components/governedpostgres", "github.com/riverqueue/river"},
			useGeneratedSchemaDefaults: true,
			verifyRuntimeDefaults:      true,
			backendOnly:                true,
		},
	}

	for _, profile := range profiles {
		profile := profile
		t.Run(profile.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), profile.id)
			_, err := starter.Create(context.Background(), starter.CreateOptions{
				Destination:   destination,
				ModulePath:    "example.com/modary-acceptance/" + profile.id,
				Name:          "Copied Admin Acceptance",
				Profile:       starter.ProfileAdmin,
				Components:    profile.components,
				ModaryReplace: root,
			})
			if err != nil {
				t.Fatal(err)
			}
			assertPhysicalPathOutside(t, destination, root)
			assertGeneratedConsumerFrontend(t, destination)

			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
			defer cancel()
			runAcceptanceCommand(t, ctx, destination, nil, goTool, "mod", "tidy")
			modules := runAcceptanceCommand(t, ctx, destination, nil, goTool, "list", "-m", "all")
			for _, modulePath := range profile.required {
				if !containsModule(modules, modulePath) {
					t.Errorf("generated module graph is missing %s", modulePath)
				}
			}
			for _, modulePath := range profile.forbidden {
				if containsModule(modules, modulePath) {
					t.Errorf("generated module graph contains unselected %s", modulePath)
				}
			}

			databaseEnvironment := []string{"DATABASE_URL=" + databaseURL}
			if profile.useGeneratedSchemaDefaults {
				databaseEnvironment = append(databaseEnvironment, "MODARY_DATABASE_SCHEMA=", "MODARY_QUEUE_SCHEMA=")
			} else {
				suffix := fmt.Sprintf("%d_%d", os.Getpid(), time.Now().UnixNano())
				databaseEnvironment = append(databaseEnvironment,
					"MODARY_DATABASE_SCHEMA=modary_copyout_"+strings.ReplaceAll(profile.id, "-", "_")+"_"+suffix)
				if len(profile.components) > 0 {
					databaseEnvironment = append(databaseEnvironment, "MODARY_QUEUE_SCHEMA=modary_copyout_queue_"+suffix)
				}
			}
			runRequiredGeneratedAdminTest(t, ctx, destination, databaseEnvironment, goTool)
			runAcceptanceCommand(t, ctx, destination, nil, goTool, "build", "./...")
			if profile.verifyRuntimeDefaults {
				assertGeneratedAdminRuntimeDefaults(t, ctx, destination, profile.id, databaseURL, goTool)
			}
			if profile.backendOnly {
				return
			}

			web := filepath.Join(destination, "web")
			frontendEnvironment := []string{"VITE_ADMIN_SELECTION=repository-only-selector-must-be-ignored"}
			runAcceptanceCommand(t, ctx, web, frontendEnvironment, pnpmTool, "install", "--frozen-lockfile")
			runAcceptanceCommand(t, ctx, web, frontendEnvironment, pnpmTool, "assets:check")
			runAcceptanceCommand(t, ctx, web, frontendEnvironment, pnpmTool, "lint")
			runAcceptanceCommand(t, ctx, web, frontendEnvironment, pnpmTool, "typecheck")
			runAcceptanceCommand(t, ctx, web, frontendEnvironment, pnpmTool, "test")
			runAcceptanceCommand(t, ctx, web, frontendEnvironment, pnpmTool, "build")
			runAcceptanceCommand(t, ctx, web, frontendEnvironment, pnpmTool, "assets:check")
		})
	}
}

func assertGeneratedAdminRuntimeDefaults(t *testing.T, ctx context.Context, directory, id, databaseURL, goTool string) {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "generated-admin")
	runAcceptanceCommand(t, ctx, directory, nil, goTool, "build", "-o", executable, "./cmd/"+id)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(processCtx, executable)
	command.Dir = directory
	command.Env = acceptanceEnvironment(
		"DATABASE_URL="+databaseURL,
		"MODARY_DATABASE_SCHEMA=",
		"MODARY_QUEUE_SCHEMA=",
		"MODARY_HTTP_ADDR="+address,
		"MODARY_ADMIN_USERNAME=admin",
		"MODARY_ADMIN_PASSWORD=development-password",
		"MODARY_ALLOW_INSECURE_COOKIE=true",
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-wait:
			t.Fatalf("generated Admin exited before readiness: %v\n%s", err, output.String())
		case <-deadline.C:
			cancel()
			<-wait
			t.Fatalf("generated Admin did not become ready with default schemas\n%s", output.String())
		case <-ticker.C:
			response, err := client.Get("http://" + address + "/healthz")
			if err != nil {
				continue
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				continue
			}
			cancel()
			<-wait
			t.Log("generated Admin reached readiness with runtime schema defaults")
			return
		}
	}
}

func assertGeneratedConsumerFrontend(t *testing.T, destination string) {
	t.Helper()
	for _, name := range []string{"web/package.json", "web/vite.config.ts", "web/scripts/check-assets.mjs"} {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"VITE_ADMIN_SELECTION", "scripts/selections", "build-variants"} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Errorf("generated consumer file %s contains repository-only contract %q", name, forbidden)
			}
		}
	}
	for _, name := range []string{"web/scripts/build-variants.mjs", "web/scripts/selections"} {
		if _, err := os.Stat(filepath.Join(destination, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("generated consumer retains repository-only path %s: %v", name, err)
		}
	}
}

func assertPhysicalPathOutside(t *testing.T, destination, root string) {
	t.Helper()
	physicalDestination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		t.Fatal(err)
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(physicalRoot, physicalDestination)
	if err != nil {
		t.Fatal(err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		t.Fatalf("generated consumer %s is inside framework checkout %s", physicalDestination, physicalRoot)
	}
}

func runRequiredGeneratedAdminTest(t *testing.T, ctx context.Context, directory string, environment []string, goTool string) {
	runRequiredGeneratedProfileTest(t, ctx, directory, environment, goTool, "TestAdminBackendProfile", "Admin")
}

func runRequiredGeneratedProfileTest(t *testing.T, ctx context.Context, directory string, environment []string, goTool, testName, profileName string) {
	t.Helper()
	started := time.Now()
	command := exec.CommandContext(ctx, goTool, "test", "-json", "-count=1", "./...")
	command.Dir = directory
	command.Env = acceptanceEnvironment(environment...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("generated %s go test failed: %v\n%s\n%s", profileName, err, stdout.Bytes(), stderr.Bytes())
	}

	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	passed := false
	for {
		var event struct {
			Action string
			Test   string
		}
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode generated Admin test events: %v\n%s", err, stdout.Bytes())
		}
		if event.Test != testName {
			continue
		}
		if event.Action == "skip" {
			t.Fatalf("generated %s PostgreSQL integration test was skipped\n%s", profileName, stdout.Bytes())
		}
		if event.Action == "pass" {
			passed = true
		}
	}
	if !passed {
		t.Fatalf("generated %s PostgreSQL integration test did not report pass\n%s", profileName, stdout.Bytes())
	}
	t.Logf("generated %s PostgreSQL integration passed in %s", profileName, time.Since(started).Round(time.Millisecond))
}

func runAcceptanceCommand(t *testing.T, ctx context.Context, directory string, environment []string, name string, args ...string) string {
	t.Helper()
	started := time.Now()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	command.Env = acceptanceEnvironment(environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	t.Logf("%s %s passed in %s", name, strings.Join(args, " "), time.Since(started).Round(time.Millisecond))
	return string(output)
}

func acceptanceEnvironment(additions ...string) []string {
	values := append([]string{
		"GO111MODULE=on",
		"GOTOOLCHAIN=local",
		"GOENV=off",
		"GOWORK=off",
		"GOFLAGS=",
	}, additions...)
	replaced := make(map[string]struct{}, len(values))
	for _, value := range values {
		name, _, _ := strings.Cut(value, "=")
		replaced[strings.ToUpper(name)] = struct{}{}
	}
	result := make([]string, 0, len(os.Environ())+len(values))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if _, ok := replaced[strings.ToUpper(name)]; !ok {
			result = append(result, value)
		}
	}
	return append(result, values...)
}

func acceptanceTool(environment, fallback string) string {
	if value := os.Getenv(environment); value != "" {
		return value
	}
	return fallback
}

func containsModule(output, modulePath string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == modulePath {
			return true
		}
	}
	return false
}
