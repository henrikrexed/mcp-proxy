package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
)

var ErrEmptyBody = errors.New("empty body")

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// IsNotification returns true if the request has no ID (JSON-RPC notification).
func (r *Request) IsNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// Error represents a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ParseResult holds the result of parsing a JSON-RPC message body.
type ParseResult struct {
	IsBatch   bool
	Requests  []Request
	Responses []Response
}

// ParseRequest parses a JSON-RPC request body. Detects batch (array) vs single request.
func ParseRequest(body []byte) (*ParseResult, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	result := &ParseResult{}

	if body[0] == '[' {
		result.IsBatch = true
		if err := json.Unmarshal(body, &result.Requests); err != nil {
			return nil, err
		}
		return result, nil
	}

	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	result.Requests = []Request{req}
	return result, nil
}

// ParseResponse parses a JSON-RPC response body. Detects batch (array) vs single response.
func ParseResponse(body []byte) (*ParseResult, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, ErrEmptyBody
	}

	result := &ParseResult{}

	if body[0] == '[' {
		result.IsBatch = true
		if err := json.Unmarshal(body, &result.Responses); err != nil {
			return nil, err
		}
		return result, nil
	}

	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	result.Responses = []Response{resp}
	return result, nil
}

// IDString returns the JSON-RPC ID as a string for use in span attributes.
func IDString(id json.RawMessage) string {
	if len(id) == 0 || string(id) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(id, &s); err == nil {
		return s
	}
	return string(id)
}
