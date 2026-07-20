// Package tracing wraps Outpost's per-call OpenTelemetry spans. Spans
// carry only call metadata (upstream, method, tool name, duration,
// success) — never arguments or results, matching the no-payload logging
// discipline used everywhere else in this codebase (internal/logging).
// No real OTel collector exists yet (open founder item); NewProvider
// exports to an io.Writer (stdout in production) until one does — the
// exporter is swappable, the span shape is the stable contract.
package tracing

import (
	"context"
	"io"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// NewProvider builds a TracerProvider that exports finished spans as JSON
// to w.
func NewProvider(w io.Writer) (*sdktrace.TracerProvider, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithWriter(w))
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(0))), nil
}

// StartCallSpan starts a span for one proxied MCP call.
func StartCallSpan(ctx context.Context, tp *sdktrace.TracerProvider, upstream, method, tool string) (context.Context, trace.Span) {
	tracer := tp.Tracer("outpost")
	return tracer.Start(ctx, method,
		trace.WithAttributes(
			attribute.String("upstream", upstream),
			attribute.String("method", method),
			attribute.String("tool", tool),
		),
	)
}

// EndCallSpan records the outcome of a call and ends span.
func EndCallSpan(span trace.Span, durationMS float64, success bool) {
	span.SetAttributes(
		attribute.Float64("duration_ms", durationMS),
		attribute.Bool("success", success),
	)
	span.End()
}
