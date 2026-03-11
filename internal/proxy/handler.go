package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/isitobservable/mcp-otel-proxy/internal/compress"
	"github.com/isitobservable/mcp-otel-proxy/internal/config"
	"github.com/isitobservable/mcp-otel-proxy/internal/jsonrpc"
	"github.com/isitobservable/mcp-otel-proxy/internal/mcp"
	"github.com/isitobservable/mcp-otel-proxy/internal/telemetry"
)

// Handler is the MCP proxy HTTP handler.
type Handler struct {
	upstream   *url.URL
	client     *http.Client
	config     *config.Config
	metrics    *telemetry.Metrics
	sessions   *mcp.SessionStore
	logger     *slog.Logger
	serverAddr string
	serverPort int
	clientSessions sync.Map
}

// New creates a new proxy handler.
func New(cfg *config.Config, metrics *telemetry.Metrics, sessions *mcp.SessionStore, logger *slog.Logger) (*Handler, error) {
	u, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		return nil, err
	}

	port := 80
	if u.Port() != "" {
		port, _ = strconv.Atoi(u.Port())
	}
	if u.Scheme == "https" && u.Port() == "" {
		port = 443
	}

	return &Handler{
		upstream:   u,
		client:     &http.Client{Timeout: 5 * time.Minute},
		config:     cfg,
		metrics:    metrics,
		sessions:   sessions,
		logger:     logger,
		serverAddr: u.Hostname(),
		serverPort: port,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Log with session ID
	h.logger.Info("incoming request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr, "accept", r.Header.Get("Accept"), "content-type", r.Header.Get("Content-Type"), "mcp-session-id", r.Header.Get("Mcp-Session-Id"))

	// Inject cached session ID if client omits it
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	if r.Header.Get("Mcp-Session-Id") == "" {
		if cachedID, ok := h.clientSessions.Load(clientIP); ok {
			r.Header.Set("Mcp-Session-Id", cachedID.(string))
			h.logger.Info("injected cached session ID", "client", clientIP, "session-id", cachedID)
		}
	}
	start := time.Now()

	// Read request body
	reqBody, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to read request body",
			"error", err,
			"upstream.url", h.config.UpstreamURL,
		)
		http.Error(w, "failed to read request body", http.StatusBadGateway)
		h.metrics.ErrorsTotal.Add(r.Context(), 1, telemetry.ErrorAttr("connection_error"))
		return
	}

	// Parse JSON-RPC request
	parsed, parseErr := jsonrpc.ParseRequest(reqBody)
	if parseErr != nil {
		h.logger.WarnContext(r.Context(), "failed to parse JSON-RPC request, forwarding raw",
			"error", parseErr,
			"upstream.url", h.config.UpstreamURL,
		)
		h.metrics.ErrorsTotal.Add(r.Context(), 1, telemetry.ErrorAttr("parse_error"))
		// Fail-open: forward raw request
		h.forwardRaw(w, r, reqBody)
		return
	}

	if parsed.IsBatch {
		h.handleBatch(w, r, reqBody, parsed, start)
	} else {
		h.handleSingle(w, r, reqBody, &parsed.Requests[0], start)
	}
}

