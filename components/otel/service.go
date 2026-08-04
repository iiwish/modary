package otel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	metricSDK "go.opentelemetry.io/otel/sdk/metric"
	traceSDK "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/iiwish/modary/observe"
)

type service struct {
	options        normalizedOptions
	traceProvider  *traceSDK.TracerProvider
	metricProvider *metricSDK.MeterProvider
	propagator     propagation.TextMapPropagator
	tracer         trace.Tracer
	requests       metric.Int64Counter
	duration       metric.Float64Histogram
	inflight       metric.Int64UpDownCounter
	operations     metric.Int64Counter
	operationTime  metric.Float64Histogram
	state          resourceState
	stopOnce       sync.Once
	stopErr        error
}

func newService(options normalizedOptions, traceProvider *traceSDK.TracerProvider, metricProvider *metricSDK.MeterProvider, propagator propagation.TextMapPropagator) (*service, error) {
	tracer := traceProvider.Tracer("github.com/iiwish/modary/components/otel")
	meter := metricProvider.Meter("github.com/iiwish/modary/components/otel")
	requests, err := meter.Int64Counter("modary.http.server.requests", metric.WithDescription("Completed HTTP requests"))
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry request counter: %w", err)
	}
	duration, err := meter.Float64Histogram("modary.http.server.duration", metric.WithUnit("s"), metric.WithDescription("HTTP request duration"))
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry duration histogram: %w", err)
	}
	inflight, err := meter.Int64UpDownCounter("modary.http.server.active_requests", metric.WithDescription("Active HTTP requests"))
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry active request counter: %w", err)
	}
	operations, err := meter.Int64Counter("modary.operation.executions", metric.WithDescription("Completed bounded framework operations"))
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry operation counter: %w", err)
	}
	operationTime, err := meter.Float64Histogram("modary.operation.duration", metric.WithUnit("s"), metric.WithDescription("Bounded framework operation duration"))
	if err != nil {
		return nil, fmt.Errorf("create OpenTelemetry operation duration histogram: %w", err)
	}
	return &service{
		options: options, traceProvider: traceProvider, metricProvider: metricProvider,
		propagator: propagator, tracer: tracer, requests: requests, duration: duration,
		inflight: inflight, operations: operations, operationTime: operationTime,
	}, nil
}

func (service *service) StartOperation(ctx context.Context, operation observe.Operation) (context.Context, func(observe.Outcome)) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !validOperation(operation) {
		return ctx, func(observe.Outcome) {}
	}
	started := time.Now()
	attributes := []attribute.KeyValue{attribute.String("modary.operation", string(operation))}
	operationCtx, span := service.tracer.Start(ctx, string(operation), trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attributes...))
	var once sync.Once
	return operationCtx, func(outcome observe.Outcome) {
		once.Do(func() {
			if outcome != observe.OutcomeSuccess && outcome != observe.OutcomeError {
				outcome = observe.OutcomeError
			}
			completed := append(attributes, attribute.String("modary.outcome", string(outcome)))
			service.operations.Add(operationCtx, 1, metric.WithAttributes(completed...))
			service.operationTime.Record(operationCtx, time.Since(started).Seconds(), metric.WithAttributes(completed...))
			if outcome == observe.OutcomeError {
				span.SetStatus(codes.Error, "operation failed")
			}
			span.End()
		})
	}
}

func validOperation(operation observe.Operation) bool {
	switch operation {
	case observe.OperationDatabaseExec, observe.OperationDatabaseQuery, observe.OperationDatabaseTransaction,
		observe.OperationTaskEnqueue, observe.OperationTaskHandle, observe.OperationTaskInspect:
		return true
	default:
		return false
	}
}

func (service *service) WrapHTTP(method, routeTemplate string, next http.Handler) http.Handler {
	if service == nil || next == nil || method == "" || routeTemplate == "" {
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		})
	}
	stable := []attribute.KeyValue{
		attribute.String("http.request.method", method),
		attribute.String("http.route", routeTemplate),
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		ctx := service.propagator.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		ctx, span := service.tracer.Start(ctx, method+" "+routeTemplate, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(stable...))
		tracked := &statusWriter{ResponseWriter: writer}
		service.inflight.Add(ctx, 1, metric.WithAttributes(stable...))
		defer func() {
			service.inflight.Add(ctx, -1, metric.WithAttributes(stable...))
			status := tracked.status
			if status == 0 {
				status = http.StatusOK
			}
			statusClass := strconv.Itoa(status/100) + "xx"
			completed := append(append([]attribute.KeyValue(nil), stable...), attribute.String("http.response.status_class", statusClass))
			service.requests.Add(ctx, 1, metric.WithAttributes(completed...))
			service.duration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(completed...))
			span.SetAttributes(attribute.Int("http.response.status_code", status))
			if status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
			span.End()
		}()
		next.ServeHTTP(tracked, request.WithContext(ctx))
	})
}

func (service *service) Ready(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("OpenTelemetry readiness context is required")
	}
	if service == nil {
		return fmt.Errorf("OpenTelemetry service is unavailable")
	}
	service.state.mu.RLock()
	stopped := service.state.stopped
	service.state.mu.RUnlock()
	if stopped {
		return fmt.Errorf("OpenTelemetry service is stopped")
	}
	checkCtx, cancel := context.WithTimeout(ctx, service.options.readyTimeout)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(checkCtx, "tcp", service.options.hostPort)
	if err != nil {
		return fmt.Errorf("OpenTelemetry collector is unavailable: %w", err)
	}
	return connection.Close()
}

func (service *service) shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("OpenTelemetry shutdown context is required")
	}
	if service == nil {
		return nil
	}
	service.stopOnce.Do(func() {
		service.state.mu.Lock()
		service.state.stopped = true
		service.state.mu.Unlock()
		service.stopErr = errors.Join(
			service.traceProvider.ForceFlush(ctx),
			service.metricProvider.ForceFlush(ctx),
			service.traceProvider.Shutdown(ctx),
			service.metricProvider.Shutdown(ctx),
		)
	})
	return service.stopErr
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (writer *statusWriter) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

func (writer *statusWriter) WriteHeader(status int) {
	if writer.status != 0 {
		return
	}
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *statusWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(body)
}
