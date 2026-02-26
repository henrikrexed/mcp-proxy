package telemetry

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

// metaCarrier implements propagation.TextMapCarrier for params._meta.
type metaCarrier struct {
	meta map[string]interface{}
}

func (c *metaCarrier) Get(key string) string {
	if v, ok := c.meta[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c *metaCarrier) Set(key, value string) {
	c.meta[key] = value
}

func (c *metaCarrier) Keys() []string {
	keys := make([]string, 0, len(c.meta))
	for k := range c.meta {
		keys = append(keys, k)
	}
	return keys
}

// ExtractContextFromMeta extracts W3C trace context from params._meta.
// Falls back to HTTP headers if no context in _meta.
func ExtractContextFromMeta(ctx context.Context, params json.RawMessage, headers propagation.TextMapCarrier) context.Context {
	if len(params) > 0 {
		var paramsMap map[string]json.RawMessage
		if json.Unmarshal(params, &paramsMap) == nil {
			if metaRaw, ok := paramsMap["_meta"]; ok {
				var meta map[string]interface{}
				if json.Unmarshal(metaRaw, &meta) == nil {
					if _, hasTP := meta["traceparent"]; hasTP {
						carrier := &metaCarrier{meta: meta}
						return otel.GetTextMapPropagator().Extract(ctx, carrier)
					}
				}
			}
		}
	}

	// Fallback to HTTP headers
	return otel.GetTextMapPropagator().Extract(ctx, headers)
}

// InjectContextIntoBody injects the current span's trace context into params._meta
// and returns the modified JSON body.
func InjectContextIntoBody(ctx context.Context, body []byte) ([]byte, error) {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return body, nil
	}

	var msg map[string]json.RawMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return body, nil // fail-open: return original body
	}

	// Parse or create params
	var params map[string]json.RawMessage
	if paramsRaw, ok := msg["params"]; ok && len(paramsRaw) > 0 {
		if err := json.Unmarshal(paramsRaw, &params); err != nil {
			return body, nil // fail-open
		}
	} else {
		params = make(map[string]json.RawMessage)
	}

	// Parse or create _meta
	var meta map[string]interface{}
	if metaRaw, ok := params["_meta"]; ok && len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			meta = make(map[string]interface{})
		}
	} else {
		meta = make(map[string]interface{})
	}

	// Inject trace context
	carrier := &metaCarrier{meta: meta}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	// Re-serialize _meta -> params -> message
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return body, nil
	}
	params["_meta"] = metaBytes

	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return body, nil
	}
	msg["params"] = paramsBytes

	return json.Marshal(msg)
}

// InjectContextIntoBatchBody injects context into each request in a batch.
func InjectContextIntoBatchBody(ctx context.Context, body []byte) ([]byte, error) {
	var msgs []json.RawMessage
	if err := json.Unmarshal(body, &msgs); err != nil {
		return body, nil
	}

	for i, msg := range msgs {
		modified, err := InjectContextIntoBody(ctx, msg)
		if err == nil {
			msgs[i] = modified
		}
	}

	return json.Marshal(msgs)
}
