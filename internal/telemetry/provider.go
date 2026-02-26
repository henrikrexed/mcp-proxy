package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.opentelemetry.io/contrib/bridges/otelslog"
)

// Providers holds all OTel SDK providers.
type Providers struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	LoggerProvider *sdklog.LoggerProvider
	Logger         *slog.Logger
}

// InitOTel initializes all OTel providers (traces, metrics, logs) and returns them.
func InitOTel(ctx context.Context, endpoint, serviceName string, isInsecure bool) (*Providers, error) {
	dialOpts := []grpc.DialOption{}
	if isInsecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(endpoint, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to %s: %w", endpoint, err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTel resource: %w", err)
	}

	// Trace exporter + provider
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// Metric exporter + provider
	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// Log exporter + provider
	logExporter, err := otlploggrpc.New(ctx, otlploggrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)

	// Create slog logger with otelslog bridge
	logger := slog.New(otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp)))

	return &Providers{
		TracerProvider: tp,
		MeterProvider:  mp,
		LoggerProvider: lp,
		Logger:         logger,
	}, nil
}

// Shutdown gracefully shuts down all OTel providers.
func (p *Providers) Shutdown(ctx context.Context) {
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if p.TracerProvider != nil {
		if err := p.TracerProvider.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown tracer provider", "error", err)
		}
	}
	if p.MeterProvider != nil {
		if err := p.MeterProvider.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown meter provider", "error", err)
		}
	}
	if p.LoggerProvider != nil {
		if err := p.LoggerProvider.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown logger provider", "error", err)
		}
	}
}
