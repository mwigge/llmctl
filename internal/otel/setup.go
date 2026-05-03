package otel

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config carries the OTel initialisation parameters.
type Config struct {
	// Endpoint is the OTLP HTTP base URL (e.g. "http://localhost:4318").
	// When empty, a stdout exporter is used instead.
	Endpoint string
	// ServiceName is recorded as the service.name resource attribute.
	ServiceName string
}

// Setup initialises the global OpenTelemetry trace and metric providers.
// When cfg.Endpoint is non-empty, OTLP HTTP exporters are used; otherwise
// stdout exporters are configured (useful for development and testing).
//
// The returned shutdown function must be called before the process exits to
// flush buffered telemetry.
func Setup(cfg Config) (shutdown func(context.Context) error, err error) {
	if cfg.Endpoint != "" {
		return setupOTLP(cfg)
	}
	return setupStdout()
}

func setupOTLP(cfg Config) (func(context.Context) error, error) {
	ctx := context.Background()

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(cfg.Endpoint))
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(cfg.Endpoint))
	if err != nil {
		return nil, fmt.Errorf("otlp metric exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(traceExporter))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)))

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		return mp.Shutdown(ctx)
	}, nil
}

func setupStdout() (func(context.Context) error, error) {
	traceExporter, err := stdouttrace.New(stdouttrace.WithWriter(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("stdout trace exporter: %w", err)
	}

	metricExporter, err := stdoutmetric.New(stdoutmetric.WithWriter(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("stdout metric exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(traceExporter))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)))

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		return mp.Shutdown(ctx)
	}, nil
}
