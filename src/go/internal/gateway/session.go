package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// GatewaySession tracks a WebSocket connection's state.
type GatewaySession struct {
	mu         sync.Mutex
	Conn       *websocket.Conn
	ClientName string
	Token      string

	Sessions    map[string]*pbSession // sessionKey → gRPC session
	seq         int
	lastEventID string
}

type pbSession struct {
	SessionID string
	Created   time.Time
	Cancel    context.CancelFunc
}

func NewGatewaySession(conn *websocket.Conn, clientName, token string) *GatewaySession {
	return &GatewaySession{
		Conn:       conn,
		ClientName: clientName,
		Token:      token,
		Sessions:   make(map[string]*pbSession),
	}
}

func (gs *GatewaySession) NextSeq() int {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.seq++
	return gs.seq
}

func (gs *GatewaySession) GetOrCreatePBSession(sessionKey string, cancel context.CancelFunc) *pbSession {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if s, ok := gs.Sessions[sessionKey]; ok {
		return s
	}
	s := &pbSession{
		SessionID: sessionKey,
		Created:   time.Now(),
		Cancel:    cancel,
	}
	gs.Sessions[sessionKey] = s
	return s
}

func (gs *GatewaySession) RemovePBSession(sessionKey string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if s, ok := gs.Sessions[sessionKey]; ok {
		if s.Cancel != nil {
			s.Cancel()
		}
		delete(gs.Sessions, sessionKey)
	}
}

func (gs *GatewaySession) CancelSession(sessionKey string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if s, ok := gs.Sessions[sessionKey]; ok && s.Cancel != nil {
		s.Cancel()
	}
}

func (gs *GatewaySession) Close() {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	for _, s := range gs.Sessions {
		if s.Cancel != nil {
			s.Cancel()
		}
	}
	gs.Sessions = make(map[string]*pbSession)
	if gs.Conn != nil {
		gs.Conn.Close()
	}
}

// SendJSON sends a JSON message over the WebSocket.
func (gs *GatewaySession) SendJSON(v interface{}) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.Conn.WriteJSON(v)
}

// GatewaySessionManager manages all connected WebSocket sessions.
type GatewaySessionManager struct {
	mu       sync.RWMutex
	sessions map[*websocket.Conn]*GatewaySession
}

func NewGatewaySessionManager() *GatewaySessionManager {
	return &GatewaySessionManager{
		sessions: make(map[*websocket.Conn]*GatewaySession),
	}
}

func (m *GatewaySessionManager) Add(gs *GatewaySession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[gs.Conn] = gs
}

func (m *GatewaySessionManager) Remove(conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gs, ok := m.sessions[conn]; ok {
		gs.Close()
		delete(m.sessions, conn)
	}
}

func (m *GatewaySessionManager) Get(conn *websocket.Conn) *GatewaySession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[conn]
}

// GetBySessionKey returns the GatewaySession that owns the given sessionKey.
func (m *GatewaySessionManager) GetBySessionKey(sessionKey string) *GatewaySession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, gs := range m.sessions {
		gs.mu.Lock()
		_, ok := gs.Sessions[sessionKey]
		gs.mu.Unlock()
		if ok {
			return gs
		}
	}
	return nil
}