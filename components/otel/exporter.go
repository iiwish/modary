package otel

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	metricSDK "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	traceSDK "go.opentelemetry.io/otel/sdk/trace"
)

var (
	errTraceExport  = errors.New("OpenTelemetry trace export failed")
	errMetricExport = errors.New("OpenTelemetry metric export failed")
)

type exportReporter struct {
	logger *slog.Logger
	mu     sync.Mutex
	failed map[string]bool
}

func newExportReporter(logger *slog.Logger) *exportReporter {
	return &exportReporter{logger: logger, failed: make(map[string]bool, 2)}
}

func (reporter *exportReporter) result(ctx context.Context, signal string, err error, publicErr error) error {
	reporter.mu.Lock()
	wasFailed := reporter.failed[signal]
	failed := err != nil
	reporter.failed[signal] = failed
	reporter.mu.Unlock()
	if failed && !wasFailed {
		reporter.log(ctx, slog.LevelError, "telemetry export failed", "telemetry.export.failed", signal)
	}
	if !failed && wasFailed {
		reporter.log(ctx, slog.LevelInfo, "telemetry export recovered", "telemetry.export.recovered", signal)
	}
	if failed {
		return publicErr
	}
	return nil
}

func (reporter *exportReporter) log(ctx context.Context, level slog.Level, message, event, signal string) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
		}
	}()
	reporter.logger.Log(ctx, level, message, "event", event, "signal", signal)
	returned = true
}

type safeTraceExporter struct {
	delegate traceSDK.SpanExporter
	reporter *exportReporter
}

func (exporter safeTraceExporter) ExportSpans(ctx context.Context, spans []traceSDK.ReadOnlySpan) error {
	return exporter.reporter.result(ctx, "traces", exporter.delegate.ExportSpans(ctx, spans), errTraceExport)
}

func (exporter safeTraceExporter) Shutdown(ctx context.Context) error {
	return exporter.reporter.result(ctx, "traces", exporter.delegate.Shutdown(ctx), errTraceExport)
}

type safeMetricExporter struct {
	delegate metricSDK.Exporter
	reporter *exportReporter
}

func (exporter safeMetricExporter) Temporality(kind metricSDK.InstrumentKind) metricdata.Temporality {
	return exporter.delegate.Temporality(kind)
}

func (exporter safeMetricExporter) Aggregation(kind metricSDK.InstrumentKind) metricSDK.Aggregation {
	return exporter.delegate.Aggregation(kind)
}

func (exporter safeMetricExporter) Export(ctx context.Context, metrics *metricdata.ResourceMetrics) error {
	return exporter.reporter.result(ctx, "metrics", exporter.delegate.Export(ctx, metrics), errMetricExport)
}

func (exporter safeMetricExporter) ForceFlush(ctx context.Context) error {
	return exporter.reporter.result(ctx, "metrics", exporter.delegate.ForceFlush(ctx), errMetricExport)
}

func (exporter safeMetricExporter) Shutdown(ctx context.Context) error {
	return exporter.reporter.result(ctx, "metrics", exporter.delegate.Shutdown(ctx), errMetricExport)
}
