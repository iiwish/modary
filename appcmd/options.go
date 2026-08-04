package appcmd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"reflect"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/processkit"
)

// Command and HTTP server defaults are bounded and local-listener safe.
const (
	DefaultListenAddress       = "127.0.0.1:8080"
	DefaultShutdownTimeout     = 10 * time.Second
	DefaultReadHeaderTimeout   = 5 * time.Second
	DefaultReadTimeout         = 30 * time.Second
	DefaultWriteTimeout        = 30 * time.Second
	DefaultIdleTimeout         = 90 * time.Second
	DefaultMaxHeaderBytes      = 1 << 20
	MaximumHeaderBytes         = 16 << 20
	DefaultMaxActionInputBytes = action.MaxJSONDocumentBytes
	MaximumActionInputBytes    = action.MaxJSONDocumentBytes
)

// Sentinel command errors support errors.Is classification.
var (
	ErrContextRequired = errors.New("command context is required")
	ErrUsage           = errors.New("invalid command usage")
	ErrCallbackPanic   = errors.New("application command callback panic")
)

// DefinitionProvider constructs the consumer's explicit application
// composition. Run invokes it once only for a command that needs Modules.
type DefinitionProvider = appkit.DefinitionProvider

// HandlerFactory constructs the complete consumer-owned HTTP mount after the
// Application is fully started. Serve invokes it once with the command context
// and it must honor cancellation and deadlines and return promptly after
// cancellation. Appcmd never mounts a transport implicitly.
type HandlerFactory func(context.Context, *appkit.Application) (http.Handler, error)

// ListenerFactory allows consumers to supply socket activation or a test
// listener while preserving appcmd's lifecycle ordering. Custom Accept errors
// fail closed; only the framework-owned default listener retains net/http's
// temporary-error retry policy. Custom Listener.Addr and accepted connections
// are cooperative data-plane extensions: Read, Write, address, and deadline
// methods must obey their net package contracts. The factory must honor its
// context and return promptly after cancellation.
type ListenerFactory func(context.Context, string, string) (net.Listener, error)

// ServerOptions configures the public HTTP server without exposing it to the
// application lifecycle.
type ServerOptions struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// Options contains consumer policy for application and command execution.
// All zero values have local-development-safe defaults.
type Options struct {
	// Metadata is the pure command identity used by Run for help and version.
	// The DefinitionProvider must return this exact value for serve and action.
	Metadata appkit.Metadata
	App      appkit.Options
	Handler  HandlerFactory
	// Process explicitly enables shared readiness probes and pre-shutdown
	// admission drain. The consumer still owns and mounts the probe handlers.
	Process       *processkit.Manager
	Listener      ListenerFactory
	ListenAddress string
	// Stdin is closed only when a command context is canceled while an Action
	// input is being read from "-". Close must be safe concurrently with Read
	// and must unblock it; ownership otherwise remains with the consumer.
	Stdin io.ReadCloser
	// Stdout and Stderr are trusted, cooperative dependencies. Write must
	// return; a context or shutdown timeout cannot interrupt a blocked Writer.
	Stdout io.Writer
	Stderr io.Writer
	// Logger receives bounded structured command lifecycle diagnostics. A nil
	// Logger uses a JSON handler backed by Stderr.
	Logger              *slog.Logger
	ShutdownTimeout     time.Duration
	MaxActionInputBytes int64
	Server              ServerOptions
}

type normalizedOptions struct {
	Options
	server       ServerOptions
	loggerOutput *recordingWriter
}

func normalizeOptions(options Options) (normalizedOptions, error) {
	options, err := prepareIOOptions(options)
	if err != nil {
		return normalizedOptions{}, err
	}
	if options.Logger == nil {
		output := &recordingWriter{destination: options.Stderr, operation: "structured logger writer"}
		options.Stderr = output
		options.Logger = slog.New(slog.NewJSONHandler(output, nil))
		return normalizePreparedOptions(options, output)
	}
	return normalizePreparedOptions(options, nil)
}

func normalizePreparedOptions(options Options, loggerOutput *recordingWriter) (normalizedOptions, error) {
	if options.ShutdownTimeout < 0 {
		return normalizedOptions{}, usageError("shutdown timeout cannot be negative")
	}
	if options.ShutdownTimeout == 0 {
		options.ShutdownTimeout = DefaultShutdownTimeout
	}
	if options.MaxActionInputBytes < 0 {
		return normalizedOptions{}, usageError("maximum Action input bytes cannot be negative")
	}
	if options.MaxActionInputBytes == 0 {
		options.MaxActionInputBytes = DefaultMaxActionInputBytes
	}
	if options.MaxActionInputBytes > MaximumActionInputBytes {
		return normalizedOptions{}, usageError("maximum Action input bytes cannot exceed %d", MaximumActionInputBytes)
	}
	server, err := normalizeServerOptions(options.Server)
	if err != nil {
		return normalizedOptions{}, err
	}
	return normalizedOptions{Options: options, server: server, loggerOutput: loggerOutput}, nil
}

func prepareIOOptions(options Options) (Options, error) {
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	} else if isNilInterface(options.Stdin) {
		return Options{}, usageError("stdin ReadCloser cannot be typed nil")
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	} else if isNilInterface(options.Stdout) {
		return Options{}, usageError("stdout Writer cannot be typed nil")
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	} else if isNilInterface(options.Stderr) {
		return Options{}, usageError("stderr Writer cannot be typed nil")
	}
	return options, nil
}

func normalizeServerOptions(options ServerOptions) (ServerOptions, error) {
	values := []struct {
		name  string
		value time.Duration
	}{
		{"read header timeout", options.ReadHeaderTimeout},
		{"read timeout", options.ReadTimeout},
		{"write timeout", options.WriteTimeout},
		{"idle timeout", options.IdleTimeout},
	}
	for _, value := range values {
		if value.value < 0 {
			return ServerOptions{}, usageError("HTTP %s cannot be negative", value.name)
		}
	}
	if options.MaxHeaderBytes < 0 {
		return ServerOptions{}, usageError("HTTP maximum header bytes cannot be negative")
	}
	if options.MaxHeaderBytes > MaximumHeaderBytes {
		return ServerOptions{}, usageError("HTTP maximum header bytes cannot exceed %d", MaximumHeaderBytes)
	}
	if options.ReadHeaderTimeout == 0 {
		options.ReadHeaderTimeout = DefaultReadHeaderTimeout
	}
	if options.ReadTimeout == 0 {
		options.ReadTimeout = DefaultReadTimeout
	}
	if options.WriteTimeout == 0 {
		options.WriteTimeout = DefaultWriteTimeout
	}
	if options.IdleTimeout == 0 {
		options.IdleTimeout = DefaultIdleTimeout
	}
	if options.MaxHeaderBytes == 0 {
		options.MaxHeaderBytes = DefaultMaxHeaderBytes
	}
	return options, nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
