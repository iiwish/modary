package appcmd

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/internal/testsupport"
	"github.com/iiwish/modary/module"
)

func TestServeFailureBoundariesAlwaysCleanupExactlyOnce(t *testing.T) {
	handlerErr := errors.New("handler failed")
	listenerErr := errors.New("listener failed")
	closeErr := errors.New("listener close failed")

	t.Run("handler error", func(t *testing.T) {
		fixture := newLifecycleFixture()
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			func(context.Context, *appkit.Application) (http.Handler, error) { return nil, handlerErr }, nil,
		))
		assertLifecycleFailure(t, fixture, err, handlerErr)
	})

	t.Run("handler panic", func(t *testing.T) {
		fixture := newLifecycleFixture()
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			func(context.Context, *appkit.Application) (http.Handler, error) { panic("handler secret") }, nil,
		))
		assertLifecycleFailure(t, fixture, err, ErrCallbackPanic)
	})

	t.Run("typed nil handler", func(t *testing.T) {
		fixture := newLifecycleFixture()
		var handler *lifecycleTypedNilHandler
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			func(context.Context, *appkit.Application) (http.Handler, error) { return handler, nil }, nil,
		))
		if err == nil || !containsErrorText(err, "Handler factory returned nil") {
			t.Fatalf("Serve() error = %v", err)
		}
		assertLifecycleCounts(t, fixture, 1, 1)
	})

	t.Run("listener error", func(t *testing.T) {
		fixture := newLifecycleFixture()
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory, func(context.Context, string, string) (net.Listener, error) { return nil, listenerErr },
		))
		assertLifecycleFailure(t, fixture, err, listenerErr)
	})

	t.Run("listener and close errors", func(t *testing.T) {
		fixture := newLifecycleFixture()
		listener := &lifecycleListener{closeErr: closeErr}
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory, func(context.Context, string, string) (net.Listener, error) { return listener, listenerErr },
		))
		assertLifecycleFailure(t, fixture, err, listenerErr, closeErr)
		if listener.closes.Load() != 1 {
			t.Fatalf("Listener.Close calls = %d, want 1", listener.closes.Load())
		}
	})

	t.Run("listener close panic", func(t *testing.T) {
		fixture := newLifecycleFixture()
		listener := &lifecycleListener{panicClose: true}
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory, func(context.Context, string, string) (net.Listener, error) { return listener, listenerErr },
		))
		assertLifecycleFailure(t, fixture, err, listenerErr, ErrCallbackPanic)
	})

	t.Run("listener factory panic", func(t *testing.T) {
		fixture := newLifecycleFixture()
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory, func(context.Context, string, string) (net.Listener, error) { panic("listener secret") },
		))
		assertLifecycleFailure(t, fixture, err, ErrCallbackPanic)
	})

	t.Run("typed nil listener", func(t *testing.T) {
		fixture := newLifecycleFixture()
		var listener *lifecycleListener
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory, func(context.Context, string, string) (net.Listener, error) { return listener, nil },
		))
		if err == nil || !containsErrorText(err, "Listener factory returned nil") {
			t.Fatalf("Serve() error = %v", err)
		}
		assertLifecycleCounts(t, fixture, 1, 1)
	})

	t.Run("canceled after listener creation", func(t *testing.T) {
		fixture := newLifecycleFixture()
		ctx, cancel := context.WithCancel(context.Background())
		listener := &lifecycleListener{}
		err := Serve(ctx, fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory,
			func(context.Context, string, string) (net.Listener, error) {
				cancel()
				return listener, nil
			},
		))
		assertLifecycleFailure(t, fixture, err, context.Canceled)
		if listener.closes.Load() != 1 {
			t.Fatalf("Listener.Close calls = %d, want 1", listener.closes.Load())
		}
	})

	t.Run("server failure", func(t *testing.T) {
		fixture := newLifecycleFixture()
		listener := &lifecycleListener{acceptErr: listenerErr}
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory, func(context.Context, string, string) (net.Listener, error) { return listener, nil },
		))
		assertLifecycleFailure(t, fixture, err, listenerErr)
		if listener.closes.Load() != 1 {
			t.Fatalf("Listener.Close calls = %d, want 1", listener.closes.Load())
		}
	})

	t.Run("server panic", func(t *testing.T) {
		fixture := newLifecycleFixture()
		listener := &lifecycleListener{panicAccept: true}
		err := Serve(context.Background(), fixture.definition(), lifecycleOptions(
			lifecycleHandlerFactory, func(context.Context, string, string) (net.Listener, error) { return listener, nil },
		))
		assertLifecycleFailure(t, fixture, err, ErrCallbackPanic)
	})
}

