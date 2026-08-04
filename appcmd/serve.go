package appcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/processkit"
)

// Serve starts the consumer Application and its explicitly supplied HTTP
// handler. Cancellation drains HTTP before Module resources are released.
func Serve(ctx context.Context, definition appkit.Definition, options Options) error {
	if ctx == nil {
		return ErrContextRequired
	}
	normalized, err := normalizeOptions(options)
	if err != nil {
		return err
	}
	return serve(ctx, definition, normalized)
}

func parseServeArgs(args []string, listenAddress string) (string, bool, error) {
	seenListen := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--help" || argument == "-h":
			if len(args) != 1 {
				return "", false, usageError("serve help accepts no other arguments")
			}
			return listenAddress, true, nil
		case argument == "--listen":
			if seenListen {
				return "", false, usageError("serve flag --listen may be provided only once")
			}
			if index+1 >= len(args) {
				return "", false, usageError("serve flag --listen requires a value")
			}
			seenListen = true
			index++
			listenAddress = args[index]
		case strings.HasPrefix(argument, "--listen="):
			if seenListen {
				return "", false, usageError("serve flag --listen may be provided only once")
			}
			seenListen = true
			listenAddress = strings.TrimPrefix(argument, "--listen=")
		case strings.HasPrefix(argument, "-"):
			return "", false, usageError("serve has unknown flag %q", argument)
		default:
			return "", false, usageError("serve has unexpected argument %q", argument)
		}
	}
	return listenAddress, false, nil
}

func writeServeHelp(writer io.Writer, name string) error {
	return writeString(writer, fmt.Sprintf("Usage:\n  %s serve [--listen address]\n", name))
}

func validateServeCommandOptions(options normalizedOptions) error {
	if isNilInterface(options.Handler) {
		return usageError("serve requires a Handler factory")
	}
	if options.ListenAddress == "" {
		return usageError("serve requires a listen address")
	}
	return validateListenAddress(options.ListenAddress)
}

