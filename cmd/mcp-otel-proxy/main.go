package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/isitobservable/mcp-otel-proxy/internal/config"
	"github.com/isitobservable/mcp-otel-proxy/internal/health"
	"github.com/isitobservable/mcp-otel-proxy/internal/mcp"
	"github.com/isitobservable/mcp-otel-proxy/internal/proxy"
	"github.com/isitobservable/mcp-otel-proxy/internal/telemetry"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	// Set up slog level for stdout logging during init
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize OTel providers
	providers, err := telemetry.InitOTel(ctx, cfg.OTELEndpoint, cfg.ServiceName, cfg.OTELInsecure)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize OpenTelemetry: %v\n", err)
		os.Exit(1)
	}
	defer providers.Shutdown(context.Background())

	// Use OTel-bridged logger with level filter
	logger := providers.Logger
	slog.SetDefault(slog.New(newLevelHandler(logLevel, logger.Handler())))

	// Initialize metrics
	metrics, err := telemetry.InitMetrics()
	if err != nil {
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	// Initialize session store
	sessions := mcp.NewSessionStore(
		time.Duration(cfg.SessionTTLSeconds)*time.Second,
		func() { metrics.ActiveSessions.Add(ctx, 1) },
		func() { metrics.ActiveSessions.Add(ctx, -1) },
	)

	// Create proxy handler
	proxyHandler, err := proxy.New(cfg, metrics, sessions, slog.Default())
	if err != nil {
		slog.Error("failed to create proxy handler", "error", err)
		os.Exit(1)
	}

	// Set up HTTP mux
	mux := http.NewServeMux()

	// Health endpoints (no telemetry)
	healthHandler := health.Handler(cfg.UpstreamURL)
	mux.Handle("GET /healthz", healthHandler)
	mux.Handle("GET /readyz", healthHandler)

	// All other requests go to proxy
	mux.Handle("/", proxyHandler)

	server := &http.Server{
		Addr:         ":" + cfg.ProxyPort,
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("mcp-otel-proxy starting",
		"port", cfg.ProxyPort,
		"upstream", cfg.UpstreamURL,
		"otel.endpoint", cfg.OTELEndpoint,
		"otel.insecure", cfg.OTELInsecure,
		"service.name", cfg.ServiceName,
		"context.propagation", cfg.ContextPropagation,
		"capture.payload", cfg.CapturePayload,
		"log.level", cfg.LogLevel,
		"session.ttl", cfg.SessionTTLSeconds,
	)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-sigCh
	slog.Info("shutting down gracefully")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
}

// levelHandler wraps a slog.Handler to filter by minimum level.
type levelHandler struct {
	level   slog.Level
	handler slog.Handler
}

func newLevelHandler(level slog.Level, handler slog.Handler) *levelHandler {
	return &levelHandler{level: level, handler: handler}
}

func (h *levelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.handler.Handle(ctx, r)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelHandler{level: h.level, handler: h.handler.WithAttrs(attrs)}
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{level: h.level, handler: h.handler.WithGroup(name)}
}
