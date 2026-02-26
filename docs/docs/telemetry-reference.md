# Telemetry Reference

This page documents every span, metric, and log field produced by mcp-otel-proxy. All attributes follow the [official OTel MCP semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/mcp/).

## Traces

### Span Overview

Every MCP JSON-RPC request produces one span with kind **SERVER**.

| Span Name | When Created |
|-----------|-------------|
| `initialize` | MCP initialize handshake |
| `tools/call {tool_name}` | Tool call (e.g., `tools/call get_pods`) |
| `tools/list` | List available tools |
| `resources/read {uri}` | Read a resource |
| `resources/list` | List available resources |
| `prompts/get {prompt_name}` | Get a prompt |
| `prompts/list` | List available prompts |
| `ping` | Ping |
| `batch` | Batch JSON-RPC request (parent span) |
| `{method}` | Any other MCP method |

### Span Attributes

#### Required (always set)

| Attribute | Type | Description |
|-----------|------|-------------|
| `mcp.method.name` | string | JSON-RPC method name (e.g., `tools/call`, `initialize`) |

#### Conditionally Required

| Attribute | Type | Condition | Description |
|-----------|------|-----------|-------------|
| `gen_ai.tool.name` | string | Method is `tools/call` | Tool name (e.g., `get_pods`) |
| `gen_ai.prompt.name` | string | Method is `prompts/get` | Prompt name |
| `mcp.resource.uri` | string | Method is `resources/read` | Resource URI |
| `jsonrpc.request.id` | string | Request has an ID | JSON-RPC request ID |
| `error.type` | string | Operation fails | JSON-RPC error code (e.g., `-32602`) or `tool_error` |
| `rpc.response.status_code` | string | Response has error | JSON-RPC error code |

#### Recommended (set when available)

| Attribute | Type | Description |
|-----------|------|-------------|
| `gen_ai.operation.name` | string | `execute_tool` (only for `tools/call`) |
| `mcp.protocol.version` | string | MCP protocol version from initialize (e.g., `2025-06-18`) |
| `mcp.session.id` | string | Session ID from Mcp-Session-Id header |
| `server.address` | string | Upstream server hostname |
| `server.port` | int | Upstream server port |
| `network.transport` | string | `tcp` |
| `network.protocol.name` | string | `http` |

#### Opt-In (CAPTURE_PAYLOAD=true)

| Attribute | Type | Description |
|-----------|------|-------------|
| `gen_ai.tool.call.arguments` | string | Tool call parameters (may contain sensitive data) |
| `gen_ai.tool.call.result` | string | Tool result/output (may contain sensitive data) |

### Span Status

- **OK** — request completed successfully
- **ERROR** — JSON-RPC error in response, with description set to error message

### Context Propagation

The proxy propagates W3C Trace Context via `params._meta`:

1. **Extract**: Reads `traceparent`/`tracestate` from incoming request's `params._meta` (falls back to HTTP headers)
2. **Inject**: Writes updated `traceparent`/`tracestate` into `params._meta` before forwarding to upstream

This enables end-to-end distributed traces: AI Agent → Proxy → MCP Server → Backend.

## Metrics

### gen_ai.server.request.duration

| Field | Value |
|-------|-------|
| Type | Histogram |
| Unit | seconds |
| Bucket Boundaries | 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 300 |
| Description | Total duration of MCP request handling (including upstream time) |

Attributes: `mcp.method.name`, `gen_ai.tool.name`, `error.type`

### gen_ai.server.request.count

| Field | Value |
|-------|-------|
| Type | Counter |
| Unit | {request} |
| Description | Count of MCP requests processed |

Attributes: `mcp.method.name`, `gen_ai.tool.name`, `error.type`

### mcp.proxy.upstream.latency

| Field | Value |
|-------|-------|
| Type | Histogram |
| Unit | seconds |
| Bucket Boundaries | 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 300 |
| Description | Time from forwarding request to receiving response from upstream MCP server |

Attributes: `mcp.method.name`, `gen_ai.tool.name`

### mcp.proxy.message.size

| Field | Value |
|-------|-------|
| Type | Histogram |
| Unit | bytes |
| Description | Request and response payload sizes |

Attributes: `mcp.method.name`, `direction` (`request` or `response`)

### mcp.proxy.errors.total

| Field | Value |
|-------|-------|
| Type | Counter |
| Unit | {error} |
| Description | Total proxy errors by type |

Attributes: `error.type` (values: `parse_error`, `upstream_timeout`, `upstream_error`, `connection_error`)

### mcp.proxy.active_sessions

| Field | Value |
|-------|-------|
| Type | UpDownCounter |
| Unit | {session} |
| Description | Number of currently active MCP sessions |

## Logs

All logs are structured and exported via OTLP gRPC using the `slog`/`otelslog` bridge. Every log record automatically includes `trace_id` and `span_id` for correlation with traces.

### Structured Fields

| Field | Type | Description |
|-------|------|-------------|
| `mcp.method.name` | string | MCP method name |
| `gen_ai.tool.name` | string | Tool name (when applicable) |
| `jsonrpc.request.id` | string | Request ID |
| `upstream.url` | string | Upstream MCP server URL |
| `duration_ms` | int64 | Request duration in milliseconds |
| `error.type` | string | Error classification |
| `error` | string | Error message (for error logs) |

### Log Levels

| Level | What's Logged |
|-------|--------------|
| DEBUG | Every MCP request and response with full structured fields |
| INFO | Proxy startup, configuration |
| WARN | JSON parse failures (fail-open), OTel export issues, large payloads |
| ERROR | Upstream connection failures, timeouts, JSON-RPC errors |

Configure with `LOG_LEVEL` environment variable.
