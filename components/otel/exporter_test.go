package otel

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	traceSDK "go.opentelemetry.io/otel/sdk/trace"
)

func TestExporterDiagnosticsAreBoundedDeduplicatedAndRecoverable(t *testing.T) {
	delegate := &controlledTraceExporter{err: errors.New("private collector response")}
	var logs bytes.Buffer
	exporter := safeTraceExporter{
		delegate: delegate,
		reporter: newExportReporter(slog.New(slog.NewJSONHandler(&logs, nil))),
	}
	for range 2 {
		if err := exporter.ExportSpans(context.Background(), nil); !errors.Is(err, errTraceExport) ||
			strings.Contains(err.Error(), "private collector response") {
			t.Fatalf("ExportSpans() error = %v", err)
		}
	}
	delegate.setError(nil)
	if err := exporter.ExportSpans(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	output := logs.String()
	if strings.Count(output, `"event":"telemetry.export.failed"`) != 1 ||
		strings.Count(output, `"event":"telemetry.export.recovered"`) != 1 ||
		strings.Contains(output, "private collector response") {
		t.Fatalf("exporter diagnostics = %q", output)
	}
}

type controlledTraceExporter struct {
	mu  sync.Mutex
	err error
}

func (exporter *controlledTraceExporter) ExportSpans(context.Context, []traceSDK.ReadOnlySpan) error {
	exporter.mu.Lock()
	defer exporter.mu.Unlock()
	return exporter.err
}

func (exporter *controlledTraceExporter) Shutdown(context.Context) error { return nil }

func (exporter *controlledTraceExporter) setError(err error) {
	exporter.mu.Lock()
	exporter.err = err
	exporter.mu.Unlock()
}