func TestServeHandlerFactoryReceivesCancelableCommandContext(t *testing.T) {
	fixture := newLifecycleFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan context.Context, 1)
	result := make(chan error, 1)
	go func() {
		result <- Serve(ctx, fixture.definition(), lifecycleOptions(
			func(factoryCtx context.Context, _ *appkit.Application) (http.Handler, error) {
				received <- factoryCtx
				<-factoryCtx.Done()
				return nil, factoryCtx.Err()
			},
			nil,
		))
	}()

	var factoryCtx context.Context
	select {
	case factoryCtx = <-received:
	case <-time.After(time.Second):
		t.Fatal("HandlerFactory did not receive the command context")
	}
	if factoryCtx != ctx {
		t.Error("HandlerFactory received a different context")
	}
	cancel()
	err := waitForError(t, result, "Serve")
	assertLifecycleFailure(t, fixture, err, context.Canceled)
}

func TestServeCancellationDrainsHTTPBeforeApplicationShutdown(t *testing.T) {
	var cleanupBeforeHandler atomic.Bool
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerFinished := make(chan struct{})
	fixture := newLifecycleFixture()
	fixture.cleanup = func(context.Context) error {
		select {
		case <-handlerFinished:
		default:
			cleanupBeforeHandler.Store(true)
		}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	address := make(chan string, 1)
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- Serve(ctx, fixture.definition(), lifecycleOptions(
			func(context.Context, *appkit.Application) (http.Handler, error) {
				return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					close(handlerStarted)
					<-releaseHandler
					_, _ = io.WriteString(writer, "ok")
					close(handlerFinished)
				}), nil
			},
			func(ctx context.Context, _, _ string) (net.Listener, error) {
				listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
				if err == nil {
					address <- listener.Addr().String()
				}
				return listener, err
			},
		))
	}()

	clientResult := make(chan error, 1)
	go func() {
		listenerAddress := <-address
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://" + listenerAddress + "/")
		if err == nil {
			defer response.Body.Close()
			_, err = io.ReadAll(response.Body)
		}
		clientResult <- err
	}()

	waitForSignal(t, handlerStarted, "HTTP handler start")
	cancel()
	select {
	case err := <-serveResult:
		t.Fatalf("Serve returned before active request drained: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if fixture.cleanups.Load() != 0 {
		t.Fatalf("Application cleanup ran before HTTP drain")
	}
	close(releaseHandler)
	if err := waitForError(t, clientResult, "HTTP client"); err != nil {
		t.Fatalf("HTTP client error = %v", err)
	}
	if err := waitForError(t, serveResult, "Serve"); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if cleanupBeforeHandler.Load() {
		t.Fatal("Application cleanup preceded HTTP handler completion")
	}
	assertLifecycleCounts(t, fixture, 1, 1)
}

func TestServeUnexpectedFailureForceClosesActiveConnectionsBeforeCleanup(t *testing.T) {
	acceptErr := errors.New("accept failed permanently")
	handlerStarted := make(chan struct{})
	handlerFinished := make(chan struct{})
	var cleanupBeforeHandler atomic.Bool
	fixture := newLifecycleFixture()
	fixture.cleanup = func(context.Context) error {
		select {
		case <-handlerFinished:
		default:
			cleanupBeforeHandler.Store(true)
		}
		return nil
	}

	underlying, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &lifecycleFailAfterAcceptListener{
		Listener: underlying,
		failure:  acceptErr,
		started:  handlerStarted,
	}
	serveResult := make(chan error, 1)
	go func() {
		options := lifecycleOptions(
			func(context.Context, *appkit.Application) (http.Handler, error) {
				return http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
					close(handlerStarted)
					<-request.Context().Done()
					close(handlerFinished)
				}), nil
			},
			func(context.Context, string, string) (net.Listener, error) { return listener, nil },
		)
		options.ShutdownTimeout = 40 * time.Millisecond
		serveResult <- Serve(context.Background(), fixture.definition(), options)
	}()

	clientResult := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://" + underlying.Addr().String() + "/")
		if err == nil {
			response.Body.Close()
		}
		clientResult <- err
	}()

	waitForSignal(t, handlerStarted, "HTTP handler start")
	serveErr := waitForError(t, serveResult, "Serve")
	if !errors.Is(serveErr, acceptErr) || !errors.Is(serveErr, context.DeadlineExceeded) {
		t.Fatalf("Serve() error = %v, want accept and drain deadline failures", serveErr)
	}
	waitForSignal(t, handlerFinished, "HTTP handler finish")
	_ = waitForError(t, clientResult, "HTTP client")
	if cleanupBeforeHandler.Load() {
		t.Fatal("Application cleanup preceded forced HTTP handler termination")
	}
	assertLifecycleCounts(t, fixture, 1, 1)
}

