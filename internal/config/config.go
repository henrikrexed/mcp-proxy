package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all proxy configuration loaded from environment variables.
type Config struct {
	UpstreamURL        string
	ProxyPort          string
	OTELEndpoint       string
	OTELInsecure       bool
	ServiceName        string
	LogLevel           string
	ContextPropagation bool
	CapturePayload     bool
	SessionTTLSeconds  int
	ResourceAttributes string
}

func Load() (*Config, error) {
	upstreamURL := os.Getenv("UPSTREAM_URL")
	if upstreamURL == "" {
		return nil, fmt.Errorf("UPSTREAM_URL environment variable is required")
	}

	upstreamURL = strings.TrimRight(upstreamURL, "/")

	cfg := &Config{
		UpstreamURL:        upstreamURL,
		ProxyPort:          envOrDefault("PROXY_PORT", "8080"),
		OTELEndpoint:       envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		OTELInsecure:       envBoolOrDefault("OTEL_EXPORTER_OTLP_INSECURE", true),
		ServiceName:        envOrDefault("OTEL_SERVICE_NAME", "mcp-otel-proxy"),
		LogLevel:           strings.ToLower(envOrDefault("LOG_LEVEL", "info")),
		ContextPropagation: envBoolOrDefault("CONTEXT_PROPAGATION", true),
		CapturePayload:     envBoolOrDefault("CAPTURE_PAYLOAD", false),
		SessionTTLSeconds:  envIntOrDefault("SESSION_TTL", 3600),
		ResourceAttributes: os.Getenv("OTEL_RESOURCE_ATTRIBUTES"),
	}

	return cfg, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envBoolOrDefault(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return b
}

func envIntOrDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}
