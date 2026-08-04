// Package otel provides an explicitly selected OpenTelemetry adapter for
// Modary HTTP requests. It owns no global providers and records only stable
// method, route-template, and status-class dimensions.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package otel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/observe"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	traceSDK "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

const (
	ModuleID              = "opentelemetry"
	DefaultExportInterval = 30 * time.Second
	DefaultExportTimeout  = 5 * time.Second
	DefaultReadyTimeout   = 2 * time.Second
)

var headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+.^_` + "`" + `|~-]{1,128}$`)

// Options configures one OTLP/HTTP destination. Endpoint is the collector base
// URL, for example https://collector.example.com:4318. HTTP requires Insecure
// to be selected explicitly. Header values are treated as credentials and are
// never copied into resource attributes, metrics, spans, or log messages.
type Options struct {
	Endpoint       string
	Headers        map[string]string
	ServiceName    string
	ServiceVersion string
	Environment    string
	Insecure       bool
	ExportInterval time.Duration
	ExportTimeout  time.Duration
	ReadyTimeout   time.Duration
	// Logger receives bounded exporter state transitions. A nil Logger uses a
	// process-local JSON logger on stderr.
	Logger *slog.Logger
}

type normalizedOptions struct {
	endpoint       string
	hostPort       string
	headers        map[string]string
	serviceName    string
	serviceVersion string
	environment    string
	insecure       bool
	exportInterval time.Duration
	exportTimeout  time.Duration
	readyTimeout   time.Duration
	logger         *slog.Logger
}

// Module returns a side-effect-free registration. Exporters and providers are
// constructed only during Host startup and are shut down with the Host.
func Module(options Options) (module.Registration, error) {
	normalized, err := normalizeOptions(options)
	if err != nil {
		return module.Registration{}, err
	}
	return module.Registration{
		Definition: module.Definition{Manifest: module.Manifest{
			SchemaVersion: module.SchemaVersion,
			ID:            ModuleID,
			Version:       "0.1.0",
			Type:          module.ModuleTypeAdapter,
			Provides:      []module.Capability{module.CapabilityObservability},
		}},
		Start: func(ctx context.Context, installation module.Scope) error {
			return start(ctx, installation, normalized)
		},
	}, nil
}

func start(ctx context.Context, installation module.Scope, options normalizedOptions) error {
	if ctx == nil {
		return fmt.Errorf("OpenTelemetry start context is required")
	}
	traceOptions := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(options.endpoint + "/v1/traces"),
		otlptracehttp.WithHeaders(cloneHeaders(options.headers)),
		otlptracehttp.WithTimeout(options.exportTimeout),
	}
	metricOptions := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpointURL(options.endpoint + "/v1/metrics"),
		otlpmetrichttp.WithHeaders(cloneHeaders(options.headers)),
		otlpmetrichttp.WithTimeout(options.exportTimeout),
	}
	if options.insecure {
		traceOptions = append(traceOptions, otlptracehttp.WithInsecure())
		metricOptions = append(metricOptions, otlpmetrichttp.WithInsecure())
	}
	createdTraceExporter, err := otlptracehttp.New(ctx, traceOptions...)
	if err != nil {
		return fmt.Errorf("create OpenTelemetry trace exporter: %w", err)
	}
	createdMetricExporter, err := otlpmetrichttp.New(ctx, metricOptions...)
	if err != nil {
		_ = createdTraceExporter.Shutdown(ctx)
		return fmt.Errorf("create OpenTelemetry metric exporter: %w", err)
	}
	reporter := newExportReporter(options.logger)
	var traceExporter traceSDK.SpanExporter = safeTraceExporter{delegate: createdTraceExporter, reporter: reporter}
	var metricExporter metric.Exporter = safeMetricExporter{delegate: createdMetricExporter, reporter: reporter}
	attributes := []resource.Option{
		resource.WithAttributes(
			semconv.ServiceName(options.serviceName),
			semconv.ServiceVersion(options.serviceVersion),
			semconv.DeploymentEnvironmentName(options.environment),
		),
	}
	applicationResource, err := resource.New(ctx, attributes...)
	if err != nil {
		_ = errors.Join(traceExporter.Shutdown(ctx), metricExporter.Shutdown(ctx))
		return fmt.Errorf("create OpenTelemetry resource: %w", err)
	}
	traceProvider := traceSDK.NewTracerProvider(
		traceSDK.WithResource(applicationResource),
		traceSDK.WithBatcher(traceExporter, traceSDK.WithBatchTimeout(options.exportInterval), traceSDK.WithExportTimeout(options.exportTimeout)),
	)
	metricProvider := metric.NewMeterProvider(
		metric.WithResource(applicationResource),
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			metric.WithInterval(options.exportInterval), metric.WithTimeout(options.exportTimeout))),
	)
	service, err := newService(options, traceProvider, metricProvider, propagation.TraceContext{})
	if err != nil {
		_ = errors.Join(traceProvider.Shutdown(ctx), metricProvider.Shutdown(ctx))
		return err
	}
	if err := module.OnStop(installation, service.shutdown); err != nil {
		_ = service.shutdown(ctx)
		return err
	}
	return module.Provide(installation, module.Observability(), observe.Service(service))
}