func TestServeClosesHijackedConnectionsBeforeApplicationShutdown(t *testing.T) {
	connectionClosed := make(chan struct{})
	var cleanupBeforeClose atomic.Bool
	fixture := newLifecycleFixture()
	fixture.cleanup = func(context.Context) error {
		select {
		case <-connectionClosed:
		default:
			cleanupBeforeClose.Store(true)
		}
		return nil
	}
	underlying, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &lifecycleWrappingListener{Listener: underlying, closed: connectionClosed}
	hijacked := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- Serve(ctx, fixture.definition(), lifecycleOptions(
			func(context.Context, *appkit.Application) (http.Handler, error) {
				return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					connection, stream, hijackErr := writer.(http.Hijacker).Hijack()
					if hijackErr != nil {
						return
					}
					_ = connection
					_, _ = stream.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: lifecycle-test\r\n\r\n")
					_ = stream.Flush()
					close(hijacked)
				}), nil
			},
			func(context.Context, string, string) (net.Listener, error) { return listener, nil },
		))
	}()

	clientConnection, err := net.DialTimeout("tcp", underlying.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConnection.Close()
	if _, err := io.WriteString(clientConnection, "GET / HTTP/1.1\r\nHost: lifecycle-test\r\nConnection: Upgrade\r\nUpgrade: lifecycle-test\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(clientConnection)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("read hijack response: %v", readErr)
		}
		if line == "\r\n" {
			break
		}
	}
	waitForSignal(t, hijacked, "HTTP hijack")
	cancel()
	if err := waitForError(t, serveResult, "Serve"); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	waitForSignal(t, connectionClosed, "hijacked connection close")
	if cleanupBeforeClose.Load() {
		t.Fatal("Application cleanup preceded hijacked connection close")
	}
	assertLifecycleCounts(t, fixture, 1, 1)
}

func TestServeJoinsPrimaryCleanupErrorsAndUsesIndependentShutdownContext(t *testing.T) {
	handlerErr := errors.New("handler failed")
	cleanupErr := errors.New("cleanup failed")
	fixture := newLifecycleFixture()
	var independent atomic.Bool
	fixture.cleanup = func(ctx context.Context) error {
		_, hasDeadline := ctx.Deadline()
		independent.Store(ctx.Err() == nil && hasDeadline)
		return cleanupErr
	}
	ctx, cancel := context.WithCancel(context.Background())
	err := Serve(ctx, fixture.definition(), lifecycleOptions(
		func(context.Context, *appkit.Application) (http.Handler, error) {
			cancel()
			return nil, handlerErr
		}, nil,
	))
	assertLifecycleFailure(t, fixture, err, handlerErr, cleanupErr)
	if !independent.Load() {
		t.Fatal("Application cleanup did not receive an independent bounded context")
	}
}

