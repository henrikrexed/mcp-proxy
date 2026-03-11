# mcp-otel-proxy

A universal sidecar proxy that adds OpenTelemetry observability to any MCP server.

[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

## Overview

`mcp-otel-proxy` sits between your AI agent and any MCP server, transparently adding OpenTelemetry traces, metrics, and logs to every MCP call. Drop it in as a sidecar — zero code changes required.

**Key features:**
- 🔌 **Universal**: Works with any MCP server using streamable-http transport
- 📊 **Full telemetry**: Traces, metrics, and logs following GenAI + MCP semantic conventions
- 🪶 **Lightweight**: ~32MB memory, minimal CPU overhead
- 🔧 **Zero config**: Point at upstream, set OTLP endpoint, done
- 🏥 **Health checks**: Built-in `/healthz` endpoint
- 📦 **Payload capture**: Optional request/response body capture for debugging

## Quick Start

### Docker

```bash
docker run -p 8080:8080 \
  -e UPSTREAM_URL=http://localhost:8081 \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317 \
  ghcr.io/henrikrexed/mcp-otel-proxy:0.0.1
```

### Kubernetes Sidecar

See `deploy/kubernetes/example-sidecar.yaml` for a complete example of adding mcp-otel-proxy as a sidecar to any MCP server deployment.

```bash
kubectl apply -f deploy/kubernetes/example-sidecar.yaml
```

### Deploy with Sympozium

[Sympozium](https://sympozium.ai) is a Kubernetes AI agent orchestrator that uses SkillPack CRDs to inject MCP servers as sidecars into agent pods.

The `deploy/kubernetes/example-sidecar.yaml` includes a Sympozium SkillPack example that shows how to use mcp-otel-proxy as a second sidecar wrapping another MCP server — adding OTel tracing transparently.

```bash
# Extract the SkillPack from the example file, or apply directly:
kubectl apply -f deploy/kubernetes/example-sidecar.yaml
```

The SkillPack defines:
- The proxy as a sidecar with OTLP export configuration
- Shared `/workspace` volume for agent communication
- Instructions for the agent on using the traced MCP endpoint

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `UPSTREAM_URL` | URL of the upstream MCP server | required |
| `PROXY_PORT` | Port for the proxy to listen on | `8080` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint | required |
| `OTEL_EXPORTER_OTLP_INSECURE` | Use insecure connection | `false` |
| `OTEL_SERVICE_NAME` | Service name for telemetry | `mcp-otel-proxy` |
| `LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `CAPTURE_PAYLOAD` | Capture request/response bodies | `false` |
| `COMPRESS_RESPONSES` | Convert JSON tool responses to markdown tables (reduces LLM token usage) | `false` |
| `CONTEXT_PROPAGATION` | Enable trace context propagation via params._meta | `true` |
| `SESSION_TTL` | Session eviction TTL in seconds | `3600` |

## Documentation

📖 Full documentation: [https://henrikrexed.github.io/mcp-otel-proxy](https://henrikrexed.github.io/mcp-otel-proxy)

## Part of IsItObservable

This project is part of the [IsItObservable](https://youtube.com/@IsItObservable) ecosystem — open-source tools for Kubernetes observability.

- [mcp-k8s-networking](https://github.com/henrikrexed/mcp-k8s-networking) — Kubernetes networking diagnostics
- [otel-collector-mcp](https://github.com/henrikrexed/otel-collector-mcp) — OTel Collector pipeline debugging

## License

Apache License 2.0
