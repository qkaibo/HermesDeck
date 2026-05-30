package session

import (
	"fmt"
	"sync"
	"time"
)

type Mode string

const ModeLocal Mode = "go_local"
const ModeRemote Mode = "hermes_remote"

type Session struct {
	ID              string
	Mode            Mode
	History         []ChatMessage
	HermesSessionID string
	CreatedAt       time.Time
	IsActive        bool
	mu              sync.RWMutex
}

type ChatMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolResult string `json:"tool_result,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

func NewSession(id string) *Session {
	return &Session{ID: id, Mode: ModeLocal, CreatedAt: time.Now(), IsActive: true}
}

func (s *Session) AddMessage(msg ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg.Timestamp = time.Now().UnixMilli()
	s.History = append(s.History, msg)
}

func (s *Session) GetHistory() []ChatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := make([]ChatMessage, len(s.History))
	copy(r, s.History)
	return r
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
}

func (m *Manager) Create(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := NewSession(id)
	m.sessions[id] = s
	return s
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) GetOrCreate(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		return s
	}
	s := NewSession(id)
	m.sessions[id] = s
	return s
}

func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		r = append(r, s)
	}
	return r
}

func (m *Manager) CreateID() string {
	return fmt.Sprintf("ses_%d", time.Now().UnixNano())
}
