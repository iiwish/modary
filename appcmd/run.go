package appcmd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/iiwish/modary/appkit"
)

// Run dispatches the consumer application command. Parsing, help, version, and
// usage failures are pure and do not construct a Definition or start Modules.
func Run(ctx context.Context, args []string, provider DefinitionProvider, options Options) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	prepared, err := prepareIOOptions(options)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return writeHelp(prepared.Stdout, commandName(prepared.Metadata))
	}
	switch args[0] {
	case "help", "--help", "-h":
		if len(args) != 1 {
			return usageError("help accepts no arguments")
		}
		return writeHelp(prepared.Stdout, commandName(prepared.Metadata))
	case "version", "--version", "-v":
		if len(args) != 1 {
			return usageError("version accepts no arguments")
		}
		if err := validateCommandMetadata(prepared.Metadata); err != nil {
			return err
		}
		return writeString(prepared.Stdout, fmt.Sprintf("%s %s\n", prepared.Metadata.ID, prepared.Metadata.Version))
	case "serve":
		listenAddress := prepared.ListenAddress
		if listenAddress == "" {
			listenAddress = DefaultListenAddress
		}
		listenAddress, help, err := parseServeArgs(args[1:], listenAddress)
		if err != nil {
			return err
		}
		if help {
			return writeServeHelp(prepared.Stdout, commandName(prepared.Metadata))
		}
		normalized, err := normalizeOptions(prepared)
		if err != nil {
			return err
		}
		normalized.ListenAddress = listenAddress
		if err := validateServeCommandOptions(normalized); err != nil {
			return err
		}
		definition, err := definitionForCommand(ctx, provider, prepared.Metadata)
		if err != nil {
			return err
		}
		return serve(ctx, definition, normalized)
	case "migrate":
		if len(args) != 1 {
			return usageError("migrate accepts no arguments")
		}
		normalized, err := normalizeOptions(prepared)
		if err != nil {
			return err
		}
		definition, err := definitionForCommand(ctx, provider, prepared.Metadata)
		if err != nil {
			return err
		}
		normalized.Logger.InfoContext(ctx, "database migration started", "event", "database.migration.started")
		if err := appkit.Migrate(ctx, definition, normalized.App); err != nil {
			return errors.Join(err, loggerOutputError(normalized))
		}
		normalized.Logger.InfoContext(ctx, "database migration completed", "event", "database.migration.completed")
		return loggerOutputError(normalized)
	case "action":
		command, help, err := parseActionCommand(args[1:])
		if err != nil {
			return err
		}
		if help {
			return writeActionHelp(prepared.Stdout, commandName(prepared.Metadata))
		}
		normalized, err := normalizeOptions(prepared)
		if err != nil {
			return err
		}
		if err := validateCommandMetadata(prepared.Metadata); err != nil {
			return err
		}
		definition, err := definitionForCommand(ctx, provider, prepared.Metadata)
		if err != nil {
			return err
		}
		return runParsedAction(ctx, command, definition, normalized)
	default:
		return usageError("unknown command %q", args[0])
	}
}

func loggerOutputError(options normalizedOptions) error {
	if options.loggerOutput == nil {
		return nil
	}
	return options.loggerOutput.Err()
}

func commandName(metadata appkit.Metadata) string {
	if appkit.ValidateMetadata(metadata) != nil {
		return "application"
	}
	return metadata.ID
}

func validateCommandMetadata(metadata appkit.Metadata) error {
	if err := appkit.ValidateMetadata(metadata); err != nil {
		return usageError("application command metadata is invalid: %v", err)
	}
	return nil
}

func definitionForCommand(ctx context.Context, provider DefinitionProvider, metadata appkit.Metadata) (appkit.Definition, error) {
	if err := validateCommandMetadata(metadata); err != nil {
		return appkit.Definition{}, err
	}
	if err := ctx.Err(); err != nil {
		return appkit.Definition{}, err
	}
	definition, err := invokeDefinitionProvider(provider)
	if err != nil {
		return appkit.Definition{}, err
	}
	if err := ctx.Err(); err != nil {
		return appkit.Definition{}, err
	}
	if definition.Metadata != metadata {
		return appkit.Definition{}, usageError("application Definition metadata must exactly match command metadata")
	}
	return definition, nil
}

func invokeDefinitionProvider(provider DefinitionProvider) (definition appkit.Definition, err error) {
	if provider == nil {
		return appkit.Definition{}, usageError("Definition provider is required")
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
		err = definitionProviderError(err)
	}
	returned = true
	return definition, err
}

func writeHelp(writer io.Writer, name string) error {
	return writeString(writer, fmt.Sprintf(`Usage:
  %s serve [--listen address]
  %s migrate
  %s action run <action-id> --token-file <path|-> --input <path|-> [--preview] [--plan <hash>] [--idempotency-key <key>] [--request-id <id>]
  %s version
  %s help
`, name, name, name, name, name))
}

func writeString(writer io.Writer, value string) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "output writer"}
		}
	}()
	written, err := io.WriteString(writer, value)
	returned = true
	if err != nil {
		return opaqueCommandError("write command output failed", err)
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}
