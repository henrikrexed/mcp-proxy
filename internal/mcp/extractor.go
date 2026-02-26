package mcp

import (
	"encoding/json"
	"strconv"

	"github.com/isitobservable/mcp-otel-proxy/internal/jsonrpc"
)

// RequestInfo holds MCP-specific fields extracted from a JSON-RPC request.
type RequestInfo struct {
	Method      string
	ToolName    string
	PromptName  string
	ResourceURI string
	RequestID   string
}

// SpanName returns the OTel span name following MCP semantic conventions.
// Format: "{mcp.method.name}" or "{mcp.method.name} {target}"
func (ri *RequestInfo) SpanName() string {
	target := ri.Target()
	if target != "" {
		return ri.Method + " " + target
	}
	return ri.Method
}

// Target returns the operation target (tool name, prompt name, or resource URI).
func (ri *RequestInfo) Target() string {
	if ri.ToolName != "" {
		return ri.ToolName
	}
	if ri.PromptName != "" {
		return ri.PromptName
	}
	if ri.ResourceURI != "" {
		return ri.ResourceURI
	}
	return ""
}

// ExtractRequestInfo extracts MCP-specific fields from a parsed JSON-RPC request.
func ExtractRequestInfo(req *jsonrpc.Request) *RequestInfo {
	info := &RequestInfo{
		Method:    req.Method,
		RequestID: jsonrpc.IDString(req.ID),
	}

	if len(req.Params) == 0 {
		return info
	}

	var params map[string]json.RawMessage
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return info
	}

	switch req.Method {
	case "tools/call":
		if name, ok := params["name"]; ok {
			var s string
			if json.Unmarshal(name, &s) == nil {
				info.ToolName = s
			}
		}
	case "resources/read", "resources/subscribe", "resources/unsubscribe":
		if uri, ok := params["uri"]; ok {
			var s string
			if json.Unmarshal(uri, &s) == nil {
				info.ResourceURI = s
			}
		}
	case "prompts/get":
		if name, ok := params["name"]; ok {
			var s string
			if json.Unmarshal(name, &s) == nil {
				info.PromptName = s
			}
		}
	}

	return info
}

// ResponseInfo holds MCP-specific fields extracted from a JSON-RPC response.
type ResponseInfo struct {
	ErrorCode    int
	ErrorMessage string
	HasError     bool
	IsToolError  bool
}

// ErrorType returns the error.type attribute value per MCP semantic conventions.
func (ri *ResponseInfo) ErrorType() string {
	if !ri.HasError {
		return ""
	}
	if ri.IsToolError {
		return "tool_error"
	}
	return strconv.Itoa(ri.ErrorCode)
}

// ExtractResponseInfo extracts MCP-specific fields from a parsed JSON-RPC response.
func ExtractResponseInfo(resp *jsonrpc.Response, method string) *ResponseInfo {
	info := &ResponseInfo{}

	if resp.Error != nil {
		info.HasError = true
		info.ErrorCode = resp.Error.Code
		info.ErrorMessage = resp.Error.Message
	}

	// Check for tool_error: tools/call that returns result with isError: true
	if method == "tools/call" && resp.Error == nil && len(resp.Result) > 0 {
		var result map[string]json.RawMessage
		if json.Unmarshal(resp.Result, &result) == nil {
			if isErr, ok := result["isError"]; ok {
				var b bool
				if json.Unmarshal(isErr, &b) == nil && b {
					info.HasError = true
					info.IsToolError = true
				}
			}
		}
	}

	return info
}
