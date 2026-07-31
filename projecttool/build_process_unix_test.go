//go:build linux || darwin

package projecttool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAwaitBuildCommandAcceptsSuccessfulPipeBackstop(t *testing.T) {
	ctx := context.Background()
	command := exec.CommandContext(ctx, "sh", "-c", "printf inherited-output")
	command.Stdout = delayedBuildWriter{delay: 50 * time.Millisecond}
	command.WaitDelay = 10 * time.Millisecond
	configureBuildCommand(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	commandErr, cleanupErr := awaitBuildCommand(ctx, command)
	if commandErr != nil || cleanupErr != nil {
		t.Fatalf("successful command with pipe backstop = (%v, %v), want nil errors", commandErr, cleanupErr)
	}
}

type delayedBuildWriter struct{ delay time.Duration }

func (writer delayedBuildWriter) Write(data []byte) (int, error) {
	time.Sleep(writer.delay)
	return len(data), nil
}

func readProcessIDForTest(name string) (int, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid child process id %q: %w", data, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid child process id %q", data)
	}
	return pid, nil
}

func processExistsForTest(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
