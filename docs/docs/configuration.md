# Configuration

All configuration is via environment variables. No config files needed.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `UPSTREAM_URL` | Yes | — | URL of the upstream MCP server (e.g., `http://localhost:3000`) |
| `PROXY_PORT` | No | `8080` | Port the proxy listens on |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | No | `localhost:4317` | OTLP gRPC endpoint for telemetry export |
| `OTEL_EXPORTER_OTLP_INSECURE` | No | `true` | Use insecure gRPC connection (no TLS) |
| `OTEL_SERVICE_NAME` | No | `mcp-otel-proxy` | OTel service name |
| `LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `CONTEXT_PROPAGATION` | No | `true` | Enable trace context propagation via params._meta |
| `CAPTURE_PAYLOAD` | No | `false` | Capture tool call arguments and results in spans |
| `SESSION_TTL` | No | `3600` | Session eviction TTL in seconds |
| `OTEL_RESOURCE_ATTRIBUTES` | No | — | Additional OTel resource attributes (key=value,key=value) |

## Examples

### Minimal

```bash
UPSTREAM_URL=http://localhost:3000 ./mcp-otel-proxy
```

### Production

```bash
UPSTREAM_URL=http://localhost:3000 \
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317 \
OTEL_SERVICE_NAME=my-mcp-proxy \
LOG_LEVEL=warn \
CONTEXT_PROPAGATION=true \
CAPTURE_PAYLOAD=false \
SESSION_TTL=7200 \
./mcp-otel-proxy
```

### Debug (all signals verbose)

```bash
UPSTREAM_URL=http://localhost:3000 \
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 \
LOG_LEVEL=debug \
CAPTURE_PAYLOAD=true \
./mcp-otel-proxy
```

## Helm Values

When deploying via Helm, these map to `values.yaml`:

```yaml
proxy:
  logLevel: info
  contextPropagation: "true"
  capturePayload: "false"
  sessionTTL: "3600"

otel:
  enabled: true
  endpoint: "otel-collector:4317"
  insecure: "true"
  serviceName: "mcp-otel-proxy"
```
