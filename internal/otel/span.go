package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/mwigge/llmctl/internal/otel"

// StartSpan starts a named span on the global tracer, forwarding any
// additional attributes. The caller is responsible for ending the span.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	return otel.GetTracerProvider().Tracer(instrumentationName).Start(
		ctx, name, trace.WithAttributes(attrs...),
	)
}

// RecordRequest starts and immediately ends a span recording a single
// inference request with model, token counts, and latency.
func RecordRequest(ctx context.Context, model string, inputTok, outputTok int, latencyMs int64) {
	_, span := StartSpan(ctx, "llmctl.inference",
		attribute.String("model", model),
		attribute.Int("tokens.input", inputTok),
		attribute.Int("tokens.output", outputTok),
		attribute.Int64("latency_ms", latencyMs),
	)
	span.End()
}
