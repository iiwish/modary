# Observability

Modary separates required process diagnostics from optional telemetry.
Generated processes write bounded JSON diagnostics with `log/slog`. Selecting
`--with otel` in an Admin project adds the independently versioned
`components/otel` module and OTLP/HTTP traces and metrics; an unselected project
has no OpenTelemetry SDK or exporter dependency.

## Select And Configure

```bash
modary new operations-admin --profile admin --with otel
```

```bash
export MODARY_OTEL_ENDPOINT='https://collector.example.com:4318'
export MODARY_OTEL_ENVIRONMENT='production'
export MODARY_OTEL_HEADERS_JSON='{"Authorization":"Bearer ..."}'
```

`MODARY_OTEL_INSECURE=true` is accepted only with an HTTP endpoint and is for an
isolated local collector. The endpoint is a base URL without path, query,
fragment, or user information. Header values, service identity, environment,
export intervals, export timeouts, and readiness timeouts are bounded before
startup.

## Signals

Selected HTTP routes emit server spans and metrics for the preflighted method
and route template, status class, duration, and active requests. Database and
task facades emit a closed operation vocabulary for execution, query,
transaction, enqueue, handling, and inspection. W3C trace context is extracted
without installing global providers.

The component never accepts raw paths, queries, SQL, payloads, credentials,
actor IDs, or scope IDs as metric dimensions. Exporter header values are used
only on collector requests. Application error text is not copied into span or
metric attributes.

Trace and metric exporters emit one structured `telemetry.export.failed`
transition per outage and one `telemetry.export.recovered` transition after
recovery. The diagnostic contains only the closed `traces` or `metrics` signal;
collector response text and configured headers are never logged. Errors returned
to the OpenTelemetry SDK are stable secret-free sentinels.

## Lifecycle And Readiness

The component owns its trace provider, meter provider, periodic reader, and
exporters. Startup publishes telemetry only after all instruments exist.
Shutdown force-flushes and closes providers under the application shutdown
context. It never changes process-global OpenTelemetry providers.

The generated telemetry Admin adds a bounded collector connectivity check to
`/readyz`. Collector loss keeps `/livez` healthy and makes `/readyz` unavailable;
it does not restart the process or grant traffic. Decide operationally whether
collector availability should gate traffic before retaining that generated
check in a product.

## Operator Checklist

- Restrict collector network access and rotate exporter credentials.
- Use TLS outside isolated local development.
- Alert on export failures, dropped telemetry, request failures, latency, and
  readiness transitions without using user or scope labels.
- Set collector retention, sampling, redaction, tenancy, and cost policy.
- Verify that logs, spans, and metrics contain no request bodies, tokens,
  authorization codes, database URLs, raw SQL, or task payloads.
- Exercise collector interruption and bounded shutdown in deployment tests.
