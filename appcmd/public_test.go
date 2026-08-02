package appcmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/adapters/postgres"
	"github.com/iiwish/modary/appcmd"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/testpostgres"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
	"github.com/iiwish/modary/transport/httpapi"
)

func TestPublicCommandConfigurationDoesNotExposeKernelExecutionInternals(t *testing.T) {
	for _, typeOf := range []reflect.Type{
		reflect.TypeOf(appcmd.Options{}),
		reflect.TypeOf(appcmd.ServerOptions{}),
	} {
		for index := range typeOf.NumField() {
			field := typeOf.Field(index)
			assertNoKernelType(t, typeOf.Name()+"."+field.Name, field.Type)
		}
	}

	handlerFactory := reflect.TypeOf((*appcmd.HandlerFactory)(nil)).Elem()
	if handlerFactory.NumIn() != 2 ||
		handlerFactory.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() ||
		handlerFactory.In(1) != reflect.TypeOf((*appkit.Application)(nil)) ||
		handlerFactory.NumOut() != 2 {
		t.Fatalf("HandlerFactory signature = %s", handlerFactory)
	}
	assertNoKernelType(t, "HandlerFactory", handlerFactory)

	listenerFactory := reflect.TypeOf((*appcmd.ListenerFactory)(nil)).Elem()
	assertNoKernelType(t, "ListenerFactory", listenerFactory)

	definitionProvider := reflect.TypeOf((*appcmd.DefinitionProvider)(nil)).Elem()
	if definitionProvider != reflect.TypeOf((*appkit.DefinitionProvider)(nil)).Elem() {
		t.Fatalf("DefinitionProvider is not the appkit alias: %s", definitionProvider)
	}
	if definitionProvider.NumIn() != 0 || definitionProvider.NumOut() != 2 ||
		definitionProvider.Out(0) != reflect.TypeOf(appkit.Definition{}) ||
		definitionProvider.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("DefinitionProvider signature = %s", definitionProvider)
	}
	assertNoKernelType(t, "DefinitionProvider", definitionProvider)
}

