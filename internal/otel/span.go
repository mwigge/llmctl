package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
	meter := otel.GetMeterProvider().Meter(instrumentationName)
	if counter, err := meter.Int64Counter("llmctl.ai.tokens"); err == nil {
		counter.Add(ctx, int64(inputTok), metric.WithAttributes(attribute.String("model", model), attribute.String("direction", "input")))
		counter.Add(ctx, int64(outputTok), metric.WithAttributes(attribute.String("model", model), attribute.String("direction", "output")))
	}
	if hist, err := meter.Int64Histogram("llmctl.ai.latency_ms"); err == nil {
		hist.Record(ctx, latencyMs, metric.WithAttributes(attribute.String("model", model)))
	}
}

// RecordOperation records model-serving operational telemetry.
func RecordOperation(ctx context.Context, source string, value float64, unit string, attrs ...attribute.KeyValue) {
	all := append([]attribute.KeyValue{
		attribute.String("source", source),
		attribute.Float64("value", value),
		attribute.String("unit", unit),
	}, attrs...)
	_, span := StartSpan(ctx, "llmctl.model.operation", all...)
	span.End()
	meter := otel.GetMeterProvider().Meter(instrumentationName)
	if gauge, err := meter.Float64Gauge("llmctl.model.operation"); err == nil {
		gauge.Record(ctx, value, metric.WithAttributes(all...))
	}
}

// RecordDrift records an AI quality/drift telemetry point.
func RecordDrift(ctx context.Context, model string, score float64, source string, attrs ...attribute.KeyValue) {
	all := append([]attribute.KeyValue{
		attribute.String("model", model),
		attribute.Float64("drift.score", score),
		attribute.String("source", source),
	}, attrs...)
	_, span := StartSpan(ctx, "llmctl.ai.drift", all...)
	span.End()
	meter := otel.GetMeterProvider().Meter(instrumentationName)
	if gauge, err := meter.Float64Gauge("llmctl.ai.drift.score"); err == nil {
		gauge.Record(ctx, score, metric.WithAttributes(all...))
	}
}
