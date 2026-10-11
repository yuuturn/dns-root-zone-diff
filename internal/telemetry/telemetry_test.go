package telemetry_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/yfujii/dns-root-diff/internal/telemetry"
)

// recordingExporter はエクスポートされたスパンを保持するテスト用エクスポーター。
// tracetest.InMemoryExporter は Shutdown 時に記録を消去するため検証に使えない。
type recordingExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *recordingExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)

	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error { return nil }

func (e *recordingExporter) Spans() []sdktrace.ReadOnlySpan {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]sdktrace.ReadOnlySpan(nil), e.spans...)
}

func TestEnabled(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		tracesEndpoint string
		want           bool
	}{
		{name: "エンドポイント未設定なら無効", want: false},
		{name: "OTEL_EXPORTER_OTLP_ENDPOINT で有効", endpoint: "http://localhost:4317", want: true},
		{name: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT で有効", tracesEndpoint: "http://localhost:4317", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", tt.endpoint)
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", tt.tracesEndpoint)

			if got := telemetry.Enabled(); got != tt.want {
				t.Errorf("Enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetupDisabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	shutdown, err := telemetry.Setup(context.Background(), "test")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("Setup() returned nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown() error = %v", err)
	}
}

func TestSetupWithExporter(t *testing.T) {
	exporter := &recordingExporter{}

	shutdown, err := telemetry.SetupWithExporter(context.Background(), exporter, "1.2.3")
	if err != nil {
		t.Fatalf("SetupWithExporter() error = %v", err)
	}

	_, span := telemetry.Tracer().Start(context.Background(), "test-span")
	span.End()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	spans := exporter.Spans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Name() != "test-span" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "test-span")
	}

	attrs := make(map[string]string)
	for _, kv := range spans[0].Resource().Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
	}

	if got := attrs["service.name"]; got != telemetry.DefaultServiceName {
		t.Errorf("service.name = %q, want %q", got, telemetry.DefaultServiceName)
	}
	if got := attrs["service.version"]; got != "1.2.3" {
		t.Errorf("service.version = %q, want %q", got, "1.2.3")
	}
	if attrs["host.name"] == "" {
		t.Error("host.name is empty")
	}
}

func TestSetupWithExporterServiceNameFromEnv(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "custom-service")

	exporter := &recordingExporter{}
	shutdown, err := telemetry.SetupWithExporter(context.Background(), exporter, "1.2.3")
	if err != nil {
		t.Fatalf("SetupWithExporter() error = %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown() error = %v", err)
		}
	})

	_, span := telemetry.Tracer().Start(context.Background(), "test-span")
	span.End()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	spans := exporter.Spans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}

	attrs := make(map[string]string)
	for _, kv := range spans[0].Resource().Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
	}
	if got := attrs["service.name"]; got != "custom-service" {
		t.Errorf("service.name = %q, want %q", got, "custom-service")
	}
}

func TestSetupWithExporterPropagatesTraceContext(t *testing.T) {
	exporter := &recordingExporter{}
	shutdown, err := telemetry.SetupWithExporter(context.Background(), exporter, "1.2.3")
	if err != nil {
		t.Fatalf("SetupWithExporter() error = %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown() error = %v", err)
		}
	})

	ctx, span := telemetry.Tracer().Start(context.Background(), "parent")
	defer span.End()

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	if carrier.Get("traceparent") == "" {
		t.Error("traceparent header is not injected")
	}
}
