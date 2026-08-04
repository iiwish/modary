package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/scope"
)

const (
	minimumCLITokenBytes = int64(32)
	maximumCLITokenBytes = int64(4096)
)

var (
	cliPlanHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

	errActionInputTooLarge = errors.New("Action input exceeds the configured limit")
)

type actionCommand struct {
	actionID       string
	tokenFile      string
	inputPath      string
	preview        bool
	planHash       string
	idempotencyKey string
	requestID      string
	scopeKind      string
	scopeID        string
}

type actionFlagState struct {
	tokenFile   bool
	input       bool
	preview     bool
	plan        bool
	idempotency bool
	requestID   bool
	scopeKind   bool
	scopeID     bool
}

// RunAction executes the Action subcommand with the same arguments accepted
// after "action" by Run. Parsing and input validation complete before Modules
// are started.
func RunAction(ctx context.Context, args []string, definition appkit.Definition, options Options) error {
	if ctx == nil {
		return ErrContextRequired
	}
	normalized, err := normalizeOptions(options)
	if err != nil {
		return err
	}
	return runAction(ctx, args, definition, normalized)
}

func runAction(ctx context.Context, args []string, definition appkit.Definition, options normalizedOptions) error {
	command, help, err := parseActionCommand(args)
	if err != nil {
		return err
	}
	if help {
		return writeActionHelp(options.Stdout, commandName(definition.Metadata))
	}
	return runParsedAction(ctx, command, definition, options)
}

func runParsedAction(ctx context.Context, command actionCommand, definition appkit.Definition, options normalizedOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	token, err := loadCLIToken(ctx, command.tokenFile, options.Stdin)
	if err != nil {
		return err
	}
	defer clear(token)
	input, err := loadActionInput(ctx, command.inputPath, options.Stdin, options.MaxActionInputBytes)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return executeActionCommand(ctx, command, token, input, definition, options)
}

func parseActionCommand(args []string) (actionCommand, bool, error) {
	if len(args) == 1 && isActionHelp(args[0]) {
		return actionCommand{}, true, nil
	}
	if len(args) == 0 || args[0] != "run" {
		return actionCommand{}, false, usageError("action requires the run subcommand")
	}
	if len(args) == 2 && isActionHelp(args[1]) {
		return actionCommand{}, true, nil
	}
	if len(args) < 2 {
		return actionCommand{}, false, usageError("action run requires an Action id")
	}

	command := actionCommand{actionID: args[1]}
	var seen actionFlagState
	for index := 2; index < len(args); index++ {
		name, inlineValue, hasInlineValue, ok := splitActionFlag(args[index])
		if !ok {
			return actionCommand{}, false, usageError("action run has unexpected argument %q", args[index])
		}
		switch name {
		case "token-file":
			value, next, err := actionFlagValue(name, inlineValue, hasInlineValue, args, index, seen.tokenFile)
			if err != nil {
				return actionCommand{}, false, err
			}
			seen.tokenFile = true
			command.tokenFile = value
			index = next
		case "input":
			value, next, err := actionFlagValue(name, inlineValue, hasInlineValue, args, index, seen.input)
			if err != nil {
				return actionCommand{}, false, err
			}
			seen.input = true
			command.inputPath = value
			index = next
		case "preview":
			if seen.preview {
				return actionCommand{}, false, usageError("action run flag --preview may be provided only once")
			}
			seen.preview = true
			command.preview = true
			if hasInlineValue {
				parsed, err := strconv.ParseBool(inlineValue)
				if err != nil {
					return actionCommand{}, false, usageError("action run flag --preview requires a boolean value")
				}
				command.preview = parsed
			}
		case "plan":
			value, next, err := actionFlagValue(name, inlineValue, hasInlineValue, args, index, seen.plan)
			if err != nil {
				return actionCommand{}, false, err
			}
			seen.plan = true
			command.planHash = value
			index = next
		case "idempotency-key":
			value, next, err := actionFlagValue(name, inlineValue, hasInlineValue, args, index, seen.idempotency)
			if err != nil {
				return actionCommand{}, false, err
			}
			seen.idempotency = true
			command.idempotencyKey = value
			index = next
		case "request-id":
			value, next, err := actionFlagValue(name, inlineValue, hasInlineValue, args, index, seen.requestID)
			if err != nil {
				return actionCommand{}, false, err
			}
			seen.requestID = true
			command.requestID = value
			index = next
		case "scope-kind":
			value, next, err := actionFlagValue(name, inlineValue, hasInlineValue, args, index, seen.scopeKind)
			if err != nil {
				return actionCommand{}, false, err
			}
			seen.scopeKind = true
			command.scopeKind = value
			index = next
		case "scope-id":
			value, next, err := actionFlagValue(name, inlineValue, hasInlineValue, args, index, seen.scopeID)
			if err != nil {
				return actionCommand{}, false, err
			}
			seen.scopeID = true
			command.scopeID = value
			index = next
		default:
			return actionCommand{}, false, usageError("action run flag --%s is not recognized", name)
		}
	}

	if !seen.tokenFile {
		return actionCommand{}, false, usageError("action run requires --token-file")
	}
	if !seen.input {
		return actionCommand{}, false, usageError("action run requires --input")
	}
	if !seen.scopeKind || !seen.scopeID {
		return actionCommand{}, false, usageError("action run requires --scope-kind and --scope-id")
	}
	if err := validateActionCommand(command, seen); err != nil {
		return actionCommand{}, false, err
	}
	return command, false, nil
}

