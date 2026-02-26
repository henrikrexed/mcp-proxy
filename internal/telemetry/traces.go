package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/isitobservable/mcp-otel-proxy/internal/mcp"
)

const tracerName = "mcp-otel-proxy"

// StartMCPSpan creates a new OTel span for an MCP request following semantic conventions.
func StartMCPSpan(ctx context.Context, reqInfo *mcp.RequestInfo, session *mcp.Session, serverAddr string, serverPort int) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)

	attrs := []attribute.KeyValue{
		attribute.String("mcp.method.name", reqInfo.Method),
		attribute.String("network.transport", "tcp"),
		attribute.String("network.protocol.name", "http"),
		attribute.String("server.address", serverAddr),
		attribute.Int("server.port", serverPort),
	}

	if reqInfo.ToolName != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.name", reqInfo.ToolName))
	}
	if reqInfo.PromptName != "" {
		attrs = append(attrs, attribute.String("gen_ai.prompt.name", reqInfo.PromptName))
	}
	if reqInfo.ResourceURI != "" {
		attrs = append(attrs, attribute.String("mcp.resource.uri", reqInfo.ResourceURI))
	}
	if reqInfo.RequestID != "" {
		attrs = append(attrs, attribute.String("jsonrpc.request.id", reqInfo.RequestID))
	}

	// Set gen_ai.operation.name for tool calls
	if reqInfo.Method == "tools/call" {
		attrs = append(attrs, attribute.String("gen_ai.operation.name", "execute_tool"))
	}

	// Session attributes
	if session != nil {
		if session.ProtocolVersion != "" {
			attrs = append(attrs, attribute.String("mcp.protocol.version", session.ProtocolVersion))
		}
		if session.ID != "" {
			attrs = append(attrs, attribute.String("mcp.session.id", session.ID))
		}
	}

	ctx, span := tracer.Start(ctx, reqInfo.SpanName(),
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
	)

	return ctx, span
}

// EndMCPSpan completes the span with response information.
func EndMCPSpan(span trace.Span, respInfo *mcp.ResponseInfo) {
	if respInfo == nil {
		span.End()
		return
	}

	if respInfo.HasError {
		errType := respInfo.ErrorType()
		span.SetAttributes(attribute.String("error.type", errType))

		if respInfo.ErrorCode != 0 {
			span.SetAttributes(attribute.String("rpc.response.status_code", respInfo.ErrorType()))
		}

		desc := respInfo.ErrorMessage
		if respInfo.IsToolError {
			desc = "tool execution failed"
		}
		span.SetStatus(codes.Error, desc)
	}

	span.End()
}

// SetPayloadAttributes sets opt-in payload attributes on a span.
func SetPayloadAttributes(span trace.Span, arguments, result string) {
	if arguments != "" {
		span.SetAttributes(attribute.String("gen_ai.tool.call.arguments", arguments))
	}
	if result != "" {
		span.SetAttributes(attribute.String("gen_ai.tool.call.result", result))
	}
}
