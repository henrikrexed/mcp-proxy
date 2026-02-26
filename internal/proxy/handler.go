package proxy

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

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

	// Forward to upstream
	upstreamStart := time.Now()
	respBody, respHeaders, statusCode, err := h.doUpstreamRequest(ctx, r, bodyToSend)
	upstreamDuration := time.Since(upstreamStart)

	if err != nil {
		h.logger.ErrorContext(ctx, "upstream request failed",
			"error", err,
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
		}

		// Opt-in payload capture
		if h.config.CapturePayload && reqInfo.Method == "tools/call" {
			telemetry.SetPayloadAttributes(span, string(req.Params), string(respParsed.Responses[0].Result))
		}
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

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