func TestServeContainsHTTPErrorWriterFailuresAndPanics(t *testing.T) {
	writeErr := errors.New("stderr failed")
	for _, test := range []struct {
		name   string
		writer io.Writer
		want   error
	}{
		{name: "error", writer: lifecycleErrorWriter{err: writeErr}, want: writeErr},
		{name: "panic", writer: lifecyclePanicWriter{}, want: ErrCallbackPanic},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLifecycleFixture()
			ctx, cancel := context.WithCancel(context.Background())
			address := make(chan string, 1)
			serveResult := make(chan error, 1)
			go func() {
				options := lifecycleOptions(
					func(context.Context, *appkit.Application) (http.Handler, error) {
						return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("request secret") }), nil
					},
					func(ctx context.Context, _, _ string) (net.Listener, error) {
						listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
						if err == nil {
							address <- listener.Addr().String()
						}
						return listener, err
					},
				)
				options.Stderr = test.writer
				serveResult <- Serve(ctx, fixture.definition(), options)
			}()
			client := &http.Client{Timeout: 3 * time.Second}
			_, _ = client.Get("http://" + <-address + "/")
			cancel()
			err := waitForError(t, serveResult, "Serve")
			if !errors.Is(err, test.want) {
				t.Fatalf("Serve() error = %v, want errors.Is(%v)", err, test.want)
			}
			assertLifecycleCounts(t, fixture, 1, 1)
		})
	}
}

func TestAppCommandDependencyBoundariesFailClosedOnTypedNilErrors(t *testing.T) {
	var typedNil *lifecycleTypedNilDependencyError

	handler, err := callHandlerFactory(func(context.Context, *appkit.Application) (http.Handler, error) {
		return http.NotFoundHandler(), typedNil
	}, context.Background(), nil)
	if !isNilInterface(handler) {
		t.Fatal("typed-nil HandlerFactory error retained its handler result")
	}
	assertTypedNilDependencyFailure(t, err, typedNil)

	factoryListener := &lifecycleListener{}
	listener, err := callListenerFactory(func(context.Context, string, string) (net.Listener, error) {
		return factoryListener, typedNil
	}, context.Background(), "tcp", "127.0.0.1:0")
	if listener != factoryListener {
		t.Fatal("typed-nil ListenerFactory error lost the listener needed for cleanup")
	}
	assertTypedNilDependencyFailure(t, err, typedNil)
	if closeErr := closeListener(listener); closeErr != nil {
		t.Fatalf("close ListenerFactory error result: %v", closeErr)
	}

	accepted, peer := net.Pipe()
	defer peer.Close()
	tracked := &trackedListener{
		Listener: &lifecycleAcceptResultListener{connection: accepted, err: typedNil},
		tracker:  newConnectionTracker(),
	}
	connection, err := tracked.Accept()
	if !isNilInterface(connection) {
		t.Fatal("typed-nil Listener.Accept error retained its connection result")
	}
	assertTypedNilDependencyFailure(t, err, typedNil)
	if _, probeErr := accepted.Write([]byte("x")); probeErr == nil {
		t.Fatal("connection returned beside a typed-nil Accept error was not closed")
	}

	assertTypedNilDependencyFailure(t, closeListener(&lifecycleListener{closeErr: typedNil}), typedNil)
	assertTypedNilDependencyFailure(t,
		(&trackedListener{Listener: &lifecycleListener{closeErr: typedNil}, tracker: newConnectionTracker()}).Close(), typedNil)

	writer := &recordingWriter{destination: lifecycleFullErrorWriter{err: typedNil}, operation: "HTTP server error writer"}
	written, err := writer.Write([]byte("diagnostic"))
	if written != len("diagnostic") {
		t.Fatalf("typed-nil recording writer count = %d", written)
	}
	assertTypedNilDependencyFailure(t, err, typedNil)
	assertTypedNilDependencyFailure(t, writer.Err(), typedNil)
	assertTypedNilDependencyFailure(t, writeString(lifecycleFullErrorWriter{err: typedNil}, "help"), typedNil)
	assertTypedNilDependencyFailure(t, writeActionJSON(lifecycleFullErrorWriter{err: typedNil}, struct{}{}), typedNil)

	tracker := newConnectionTracker()
	typedNilConnection := &trackedConnection{
		Conn:    &lifecycleCloseResultConnection{err: typedNil},
		tracker: tracker,
	}
	assertTypedNilDependencyFailure(t, typedNilConnection.Close(), typedNil)
	assertTypedNilDependencyFailure(t, tracker.Err(), typedNil)
	panickingConnection := &trackedConnection{
		Conn:    &lifecycleCloseResultConnection{panicClose: true},
		tracker: newConnectionTracker(),
	}
	if err := panickingConnection.Close(); !errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("panicking Conn.Close classification = %v", err)
	}
}

