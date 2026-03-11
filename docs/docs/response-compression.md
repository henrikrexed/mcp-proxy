# Response Compression

mcp-otel-proxy can automatically compress JSON responses from upstream MCP servers into markdown tables, significantly reducing token usage when LLMs consume the output.

## Why Compress?

MCP tool responses are often verbose JSON arrays — lists of services, pods, routes, collectors. A single `tools/call` response can be 2-5KB of JSON, consuming 500-1500 LLM tokens. The same data in a markdown table is 40-60% smaller.

**Example: `list_services` response**

=== "JSON (original)"

    ```json
    [
      {"name": "frontend", "namespace": "otel-demo", "type": "ClusterIP", "clusterIP": "10.96.0.1", "ports": "8080/TCP"},
      {"name": "cartservice", "namespace": "otel-demo", "type": "ClusterIP", "clusterIP": "10.96.0.2", "ports": "7070/TCP"}
    ]
    ```

=== "Markdown Table (compressed)"

    ```markdown
    | name | namespace | type | clusterIP | ports |
    |---|---|---|---|---|
    | frontend | otel-demo | ClusterIP | 10.96.0.1 | 8080/TCP |
    | cartservice | otel-demo | ClusterIP | 10.96.0.2 | 7070/TCP |
    ```

## How It Works

1. The proxy intercepts `tools/call` responses from the upstream MCP server
2. If the response content is a JSON array of flat objects, it converts to a markdown table
3. Nested objects/arrays within rows are collapsed to `{...}` / `[...]`
4. The compressed response is forwarded to the client

The conversion is **mechanical** — no semantic understanding, no data loss. Every field is preserved.

## Enabling Compression

Set the `COMPRESS_RESPONSES` environment variable:

```bash
COMPRESS_RESPONSES=true UPSTREAM_URL=http://localhost:3000 ./mcp-otel-proxy
```

Or in Helm:

```yaml
proxy:
  compressResponses: "true"
```

## What Gets Compressed

| Content Type | Compressed? | Notes |
|---|---|---|
| JSON array of flat objects | ✅ Yes | Converted to markdown table |
| JSON array with nested objects | ✅ Yes | Nested values shown as `{...}` |
| Single JSON object | ❌ No | Passed through unchanged |
| Plain text / markdown | ❌ No | Already compact |
| Empty or null content | ❌ No | Passed through unchanged |

## SSE / StreamableHTTP Compatibility

Compression works with all transport modes:

- **HTTP POST** (`/mcp`): Response JSON is compressed before sending
- **SSE streaming**: Each SSE event is buffered, the `result.content` is compressed, then the event is re-serialized and forwarded
- **stdio** (via supergateway): Same as HTTP POST — supergateway wraps stdio as StreamableHTTP, proxy compresses on the HTTP layer

!!! note
    Compression operates on complete `tools/call` responses, not partial streams. MCP tool responses are always a single complete message (not chunked), so this is transparent to clients.

## Telemetry

When compression is enabled, the proxy emits additional telemetry:

### Span Attributes

| Attribute | Type | Description |
|---|---|---|
| `mcp.response.original_size` | int | Original response size in bytes |
| `mcp.response.compressed_size` | int | Compressed response size in bytes |
| `mcp.response.compression_ratio` | float | Ratio (compressed/original), lower is better |
| `mcp.response.compression_applied` | bool | Whether compression was applied |

### Metrics

| Metric | Type | Description |
|---|---|---|
| `mcp.proxy.compression.ratio` | histogram | Distribution of compression ratios |
| `mcp.proxy.compression.bytes_saved` | counter | Total bytes saved by compression |

## Token Impact

Measured with real MCP servers:

| MCP Server | Tool | JSON tokens | Markdown tokens | Savings |
|---|---|---|---|---|
| k8s-networking | `list_services` (19 svcs) | ~700 | ~400 | 43% |
| k8s-networking | `list_endpoints` (19 eps) | ~500 | ~300 | 40% |
| otel-collector | `list_collectors` | ~400 | ~250 | 38% |
| Dynatrace | `list_problems` | ~1200 | ~700 | 42% |

!!! tip
    Compression has the biggest impact on list/scan tools that return arrays. Detail tools (`get_service`, `get_gateway`) that return single objects are passed through unchanged.

### Spans

When compression is enabled, a dedicated child span is created under the `tools/call` span:

| Span Name | Parent | Description |
|---|---|---|
| `mcp.response.compress` | `tools/call` | Compression operation with duration, original/compressed sizes |

This lets you see compression latency separately in your trace waterfall (e.g., in Dynatrace or Jaeger).