func TestExternalConsumerCanRunActionAndServeExplicitMount(t *testing.T) {
	definition := externalDefinition(t)
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(externalCommandToken), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := appcmd.RunAction(context.Background(), []string{
		"run", "probe.read", "--token-file", tokenFile, "--input", "-",
	}, definition, appcmd.Options{Stdin: io.NopCloser(strings.NewReader(`{}`)), Stdout: &output}); err != nil {
		t.Fatalf("RunAction() error = %v", err)
	}
	if !strings.Contains(output.String(), `"value":1`) {
		t.Fatalf("RunAction() output = %q", output.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- appcmd.Serve(ctx, definition, appcmd.Options{
			ListenAddress:   "127.0.0.1:0",
			ShutdownTimeout: time.Second,
			Stdout:          io.Discard,
			Stderr:          io.Discard,
			Handler: func(_ context.Context, application *appkit.Application) (http.Handler, error) {
				health, err := httpapi.NewHealth(application)
				if err != nil {
					return nil, err
				}
				mux := http.NewServeMux()
				mux.Handle("/health", health)
				return mux, nil
			},
			Listener: func(_ context.Context, _, _ string) (net.Listener, error) {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err == nil {
					ready <- listener.Addr().String()
				}
				return listener, err
			},
		})
	}()
	var address string
	select {
	case address = <-ready:
	case err := <-serveResult:
		t.Fatalf("Serve() returned before listening: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Serve() did not create its listener")
	}
	response, err := (&http.Client{Timeout: time.Second}).Get("http://" + address + "/health")
	if err != nil {
		cancel()
		t.Fatalf("GET /health: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("GET /health status = %d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve() did not drain after cancellation")
	}
}

func TestExternalDependencyErrorsRemainOpaqueAtCommandBoundaries(t *testing.T) {
	t.Run("command output writer", func(t *testing.T) {
		assertOpaqueExternalError(t, "write command output failed", func(cause error) error {
			return appcmd.Run(context.Background(), []string{"help"}, nil, appcmd.Options{
				Metadata: externalMetadata(),
				Stdout:   externalFullErrorWriter{err: cause},
			})
		})
	})

	t.Run("Action output writer", func(t *testing.T) {
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte(externalCommandToken), 0o600); err != nil {
			t.Fatal(err)
		}
		assertOpaqueExternalError(t, "write Action output failed", func(cause error) error {
			return appcmd.RunAction(context.Background(), []string{
				"run", "probe.read", "--token-file", tokenFile, "--input", "-",
			}, externalDefinition(t), appcmd.Options{
				Stdin:  io.NopCloser(strings.NewReader(`{}`)),
				Stdout: externalFullErrorWriter{err: cause},
			})
		})
	})

	t.Run("Handler factory", func(t *testing.T) {
		assertOpaqueExternalError(t, "HTTP Handler factory failed", func(cause error) error {
			return appcmd.Serve(context.Background(), externalDefinition(t), externalServeOptions(
				func(context.Context, *appkit.Application) (http.Handler, error) { return nil, cause }, nil,
			))
		})
	})

	t.Run("HTTP Handler panic value", func(t *testing.T) {
		cause := newExternalHostileError()
		defer cause.releaseOnce.Do(func() { close(cause.release) })
		var diagnostics bytes.Buffer
		result := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			address := make(chan string, 1)
			serveResult := make(chan error, 1)
			options := externalServeOptions(
				func(context.Context, *appkit.Application) (http.Handler, error) {
					return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(cause) }), nil
				},
				func(_ context.Context, _, _ string) (net.Listener, error) {
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					if err == nil {
						address <- listener.Addr().String()
					}
					return listener, err
				},
			)
			options.Stderr = &diagnostics
			go func() { serveResult <- appcmd.Serve(ctx, externalDefinition(t), options) }()
			var listenAddress string
			select {
			case listenAddress = <-address:
			case err := <-serveResult:
				result <- err
				return
			case <-time.After(2 * time.Second):
				result <- context.DeadlineExceeded
				return
			}
			response, _ := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + listenAddress + "/")
			if response != nil {
				_ = response.Body.Close()
			}
			cancel()
			result <- awaitExternalResult(serveResult)
		}()
		if err := awaitWithoutInspection(t, cause, result, "HTTP Handler panic containment"); err != nil {
			t.Fatal("contained HTTP Handler panic made Serve fail")
		}
		if text := diagnostics.String(); !strings.Contains(text, "HTTP Handler callback panicked") || strings.Contains(text, cause.secret) {
			t.Fatalf("HTTP Handler panic diagnostic = %q", text)
		}
		select {
		case method := <-cause.entered:
			t.Fatalf("HTTP Handler panic invoked dependency %s", method)
		default:
		}
	})

	t.Run("Listener factory", func(t *testing.T) {
		assertOpaqueExternalError(t, "Listener factory failed", func(cause error) error {
			return appcmd.Serve(context.Background(), externalDefinition(t), externalServeOptions(
				externalHandlerFactory,
				func(context.Context, string, string) (net.Listener, error) { return nil, cause },
			))
		})
	})

	t.Run("Listener accept", func(t *testing.T) {
		assertOpaqueExternalError(t, "serve HTTP failed", func(cause error) error {
			listener := &externalFailingListener{acceptErr: cause}
			return appcmd.Serve(context.Background(), externalDefinition(t), externalServeOptions(
				externalHandlerFactory,
				func(context.Context, string, string) (net.Listener, error) { return listener, nil },
			))
		})
	})

	t.Run("Listener close", func(t *testing.T) {
		assertOpaqueExternalError(t, "drain HTTP server failed", func(cause error) error {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return err
			}
			accepting := make(chan struct{})
			wrapped := &externalCloseErrorListener{Listener: listener, closeErr: cause, accepting: accepting}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- appcmd.Serve(ctx, externalDefinition(t), externalServeOptions(
					externalHandlerFactory,
					func(context.Context, string, string) (net.Listener, error) { return wrapped, nil },
				))
			}()
			select {
			case <-accepting:
				cancel()
			case err := <-result:
				return err
			case <-time.After(2 * time.Second):
				return context.DeadlineExceeded
			}
			return awaitExternalResult(result)
		})
	})

	t.Run("HTTP server error writer", func(t *testing.T) {
		assertOpaqueExternalError(t, "HTTP server error writer failed", func(cause error) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			address := make(chan string, 1)
			result := make(chan error, 1)
			options := externalServeOptions(
				func(context.Context, *appkit.Application) (http.Handler, error) {
					return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("request panic") }), nil
				},
				func(_ context.Context, _, _ string) (net.Listener, error) {
					listener, err := net.Listen("tcp", "127.0.0.1:0")
					if err == nil {
						address <- listener.Addr().String()
					}
					return listener, err
				},
			)
			options.Stderr = externalFullErrorWriter{err: cause}
			go func() { result <- appcmd.Serve(ctx, externalDefinition(t), options) }()
			var listenAddress string
			select {
			case listenAddress = <-address:
			case err := <-result:
				return err
			case <-time.After(2 * time.Second):
				return context.DeadlineExceeded
			}
			response, _ := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + listenAddress + "/")
			if response != nil {
				_ = response.Body.Close()
			}
			cancel()
			return awaitExternalResult(result)
		})
	})

	t.Run("HTTP connection close", func(t *testing.T) {
		assertOpaqueExternalError(t, "close HTTP connection failed", func(cause error) error {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return err
			}
			wrapped := &externalConnectionErrorListener{Listener: listener, closeErr: cause}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- appcmd.Serve(ctx, externalDefinition(t), externalServeOptions(
					func(context.Context, *appkit.Application) (http.Handler, error) {
						return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
							_, _ = io.WriteString(writer, "ok")
						}), nil
					},
					func(context.Context, string, string) (net.Listener, error) { return wrapped, nil },
				))
			}()
			client := &http.Client{Timeout: 2 * time.Second}
			response, requestErr := client.Get("http://" + listener.Addr().String() + "/")
			if requestErr != nil {
				cancel()
				_ = awaitExternalResult(result)
				return requestErr
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			cancel()
			return awaitExternalResult(result)
		})
	})
}

