package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type reinitializer struct {
	mu              sync.Mutex
	lastInitReqBody []byte
	lastSessionID   string
	upstream        string
	client          *http.Client
	logger          *slog.Logger
	reinitInFlight  bool
}

func newReinitializer(upstream string, client *http.Client, logger *slog.Logger) *reinitializer {
	return &reinitializer{
		upstream: upstream,
		client:   client,
		logger:   logger,
	}
}

func (r *reinitializer) cacheInitRequest(body []byte, sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastInitReqBody = make([]byte, len(body))
	copy(r.lastInitReqBody, body)
	r.lastSessionID = sessionID
	r.logger.Info("cached initialize request for reinit", "session-id", sessionID)
}

func shouldReinit(statusCode int, method string) bool {
	if method == "initialize" || method == "notifications/initialized" {
		return false
	}
	return statusCode == 400 || statusCode == 404 || statusCode == 502
}

func (r *reinitializer) reinitAndRetry(originalReq *http.Request, originalBody []byte, path string) ([]byte, http.Header, int, error) {
	r.mu.Lock()
	if r.reinitInFlight {
		r.mu.Unlock()
		return nil, nil, 0, fmt.Errorf("reinit already in flight")
	}
	if r.lastInitReqBody == nil {
		r.mu.Unlock()
		return nil, nil, 0, fmt.Errorf("no cached initialize request to replay")
	}
	r.reinitInFlight = true
	initBody := make([]byte, len(r.lastInitReqBody))
	copy(initBody, r.lastInitReqBody)
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.reinitInFlight = false
		r.mu.Unlock()
	}()

	r.logger.Info("starting reinit sequence")

	initResp, initHeaders, initStatus, err := r.doRawRequest("POST", path, initBody, "")
	if err != nil {
		return nil, nil, 0, fmt.Errorf("reinit initialize failed: %w", err)
	}
	if initStatus != 200 {
		return nil, nil, 0, fmt.Errorf("reinit initialize returned status %d", initStatus)
	}

	_ = initResp
	newSessionID := initHeaders.Get("Mcp-Session-Id")
	r.logger.Info("reinit initialize succeeded", "new-session-id", newSessionID)

	r.mu.Lock()
	r.lastSessionID = newSessionID
	r.mu.Unlock()

	notifBody := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	_, _, _, notifErr := r.doRawRequest("POST", path, notifBody, newSessionID)
	if notifErr != nil {
		r.logger.Warn("reinit notifications/initialized failed", "error", notifErr)
	}

	time.Sleep(100 * time.Millisecond)

	r.logger.Info("retrying original request after reinit", "session-id", newSessionID)
	return r.doRawRequest("POST", path, originalBody, newSessionID)
}

func (r *reinitializer) doRawRequest(method, path string, body []byte, sessionID string) ([]byte, http.Header, int, error) {
	reqURL := r.upstream + path
	req, err := http.NewRequest(method, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	req.Header.Set("Accept", "text/event-stream, application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, 0, err
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		_ = extractSSEData(respBody)
	}

	return respBody, resp.Header, resp.StatusCode, nil
}

func (r *reinitializer) getSessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastSessionID
}

