# Architecture

## Overview

mcp-otel-proxy is a transparent reverse proxy written in Go. It intercepts JSON-RPC 2.0 traffic between MCP clients and servers, producing OpenTelemetry telemetry without modifying the MCP server.

## Request Flow

```
1. MCP Client sends HTTP POST to proxy (port 8080)
2. Proxy reads request body
3. Parses JSON-RPC message (method, params, id)
4. Extracts MCP fields (tool name, resource URI, etc.)
5. Extracts trace context from params._meta or HTTP headers
6. Starts OTel span (kind: SERVER)
7. Records request metrics and logs
8. Injects updated trace context into params._meta
9. Forwards modified request to upstream MCP server
10. Streams response back to client
11. Parses response for error info and telemetry
12. Records response metrics and logs
13. Ends span with status
```

## Components

| Component | Path | Responsibility |
|-----------|------|---------------|
| **main** | `cmd/mcp-otel-proxy/` | Entry point, config, OTel init, HTTP server |
| **config** | `internal/config/` | Environment variable loading |
| **proxy** | `internal/proxy/` | HTTP handler, upstream forwarding |
| **jsonrpc** | `internal/jsonrpc/` | JSON-RPC 2.0 message parsing |
| **mcp** | `internal/mcp/` | MCP field extraction, session tracking |
| **telemetry** | `internal/telemetry/` | OTel SDK init, spans, metrics, context propagation |
| **health** | `internal/health/` | Health check endpoints |

## Design Decisions

1. **Reverse proxy with selective mutation** — forward everything transparently, only modify params._meta for context propagation
2. **Fail-open** — malformed JSON is forwarded unchanged, parse errors don't block traffic
3. **Session tracking** — initialize handshake parsed to carry protocol version and session ID
4. **All 3 OTel signals** — traces, metrics, and logs from a single proxy
5. **slog + otelslog bridge** — structured logging with automatic trace correlation

## Semantic Conventions

All telemetry strictly follows the [OTel MCP semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/mcp/). No custom attribute names are used when a semconv equivalent exists.