func externalServeOptions(handler appcmd.HandlerFactory, listener appcmd.ListenerFactory) appcmd.Options {
	return appcmd.Options{
		Handler: handler, Listener: listener, ListenAddress: "127.0.0.1:0",
		Stdout: io.Discard, Stderr: io.Discard, ShutdownTimeout: 250 * time.Millisecond,
	}
}

func externalHandlerFactory(context.Context, *appkit.Application) (http.Handler, error) {
	return http.NotFoundHandler(), nil
}

func awaitExternalResult(result <-chan error) error {
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		return context.DeadlineExceeded
	}
}

func assertOpaqueExternalError(t *testing.T, want string, invoke func(error) error) {
	t.Helper()
	hostile := newExternalHostileError()
	defer hostile.releaseOnce.Do(func() { close(hostile.release) })
	cause := errors.Join(hostile, io.EOF)

	result := make(chan error, 1)
	go func() { result <- invoke(cause) }()
	err := awaitWithoutInspection(t, hostile, result, "dependency operation")
	if err == nil {
		t.Fatal("dependency error was discarded")
	}

	textResult := make(chan string, 1)
	go func() { textResult <- err.Error() }()
	text := awaitWithoutInspection(t, hostile, textResult, "error diagnostic")
	if !strings.Contains(text, want) || strings.Contains(text, hostile.secret) {
		t.Fatalf("stable dependency diagnostic = %q, want %q without secret", text, want)
	}

	type classification struct {
		isCause   bool
		isHostile bool
		isEOF     bool
		asHostile bool
		found     *externalHostileError
	}
	classificationResult := make(chan classification, 1)
	go func() {
		var found *externalHostileError
		asHostile := errors.As(err, &found)
		classificationResult <- classification{
			isCause:   errors.Is(err, cause),
			isHostile: errors.Is(err, hostile),
			isEOF:     errors.Is(err, io.EOF),
			asHostile: asHostile,
			found:     found,
		}
	}()
	classified := awaitWithoutInspection(t, hostile, classificationResult, "standard error classification")
	if !classified.isCause || !classified.isHostile || !classified.isEOF || !classified.asHostile || classified.found != hostile {
		t.Fatal("opaque dependency cause was not available to standard error classification")
	}
	select {
	case method := <-hostile.entered:
		t.Fatalf("dependency %s method was invoked", method)
	default:
	}
}

