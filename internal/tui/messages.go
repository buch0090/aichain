package tui

import "github.com/aichain/aichain/internal/session"

// Messages for the TUI event system

// SessionUpdateMsg indicates a session has been updated
type SessionUpdateMsg struct {
	SessionID string
	Message   session.Message
}

// SessionCreatedMsg indicates a new session was created
type SessionCreatedMsg struct {
	Session *session.Session
}

// SessionSwitchedMsg indicates the active session changed
type SessionSwitchedMsg struct {
	SessionID string
}

// DebateStartedMsg indicates a debate session was started
type DebateStartedMsg struct {
	PipelineID string
	Topic      string
}

// DualSessionCreatedMsg indicates a dual AI session was created
type DualSessionCreatedMsg struct {
	PipelineID string
}

// ErrorMsg indicates an error occurred
type ErrorMsg struct {
	Error error
}

// FileSelectedMsg indicates a file was selected in explorer
type FileSelectedMsg struct {
	FilePath string
}

// MessageSentMsg indicates a message was sent to AI
type MessageSentMsg struct {
	SessionID string
	Content   string
}