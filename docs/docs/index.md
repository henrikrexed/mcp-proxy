# mcp-otel-proxy

A universal sidecar proxy that adds **OpenTelemetry observability** (traces, metrics, logs) to **any MCP server** — without modifying it.

## The Problem

MCP (Model Context Protocol) servers are black boxes. You deploy them, point your AI agent at them, and hope for the best. There are no traces, no metrics, no structured logs. When something fails, you're flying blind.

## The Solution

**mcp-otel-proxy** is a transparent reverse proxy that sits between your MCP client (AI agent) and MCP server. It:

- Intercepts JSON-RPC 2.0 traffic
- Produces **traces**, **metrics**, and **logs** following the [official OTel MCP semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/mcp/)
- Propagates distributed trace context via `params._meta`
- Exports all signals via **OTLP gRPC**
- Works with **any** MCP server (Go, Python, TypeScript, Rust — anything)

## Architecture

```
AI Agent (MCP Client)
        │
        ▼
┌─────────────────┐
│  mcp-otel-proxy │  ← Sidecar container
│  (Go reverse    │  ← Parses JSON-RPC
│   proxy)        │  ← Emits OTel signals
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   MCP Server    │  ← Any language, unmodified
│  (upstream)     │
└─────────────────┘
         │
         ▼
   OTLP gRPC → OTel Collector → Backends
```

## Quick Start

```bash
docker run -d \
  -e UPSTREAM_URL=http://localhost:3000 \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317 \
  -p 8080:8080 \
  ghcr.io/henrikrexed/mcp-otel-proxy:latest
```

Or with Helm:

```bash
helm install my-mcp deploy/helm/mcp-otel-proxy \
  --set mcpServer.image=your-mcp-server:latest \
  --set otel.endpoint=otel-collector:4317
```

## Key Features

- **Zero-code observability** — no SDK changes, no code changes
- **All 3 OTel signals** — traces, metrics, and logs with trace correlation
- **MCP semantic conventions** — follows the official OTel GenAI + MCP spec
- **Context propagation** — injects traceparent into params._meta for end-to-end traces
- **Session tracking** — understands MCP initialize handshake
- **Kubernetes-native** — Helm chart with sidecar pattern and Gateway API support

## Part of henrikrexed

This project is part of the [henrikrexed](https://github.com/henrikrexed) ecosystem by Henrik Rexed, alongside:

- [k8s-networking-mcp](https://github.com/henrikrexed/k8s-networking-mcp)
- [otel-collector-mcp](https://github.com/henrikrexed/otel-collector-mcp)
