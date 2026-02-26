package mcp

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/isitobservable/mcp-otel-proxy/internal/jsonrpc"
)

// Session holds state from an MCP initialize handshake.
type Session struct {
	ID              string
	ProtocolVersion string
	CreatedAt       time.Time
	LastAccessedAt  time.Time
}

// SessionStore manages MCP session state with TTL-based eviction.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
	onAdd    func()
	onRemove func()
}

// NewSessionStore creates a new session store with TTL eviction.
// onAdd and onRemove callbacks are called when sessions are added/removed (for metrics).
func NewSessionStore(ttl time.Duration, onAdd, onRemove func()) *SessionStore {
	ss := &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
		onAdd:    onAdd,
		onRemove: onRemove,
	}
	go ss.evictionLoop()
	return ss
}

// Get returns the session for the given ID, or nil if not found.
func (ss *SessionStore) Get(id string) *Session {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	s, ok := ss.sessions[id]
	if !ok {
		return nil
	}
	s.LastAccessedAt = time.Now()
	return s
}

// TrackInitialize extracts session info from initialize request/response.
func (ss *SessionStore) TrackInitialize(resp *jsonrpc.Response, sessionID string) {
	if sessionID == "" {
		return
	}

	var protocolVersion string
	if len(resp.Result) > 0 {
		var result map[string]json.RawMessage
		if json.Unmarshal(resp.Result, &result) == nil {
			if pv, ok := result["protocolVersion"]; ok {
				var s string
				if json.Unmarshal(pv, &s) == nil {
					protocolVersion = s
				}
			}
		}
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	ss.sessions[sessionID] = &Session{
		ID:              sessionID,
		ProtocolVersion: protocolVersion,
		CreatedAt:       time.Now(),
		LastAccessedAt:  time.Now(),
	}
	if ss.onAdd != nil {
		ss.onAdd()
	}
}

// ActiveCount returns the number of active sessions.
func (ss *SessionStore) ActiveCount() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.sessions)
}

func (ss *SessionStore) evictionLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ss.evict()
	}
}

func (ss *SessionStore) evict() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	now := time.Now()
	for id, s := range ss.sessions {
		if now.Sub(s.LastAccessedAt) > ss.ttl {
			delete(ss.sessions, id)
			if ss.onRemove != nil {
				ss.onRemove()
			}
		}
	}
}
