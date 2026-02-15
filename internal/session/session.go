package session

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Session represents a single AI session in AIChain
type Session struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Model       string                 `json:"model"`        // claude-3, gpt-4, etc.
	Role        string                 `json:"role"`         // "developer", "reviewer", "architect"
	Messages    []Message              `json:"messages"`
	Context     SessionContext         `json:"context"`
	Links       []SessionLink          `json:"links"`        // Connected sessions
	Status      SessionStatus          `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	SystemPrompt string                `json:"system_prompt"`
	mu          sync.RWMutex           `json:"-"`
}

// Message represents a conversation message
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`      // "user", "assistant", "system"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`    // Which session/user sent this
}

// SessionContext holds the current context for a session
type SessionContext struct {
	Files           []string         `json:"files"`          // Currently focused files
	Directory       string           `json:"directory"`
	CodeSelection   *CodeSelection   `json:"code_selection"` // VIM selection
	ConversationMode string          `json:"conversation_mode"` // "chat", "code_review", "debug"
}

// CodeSelection represents selected code from VIM
type CodeSelection struct {
	Filename  string `json:"filename"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
}

// SessionLink represents a connection between sessions
type SessionLink struct {
	TargetSessionID string            `json:"target_session_id"`
	Direction       LinkDirection     `json:"direction"`     // "one_way", "two_way"
	AutoForward     bool             `json:"auto_forward"`
	Conditions      []ForwardRule    `json:"conditions"`    // When to auto-forward
}

// LinkDirection defines how sessions communicate
type LinkDirection string

const (
	OneWayLink LinkDirection = "one_way"
	TwoWayLink LinkDirection = "two_way"
)

// ForwardRule defines when to forward messages between sessions
type ForwardRule struct {
	Condition string `json:"condition"` // "on_response", "on_keyword", "manual"
	Keywords  []string `json:"keywords,omitempty"` // For keyword-based forwarding
}

// SessionStatus tracks the current state of a session
type SessionStatus string

const (
	StatusActive    SessionStatus = "active"
	StatusIdle      SessionStatus = "idle"
	StatusProcessing SessionStatus = "processing"
	StatusError     SessionStatus = "error"
)

// Manager handles all sessions
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewManager creates a new session manager
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new AI session
func (m *Manager) CreateSession(name, model, role, systemPrompt string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate unique ID
	id := fmt.Sprintf("session_%d", time.Now().UnixNano())

	session := &Session{
		ID:           id,
		Name:         name,
		Model:        model,
		Role:         role,
		SystemPrompt: systemPrompt,
		Messages:     []Message{},
		Context: SessionContext{
			ConversationMode: "chat",
		},
		Links:     []SessionLink{},
		Status:    StatusIdle,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.sessions[id] = session
	return session, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}
	return session, nil
}

// GetSessionByName retrieves a session by name
func (m *Manager) GetSessionByName(name string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, session := range m.sessions {
		if session.Name == name {
			return session, nil
		}
	}
	return nil, fmt.Errorf("session %s not found", name)
}

// ListSessions returns all sessions
func (m *Manager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// AddMessage adds a message to a session
func (s *Session) AddMessage(role, content, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	message := Message{
		ID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
		Source:    source,
	}

	s.Messages = append(s.Messages, message)
	s.UpdatedAt = time.Now()
}

// SetStatus updates the session status
func (s *Session) SetStatus(status SessionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Status = status
	s.UpdatedAt = time.Now()
}

// LinkTo creates a link to another session
func (s *Session) LinkTo(targetID string, direction LinkDirection, autoForward bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link := SessionLink{
		TargetSessionID: targetID,
		Direction:       direction,
		AutoForward:     autoForward,
	}

	s.Links = append(s.Links, link)
	s.UpdatedAt = time.Now()
}

// GetLinks returns all session links
func (s *Session) GetLinks() []SessionLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return append([]SessionLink(nil), s.Links...)
}

// UpdateContext updates the session context
func (s *Session) UpdateContext(context SessionContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Context = context
	s.UpdatedAt = time.Now()
}

// ToJSON serializes the session to JSON
func (s *Session) ToJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return json.Marshal(s)
}

// FromJSON deserializes a session from JSON
func FromJSON(data []byte) (*Session, error) {
	var session Session
	err := json.Unmarshal(data, &session)
	return &session, err
}