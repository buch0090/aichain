package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aichain/aichain/pkg/claude"
	"github.com/aichain/aichain/pkg/session"
)

// Server represents the AIChain backend server
type Server struct {
	port         int
	sessionMgr   *session.Manager
	claudeClient *claude.Client
}

// Request represents a request from VIM
type Request struct {
	Command   string            `json:"command"`
	Session   string            `json:"session,omitempty"`
	Code      string            `json:"code,omitempty"`
	Filename  string            `json:"filename,omitempty"`
	Filetype  string            `json:"filetype,omitempty"`
	LineStart int               `json:"line_start,omitempty"`
	LineEnd   int               `json:"line_end,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// Response represents a response to VIM
type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// New creates a new AIChain server
func New(port int) *Server {
	return &Server{
		port:         port,
		sessionMgr:   session.NewManager(),
		claudeClient: claude.NewClient(),
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	http.HandleFunc("/ping", s.handlePing)
	http.HandleFunc("/test", s.handleTest)
	http.HandleFunc("/create_session", s.handleCreateSession)
	http.HandleFunc("/claude", s.handleClaude)
	http.HandleFunc("/status", s.handleStatus)

	// Add CORS headers for local development
	http.HandleFunc("/", s.addCORS(http.NotFoundHandler()).ServeHTTP)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("AIChain server listening on %s", addr)

	return http.ListenAndServe(addr, nil)
}

func (s *Server) addCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Handler for /ping - health check
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, Response{
		Status:  "ok",
		Message: "AIChain server is running",
	})
}

// Handler for /test - test connection
func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	message := "no message"
	if req.Data != nil && req.Data["message"] != nil {
		message = fmt.Sprintf("%v", req.Data["message"])
	}

	s.sendJSON(w, Response{
		Status:  "ok",
		Message: fmt.Sprintf("Hello from AIChain server! Received: %s", message),
	})
}

// Handler for /create_session - create new Claude session
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	sessionName := req.Data["name"].(string)
	session := s.sessionMgr.CreateSession(sessionName)

	log.Printf("Created new session: %s", sessionName)

	s.sendJSON(w, Response{
		Status:  "ok",
		Message: fmt.Sprintf("Session '%s' created", sessionName),
		Data: map[string]interface{}{
			"session_id": session.ID,
			"name":       session.Name,
			"created":    session.CreatedAt,
		},
	})
}

// Handler for /claude - send request to Claude API
func (s *Server) handleClaude(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Get or create session
	session := s.sessionMgr.GetSession(req.Session)
	if session == nil {
		session = s.sessionMgr.CreateSession(req.Session)
	}

	// Build prompt based on command
	prompt := s.buildPrompt(req)

	// Send to Claude API
	response, err := s.claudeClient.SendMessage(prompt)
	if err != nil {
		log.Printf("Claude API error: %v", err)
		s.sendError(w, fmt.Sprintf("Claude API error: %v", err), http.StatusInternalServerError)
		return
	}

	// Add to session history
	session.AddMessage("user", s.buildUserMessage(req))
	session.AddMessage("claude", response)

	log.Printf("Claude response for session %s: %s", req.Session, response[:min(100, len(response))])

	s.sendJSON(w, Response{
		Status:  "ok",
		Message: response,
		Data: map[string]interface{}{
			"session":   req.Session,
			"timestamp": time.Now(),
		},
	})
}

// Handler for /status - get server status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	sessions := s.sessionMgr.ListSessions()
	
	s.sendJSON(w, Response{
		Status: "ok",
		Data: map[string]interface{}{
			"uptime":         time.Since(time.Now()).String(), // TODO: track actual uptime
			"sessions_count": len(sessions),
			"sessions":       sessions,
			"claude_status":  s.claudeClient.Status(),
		},
	})
}

// Helper functions
func (s *Server) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(Response{
		Status: "error",
		Error:  message,
	})
}

func (s *Server) buildPrompt(req Request) string {
	switch req.Command {
	case "explain":
		return fmt.Sprintf(`Explain this %s code from %s:

%s

Please provide a clear explanation of what this code does, how it works, and any potential issues or improvements.`, 
			req.Filetype, req.Filename, req.Code)

	case "fix":
		return fmt.Sprintf(`Fix any bugs or issues in this %s code from %s:

%s

Identify problems and provide corrected code with explanations of what was wrong.`, 
			req.Filetype, req.Filename, req.Code)

	case "optimize":
		return fmt.Sprintf(`Optimize this %s code from %s for better performance:

%s

Suggest performance improvements and provide optimized code with explanations.`, 
			req.Filetype, req.Filename, req.Code)

	case "review":
		return fmt.Sprintf(`Review this %s code from %s:

%s

Provide a code review covering: functionality, bugs, performance, readability, and best practices.`, 
			req.Filetype, req.Filename, req.Code)

	default:
		return req.Command
	}
}

func (s *Server) buildUserMessage(req Request) string {
	if req.Code != "" {
		return fmt.Sprintf("%s: %s code from %s", req.Command, req.Filetype, req.Filename)
	}
	return req.Command
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}