func awaitWithoutInspection[T any](t *testing.T, cause *externalHostileError, result <-chan T, operation string) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case method := <-cause.entered:
		cause.releaseOnce.Do(func() { close(cause.release) })
		select {
		case <-result:
		case <-time.After(2 * time.Second):
		}
		t.Fatalf("dependency %s method was invoked during %s", method, operation)
		var zero T
		return zero
	case <-time.After(3 * time.Second):
		cause.releaseOnce.Do(func() { close(cause.release) })
		t.Fatalf("timed out during %s", operation)
		var zero T
		return zero
	}
}

type externalHostileError struct {
	secret      string
	entered     chan string
	release     chan struct{}
	releaseOnce sync.Once
}

func newExternalHostileError() *externalHostileError {
	return &externalHostileError{
		secret: "external-dependency-secret", entered: make(chan string, 8), release: make(chan struct{}),
	}
}

func (err *externalHostileError) inspect(method string) {
	select {
	case err.entered <- method:
	default:
	}
	<-err.release
}

func (err *externalHostileError) Error() string {
	err.inspect("Error")
	return err.secret
}

func (err *externalHostileError) Is(error) bool {
	err.inspect("Is")
	return false
}

func (err *externalHostileError) As(any) bool {
	err.inspect("As")
	return false
}

func (err *externalHostileError) Unwrap() error {
	err.inspect("Unwrap")
	return nil
}

type externalFullErrorWriter struct{ err error }

func (writer externalFullErrorWriter) Write(data []byte) (int, error) {
	return len(data), writer.err
}

type externalFailingListener struct{ acceptErr error }

func (listener *externalFailingListener) Accept() (net.Conn, error) { return nil, listener.acceptErr }
func (*externalFailingListener) Close() error                       { return nil }
func (*externalFailingListener) Addr() net.Addr                     { return externalAddress("127.0.0.1:0") }

type externalCloseErrorListener struct {
	net.Listener
	closeErr      error
	accepting     chan struct{}
	acceptingOnce sync.Once
}

func (listener *externalCloseErrorListener) Accept() (net.Conn, error) {
	listener.acceptingOnce.Do(func() { close(listener.accepting) })
	return listener.Listener.Accept()
}

func (listener *externalCloseErrorListener) Close() error {
	_ = listener.Listener.Close()
	return listener.closeErr
}

type externalConnectionErrorListener struct {
	net.Listener
	closeErr error
}

func (listener *externalConnectionErrorListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &externalCloseErrorConnection{Conn: connection, closeErr: listener.closeErr}, nil
}

type externalCloseErrorConnection struct {
	net.Conn
	closeErr error
	once     sync.Once
}

func (connection *externalCloseErrorConnection) Close() error {
	connection.once.Do(func() { _ = connection.Conn.Close() })
	return connection.closeErr
}

type externalAddress string

func (externalAddress) Network() string        { return "tcp" }
func (address externalAddress) String() string { return string(address) }