func isActionHelp(argument string) bool {
	return argument == "help" || argument == "--help" || argument == "-h"
}

func splitActionFlag(argument string) (name, value string, hasValue, ok bool) {
	if !strings.HasPrefix(argument, "--") || len(argument) <= 2 {
		return "", "", false, false
	}
	name, value, hasValue = strings.Cut(argument[2:], "=")
	if name == "" {
		return "", "", false, false
	}
	return name, value, hasValue, true
}

func actionFlagValue(name, inlineValue string, hasInlineValue bool, args []string, index int, duplicate bool) (string, int, error) {
	if duplicate {
		return "", index, usageError("action run flag --%s may be provided only once", name)
	}
	if hasInlineValue {
		if inlineValue == "" {
			return "", index, usageError("action run flag --%s requires a value", name)
		}
		return inlineValue, index, nil
	}
	if index+1 >= len(args) {
		return "", index, usageError("action run flag --%s requires a value", name)
	}
	return args[index+1], index + 1, nil
}

func validateActionCommand(command actionCommand, seen actionFlagState) error {
	if !action.ValidIdentifier(command.actionID) {
		return usageError("Action id %q is invalid", command.actionID)
	}
	if err := validateActionPath("CLI token file", command.tokenFile); err != nil {
		return err
	}
	if err := validateActionPath("Action input", command.inputPath); err != nil {
		return err
	}
	if command.tokenFile == "-" && command.inputPath == "-" {
		return usageError("Action token and input cannot both be read from stdin")
	}
	if seen.plan && !cliPlanHashPattern.MatchString(command.planHash) {
		return usageError("action plan hash must be a lowercase SHA-256 digest")
	}
	if seen.idempotency {
		if err := validateActionArgument("idempotency key", command.idempotencyKey, 256); err != nil {
			return err
		}
	}
	if seen.requestID {
		if err := validateActionArgument("request id", command.requestID, 128); err != nil {
			return err
		}
	}
	if command.preview && (seen.plan || seen.idempotency) {
		return usageError("action preview cannot use --plan or --idempotency-key")
	}
	if _, err := scope.New(command.scopeKind, command.scopeID); err != nil {
		return usageError("action execution scope is invalid: %v", err)
	}
	return nil
}

func validateActionArgument(name, value string, maximumRunes int) error {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		utf8.RuneCountInString(value) > maximumRunes || strings.ContainsFunc(value, unicode.IsControl) {
		return usageError("action %s is invalid", name)
	}
	return nil
}

