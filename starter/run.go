package starter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
)

// CommandUsage is the global create-only Starter command help.
const CommandUsage = `Usage:
  modary new <destination> [--profile api|admin|governed] [--module module/path] [--name display-name]
  modary help
`

var (
	// ErrUsage identifies invalid global command syntax.
	ErrUsage = errors.New("starter command usage is invalid")
)

// Options configures the global Starter command. ModaryVersion and
// ModaryReplace are release/conformance inputs rather than generated runtime
// configuration.
type Options struct {
	Stdout        io.Writer
	Stderr        io.Writer
	ModaryVersion string
	ModaryReplace string
}

// Run dispatches the create-only global Starter command.
func Run(ctx context.Context, args []string, options Options) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stdout, _, err := commandWriters(options)
	if err != nil {
		return err
	}
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")) {
		_, err := io.WriteString(stdout, CommandUsage)
		return err
	}
	if args[0] != "new" {
		return fmt.Errorf("%w: unknown command %q", ErrUsage, args[0])
	}
	createOptions, err := parseNewCommand(args[1:])
	if err != nil {
		return err
	}
	createOptions.ModaryVersion = options.ModaryVersion
	createOptions.ModaryReplace = options.ModaryReplace
	result, err := Create(ctx, createOptions)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode Starter result: %w", err)
	}
	_, err = stdout.Write(append(encoded, '\n'))
	return err
}

func parseNewCommand(args []string) (CreateOptions, error) {
	values := make(map[string]string)
	destination := ""
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "--") {
			if destination != "" {
				return CreateOptions{}, fmt.Errorf("%w: new accepts one destination", ErrUsage)
			}
			destination = argument
			continue
		}
		name := strings.TrimPrefix(argument, "--")
		value := ""
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			value = name[separator+1:]
			name = name[:separator]
		} else {
			index++
			if index >= len(args) {
				return CreateOptions{}, fmt.Errorf("%w: --%s requires a value", ErrUsage, name)
			}
			value = args[index]
		}
		if name != "profile" && name != "module" && name != "name" {
			return CreateOptions{}, fmt.Errorf("%w: unknown new option --%s", ErrUsage, name)
		}
		if value == "" || strings.HasPrefix(value, "--") {
			return CreateOptions{}, fmt.Errorf("%w: --%s requires a value", ErrUsage, name)
		}
		if _, duplicate := values[name]; duplicate {
			return CreateOptions{}, fmt.Errorf("%w: --%s was provided more than once", ErrUsage, name)
		}
		values[name] = value
	}
	if destination == "" {
		return CreateOptions{}, fmt.Errorf("%w: new requires a destination", ErrUsage)
	}
	return CreateOptions{
		Destination: destination,
		ModulePath:  values["module"],
		Name:        values["name"],
		Profile:     Profile(values["profile"]),
	}, nil
}

func commandWriters(options Options) (io.Writer, io.Writer, error) {
	if isTypedNil(options.Stdout) || isTypedNil(options.Stderr) {
		return nil, nil, fmt.Errorf("%w: command writer cannot be typed nil", ErrUsage)
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

func isTypedNil(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
