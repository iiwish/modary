package projecttool

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
)

type checkpointContext struct {
	mu       sync.Mutex
	calls    int
	trigger  int
	cancel   bool
	fired    bool
	callback func()
	done     chan struct{}
}

func newCheckpointContext(trigger int, cancel bool, callback func()) *checkpointContext {
	return &checkpointContext{trigger: trigger, cancel: cancel, callback: callback, done: make(chan struct{})}
}

func (*checkpointContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *checkpointContext) Done() <-chan struct{}   { return ctx.done }
func (*checkpointContext) Value(any) any               { return nil }

func (ctx *checkpointContext) Err() error {
	ctx.mu.Lock()
	ctx.calls++
	shouldFire := !ctx.fired && ctx.calls == ctx.trigger
	if shouldFire {
		ctx.fired = true
	}
	callback := ctx.callback
	canceled := ctx.cancel && ctx.fired
	if shouldFire && ctx.cancel {
		close(ctx.done)
	}
	ctx.mu.Unlock()
	if shouldFire && callback != nil {
		callback()
	}
	if canceled {
		return context.Canceled
	}
	return nil
}

func TestContextAwarePublicAPIsRejectNilAndCanceledContexts(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	project := loadFixtureProject(t, root)
	definition := fixtureDefinition(&inspectionCounters{}, false)

	if _, err := LoadContext(nil, root); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("LoadContext nil error = %v", err)
	}
	if _, err := InspectContext(nil, definition); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("InspectContext nil error = %v", err)
	}
	if _, err := project.VerifyContext(nil, definition); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("VerifyContext nil error = %v", err)
	}
	if _, err := project.GenerateContext(nil, definition); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("GenerateContext nil error = %v", err)
	}
	if _, err := project.CheckContext(nil, definition); !errors.Is(err, ErrContextRequired) {
		t.Fatalf("CheckContext nil error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, call := range map[string]func() error{
		"load":     func() error { _, err := LoadContext(ctx, root); return err },
		"inspect":  func() error { _, err := InspectContext(ctx, definition); return err },
		"verify":   func() error { _, err := project.VerifyContext(ctx, definition); return err },
		"generate": func() error { _, err := project.GenerateContext(ctx, definition); return err },
		"check":    func() error { _, err := project.CheckContext(ctx, definition); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 1 {
		t.Fatalf("canceled APIs changed project: entries=%v err=%v", entries, err)
	}
}

func TestRunObservesCancellationAfterDefinitionProvider(t *testing.T) {
	root := writeFixtureProject(t, validProjectManifest)
	ctx, cancel := context.WithCancel(context.Background())
	err := Run(ctx, []string{"generate"}, func() (appkit.Definition, error) {
		cancel()
		return fixtureDefinition(&inspectionCounters{}, false), nil
	}, Options{Root: root, Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if entries, readErr := os.ReadDir(root); readErr != nil || len(entries) != 1 {
		t.Fatalf("provider cancellation changed project: entries=%v err=%v", entries, readErr)
	}
}

func TestGenerateContextCancellationAfterPreparationLeavesNoMutation(t *testing.T) {
	for trigger := 1; trigger <= 160; trigger++ {
		root := writeFixtureProject(t, validProjectManifest)
		project := loadFixtureProject(t, root)
		prepared := false
		ctx := newCheckpointContext(trigger, true, func() {
			_ = filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
				if err == nil && strings.HasPrefix(entry.Name(), ".modary-") {
					prepared = true
				}
				return nil
			})
		})
		_, err := project.GenerateContext(ctx, fixtureDefinition(&inspectionCounters{}, false))
		if !prepared {
			continue
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("checkpoint %d error = %v, want context.Canceled", trigger, err)
		}
		assertNoTemporaryArtifacts(t, root)
		if entries, readErr := os.ReadDir(root); readErr != nil || len(entries) != 1 {
			t.Fatalf("checkpoint %d left filesystem changes: entries=%v err=%v", trigger, entries, readErr)
		}
		return
	}
	t.Fatal("no cancellation checkpoint was observed after temporary preparation")
}

func TestVerifiedRootHandleRejectsPathnameSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming an opened directory is not portable to Windows")
	}
	for _, operation := range []string{"load", "generate"} {
		t.Run(operation, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "project")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ProjectManifestName), []byte(validProjectManifest), 0o644); err != nil {
				t.Fatal(err)
			}
			var project *Project
			if operation == "generate" {
				project = loadFixtureProject(t, root)
			}
			moved := filepath.Join(parent, "opened-root")
			swap := func() {
				if err := os.Rename(root, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, ProjectManifestName), []byte(validProjectManifest), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ctx := newCheckpointContext(3, false, swap)
			var err error
			if operation == "load" {
				_, err = LoadContext(ctx, root)
			} else {
				_, err = project.GenerateContext(ctx, fixtureDefinition(&inspectionCounters{}, false))
			}
			if err == nil || (!strings.Contains(err.Error(), "root pathname changed") && !strings.Contains(err.Error(), "root handle changed")) {
				t.Fatalf("%s error = %v", operation, err)
			}
			assertNoTemporaryArtifacts(t, root)
			assertNoTemporaryArtifacts(t, moved)
		})
	}
}