func serve(ctx context.Context, definition appkit.Definition, options normalizedOptions) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if isNilInterface(options.Handler) {
		return usageError("serve requires a Handler factory")
	}
	if options.ListenAddress == "" {
		options.ListenAddress = DefaultListenAddress
	}
	if err := validateListenAddress(options.ListenAddress); err != nil {
		return err
	}
	listenerFactory := options.Listener
	trustedAcceptErrors := isNilInterface(listenerFactory)
	if trustedAcceptErrors {
		listenerFactory = defaultListener
	}

	application, err := appkit.Start(ctx, definition, options.App)
	if err != nil {
		return opaqueCommandError("start application failed", err)
	}
	defer func() {
		if options.Process != nil {
			options.Process.BeginDrain()
			drainCtx, cancel := context.WithTimeout(context.Background(), options.ShutdownTimeout)
			result = errors.Join(result, options.Process.Drain(drainCtx))
			cancel()
		}
		result = errors.Join(result, shutdownApplication(application, options.ShutdownTimeout))
		if options.Process != nil {
			stopErr := options.Process.MarkStopped()
			result = errors.Join(result, stopErr)
			if stopErr == nil {
				options.Logger.Info("process stopped", "event", "process.stopped")
			} else {
				options.Logger.Error("process did not stop cleanly", "event", "process.stop_failed")
			}
		}
		if options.loggerOutput != nil {
			result = errors.Join(result, options.loggerOutput.Err())
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	handler, err := callHandlerFactory(options.Handler, ctx, application)
	if err != nil {
		return err
	}
	if isNilInterface(handler) {
		return fmt.Errorf("HTTP Handler factory returned nil")
	}
	if options.Process != nil {
		handler, err = options.Process.Middleware(handler)
		if err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	listener, err := callListenerFactory(listenerFactory, ctx, "tcp", options.ListenAddress)
	if err != nil {
		return errors.Join(err, closeListener(listener))
	}
	if isNilInterface(listener) {
		return fmt.Errorf("Listener factory returned nil")
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, closeListener(listener))
	}

	baseCtx, cancelBase := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelBase()
	errorOutput := &recordingWriter{destination: options.Stderr, operation: "HTTP server error writer"}
	errorLog := log.New(errorOutput, "", 0)
	connections := newConnectionTracker()
	server := &http.Server{
		Handler:           containHTTPHandlerPanics(handler, errorLog),
		ErrorLog:          errorLog,
		ReadHeaderTimeout: options.server.ReadHeaderTimeout,
		ReadTimeout:       options.server.ReadTimeout,
		WriteTimeout:      options.server.WriteTimeout,
		IdleTimeout:       options.server.IdleTimeout,
		MaxHeaderBytes:    options.server.MaxHeaderBytes,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
		ConnState:         connections.observe,
	}
	return errors.Join(runHTTPServer(ctx, server, listener, connections, cancelBase, options.ShutdownTimeout, trustedAcceptErrors, options.Process, options.Logger), errorOutput.Err())
}

func validateListenAddress(address string) error {
	if address == "" || len(address) > 512 || strings.TrimSpace(address) != address || !utf8.ValidString(address) || strings.ContainsFunc(address, unicode.IsControl) {
		return usageError("HTTP listen address is invalid")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return usageError("HTTP listen address must use host:port form")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber > 65535 {
		return usageError("HTTP listen port must be numeric and between 0 and 65535")
	}
	if !validListenHost(host) {
		return usageError("HTTP listen host is invalid")
	}
	return nil
}

func validListenHost(host string) bool {
	if host == "" || net.ParseIP(host) != nil {
		return true
	}
	if separator := strings.LastIndexByte(host, '%'); separator >= 0 {
		address, zone := host[:separator], host[separator+1:]
		return strings.Contains(address, ":") && net.ParseIP(address) != nil && validListenZone(zone)
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range []byte(label) {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func validListenZone(zone string) bool {
	if zone == "" || len(zone) > 64 {
		return false
	}
	for _, character := range []byte(zone) {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func defaultListener(ctx context.Context, network, address string) (net.Listener, error) {
	var config net.ListenConfig
	return config.Listen(ctx, network, address)
}

func callHandlerFactory(factory HandlerFactory, ctx context.Context, application *appkit.Application) (handler http.Handler, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			handler = nil
			err = &CallbackPanicError{Operation: "HTTP Handler factory"}
		}
	}()
	handler, err = factory(ctx, application)
	returned = true
	if err != nil {
		handler = nil
		err = opaqueCommandError("HTTP Handler factory failed", err)
	}
	return handler, err
}

func callListenerFactory(factory ListenerFactory, ctx context.Context, network, address string) (listener net.Listener, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			listener = nil
			err = &CallbackPanicError{Operation: "Listener factory"}
		}
	}()
	listener, err = factory(ctx, network, address)
	returned = true
	if err != nil {
		err = opaqueCommandError("Listener factory failed", err)
	}
	return listener, err
}

func containHTTPHandlerPanics(handler http.Handler, errorLog *log.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		returned := false
		defer func() {
			if returned {
				return
			}
			_ = recover()
			if errorLog != nil {
				errorLog.Print("HTTP Handler callback panicked")
			}
			// Preserve net/http's connection-abort behavior without allowing it to
			// inspect or format the consumer-owned panic value.
			panic(http.ErrAbortHandler)
		}()
		handler.ServeHTTP(writer, request)
		returned = true
	})
}

func runHTTPServer(ctx context.Context, server *http.Server, listener net.Listener, connections *connectionTracker, cancelActive context.CancelFunc, timeout time.Duration, trustedAcceptErrors bool, process *processkit.Manager, logger *slog.Logger) error {
	serveResult := make(chan error, 1)
	listener = &trackedListener{Listener: listener, tracker: connections, trustedAcceptErrors: trustedAcceptErrors}
	go func() {
		serveResult <- callHTTPServe(server, listener)
	}()
	if process != nil {
		if err := process.MarkReady(); err != nil {
			_ = server.Close()
			<-serveResult
			return err
		}
		logger.InfoContext(ctx, "process ready", "event", "process.ready")
	}
	logger.InfoContext(ctx, "HTTP server started", "event", "http.server.started", "address", listener.Addr().String())

	var serveErr error
	serveCompleted := false
	select {
	case serveErr = <-serveResult:
		serveCompleted = true
	case <-ctx.Done():
	}
	if process != nil {
		process.BeginDrain()
		logger.Info("process draining", "event", "process.draining")
	}
	logger.Info("HTTP server draining", "event", "http.server.draining")
	drainErr := drainHTTP(server, connections, cancelActive, timeout)
	cancelActive()
	var waitErr error
	if !serveCompleted {
		serveErr, waitErr = waitHTTPServe(serveResult, timeout)
	}
	connectionWaitErr := connections.wait(timeout)
	if serveErr == nil || safeerr.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	logger.Info("HTTP server stopped", "event", "http.server.stopped")
	return errors.Join(serveErr, drainErr, waitErr, connectionWaitErr, connections.Err())
}

func callHTTPServe(server *http.Server, listener net.Listener) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "HTTP server Serve"}
		}
	}()
	err = server.Serve(listener)
	returned = true
	if err == nil {
		return nil
	}
	return opaqueCommandError("serve HTTP failed", err)
}

