package projecttool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/iiwish/modary/appkit"
)

// CommandUsage is the stable usage text for a consumer-owned project tool.
const CommandUsage = `Usage:
  modary verify
  modary generate
  modary generate --check
  modary check
  modary build
  modary help
`

// DefinitionProvider constructs the consumer's explicit Go composition. Run
// calls it once only after command syntax and modary.yaml have been validated.
type DefinitionProvider = appkit.DefinitionProvider

// Options configures the consumer-owned project command. An empty root selects
// the current directory and nil writers select the process standard streams.
type Options struct {
	Root   string
	Stdout io.Writer
	Stderr io.Writer
}

type projectCommand uint8

const (
	commandHelp projectCommand = iota
	commandVerify
	commandGenerate
	commandCheck
	commandBuild
)

// Run dispatches pure consumer project commands. Invalid syntax and help do not
// read the project, while an invalid manifest fails before DefinitionProvider.
func Run(ctx context.Context, args []string, provider DefinitionProvider, options Options) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	command, err := parseProjectCommand(args)
	if err != nil {
		return err
	}
	stdout, stderr, err := normalizeCommandWriters(options)
	if err != nil {
		return err
	}
	if command == commandHelp {
		return writeCommandBytesContext(ctx, stdout, []byte(CommandUsage))
	}

	project, err := LoadContext(ctx, options.Root)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	definition, err := invokeDefinitionProvider(provider)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	switch command {
	case commandVerify:
		snapshot, err := project.VerifyContext(ctx, definition)
		if err != nil {
			return err
		}
		return writeCommandJSONContext(ctx, stdout, struct {
			Application appkit.Metadata `json:"application"`
			Modules     int             `json:"modules"`
			Actions     int             `json:"actions"`
		}{Application: snapshot.Application, Modules: len(snapshot.Modules), Actions: len(snapshot.Actions)})
	case commandGenerate:
		result, err := project.GenerateContext(ctx, definition)
		if err != nil {
			return err
		}
		return writeCommandJSONContext(ctx, stdout, result)
	case commandCheck:
		drift, err := project.CheckContext(ctx, definition)
		if err != nil {
			return err
		}
		if len(drift) != 0 {
			return &DriftError{Items: append([]Drift(nil), drift...)}
		}
		return writeCommandJSONContext(ctx, stdout, struct {
			Current bool `json:"current"`
		}{Current: true})
	case commandBuild:
		result, err := project.Build(ctx, definition, BuildOptions{Stdout: stdout, Stderr: stderr})
		if err != nil {
			return err
		}
		return writeCommandJSONContext(ctx, stdout, result)
	default:
		return newUsageError("unsupported project command")
	}
}

func parseProjectCommand(args []string) (projectCommand, error) {
	if len(args) == 0 {
		return commandHelp, nil
	}
	switch args[0] {
	case "help", "--help", "-h":
		if len(args) != 1 {
			return 0, newUsageError("help accepts no arguments")
		}
		return commandHelp, nil
	case "verify":
		if len(args) != 1 {
			return 0, newUsageError("verify accepts no arguments")
		}
		return commandVerify, nil
	case "generate":
		if len(args) == 1 {
			return commandGenerate, nil
		}
		if len(args) == 2 && args[1] == "--check" {
			return commandCheck, nil
		}
		return 0, newUsageError("generate accepts only --check")
	case "check":
		if len(args) != 1 {
			return 0, newUsageError("check accepts no arguments")
		}
		return commandCheck, nil
	case "build":
		if len(args) != 1 {
			return 0, newUsageError("build accepts no arguments")
		}
		return commandBuild, nil
	default:
		return 0, newUsageError("unknown project command %q", args[0])
	}
}

func normalizeCommandWriters(options Options) (io.Writer, io.Writer, error) {
	if isTypedNil(options.Stdout) {
		return nil, nil, newUsageError("stdout Writer cannot be typed nil")
	}
	if isTypedNil(options.Stderr) {
		return nil, nil, newUsageError("stderr Writer cannot be typed nil")
	}
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := options.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	return stdout, stderr, nil
}

func invokeDefinitionProvider(provider DefinitionProvider) (definition appkit.Definition, err error) {
	if provider == nil {
		return appkit.Definition{}, newUsageError("Definition provider is required")
	}
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			definition = appkit.Definition{}
			err = &CallbackPanicError{Operation: "Definition provider"}
		}
	}()
	definition, err = provider()
	if err != nil {
		definition = appkit.Definition{}
		err = newDefinitionProviderError(err)
	}
	returned = true
	return definition, err
}

func writeCommandJSON(writer io.Writer, value any) error {
	return writeCommandJSONContext(context.Background(), writer, value)
}

func writeCommandJSONContext(ctx context.Context, writer io.Writer, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode project command result: %w", err)
	}
	return writeCommandBytesContext(ctx, writer, append(data, '\n'))
}

func writeCommandBytes(writer io.Writer, data []byte) error {
	return writeCommandBytesContext(context.Background(), writer, data)
}

func writeCommandBytesContext(ctx context.Context, writer io.Writer, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	guarded := &guardedWriter{gate: &sync.Mutex{}, target: writer, operation: "command output"}
	_, err := guarded.Write(data)
	if err != nil {
		return err
	}
	return ctx.Err()
}