func (h *Handler) handleSingle(w http.ResponseWriter, r *http.Request, reqBody []byte, req *jsonrpc.Request, start time.Time) {
	ctx := r.Context()
	reqInfo := mcp.ExtractRequestInfo(req)

	// Get session for this request
	sessionID := r.Header.Get("Mcp-Session-Id")
	var session *mcp.Session
	if sessionID != "" {
		session = h.sessions.Get(sessionID)
	}

	// Extract context from params._meta or HTTP headers
	ctx = telemetry.ExtractContextFromMeta(ctx, req.Params, propagation.HeaderCarrier(r.Header))

	// Start span
	ctx, span := telemetry.StartMCPSpan(ctx, reqInfo, session, h.serverAddr, h.serverPort)
	defer span.End()

	// Record request metrics
	h.metrics.RequestCount.Add(ctx, 1, telemetry.MethodToolAttrs(reqInfo.Method, reqInfo.ToolName))
	h.metrics.MessageSize.Record(ctx, int64(len(reqBody)), telemetry.DirectionAttr("request"), telemetry.MethodAttr(reqInfo.Method))

	// Log request
	h.logger.DebugContext(ctx, "MCP request",
		"mcp.method.name", reqInfo.Method,
		"gen_ai.tool.name", reqInfo.ToolName,
		"jsonrpc.request.id", reqInfo.RequestID,
		"upstream.url", h.config.UpstreamURL,
	)

	// Inject context propagation into params._meta
	bodyToSend := reqBody
	if h.config.ContextPropagation {
		modified, err := telemetry.InjectContextIntoBody(ctx, reqBody)
		if err == nil {
			bodyToSend = modified
		}
	}

	// Forward to upstream — use streaming for SSE-capable clients
	upstreamStart := time.Now()
	acceptsSSE := strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	var respBody []byte
	var respHeaders http.Header
	var statusCode int

	if acceptsSSE {
		// StreamableHTTP: stream SSE response directly to client while buffering for telemetry
		var sseErr error
		respBody, respHeaders, _, sseErr = h.doUpstreamStreamingRequest(ctx, w, r, bodyToSend)
		upstreamDuration := time.Since(upstreamStart)
		h.metrics.UpstreamLatency.Record(ctx, upstreamDuration.Seconds(), telemetry.MethodToolAttrs(reqInfo.Method, reqInfo.ToolName))
		if sseErr != nil {
			h.logger.ErrorContext(ctx, "upstream SSE stream error",
				"error", sseErr,
				"mcp.method.name", reqInfo.Method,
			)
		}
		// Parse SSE data lines for telemetry
		sseData := extractSSEData(respBody)
		var respInfo *mcp.ResponseInfo
		if sseData != nil {
			respParsed, parseErr := jsonrpc.ParseResponse(sseData)
			if parseErr == nil && len(respParsed.Responses) > 0 {
				respInfo = mcp.ExtractResponseInfo(&respParsed.Responses[0], reqInfo.Method)
				if reqInfo.Method == "initialize" && !respInfo.HasError {
					respSessionID := sessionID
					if respSessionID == "" {
						respSessionID = respHeaders.Get("Mcp-Session-Id")
					}
					h.sessions.TrackInitialize(&respParsed.Responses[0], respSessionID)
					// Cache session ID for clients that do not forward it
					if respSessionID != "" {
						clientIP := strings.Split(r.RemoteAddr, ":")[0]
						h.clientSessions.Store(clientIP, respSessionID)
						h.logger.Info("cached session ID for client", "client", clientIP, "session-id", respSessionID)
					}
				}
				if h.config.CapturePayload && reqInfo.Method == "tools/call" {
					telemetry.SetPayloadAttributes(span, string(req.Params), string(respParsed.Responses[0].Result))
				}
			}
		}
		h.metrics.MessageSize.Record(ctx, int64(len(respBody)), telemetry.DirectionAttr("response"), telemetry.MethodAttr(reqInfo.Method))
		duration := time.Since(start)
		h.metrics.RequestDuration.Record(ctx, duration.Seconds(),
			telemetry.MethodToolErrorAttrs(reqInfo.Method, reqInfo.ToolName, respInfo))
		telemetry.EndMCPSpan(span, respInfo)
		return
	}

	var upErr error
	respBody, respHeaders, statusCode, upErr = h.doUpstreamRequest(ctx, r, bodyToSend)
	upstreamDuration := time.Since(upstreamStart)

	if upErr != nil {
		h.logger.ErrorContext(ctx, "upstream request failed",
			"error", upErr,
			"mcp.method.name", reqInfo.Method,
			"upstream.url", h.config.UpstreamURL,
		)
		h.metrics.ErrorsTotal.Add(ctx, 1, telemetry.ErrorAttr("upstream_error"))
		span.SetAttributes(attribute.String("error.type", "upstream_error"))
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}

	// Record upstream latency
	h.metrics.UpstreamLatency.Record(ctx, upstreamDuration.Seconds(), telemetry.MethodToolAttrs(reqInfo.Method, reqInfo.ToolName))

	// Parse response for telemetry
	var respInfo *mcp.ResponseInfo
	respParsed, err := jsonrpc.ParseResponse(respBody)
	if err == nil && len(respParsed.Responses) > 0 {
		respInfo = mcp.ExtractResponseInfo(&respParsed.Responses[0], reqInfo.Method)

		// Track initialize handshake
		if reqInfo.Method == "initialize" && !respInfo.HasError {
			respSessionID := sessionID
			if respSessionID == "" {
				respSessionID = respHeaders.Get("Mcp-Session-Id")
			}
			h.sessions.TrackInitialize(&respParsed.Responses[0], respSessionID)
			// Cache session ID for clients that do not forward it
			if respSessionID != "" {
				clientIP := strings.Split(r.RemoteAddr, ":")[0]
				h.clientSessions.Store(clientIP, respSessionID)
				h.logger.Info("cached session ID for client", "client", clientIP, "session-id", respSessionID)
			}
		}

		// Opt-in payload capture
		if h.config.CapturePayload && reqInfo.Method == "tools/call" {
			telemetry.SetPayloadAttributes(span, string(req.Params), string(respParsed.Responses[0].Result))
		}
	}

	// Apply JSON→Markdown compression if enabled for tools/call responses
	if h.config.CompressResponses && reqInfo.Method == "tools/call" && respInfo != nil && !respInfo.HasError {
		respBody = h.compressResponse(ctx, span, respBody, respParsed)
	}

	// Record response metrics
	totalDuration := time.Since(start)
	h.metrics.RequestDuration.Record(ctx, totalDuration.Seconds(),
		telemetry.MethodToolErrorAttrs(reqInfo.Method, reqInfo.ToolName, respInfo))
	h.metrics.MessageSize.Record(ctx, int64(len(respBody)), telemetry.DirectionAttr("response"), telemetry.MethodAttr(reqInfo.Method))

	// End span with response info
	telemetry.EndMCPSpan(span, respInfo)

	// Log response
	errType := ""
	if respInfo != nil {
		errType = respInfo.ErrorType()
	}
	logLevel := slog.LevelDebug
	if respInfo != nil && respInfo.HasError {
		logLevel = slog.LevelError
	}
	h.logger.Log(ctx, logLevel, "MCP response",
		"mcp.method.name", reqInfo.Method,
		"gen_ai.tool.name", reqInfo.ToolName,
		"duration_ms", totalDuration.Milliseconds(),
		"upstream.url", h.config.UpstreamURL,
		"error.type", errType,
	)

	// Write response to client
	copyHeaders(w.Header(), respHeaders)
	w.WriteHeader(statusCode)
	if _, err := w.Write(respBody); err != nil {
		h.logger.ErrorContext(ctx, "failed to write response to client", "error", err)
	}
}