func TestCloseBoundariesDistinguishNilFromTypedNilDependencies(t *testing.T) {
	if err := closeListener(nil); err != nil {
		t.Fatalf("closeListener(nil) = %v", err)
	}
	if err := closeConnection(nil); err != nil {
		t.Fatalf("closeConnection(nil) = %v", err)
	}

	var typedNilListener *lifecycleListener
	if err := closeListener(typedNilListener); err == nil || err.Error() != "close Listener failed: Listener is unavailable" || errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("closeListener(typed nil) = %v", err)
	}
	tracked := &trackedListener{Listener: typedNilListener, tracker: newConnectionTracker()}
	if err := tracked.Close(); err == nil || err.Error() != "close Listener failed: Listener is unavailable" || errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("tracked Listener.Close(typed nil) = %v", err)
	}

	var typedNilConnection *lifecycleCloseResultConnection
	if err := closeConnection(typedNilConnection); err == nil || err.Error() != "close HTTP connection failed: connection is unavailable" || errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("closeConnection(typed nil) = %v", err)
	}
	tracker := newConnectionTracker()
	connection := &trackedConnection{Conn: typedNilConnection, tracker: tracker}
	if err := connection.Close(); err == nil || err.Error() != "close HTTP connection failed: connection is unavailable" || errors.Is(err, ErrCallbackPanic) {
		t.Fatalf("tracked Connection.Close(typed nil) = %v", err)
	}
	if err := tracker.Err(); err == nil || err.Error() != "close HTTP connection failed: connection is unavailable" {
		t.Fatalf("tracked typed-nil connection error = %v", err)
	}
}

func assertTypedNilDependencyFailure(t *testing.T, err error, cause *lifecycleTypedNilDependencyError) {
	t.Helper()
	if err == nil {
		t.Fatal("typed-nil dependency error was treated as success")
	}
	if got := err.Error(); got == "" || strings.Contains(got, "secret") {
		t.Fatalf("typed-nil dependency diagnostic = %q", got)
	}
	var found *lifecycleTypedNilDependencyError
	if !errors.Is(err, cause) || !errors.As(err, &found) || found != cause {
		t.Fatalf("typed-nil dependency cause was not safely preserved: Is=%t As=%t value=%#v",
			errors.Is(err, cause), errors.As(err, &found), found)
	}
}

func TestTrackedListenerPreservesOnlyFrameworkOwnedTemporaryAcceptErrors(t *testing.T) {
	temporary := &net.DNSError{Err: "temporary accept failure", IsTemporary: true}
	terminal := net.ErrClosed
	underlying := &lifecycleSequenceListener{acceptErrors: []error{temporary, terminal}}
	tracked := &trackedListener{
		Listener: underlying, tracker: newConnectionTracker(), trustedAcceptErrors: true,
	}
	server := &http.Server{Handler: http.NotFoundHandler(), ErrorLog: log.New(io.Discard, "", 0)}
	if err := server.Serve(tracked); !errors.Is(err, terminal) {
		t.Fatalf("trusted temporary Accept sequence error = %v, want terminal error", err)
	}
	if underlying.accepts != 2 {
		t.Fatalf("trusted temporary Accept calls = %d, want retry", underlying.accepts)
	}

	custom := &trackedListener{
		Listener: &lifecycleSequenceListener{acceptErrors: []error{temporary}},
		tracker:  newConnectionTracker(),
	}
	_, err := custom.Accept()
	if err == nil || !errors.Is(err, temporary) {
		t.Fatalf("custom temporary Accept error was not retained")
	}
	if _, exposesRetryPolicy := err.(net.Error); exposesRetryPolicy {
		t.Fatal("custom Listener error retained net.Error retry behavior")
	}
}

func TestTrackedListenerClosesMalformedAcceptConnection(t *testing.T) {
	accepted, peer := net.Pipe()
	defer peer.Close()
	acceptErr := errors.New("malformed Accept tuple")
	tracked := &trackedListener{
		Listener: &lifecycleAcceptResultListener{connection: accepted, err: acceptErr},
		tracker:  newConnectionTracker(),
	}
	connection, err := tracked.Accept()
	if connection != nil || !errors.Is(err, acceptErr) {
		t.Fatalf("malformed Accept result = connection nil %t, classified %t", connection == nil, errors.Is(err, acceptErr))
	}
	readResult := make(chan error, 1)
	go func() {
		_, readErr := peer.Read(make([]byte, 1))
		readResult <- readErr
	}()
	select {
	case readErr := <-readResult:
		if readErr == nil {
			t.Fatal("connection returned alongside Accept error was not closed")
		}
	case <-time.After(time.Second):
		t.Fatal("connection returned alongside Accept error remained open")
	}
}

