package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds all OTel metric instruments for the proxy.
type Metrics struct {
	RequestDuration metric.Float64Histogram
	RequestCount    metric.Int64Counter
	UpstreamLatency metric.Float64Histogram
	MessageSize     metric.Int64Histogram
	ErrorsTotal     metric.Int64Counter
	ActiveSessions  metric.Int64UpDownCounter
}

// InitMetrics creates and registers all metric instruments.
func InitMetrics() (*Metrics, error) {
	meter := otel.Meter("mcp-otel-proxy")

	// OTel MCP semconv bucket boundaries
	durationBuckets := metric.WithExplicitBucketBoundaries(
		0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 300,
	)

	requestDuration, err := meter.Float64Histogram(
		"gen_ai.server.request.duration",
		metric.WithDescription("Duration of MCP request handling"),
		metric.WithUnit("s"),
		durationBuckets,
	)
	if err != nil {
		return nil, err
	}

	requestCount, err := meter.Int64Counter(
		"gen_ai.server.request.count",
		metric.WithDescription("Count of MCP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	upstreamLatency, err := meter.Float64Histogram(
		"mcp.proxy.upstream.latency",
		metric.WithDescription("Time to get response from upstream MCP server"),
		metric.WithUnit("s"),
		durationBuckets,
	)
	if err != nil {
		return nil, err
	}

	messageSize, err := meter.Int64Histogram(
		"mcp.proxy.message.size",
		metric.WithDescription("Request and response payload sizes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	errorsTotal, err := meter.Int64Counter(
		"mcp.proxy.errors.total",
		metric.WithDescription("Total proxy errors by type"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	activeSessions, err := meter.Int64UpDownCounter(
		"mcp.proxy.active_sessions",
		metric.WithDescription("Number of active MCP sessions"),
		metric.WithUnit("{session}"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		RequestDuration: requestDuration,
		RequestCount:    requestCount,
		UpstreamLatency: upstreamLatency,
		MessageSize:     messageSize,
		ErrorsTotal:     errorsTotal,
		ActiveSessions:  activeSessions,
	}, nil
}
