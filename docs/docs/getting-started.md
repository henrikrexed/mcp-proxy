# Getting Started

## Prerequisites

- An MCP server accessible via HTTP (Streamable HTTP transport)
- An OpenTelemetry Collector (or compatible OTLP endpoint)
- Docker or Kubernetes for deployment

## Option 1: Docker

```bash
docker run -d \
  --name mcp-otel-proxy \
  -e UPSTREAM_URL=http://your-mcp-server:3000 \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=your-otel-collector:4317 \
  -e OTEL_SERVICE_NAME=my-mcp-proxy \
  -e LOG_LEVEL=debug \
  -p 8080:8080 \
  ghcr.io/henrikrexed/mcp-otel-proxy:latest
```

Point your MCP client at `http://localhost:8080` instead of the MCP server directly.

## Option 2: Kubernetes (Helm)

```bash
helm install my-mcp deploy/helm/mcp-otel-proxy \
  --set mcpServer.image=your-org/your-mcp-server:latest \
  --set mcpServer.port=3000 \
  --set otel.endpoint=otel-collector.observability.svc.cluster.local:4317 \
  --set otel.serviceName=my-mcp-proxy
```

## Option 3: Binary

Download from [GitHub Releases](https://github.com/henrikrexed/mcp-otel-proxy/releases):

```bash
export UPSTREAM_URL=http://localhost:3000
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
./mcp-otel-proxy
```

## Verify It Works

1. Send a request through the proxy to your MCP server
2. Check your tracing backend (Jaeger, Grafana Tempo, etc.)
3. You should see spans with names like `tools/call get_pods` or `initialize`

## Health Checks

```bash
curl http://localhost:8080/healthz    # Liveness
curl http://localhost:8080/readyz     # Readiness (checks upstream)
```