func (h *Handler) handleBatch(w http.ResponseWriter, r *http.Request, reqBody []byte, parsed *jsonrpc.ParseResult, start time.Time) {
	// For batch: inject context into each request, forward as batch, parse batch response
	ctx := r.Context()

	// Extract context from HTTP headers for batch
	ctx = telemetry.ExtractContextFromMeta(ctx, nil, propagation.HeaderCarrier(r.Header))

	// Create parent batch span
	batchInfo := &mcp.RequestInfo{Method: "batch"}
	ctx, batchSpan := telemetry.StartMCPSpan(ctx, batchInfo, nil, h.serverAddr, h.serverPort)
	batchSpan.SetAttributes(attribute.Int("jsonrpc.batch.size", len(parsed.Requests)))
	defer batchSpan.End()

	// Inject context into batch
	bodyToSend := reqBody
	if h.config.ContextPropagation {
		modified, err := telemetry.InjectContextIntoBatchBody(ctx, reqBody)
		if err == nil {
			bodyToSend = modified
		}
	}

	// Forward to upstream
	upstreamStart := time.Now()
	respBody, respHeaders, statusCode, err := h.doUpstreamRequest(ctx, r, bodyToSend)
	upstreamDuration := time.Since(upstreamStart)

	if err != nil {
		h.logger.ErrorContext(ctx, "upstream batch request failed",
			"error", err,
			"upstream.url", h.config.UpstreamURL,
		)
		h.metrics.ErrorsTotal.Add(ctx, 1, telemetry.ErrorAttr("upstream_error"))
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}

	h.metrics.UpstreamLatency.Record(ctx, upstreamDuration.Seconds(), telemetry.MethodAttr("batch"))

	// Record metrics for each request in the batch
	for i := range parsed.Requests {
		reqInfo := mcp.ExtractRequestInfo(&parsed.Requests[i])
		h.metrics.RequestCount.Add(ctx, 1, telemetry.MethodToolAttrs(reqInfo.Method, reqInfo.ToolName))
	}

	totalDuration := time.Since(start)
	h.metrics.RequestDuration.Record(ctx, totalDuration.Seconds(), telemetry.MethodAttr("batch"))
	h.metrics.MessageSize.Record(ctx, int64(len(reqBody)), telemetry.DirectionAttr("request"), telemetry.MethodAttr("batch"))
	h.metrics.MessageSize.Record(ctx, int64(len(respBody)), telemetry.DirectionAttr("response"), telemetry.MethodAttr("batch"))

	// Write response
	copyHeaders(w.Header(), respHeaders)
	w.WriteHeader(statusCode)
	if _, err := w.Write(respBody); err != nil {
		h.logger.ErrorContext(ctx, "failed to write batch response to client", "error", err)
	}
}