func externalDefinition(t *testing.T) appkit.Definition {
	t.Helper()
	executionScope := scope.Must("tenant", "external-appcmd")
	actor := identity.Actor{ID: "external-user", Type: "user", DisplayName: "External User", Scope: executionScope}
	descriptor := action.Descriptor{
		ID: "probe.read", Version: "1.0.0", Title: "Read probe", Permission: "probe.read",
		Preview: action.PreviewNone, AuditLevel: action.AuditMetadata, Channels: []action.Channel{action.ChannelCLI},
		InputSchema:  action.Object(nil).JSON(),
		OutputSchema: action.Object(map[string]action.Field{"value": action.RequiredField(action.Integer())}).JSON(),
	}
	registration := module.Register(module.Manifest{
		SchemaVersion: module.SchemaVersion, ID: "external-command-probe", Version: "1.0.0", Type: module.ModuleTypeFeature,
		Provides: []module.Capability{
			module.CapabilityAuthorization,
			module.CapabilityAudit,
			module.CapabilityIdentity,
			"probe",
		},
	}, func(_ context.Context, installation module.Scope) error {
		providers := []func() error{
			func() error {
				return module.Provide(installation, module.Authorizer(), authz.Authorizer(commandAllowAll{}))
			},
			func() error { return module.Provide(installation, module.AuditHook(), audit.Hook(commandAudit{})) },
			func() error {
				return module.Provide(installation, module.TokenAuthenticator(), identity.TokenAuthenticator(commandTokens{actor: actor}))
			},
		}
		for _, provide := range providers {
			if err := provide(); err != nil {
				return err
			}
		}
		return nil
	}, module.ActionBinding{
		Descriptor: descriptor,
		NewHandler: func(context.Context, module.Resolver) (action.Handler, error) {
			return commandProbeHandler{}, nil
		},
	})
	databaseConfig := testpostgres.New(t)
	databaseRegistration, err := postgres.Module(postgres.Options{
		URL: databaseConfig.URL, ApplicationSchema: databaseConfig.ApplicationSchema, QueueSchema: databaseConfig.QueueSchema,
	})
	if err != nil {
		panic(err)
	}
	return appkit.Definition{
		Metadata: externalMetadata(),
		Modules:  []module.Registration{databaseRegistration, registration},
	}
}

func externalMetadata() appkit.Metadata {
	return appkit.Metadata{ID: "external-command", Name: "External Command", Version: "0.1.0"}
}

type commandAllowAll struct{}

type commandAudit struct{}

func (commandAudit) Record(context.Context, audit.Event) error { return nil }

func (commandAllowAll) Authorize(context.Context, authz.Request) (authz.Decision, error) {
	return authz.Decision{Allowed: true, Fingerprint: "external-command-policy-v1"}, nil
}

const externalCommandToken = "external-command-token-000000000000000000000001"

type commandTokens struct{ actor identity.Actor }

func (authenticator commandTokens) AuthenticateToken(_ context.Context, token string) (identity.Actor, error) {
	if token != externalCommandToken {
		return identity.Actor{}, identity.ErrActorNotFound
	}
	return authenticator.actor, nil
}

type commandProbeHandler struct{}

func (commandProbeHandler) Plan(context.Context, action.Request) (action.PlanData, error) {
	return action.PlanData{Payload: json.RawMessage(`{}`), Impact: authz.Impact{}}, nil
}

func (commandProbeHandler) Execute(context.Context, action.Plan) (action.Result, error) {
	return action.Result{Data: json.RawMessage(`{"value":1}`)}, nil
}

func assertNoKernelType(t *testing.T, owner string, typeOf reflect.Type) {
	t.Helper()
	text := typeOf.String()
	for _, forbidden := range []string{
		"action.Registry",
		"action.Handler",
		"module.Host",
		"module.Scope",
		"module.Resolver",
		"database/sql.DB",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("%s exposes %s through %s", owner, forbidden, text)
		}
	}
}