func normalizeOptions(options Options) (normalizedOptions, error) {
	if len(options.Endpoint) == 0 || len(options.Endpoint) > 2048 || strings.TrimSpace(options.Endpoint) != options.Endpoint {
		return normalizedOptions{}, fmt.Errorf("OpenTelemetry endpoint is invalid")
	}
	endpoint, err := url.Parse(options.Endpoint)
	if err != nil || !endpoint.IsAbs() || endpoint.Opaque != "" || endpoint.Host == "" || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" || endpoint.RawPath != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return normalizedOptions{}, fmt.Errorf("OpenTelemetry endpoint is invalid")
	}
	if endpoint.Scheme != "https" && !(options.Insecure && endpoint.Scheme == "http") {
		return normalizedOptions{}, fmt.Errorf("OpenTelemetry endpoint must use HTTPS")
	}
	if endpoint.Scheme == "https" && options.Insecure {
		return normalizedOptions{}, fmt.Errorf("OpenTelemetry insecure mode requires an HTTP endpoint")
	}
	endpoint.Path = ""
	hostPort := endpoint.Host
	if endpoint.Port() == "" {
		port := "443"
		if endpoint.Scheme == "http" {
			port = "80"
		}
		hostPort = net.JoinHostPort(endpoint.Hostname(), port)
	}
	if err := validateText("service name", options.ServiceName, 128, true); err != nil {
		return normalizedOptions{}, err
	}
	if err := validateText("service version", options.ServiceVersion, 128, true); err != nil {
		return normalizedOptions{}, err
	}
	if err := validateText("environment", options.Environment, 64, true); err != nil {
		return normalizedOptions{}, err
	}
	if len(options.Headers) > 32 {
		return normalizedOptions{}, fmt.Errorf("OpenTelemetry header count exceeds 32")
	}
	headers := make(map[string]string, len(options.Headers))
	headerNames := make(map[string]struct{}, len(options.Headers))
	for name, value := range options.Headers {
		if !headerNamePattern.MatchString(name) || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n") {
			return normalizedOptions{}, fmt.Errorf("OpenTelemetry header configuration is invalid")
		}
		canonicalName := strings.ToLower(name)
		if _, duplicate := headerNames[canonicalName]; duplicate {
			return normalizedOptions{}, fmt.Errorf("OpenTelemetry header configuration contains duplicate names")
		}
		headerNames[canonicalName] = struct{}{}
		headers[name] = value
	}
	if err := normalizeDuration(&options.ExportInterval, DefaultExportInterval, time.Second, 10*time.Minute, "export interval"); err != nil {
		return normalizedOptions{}, err
	}
	if err := normalizeDuration(&options.ExportTimeout, DefaultExportTimeout, 100*time.Millisecond, time.Minute, "export timeout"); err != nil {
		return normalizedOptions{}, err
	}
	if err := normalizeDuration(&options.ReadyTimeout, DefaultReadyTimeout, 100*time.Millisecond, 30*time.Second, "readiness timeout"); err != nil {
		return normalizedOptions{}, err
	}
	return normalizedOptions{
		endpoint: endpoint.String(), hostPort: hostPort, headers: headers,
		serviceName: options.ServiceName, serviceVersion: options.ServiceVersion,
		environment: options.Environment, insecure: options.Insecure,
		exportInterval: options.ExportInterval, exportTimeout: options.ExportTimeout,
		readyTimeout: options.ReadyTimeout,
		logger: func() *slog.Logger {
			if options.Logger != nil {
				return options.Logger
			}
			return slog.New(slog.NewJSONHandler(os.Stderr, nil))
		}(),
	}, nil
}

func normalizeDuration(value *time.Duration, fallback, minimum, maximum time.Duration, name string) error {
	if *value < 0 || *value > maximum {
		return fmt.Errorf("OpenTelemetry %s must be between zero and %s", name, maximum)
	}
	if *value == 0 {
		*value = fallback
	}
	if *value < minimum {
		return fmt.Errorf("OpenTelemetry %s must be at least %s", name, minimum)
	}
	return nil
}

func validateText(name, value string, maximum int, required bool) error {
	if !utf8.ValidString(value) || len(value) > maximum || strings.TrimSpace(value) != value || strings.ContainsFunc(value, unicode.IsControl) || (required && value == "") {
		return fmt.Errorf("OpenTelemetry %s is invalid", name)
	}
	return nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for name, value := range headers {
		cloned[name] = value
	}
	return cloned
}

type resourceState struct {
	mu      sync.RWMutex
	stopped bool
}