func TestListenAndServerConfigurationValidation(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1:0",
		"localhost:8080",
		"example.internal.:443",
		":8080",
		"[::1]:8080",
		"[fe80::1%en0]:8080",
	} {
		if err := validateListenAddress(address); err != nil {
			t.Errorf("validateListenAddress(%q) = %v", address, err)
		}
	}
	for _, address := range []string{
		"",
		"localhost",
		" bad:80",
		"bad host:80",
		"-bad.example:80",
		"bad-.example:80",
		"bad_name:80",
		"[::1]:invalid",
		"[::1]:65536",
		"[fe80::1%bad zone]:80",
	} {
		if err := validateListenAddress(address); !errors.Is(err, ErrUsage) {
			t.Errorf("validateListenAddress(%q) = %v, want ErrUsage", address, err)
		}
	}
	if _, err := normalizeServerOptions(ServerOptions{MaxHeaderBytes: MaximumHeaderBytes + 1}); !errors.Is(err, ErrUsage) {
		t.Fatalf("normalizeServerOptions(oversize) error = %v", err)
	}
	server, err := normalizeServerOptions(ServerOptions{})
	if err != nil || server.ReadHeaderTimeout != DefaultReadHeaderTimeout || server.ReadTimeout != DefaultReadTimeout ||
		server.WriteTimeout != DefaultWriteTimeout || server.IdleTimeout != DefaultIdleTimeout || server.MaxHeaderBytes != DefaultMaxHeaderBytes {
		t.Fatalf("normalizeServerOptions() = %#v, %v", server, err)
	}
}

type lifecycleFixture struct {
	starts   atomic.Int32
	cleanups atomic.Int32
	cleanup  func(context.Context) error
}

func newLifecycleFixture() *lifecycleFixture { return &lifecycleFixture{} }

func (fixture *lifecycleFixture) definition() appkit.Definition {
	registration := module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            "lifecycle-test",
		Version:       "1.0.0",
		Type:          module.ModuleTypeAdapter,
		Provides: []module.Capability{
			module.CapabilityAudit,
			module.CapabilityAuthorization,
			module.CapabilityDatabase,
		},
	}, func(_ context.Context, install module.Scope) error {
		fixture.starts.Add(1)
		if err := module.OnStop(install, func(ctx context.Context) error {
			fixture.cleanups.Add(1)
			if fixture.cleanup != nil {
				return fixture.cleanup(ctx)
			}
			return nil
		}); err != nil {
			return err
		}
		if err := moduleassembly.ProvideActionPersistence(install, testsupport.NewMemoryPlanStore(), testsupport.NewMemoryIdempotencyStore(), testsupport.DirectTransactions{}); err != nil {
			return err
		}
		if err := module.Provide(install, module.Authorizer(), authz.Authorizer(lifecycleAuthorizer{})); err != nil {
			return err
		}
		return module.Provide(install, module.AuditHook(), audit.Hook(testsupport.DiscardAudit{}))
	})
	return appkit.Definition{
		Metadata: appkit.Metadata{ID: "lifecycle-test", Name: "Lifecycle Test", Version: "1.0.0"},
		Modules:  []module.Registration{registration},
	}
}

type lifecycleAuthorizer struct{}

func (lifecycleAuthorizer) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "lifecycle-test"}, nil
}

func lifecycleHandlerFactory(context.Context, *appkit.Application) (http.Handler, error) {
	return http.NotFoundHandler(), nil
}

func lifecycleOptions(handler HandlerFactory, listener ListenerFactory) Options {
	return Options{
		Handler:         handler,
		Listener:        listener,
		ListenAddress:   "127.0.0.1:0",
		Stdout:          io.Discard,
		Stderr:          io.Discard,
		ShutdownTimeout: 250 * time.Millisecond,
	}
}

func assertLifecycleFailure(t *testing.T, fixture *lifecycleFixture, err error, wanted ...error) {
	t.Helper()
	if err == nil {
		t.Fatal("Serve() unexpectedly succeeded")
	}
	for _, target := range wanted {
		if !errors.Is(err, target) {
			t.Errorf("Serve() error = %v, want errors.Is(%v)", err, target)
		}
	}
	assertLifecycleCounts(t, fixture, 1, 1)
}