func validateActionPath(label, value string) error {
	if value == "" || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsFunc(value, unicode.IsControl) {
		return usageError("%s path is invalid", label)
	}
	return nil
}

func writeActionHelp(writer io.Writer, name string) error {
	return writeString(writer, fmt.Sprintf(
		"Usage:\n  %s action run <action-id> --token-file <path|-> --input <path|-> --scope-kind <kind> --scope-id <id> [--preview] [--plan <hash>] [--idempotency-key <key>] [--request-id <id>]\n",
		name,
	))
}

func loadCLIToken(ctx context.Context, tokenFile string, stdin io.ReadCloser) ([]byte, error) {
	const rawLimit = maximumCLITokenBytes + 2
	var (
		data []byte
		err  error
	)
	if tokenFile == "-" {
		data, err = readCancelableActionInput(ctx, &actionInput{ReadCloser: stdin}, rawLimit)
	} else {
		data, err = readCLITokenFile(ctx, tokenFile, rawLimit)
	}
	if err != nil {
		if safeerr.Is(err, context.Canceled) || safeerr.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if safeerr.Is(err, errActionInputTooLarge) {
			return nil, usageError("CLI token file exceeds the configured limit")
		}
		return nil, &commandError{kind: ErrUsage, message: "read CLI token file", cause: err}
	}
	if bytes.HasSuffix(data, []byte("\r\n")) {
		data = data[:len(data)-2]
	} else if bytes.HasSuffix(data, []byte("\n")) {
		data = data[:len(data)-1]
	}
	if int64(len(data)) < minimumCLITokenBytes || int64(len(data)) > maximumCLITokenBytes || !utf8.Valid(data) {
		clear(data)
		return nil, usageError("CLI token file must contain one UTF-8 token between %d and %d bytes", minimumCLITokenBytes, maximumCLITokenBytes)
	}
	token := string(data)
	if strings.TrimSpace(token) != token || strings.ContainsFunc(token, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character)
	}) {
		clear(data)
		return nil, usageError("CLI token file contains invalid whitespace or control characters")
	}
	return data, nil
}

func readCLITokenFile(ctx context.Context, name string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateCLITokenFilePathSupport(); err != nil {
		return nil, err
	}
	before, err := os.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect token file: %w", err)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("token path is not a regular file")
	}
	if err := validateCLITokenFileMetadata(before); err != nil {
		return nil, err
	}
	if before.Size() > limit {
		return nil, errActionInputTooLarge
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open token file: %w", err)
	}
	input := &actionInput{ReadCloser: file}
	openedBefore, statErr := file.Stat()
	if statErr != nil {
		return nil, errors.Join(fmt.Errorf("inspect opened token file: %w", statErr), input.Close())
	}
	if !sameCLIFileState(before, openedBefore) {
		return nil, errors.Join(fmt.Errorf("token file changed while it was opened"), input.Close())
	}
	if err := validateOpenedCLITokenFile(file, openedBefore); err != nil {
		return nil, errors.Join(err, input.Close())
	}
	data, readErr := readCancelableActionInput(ctx, input, limit)
	openedAfter, restatErr := file.Stat()
	after, lstatErr := os.Lstat(name)
	if readErr != nil || restatErr != nil || lstatErr != nil {
		clear(data)
		return nil, fmt.Errorf("read token file consistently: %w", errors.Join(readErr, restatErr, lstatErr, input.Close()))
	}
	if !sameCLIFileState(openedBefore, openedAfter) || !sameCLIFileState(openedAfter, after) {
		clear(data)
		return nil, errors.Join(fmt.Errorf("token file changed while it was read"), input.Close())
	}
	if err := validateOpenedCLITokenFile(file, openedAfter); err != nil {
		clear(data)
		return nil, errors.Join(err, input.Close())
	}
	if err := validateCLITokenFileMetadata(after); err != nil {
		clear(data)
		return nil, errors.Join(err, input.Close())
	}
	if err := input.Close(); err != nil {
		clear(data)
		return nil, fmt.Errorf("close token file: %w", err)
	}
	return data, nil
}

