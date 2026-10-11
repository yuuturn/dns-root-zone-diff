// Package telemetry は OpenTelemetry (OTLP) によるトレース計装を提供する。
//
// 標準の環境変数で有効化する。OTEL_EXPORTER_OTLP_ENDPOINT または
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT が未設定の場合、Setup は何もせず
// no-op の ShutdownFunc を返す (トレースは送信されない)。
//
//	OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector.example.com:4317
//	OTEL_EXPORTER_OTLP_INSECURE=true    # TLS なしの OTLP/gRPC を使う場合
//	OTEL_SERVICE_NAME=dns-root-diff     # 既定値と同じなら不要
//	OTEL_RESOURCE_ATTRIBUTES=deployment.environment=production
//
// エンドポイント以外の設定 (ヘッダー / TLS / 圧縮 / タイムアウト) は
// OpenTelemetry の標準環境変数 (OTEL_EXPORTER_OTLP_*) に従う。
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultServiceName は OTEL_SERVICE_NAME が未設定のときに使うサービス名。
	DefaultServiceName = "dns-root-diff"

	// scopeName は Tracer の instrumentation scope 名。
	scopeName = "github.com/yfujii/dns-root-diff"
)

// ShutdownFunc は TracerProvider を終了し、未送信のスパンをフラッシュする。
type ShutdownFunc func(context.Context) error

// Enabled は OTLP エンドポイントが設定されているか (トレース送信が有効か) を返す。
func Enabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}

// Tracer は計装用の Tracer を返す。
// Setup が呼ばれていない場合は no-op の TracerProvider が使われる。
func Tracer() trace.Tracer {
	return otel.Tracer(scopeName)
}

// Setup はグローバルの TracerProvider を OTLP エクスポーターで初期化する。
// エンドポイントが未設定の場合は no-op の ShutdownFunc を返す。
func Setup(ctx context.Context, version string) (ShutdownFunc, error) {
	if !Enabled() {
		return noopShutdown, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	return SetupWithExporter(ctx, exporter, version)
}

// SetupWithExporter は任意の SpanExporter で TracerProvider を構成する。
// テストからインメモリエクスポーターを注入するためにも使う。
func SetupWithExporter(ctx context.Context, exporter sdktrace.SpanExporter, version string) (ShutdownFunc, error) {
	res, err := newResource(ctx, version)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	// トレースコンテキストを HTTP ヘッダーで伝播させる
	// (Web リクエストのスパンと root zone 取得のクライアントスパンを親子付ける)。
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}

func noopShutdown(context.Context) error { return nil }

func newResource(ctx context.Context, version string) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{semconv.ServiceName(DefaultServiceName)}
	if version != "" {
		attrs = append(attrs, semconv.ServiceVersion(version))
	}

	// WithFromEnv を後段に置き、OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES で
	// 既定値を上書きできるようにする。
	res, err := resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithFromEnv(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	return res, nil
}
