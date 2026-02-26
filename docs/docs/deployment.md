# Deployment

## Kubernetes (Helm)

The recommended deployment method. The Helm chart deploys your MCP server with the proxy as a sidecar.

### Install

```bash
helm install my-mcp deploy/helm/mcp-otel-proxy \
  --set mcpServer.image=your-org/your-mcp-server:latest \
  --set mcpServer.port=3000 \
  --set otel.endpoint=otel-collector.observability.svc.cluster.local:4317 \
  --set otel.serviceName=my-mcp-proxy
```

### With Gateway API

Expose the MCP server externally via Gateway API:

```bash
helm install my-mcp deploy/helm/mcp-otel-proxy \
  --set mcpServer.image=your-org/your-mcp-server:latest \
  --set gateway.enabled=true \
  --set gateway.name=my-gateway \
  --set gateway.namespace=gateway-system \
  --set otel.endpoint=otel-collector:4317
```

### Values Reference

See [values.yaml](https://github.com/isitobservable/mcp-otel-proxy/blob/main/deploy/helm/mcp-otel-proxy/values.yaml) for all available options.

## Docker Compose

```yaml
services:
  mcp-server:
    image: your-org/your-mcp-server:latest
    ports:
      - "3000:3000"

  mcp-otel-proxy:
    image: ghcr.io/isitobservable/mcp-otel-proxy:latest
    environment:
      UPSTREAM_URL: http://mcp-server:3000
      OTEL_EXPORTER_OTLP_ENDPOINT: otel-collector:4317
      OTEL_SERVICE_NAME: my-mcp-proxy
      LOG_LEVEL: info
    ports:
      - "8080:8080"
    depends_on:
      - mcp-server

  otel-collector:
    image: otel/opentelemetry-collector-contrib:latest
    ports:
      - "4317:4317"
```

Point your MCP client at `http://localhost:8080`.

## Standalone Binary

Download from [GitHub Releases](https://github.com/isitobservable/mcp-otel-proxy/releases).

```bash
export UPSTREAM_URL=http://localhost:3000
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
./mcp-otel-proxy
```

## Health Checks

The proxy exposes two health endpoints (no telemetry produced):

- `GET /healthz` — Liveness probe (always 200 if process is running)
- `GET /readyz` — Readiness probe (200 if upstream is reachable, 503 otherwise)