func sameCLIFileState(first, second os.FileInfo) bool {
	return first != nil && second != nil && os.SameFile(first, second) &&
		first.Mode() == second.Mode() && first.Size() == second.Size() && first.ModTime().Equal(second.ModTime())
}

func loadActionInput(ctx context.Context, inputPath string, stdin io.ReadCloser, limit int64) (json.RawMessage, error) {
	var data []byte
	var err error
	if inputPath == "-" {
		data, err = readCancelableActionInput(ctx, &actionInput{ReadCloser: stdin}, limit)
	} else {
		data, err = readActionInputFile(ctx, inputPath, limit)
	}
	if err != nil {
		if safeerr.Is(err, context.Canceled) || safeerr.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if safeerr.Is(err, errActionInputTooLarge) {
			return nil, usageError("Action input exceeds the configured %d-byte limit", limit)
		}
		return nil, &commandError{kind: ErrUsage, message: "read Action input", cause: err}
	}
	trimmed := bytes.TrimSpace(data)
	if err := validateSingleActionJSON(trimmed); err != nil {
		return nil, &commandError{kind: ErrUsage, message: "Action input must contain exactly one valid JSON value without duplicate object members", cause: err}
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func readActionInputFile(ctx context.Context, name string, limit int64) (data []byte, err error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect input file: %w", err)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("input path is not a regular file")
	}
	if before.Size() > limit {
		return nil, errActionInputTooLarge
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open input file: %w", err)
	}
	input := &actionInput{ReadCloser: file}
	defer func() {
		if closeErr := input.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close input file: %w", closeErr))
		}
	}()
	after, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened input file: %w", err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, fmt.Errorf("input file changed while it was opened")
	}
	return readCancelableActionInput(ctx, input, limit)
}

type actionInput struct {
	io.ReadCloser
	once sync.Once
	err  error
}

// Close closes the Action input at most once and contains closer panics.
func (input *actionInput) Close() error {
	input.once.Do(func() {
		returned := false
		defer func() {
			if !returned {
				_ = recover()
				input.err = &CallbackPanicError{Operation: "Action input closer"}
			}
		}()
		input.err = input.ReadCloser.Close()
		returned = true
		if input.err != nil {
			input.err = opaqueCommandError("close Action input failed", input.err)
		}
	})
	return input.err
}

func readCancelableActionInput(ctx context.Context, input *actionInput, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(closed)
		_ = input.Close()
	})
	data, err := readLimitedActionInput(input, limit)
	if !stop() {
		<-closed
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	return data, err
}

func readLimitedActionInput(reader io.Reader, limit int64) (data []byte, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			data = nil
			err = &CallbackPanicError{Operation: "Action input reader"}
		}
	}()
	limited := &io.LimitedReader{R: reader, N: limit + 1}
	data, err = io.ReadAll(limited)
	returned = true
	if err != nil {
		return nil, opaqueCommandError("read Action input failed", err)
	}
	if int64(len(data)) > limit {
		return nil, errActionInputTooLarge
	}
	return data, nil
}

func validateSingleActionJSON(data []byte) error {
	return action.ValidateJSONDocument(json.RawMessage(data))
}

func executeActionCommand(ctx context.Context, command actionCommand, token []byte, input json.RawMessage, definition appkit.Definition, options normalizedOptions) error {
	application, err := appkit.Start(ctx, definition, options.App)
	if err != nil {
		return fmt.Errorf("start application: %w", err)
	}
	if application == nil {
		return fmt.Errorf("start application: application is unavailable")
	}
	primary := callStartedAction(ctx, command, token, input, application, options.Stdout)
	return joinApplicationShutdown(primary, application, options.ShutdownTimeout)
}

func callStartedAction(ctx context.Context, command actionCommand, token []byte, input json.RawMessage, application *appkit.Application, output io.Writer) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "Action command"}
		}
	}()
	err = callStartedActionUnchecked(ctx, command, token, input, application, output)
	returned = true
	return err
}

