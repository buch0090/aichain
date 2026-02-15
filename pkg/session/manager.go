package session

import (
	"fmt"
	"sync"
	"time"
)

// Message represents a message in a conversation
type Message struct {
	Type      string    `json:"type"`      // "user" or "claude"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Session represents a Claude conversation session
type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Manager manages Claude sessions
type Manager struct {
	sessions map[string]*Session
	mutex    sync.RWMutex
}

// NewManager creates a new session manager
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session
func (m *Manager) CreateSession(name string) *Session {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	session := &Session{
		ID:        fmt.Sprintf("session_%d", time.Now().Unix()),
		Name:      name,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.sessions[name] = session
	return session
}

// GetSession retrieves a session by name
func (m *Manager) GetSession(name string) *Session {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.sessions[name]
}

// ListSessions returns all sessions
func (m *Manager) ListSessions() map[string]*Session {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to avoid race conditions
	sessions := make(map[string]*Session)
	for name, session := range m.sessions {
		sessions[name] = session
	}
	return sessions
}

// DeleteSession removes a session
func (m *Manager) DeleteSession(name string) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.sessions[name]; exists {
		delete(m.sessions, name)
		return true
	}
	return false
}

// AddMessage adds a message to a session
func (s *Session) AddMessage(msgType, content string) {
	message := Message{
		Type:      msgType,
		Content:   content,
		Timestamp: time.Now(),
	}
	
	s.Messages = append(s.Messages, message)
	s.UpdatedAt = time.Now()
}

// GetHistory returns formatted conversation history
func (s *Session) GetHistory() string {
	var history string
	for _, msg := range s.Messages {
		if msg.Type == "user" {
			history += fmt.Sprintf("You: %s\n\n", msg.Content)
		} else {
			history += fmt.Sprintf("Claude: %s\n\n", msg.Content)
		}
	}
	return history
}

// GetMessageCount returns the number of messages in the session
func (s *Session) GetMessageCount() int {
	return len(s.Messages)
}