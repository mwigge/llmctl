package otel_test

import (
	"context"
	"testing"
	"time"

	llmotel "github.com/mwigge/llmctl/internal/otel"
)

func TestSetup_StdoutExporter(t *testing.T) {
	t.Parallel()

	shutdown, err := llmotel.Setup(llmotel.Config{})
	if err != nil {
		t.Fatalf("Setup() with empty endpoint error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("Setup() returned nil shutdown function")
	}

	ctx := context.Background()
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown() error = %v", err)
	}
}

func TestSetup_ServiceNameDefault(t *testing.T) {
	t.Parallel()

	// Verify Setup succeeds with a ServiceName but no Endpoint.
	shutdown, err := llmotel.Setup(llmotel.Config{ServiceName: "llmctl-test"})
	if err != nil {
		t.Fatalf("Setup(ServiceName) error = %v", err)
	}
	ctx := context.Background()
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown() error = %v", err)
	}
}

func TestStartSpan_DoesNotPanic(t *testing.T) {
	t.Parallel()

	shutdown, err := llmotel.Setup(llmotel.Config{})
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Errorf("shutdown() error = %v", err)
		}
	}()

	ctx, span := llmotel.StartSpan(context.Background(), "test.span")
	if ctx == nil {
		t.Error("StartSpan() returned nil context")
	}
	if span == nil {
		t.Error("StartSpan() returned nil span")
	}
	span.End()
}

func TestRecordRequest_DoesNotPanic(t *testing.T) {
	t.Parallel()

	shutdown, err := llmotel.Setup(llmotel.Config{})
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Errorf("shutdown() error = %v", err)
		}
	}()

	// Should not panic; no return value to assert.
	llmotel.RecordRequest(context.Background(), "llama3", 100, 50, 120)
}

func TestSetup_OTLPEndpoint(t *testing.T) {
	t.Parallel()

	// OTLP HTTP exporters connect lazily — Setup should succeed even when
	// the collector endpoint is unreachable.
	shutdown, err := llmotel.Setup(llmotel.Config{
		Endpoint:    "http://localhost:14318",
		ServiceName: "llmctl-test",
	})
	if err != nil {
		t.Fatalf("Setup(OTLP) error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("Setup(OTLP) returned nil shutdown")
	}

	// Shutdown may produce a connection error; that is acceptable here.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = shutdown(ctx) // best-effort flush; ignore unreachable collector error
}

func TestStartSpan_NilContext(t *testing.T) {
	t.Parallel()

	shutdown, err := llmotel.Setup(llmotel.Config{})
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Errorf("shutdown() error = %v", err)
		}
	}()

	// StartSpan must not panic when given a nil context.
	ctx, span := llmotel.StartSpan(nil, "nil.ctx.span")
	if ctx == nil {
		t.Error("StartSpan(nil) returned nil context")
	}
	span.End()
}