func callStartedActionUnchecked(ctx context.Context, command actionCommand, token []byte, input json.RawMessage, application *appkit.Application, output io.Writer) error {
	tokens, err := application.Tokens()
	if err != nil {
		return fmt.Errorf("resolve token authentication service: %w", err)
	}
	if isNilInterface(tokens) {
		return fmt.Errorf("resolve token authentication service: authenticator is unavailable")
	}
	actor, err := callTokenAuthenticator(ctx, tokens, string(token))
	if err != nil {
		return &commandError{message: "authenticate CLI token failed", cause: err}
	}
	runtime := application.Runtime()
	if isNilInterface(runtime) {
		return fmt.Errorf("Action Runtime is unavailable")
	}
	request := action.Request{
		RequestID:      command.requestID,
		Actor:          actor,
		Channel:        action.ChannelCLI,
		ActionID:       command.actionID,
		Scope:          scope.Must(command.scopeKind, command.scopeID),
		Input:          append(json.RawMessage(nil), input...),
		IdempotencyKey: command.idempotencyKey,
		PlanHash:       command.planHash,
	}
	if command.preview {
		preview, err := callActionPreview(ctx, runtime, request)
		if err != nil {
			return fmt.Errorf("preview Action %s: %w", command.actionID, err)
		}
		return writeActionJSON(output, preview)
	}
	result, err := callActionExecute(ctx, runtime, request)
	if err != nil {
		return fmt.Errorf("execute Action %s: %w", command.actionID, err)
	}
	return writeActionJSON(output, result)
}

func callTokenAuthenticator(ctx context.Context, authenticator identity.TokenAuthenticator, token string) (actor identity.Actor, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			actor = identity.Actor{}
			err = &CallbackPanicError{Operation: "token authenticator"}
		}
	}()
	actor, err = authenticator.AuthenticateToken(ctx, token)
	returned = true
	if err != nil {
		actor = identity.Actor{}
		return actor, opaqueCommandError("token authenticator failed", err)
	}
	if validationErr := identity.ValidateActor(actor); validationErr != nil {
		actor = identity.Actor{}
		err = fmt.Errorf("token authenticator returned an invalid actor")
	}
	return actor, err
}

func callActionPreview(ctx context.Context, runtime appkit.Runtime, request action.Request) (preview action.Preview, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			preview = action.Preview{}
			err = &CallbackPanicError{Operation: "Action Runtime preview"}
		}
	}()
	preview, err = runtime.Preview(ctx, request)
	returned = true
	if err != nil {
		preview = action.Preview{}
		err = actionCommandError("Action Runtime preview failed", err)
	}
	return preview, err
}

func callActionExecute(ctx context.Context, runtime appkit.Runtime, request action.Request) (result action.Result, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			result = action.Result{}
			err = &CallbackPanicError{Operation: "Action Runtime execute"}
		}
	}()
	result, err = runtime.Execute(ctx, request)
	returned = true
	if err != nil {
		result = action.Result{}
		err = actionCommandError("Action Runtime execute failed", err)
	}
	return result, err
}

func writeActionJSON(writer io.Writer, value any) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "output writer"}
		}
	}()
	err = writeActionJSONUnchecked(writer, value)
	returned = true
	return err
}

func writeActionJSONUnchecked(writer io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal Action output: %w", err)
	}
	data = append(data, '\n')
	for len(data) != 0 {
		written, writeErr := writer.Write(data)
		if writeErr != nil {
			writeErr = opaqueCommandError("write Action output failed", writeErr)
		}
		if written < 0 || written > len(data) {
			if writeErr != nil {
				return writeErr
			}
			return fmt.Errorf("write Action output: invalid write count")
		}
		data = data[written:]
		if writeErr != nil {
			return writeErr
		}
		if written == 0 {
			return fmt.Errorf("write Action output: %w", io.ErrShortWrite)
		}
	}
	return nil
}