func (h *Handler) forwardRaw(w http.ResponseWriter, r *http.Request, reqBody []byte) {
	// Use streaming for SSE-capable clients
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		_, _, _, err := h.doUpstreamStreamingRequest(r.Context(), w, r, reqBody)
		if err != nil {
			h.logger.ErrorContext(r.Context(), "failed to stream raw SSE response", "error", err)
		}
		return
	}
	respBody, respHeaders, statusCode, err := h.doUpstreamRequest(r.Context(), r, reqBody)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	copyHeaders(w.Header(), respHeaders)
	w.WriteHeader(statusCode)
	if _, err := w.Write(respBody); err != nil {
		h.logger.ErrorContext(r.Context(), "failed to write raw response to client", "error", err)
	}
}

func (h *Handler) doUpstreamRequest(ctx context.Context, originalReq *http.Request, body []byte) ([]byte, http.Header, int, error) {
	upstreamURL := h.upstream.String() + originalReq.URL.Path
	if originalReq.URL.RawQuery != "" {
		upstreamURL += "?" + originalReq.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(ctx, originalReq.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, err
	}

	// Copy all headers transparently
	for k, vv := range originalReq.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	// Ensure upstream always gets Accept: text/event-stream (supergateway requires it)
	req.Header.Set("Accept", "text/event-stream, application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, err
	}

	return respBody, resp.Header, resp.StatusCode, nil
}


// extractSSEData extracts the last "data: " payload from SSE response bytes.
func extractSSEData(sseBody []byte) []byte {
	var lastData []byte
	for _, line := range bytes.Split(sseBody, []byte{0x0a}) {
		if bytes.HasPrefix(line, []byte("data: ")) {
			lastData = line[6:]
		}
	}
	return lastData
}
// doUpstreamStreamingRequest sends the request upstream and streams SSE response directly to the client.
// Returns the full buffered SSE data for telemetry parsing, plus the response headers.
func (h *Handler) doUpstreamStreamingRequest(ctx context.Context, w http.ResponseWriter, originalReq *http.Request, body []byte) ([]byte, http.Header, int, error) {
	upstreamURL := h.upstream.String() + originalReq.URL.Path
	if originalReq.URL.RawQuery != "" {
		upstreamURL += "?" + originalReq.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(ctx, originalReq.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, err
	}

	for k, vv := range originalReq.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	// Ensure upstream always gets Accept: text/event-stream (supergateway requires it)
	req.Header.Set("Accept", "text/event-stream, application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Copy response headers to client
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Stream SSE and buffer for telemetry.
	// SSE streams stay open, so we read with a timeout: once we see a blank line
	// (SSE event boundary) after data lines, we stop reading.
	flusher, canFlush := w.(http.Flusher)
	var buf bytes.Buffer
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	// Use a goroutine + channel to avoid blocking forever on scanner.Scan()
	type scanResult struct {
		line []byte
		ok   bool
	}
	lineCh := make(chan scanResult, 1)
	go func() {
		for scanner.Scan() {
			line := make([]byte, len(scanner.Bytes()))
			copy(line, scanner.Bytes())
			lineCh <- scanResult{line: line, ok: true}
		}
		lineCh <- scanResult{ok: false}
	}()

	sawData := false
	idleTimeout := 5 * time.Second
	for {
		select {
		case result := <-lineCh:
			if !result.ok {
				goto done
			}
			buf.Write(result.line)
			buf.WriteByte(0x0a)
			_, _ = w.Write(result.line)
			_, _ = w.Write([]byte{0x0a})
			if canFlush {
				flusher.Flush()
			}
			if bytes.HasPrefix(result.line, []byte("data: ")) {
				sawData = true
			}
			// Empty line after data = end of SSE event
			if sawData && len(result.line) == 0 {
				goto done
			}
		case <-time.After(idleTimeout):
			goto done
		case <-ctx.Done():
			goto done
		}
	}
done:

	return buf.Bytes(), resp.Header, resp.StatusCode, scanner.Err()
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// compressResponse applies JSON→Markdown compression to MCP tool call response content blocks.
// It modifies text content blocks in-place and returns the re-serialized response body.
func (h *Handler) compressResponse(ctx context.Context, span trace.Span, respBody []byte, respParsed *jsonrpc.ParseResult) []byte {
	if respParsed == nil || len(respParsed.Responses) == 0 {
		return respBody
	}

	// Create a child span for the compression operation
	ctx, compressSpan := otel.Tracer("mcp-otel-proxy").Start(ctx, "mcp.response.compress",
		trace.WithAttributes(
			attribute.Int("mcp.response.original_bytes", len(respBody)),
		),
	)
	defer compressSpan.End()

	resp := &respParsed.Responses[0]
	if len(resp.Result) == 0 {
		return respBody
	}

	// Parse the result to find content blocks
	var result map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return respBody
	}

	contentRaw, ok := result["content"]
	if !ok {
		return respBody
	}

	var content []map[string]json.RawMessage
	if err := json.Unmarshal(contentRaw, &content); err != nil {
		return respBody
	}

	originalSize := len(respBody)
	compressed := false

	for i, block := range content {
		typeRaw, ok := block["type"]
		if !ok {
			continue
		}
		var blockType string
		if err := json.Unmarshal(typeRaw, &blockType); err != nil || blockType != "text" {
			continue
		}

		textRaw, ok := block["text"]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(textRaw, &text); err != nil {
			continue
		}

		converted, didConvert := compress.CompressJSONToMarkdown(text)
		if didConvert {
			newText, _ := json.Marshal(converted)
			content[i]["text"] = newText
			compressed = true
		}
	}

	if !compressed {
		return respBody
	}

	// Re-serialize the modified content back into the response
	newContent, err := json.Marshal(content)
	if err != nil {
		return respBody
	}
	result["content"] = newContent

	newResult, err := json.Marshal(result)
	if err != nil {
		return respBody
	}
	resp.Result = newResult

	// Re-serialize the full JSON-RPC response
	newRespBody, err := json.Marshal(resp)
	if err != nil {
		return respBody
	}

	// Set compression telemetry attributes on both parent and child spans
	span.SetAttributes(
		attribute.Bool("mcp.response.compressed", true),
		attribute.Int("mcp.response.original_bytes", originalSize),
		attribute.Int("mcp.response.compressed_bytes", len(newRespBody)),
	)
	compressSpan.SetAttributes(
		attribute.Bool("mcp.response.compressed", true),
		attribute.Int("mcp.response.original_bytes", originalSize),
		attribute.Int("mcp.response.compressed_bytes", len(newRespBody)),
	)

	// Record compression ratio metric
	if originalSize > 0 {
		ratio := float64(len(newRespBody)) / float64(originalSize)
		h.metrics.CompressionRatio.Record(ctx, ratio)
	}

	h.logger.DebugContext(ctx, "compressed response",
		"original_bytes", originalSize,
		"compressed_bytes", len(newRespBody),
	)

	return newRespBody
}
