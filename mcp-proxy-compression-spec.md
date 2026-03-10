# Feature Spec: JSON→Markdown Response Compression

## Overview

Add an opt-in response compression feature to mcp-otel-proxy that converts JSON responses from upstream MCP servers into markdown table format before forwarding to the agent. This reduces token consumption and improves LLM readability.

## Motivation

MCP servers often return JSON responses (especially K8s-related tools). JSON is verbose and token-expensive for LLMs. Markdown tables convey the same information in fewer tokens and are easier for LLMs to parse.

The proxy is the ideal layer for this — it catches ALL upstream MCP servers without requiring changes to each one individually.

## Design Decisions

- **Opt-in per upstream**: Configured per upstream server in the proxy config. Default: off.
- **Flat conversion only**: Convert top-level JSON arrays of objects into markdown tables. Nested objects are stringified as `{...}` — no deep flattening, no data loss.
- **Always convert when enabled**: No size threshold. JSON always costs more tokens than equivalent markdown tables regardless of response size.
- **No field stripping**: The proxy does NOT strip or filter any fields (e.g., no K8s-specific `managedFields` removal). That's the MCP server's responsibility. The proxy is a format converter, not a summarizer.

## Configuration

Add `compressResponses` boolean to upstream server config:

```yaml
upstreams:
  - name: k8s-networking
    address: "localhost:9090"
    compressResponses: true    # Enable JSON→markdown conversion
  - name: some-other-mcp
    address: "localhost:9091"
    # compressResponses defaults to false
```

## Conversion Rules

### Input Detection

1. Check if MCP tool response `content` contains a text block with valid JSON
2. Only convert if the parsed JSON is one of:
   - A JSON array of objects (`[]map[string]any`)
   - A JSON object with a top-level key whose value is an array of objects (e.g., `{"items": [...]}`, `{"collectors": [...]}`)

If the JSON doesn't match these patterns, pass through unchanged.

### Conversion Logic

For a JSON array of objects:

**Input:**
```json
[
  {"name": "svc-a", "namespace": "default", "type": "ClusterIP", "ports": [8080, 443]},
  {"name": "svc-b", "namespace": "kube-system", "type": "NodePort", "ports": [53]}
]
```

**Output:**
```markdown
| name | namespace | type | ports |
|------|-----------|------|-------|
| svc-a | default | ClusterIP | [8080,443] |
| svc-b | kube-system | NodePort | [53] |
```

### Value Rendering Rules

| JSON Type | Markdown Rendering |
|-----------|-------------------|
| string | as-is |
| number | as-is |
| boolean | `true` / `false` |
| null | _(empty)_ |
| array (flat, ≤5 items) | comma-separated: `a, b, c` |
| array (flat, >5 items) | first 5 + `... (+N more)` |
| array of objects | `[{...} x N]` |
| nested object | `{...}` |

### Column Ordering

Use the key order from the first object in the array. All unique keys across all objects become columns (missing keys → empty cell).

### Wrapper Object Handling

If the JSON is an object like:
```json
{"collectors": [...], "count": 3}
```

Extract the array value, convert to table, and prepend scalar fields as a header:

```markdown
count: 3

| name | namespace | ... |
|------|-----------|-----|
| ... | ... | ... |
```

## Implementation Location

- New package: `internal/compress/` (or `internal/format/`)
- Main function: `CompressJSONToMarkdown(content string) (string, bool)`
  - Returns converted string and `true` if conversion happened
  - Returns original string and `false` if not convertible
- Integration point: in the proxy response handler, after receiving upstream response, before forwarding to client
- Only called when `compressResponses: true` for the upstream

## OTel Telemetry

Add a span event or attribute when compression is applied:
- Span attribute: `mcp.response.compressed = true`
- Span attribute: `mcp.response.original_bytes = N`
- Span attribute: `mcp.response.compressed_bytes = N`
- Metric: `mcp_proxy.response.compression_ratio` (histogram)

## Testing

1. **Unit tests** for `CompressJSONToMarkdown`:
   - Flat array of objects → table
   - Wrapper object with array → header + table
   - Nested objects → `{...}` rendering
   - Non-convertible JSON (scalar, string) → passthrough
   - Invalid JSON → passthrough
   - Empty array → empty table with headers
   - Mixed keys across objects → union of all keys
   - Large arrays (100+ items) → verify performance

2. **Integration tests**:
   - Proxy with `compressResponses: true` → verify markdown output
   - Proxy with `compressResponses: false` → verify JSON passthrough
   - Verify OTel attributes are set correctly

## Files to Modify

- `internal/config/config.go` — add `CompressResponses` field to upstream config struct
- `internal/proxy/handler.go` (or equivalent) — call compression in response path
- `internal/compress/markdown.go` — new file, conversion logic
- `internal/compress/markdown_test.go` — new file, unit tests
- `deploy/helm/values.yaml` — add config option
- `docs/` — document the feature