func waitHTTPServe(result <-chan error, timeout time.Duration) (error, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err, nil
	case <-timer.C:
		return nil, fmt.Errorf("wait for HTTP server to stop: %w", context.DeadlineExceeded)
	}
}

func drainHTTP(server *http.Server, connections *connectionTracker, cancelActive context.CancelFunc, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := callHTTPShutdown(server, ctx); err != nil {
		cancelActive()
		closeErr := callHTTPClose(server)
		connections.closeHijacked()
		return errors.Join(err, closeErr)
	}
	connections.closeHijacked()
	return nil
}

func callHTTPShutdown(server *http.Server, ctx context.Context) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "HTTP server shutdown"}
		}
	}()
	err = server.Shutdown(ctx)
	returned = true
	if err == nil {
		return nil
	}
	return opaqueCommandError("drain HTTP server failed", err)
}

func callHTTPClose(server *http.Server) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "HTTP server close"}
		}
	}()
	err = server.Close()
	returned = true
	if err == nil {
		return nil
	}
	return opaqueCommandError("close HTTP server failed", err)
}

func closeListener(listener net.Listener) (err error) {
	if listener == nil {
		return nil
	}
	if isNilInterface(listener) {
		return fmt.Errorf("close Listener failed: Listener is unavailable")
	}
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "Listener close"}
		}
	}()
	err = listener.Close()
	returned = true
	if err == nil {
		return nil
	}
	return opaqueCommandError("close Listener failed", err)
}

func shutdownApplication(application *appkit.Application, timeout time.Duration) (err error) {
	if application == nil {
		return fmt.Errorf("shutdown application: application is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "Application shutdown"}
		}
	}()
	err = application.Shutdown(ctx)
	returned = true
	if err == nil {
		return nil
	}
	return opaqueCommandError("shutdown application failed", err)
}

func joinApplicationShutdown(primary error, application *appkit.Application, timeout time.Duration) error {
	return errors.Join(primary, shutdownApplication(application, timeout))
}

type recordingWriter struct {
	destination io.Writer
	operation   string
	mu          sync.Mutex
	err         error
}

// Write forwards one output write while containing panics and recording the
// first invalid or failed write.
func (writer *recordingWriter) Write(data []byte) (written int, err error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			written = 0
			err = &CallbackPanicError{Operation: writer.operation}
		}
		if written < 0 || written > len(data) {
			written = 0
			if err == nil {
				err = fmt.Errorf("%s returned an invalid write count", writer.operation)
			}
		} else if err == nil && written != len(data) {
			err = io.ErrShortWrite
		}
		if err != nil && writer.err == nil {
			writer.err = err
		}
	}()
	written, err = writer.destination.Write(data)
	returned = true
	if err != nil {
		err = opaqueCommandError(writer.operation+" failed", err)
	}
	return written, err
}

// Err returns the first error observed by Write.
func (writer *recordingWriter) Err() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.err
}

type connectionTracker struct {
	mu       sync.Mutex
	active   map[net.Conn]struct{}
	hijacked map[net.Conn]struct{}
	changed  chan struct{}
	closing  bool
	err      error
}

func newConnectionTracker() *connectionTracker {
	return &connectionTracker{
		active:   make(map[net.Conn]struct{}),
		hijacked: make(map[net.Conn]struct{}),
		changed:  make(chan struct{}),
	}
}