func assertLifecycleCounts(t *testing.T, fixture *lifecycleFixture, starts, cleanups int32) {
	t.Helper()
	if fixture.starts.Load() != starts || fixture.cleanups.Load() != cleanups {
		t.Fatalf("lifecycle starts=%d cleanups=%d, want %d/%d", fixture.starts.Load(), fixture.cleanups.Load(), starts, cleanups)
	}
}

func containsErrorText(err error, text string) bool {
	return err != nil && strings.Contains(err.Error(), text)
}

func waitForSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func waitForError(t *testing.T, result <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
		return nil
	}
}

type lifecycleTypedNilHandler struct{}

func (*lifecycleTypedNilHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

type lifecycleTypedNilDependencyError struct{}

func (*lifecycleTypedNilDependencyError) Error() string { panic("typed-nil Error invoked") }
func (*lifecycleTypedNilDependencyError) Is(error) bool { panic("typed-nil Is invoked") }
func (*lifecycleTypedNilDependencyError) As(any) bool   { panic("typed-nil As invoked") }
func (*lifecycleTypedNilDependencyError) Unwrap() error { panic("typed-nil Unwrap invoked") }

type lifecycleFullErrorWriter struct{ err error }

func (writer lifecycleFullErrorWriter) Write(data []byte) (int, error) {
	return len(data), writer.err
}

type lifecycleAcceptResultListener struct {
	connection net.Conn
	err        error
}

func (listener *lifecycleAcceptResultListener) Accept() (net.Conn, error) {
	return listener.connection, listener.err
}

func (*lifecycleAcceptResultListener) Close() error   { return nil }
func (*lifecycleAcceptResultListener) Addr() net.Addr { return lifecycleAddress("127.0.0.1:0") }

type lifecycleSequenceListener struct {
	acceptErrors []error
	accepts      int
}

func (listener *lifecycleSequenceListener) Accept() (net.Conn, error) {
	index := listener.accepts
	listener.accepts++
	if index >= len(listener.acceptErrors) {
		return nil, net.ErrClosed
	}
	return nil, listener.acceptErrors[index]
}

func (*lifecycleSequenceListener) Close() error   { return nil }
func (*lifecycleSequenceListener) Addr() net.Addr { return lifecycleAddress("127.0.0.1:0") }

type lifecycleCloseResultConnection struct {
	net.Conn
	err        error
	panicClose bool
}

func (connection *lifecycleCloseResultConnection) Close() error {
	if connection.panicClose {
		panic("connection close panic")
	}
	return connection.err
}

type lifecycleListener struct {
	closes      atomic.Int32
	acceptErr   error
	closeErr    error
	panicAccept bool
	panicClose  bool
}

func (listener *lifecycleListener) Accept() (net.Conn, error) {
	if listener.panicAccept {
		panic("accept secret")
	}
	if listener.acceptErr != nil {
		return nil, listener.acceptErr
	}
	return nil, errors.New("listener has no connections")
}

func (listener *lifecycleListener) Close() error {
	listener.closes.Add(1)
	if listener.panicClose {
		panic("close secret")
	}
	return listener.closeErr
}

func (*lifecycleListener) Addr() net.Addr { return lifecycleAddress("127.0.0.1:0") }

type lifecycleAddress string

func (lifecycleAddress) Network() string        { return "tcp" }
func (address lifecycleAddress) String() string { return string(address) }

type lifecycleFailAfterAcceptListener struct {
	net.Listener
	failure  error
	started  <-chan struct{}
	accepted atomic.Bool
}

func (listener *lifecycleFailAfterAcceptListener) Accept() (net.Conn, error) {
	if listener.accepted.CompareAndSwap(false, true) {
		return listener.Listener.Accept()
	}
	<-listener.started
	return nil, listener.failure
}

type lifecycleWrappingListener struct {
	net.Listener
	closed chan struct{}
}

func (listener *lifecycleWrappingListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &lifecycleCloseSignalConnection{Conn: connection, closed: listener.closed}, nil
}

type lifecycleCloseSignalConnection struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (connection *lifecycleCloseSignalConnection) Close() error {
	connection.once.Do(func() { close(connection.closed) })
	return connection.Conn.Close()
}
