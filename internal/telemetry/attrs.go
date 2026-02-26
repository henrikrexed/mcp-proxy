package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/isitobservable/mcp-otel-proxy/internal/mcp"
)

// ErrorAttr returns a metric option with error.type attribute.
func ErrorAttr(errType string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("error.type", errType))
}

// MethodAttr returns a metric option with mcp.method.name attribute.
func MethodAttr(method string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("mcp.method.name", method))
}

// DirectionAttr returns a metric option with direction attribute.
func DirectionAttr(direction string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("direction", direction))
}

// MethodToolAttrs returns metric options with mcp.method.name and gen_ai.tool.name.
func MethodToolAttrs(method, toolName string) metric.MeasurementOption {
	attrs := []attribute.KeyValue{
		attribute.String("mcp.method.name", method),
	}
	if toolName != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.name", toolName))
	}
	return metric.WithAttributes(attrs...)
}

// MethodToolErrorAttrs returns metric options with method, tool, and error attributes.
func MethodToolErrorAttrs(method, toolName string, respInfo *mcp.ResponseInfo) metric.MeasurementOption {
	attrs := []attribute.KeyValue{
		attribute.String("mcp.method.name", method),
	}
	if toolName != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.name", toolName))
	}
	if respInfo != nil && respInfo.HasError {
		attrs = append(attrs, attribute.String("error.type", respInfo.ErrorType()))
	}
	return metric.WithAttributes(attrs...)
}