func (tracker *connectionTracker) observe(connection net.Conn, state http.ConnState) {
	tracker.mu.Lock()
	switch state {
	case http.StateNew:
		tracker.active[connection] = struct{}{}
		tracker.notifyLocked()
		tracker.mu.Unlock()
		return
	case http.StateClosed:
		delete(tracker.active, connection)
		delete(tracker.hijacked, connection)
		tracker.notifyLocked()
		tracker.mu.Unlock()
		return
	case http.StateHijacked:
		delete(tracker.active, connection)
	default:
		tracker.mu.Unlock()
		return
	}
	if !tracker.closing {
		tracker.hijacked[connection] = struct{}{}
		tracker.notifyLocked()
		tracker.mu.Unlock()
		return
	}
	tracker.notifyLocked()
	tracker.mu.Unlock()
	tracker.close(connection)
}

func (tracker *connectionTracker) closeHijacked() {
	tracker.mu.Lock()
	tracker.closing = true
	connections := make([]net.Conn, 0, len(tracker.hijacked))
	for connection := range tracker.hijacked {
		connections = append(connections, connection)
	}
	clear(tracker.hijacked)
	tracker.notifyLocked()
	tracker.mu.Unlock()
	for _, connection := range connections {
		tracker.close(connection)
	}
}

func (tracker *connectionTracker) close(connection net.Conn) {
	err := closeConnection(connection)
	if _, recordsOwnClose := connection.(*trackedConnection); !recordsOwnClose {
		tracker.recordCloseError(err)
	}
}

func (tracker *connectionTracker) recordCloseError(err error) {
	if err == nil {
		return
	}
	tracker.mu.Lock()
	tracker.err = errors.Join(tracker.err, err)
	tracker.mu.Unlock()
}

func (tracker *connectionTracker) forget(connection net.Conn) {
	tracker.mu.Lock()
	if _, exists := tracker.hijacked[connection]; exists {
		delete(tracker.hijacked, connection)
		tracker.notifyLocked()
	}
	tracker.mu.Unlock()
}

func (tracker *connectionTracker) wait(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		tracker.mu.Lock()
		if len(tracker.active) == 0 && len(tracker.hijacked) == 0 {
			tracker.mu.Unlock()
			return nil
		}
		changed := tracker.changed
		tracker.mu.Unlock()
		select {
		case <-changed:
		case <-timer.C:
			return fmt.Errorf("wait for HTTP connections to stop: %w", context.DeadlineExceeded)
		}
	}
}

func (tracker *connectionTracker) notifyLocked() {
	close(tracker.changed)
	tracker.changed = make(chan struct{})
}

// Err returns errors observed while closing HTTP connections.
func (tracker *connectionTracker) Err() error {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.err
}

func closeConnection(connection net.Conn) (err error) {
	if connection == nil {
		return nil
	}
	if isNilInterface(connection) {
		return fmt.Errorf("close HTTP connection failed: connection is unavailable")
	}
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &CallbackPanicError{Operation: "HTTP connection close"}
		}
	}()
	err = connection.Close()
	returned = true
	if err == nil {
		return nil
	}
	if safeerr.Is(err, net.ErrClosed) {
		return nil
	}
	return opaqueCommandError("close HTTP connection failed", err)
}

type trackedListener struct {
	net.Listener
	tracker             *connectionTracker
	trustedAcceptErrors bool
}

// Accept wraps each accepted connection so explicit and server-driven closes
// update the shared connection tracker.
func (listener *trackedListener) Accept() (connection net.Conn, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			connection = nil
			err = &CallbackPanicError{Operation: "Listener accept"}
		}
	}()
	connection, err = listener.Listener.Accept()
	returned = true
	if err != nil {
		if !isNilInterface(connection) {
			return nil, errors.Join(opaqueCommandError("Listener accept failed", err), closeConnection(connection))
		}
		if listener.trustedAcceptErrors {
			return nil, err
		}
		return nil, opaqueCommandError("Listener accept failed", err)
	}
	if isNilInterface(connection) {
		return nil, fmt.Errorf("Listener returned a nil connection")
	}
	return &trackedConnection{Conn: connection, tracker: listener.tracker}, nil
}

// Close contains extension Listener close failures even when net/http owns the
// close call.
func (listener *trackedListener) Close() error {
	return closeListener(listener.Listener)
}

type trackedConnection struct {
	net.Conn
	tracker *connectionTracker
}

// Close closes the underlying connection and removes it from tracking once the
// close attempt finishes.
func (connection *trackedConnection) Close() error {
	defer connection.tracker.forget(connection)
	err := closeConnection(connection.Conn)
	connection.tracker.recordCloseError(err)
	return err
}
