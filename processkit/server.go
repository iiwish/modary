package processkit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// DefaultShutdownTimeout bounds server drain and application cleanup.
const DefaultShutdownTimeout = 10 * time.Second

const (
	// DefaultReadHeaderTimeout bounds receipt of request headers.
	DefaultReadHeaderTimeout = 5 * time.Second
	// DefaultReadTimeout bounds receipt of the complete request.
	DefaultReadTimeout = 30 * time.Second
	// DefaultWriteTimeout bounds writing the complete response.
	DefaultWriteTimeout = 30 * time.Second
	// DefaultIdleTimeout bounds keep-alive connection idleness.
	DefaultIdleTimeout = 60 * time.Second
	// DefaultMaxHeaderBytes bounds request headers to one MiB.
	DefaultMaxHeaderBytes = 1 << 20
	// MaximumHeaderBytes is the largest configurable request-header boundary.
	MaximumHeaderBytes = 16 << 20
)

// ServerOptions is the shared generated HTTP process contract.
type ServerOptions struct {
	Address           string
	Handler           http.Handler
	Manager           *Manager
	Logger            *slog.Logger
	Shutdown          func(context.Context) error
	ShutdownTimeout   time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	Build             BuildInfo
}

// Serve listens, marks the process ready, serves until cancellation or failure,
// transitions readiness before shutdown, drains accepted requests, and then
// stops application resources under one timeout.
func Serve(ctx context.Context, options ServerOptions) (resultErr error) {
	if ctx == nil {
		return fmt.Errorf("process serve context is required")
	}
	if options.Address == "" || options.Handler == nil || options.Manager == nil || options.Shutdown == nil {
		return fmt.Errorf("process serve dependencies are required")
	}
	cleanupTimeout := DefaultShutdownTimeout
	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			resultErr = errors.Join(resultErr, stopOwnedApplication(options.Manager, options.Shutdown, cleanupTimeout))
		}
	}()
	for _, timeout := range []struct {
		name  string
		value time.Duration
	}{
		{name: "shutdown", value: options.ShutdownTimeout},
		{name: "read header", value: options.ReadHeaderTimeout},
		{name: "read", value: options.ReadTimeout},
		{name: "write", value: options.WriteTimeout},
		{name: "idle", value: options.IdleTimeout},
	} {
		if timeout.value < 0 || timeout.value > 10*time.Minute {
			return fmt.Errorf("process %s timeout must be between zero and ten minutes", timeout.name)
		}
	}
	if options.MaxHeaderBytes < 0 || options.MaxHeaderBytes > MaximumHeaderBytes {
		return fmt.Errorf("process maximum header bytes must be between zero and %d", MaximumHeaderBytes)
	}
	if options.ShutdownTimeout == 0 {
		options.ShutdownTimeout = DefaultShutdownTimeout
	}
	cleanupTimeout = options.ShutdownTimeout
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
	build, err := NormalizeBuildInfo(options.Build)
	if err != nil {
		return err
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	listener, err := net.Listen("tcp", options.Address)
	if err != nil {
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	server := &http.Server{
		Handler: containHandlerPanics(options.Handler, logger), ReadHeaderTimeout: options.ReadHeaderTimeout,
		ReadTimeout: options.ReadTimeout, WriteTimeout: options.WriteTimeout,
		IdleTimeout: options.IdleTimeout, MaxHeaderBytes: options.MaxHeaderBytes,
		ErrorLog: log.New(serverDiagnosticWriter{logger: logger}, "", 0),
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	if err := options.Manager.MarkReady(); err != nil {
		_ = listener.Close()
		serveErr := <-result
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(err, serveErr)
	}
	logInfo(ctx, logger, "process ready", "process.ready")
	logInfo(ctx, logger, "HTTP server started", "http.server.started", "address", listener.Addr().String(), "build", build)

	var serveErr error
	serveCompleted := false
	select {
	case serveErr = <-result:
		serveCompleted = true
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
	case <-ctx.Done():
	}
	options.Manager.BeginDrain()
	logInfo(context.Background(), logger, "process draining", "process.draining")
	logInfo(context.Background(), logger, "HTTP server draining", "http.server.draining")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), options.ShutdownTimeout)
	defer cancel()
	serverErr := server.Shutdown(shutdownCtx)
	if serverErr != nil {
		serverErr = errors.Join(serverErr, server.Close())
	}
	if !serveCompleted {
		completedErr := <-result
		if !errors.Is(completedErr, http.ErrServerClosed) {
			serveErr = completedErr
		}
	}
	drainErr := options.Manager.Drain(shutdownCtx)
	applicationErr := invokeShutdown(options.Shutdown, shutdownCtx)
	stopErr := options.Manager.MarkStopped()
	cleanupNeeded = false
	logInfo(context.Background(), logger, "HTTP server stopped", "http.server.stopped")
	if stopErr == nil {
		logInfo(context.Background(), logger, "process stopped", "process.stopped")
	} else {
		logError(context.Background(), logger, "process did not stop cleanly", "process.stop_failed")
	}
	return errors.Join(serveErr, serverErr, drainErr, applicationErr, stopErr)
}

func stopOwnedApplication(manager *Manager, shutdown func(context.Context) error, timeout time.Duration) error {
	manager.BeginDrain()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return errors.Join(manager.Drain(ctx), invokeShutdown(shutdown, ctx), manager.MarkStopped())
}

func invokeShutdown(shutdown func(context.Context) error, ctx context.Context) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = fmt.Errorf("application shutdown callback panicked")
		}
	}()
	err = shutdown(ctx)
	returned = true
	return err
}

func containHandlerPanics(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		returned := false
		defer func() {
			if returned {
				return
			}
			_ = recover()
			logError(request.Context(), logger, "HTTP handler panicked", "http.handler.panicked")
			panic(http.ErrAbortHandler)
		}()
		next.ServeHTTP(writer, request)
		returned = true
	})
}

type serverDiagnosticWriter struct{ logger *slog.Logger }

func (writer serverDiagnosticWriter) Write(data []byte) (int, error) {
	logError(context.Background(), writer.logger, "HTTP server diagnostic", "http.server.diagnostic")
	return len(data), nil
}

func logInfo(ctx context.Context, logger *slog.Logger, message, event string, attributes ...any) {
	logSafely(ctx, logger, slog.LevelInfo, message, append([]any{"event", event}, attributes...)...)
}

func logError(ctx context.Context, logger *slog.Logger, message, event string) {
	logSafely(ctx, logger, slog.LevelError, message, "event", event)
}

func logSafely(ctx context.Context, logger *slog.Logger, level slog.Level, message string, attributes ...any) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
		}
	}()
	logger.Log(ctx, level, message, attributes...)
	returned = true
}
