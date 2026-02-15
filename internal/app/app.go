package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/aichain/aichain/internal/ai"
	"github.com/aichain/aichain/internal/pipeline"
	"github.com/aichain/aichain/internal/session"
	"github.com/aichain/aichain/internal/vim"
)

// Import session status constants
const (
	StatusActive    = session.StatusActive
	StatusIdle      = session.StatusIdle
	StatusProcessing = session.StatusProcessing
	StatusError     = session.StatusError
)

// Application represents the main AIChain application
type Application struct {
	sessionManager  *session.Manager
	pipelineManager *pipeline.Manager
	aiManager       *ai.Manager
	vimEngine       *vim.Engine
	
	// Current state
	activeSessionID string
	activePipeline  string
	workingDir      string
	
	// Configuration
	config *Config
	
	// Synchronization
	mu sync.RWMutex
}

// Config holds application configuration
type Config struct {
	DefaultModel     string                 `yaml:"default_model"`
	DefaultProvider  string                 `yaml:"default_provider"`
	AutoStartBackend bool                   `yaml:"auto_start_backend"`
	AllowedDirectory string                 `yaml:"allowed_directory"`
	Keybindings      map[string]interface{} `yaml:"keybindings"`
	SessionTemplates map[string]interface{} `yaml:"session_templates"`
	UIConfig         UIConfig               `yaml:"ui"`
}

// UIConfig holds UI-specific configuration
type UIConfig struct {
	Theme        string `yaml:"theme"`
	ShowLineNumbers bool `yaml:"show_line_numbers"`
	TabSize      int    `yaml:"tab_size"`
	WordWrap     bool   `yaml:"word_wrap"`
}

// NewApplication creates a new AIChain application
func NewApplication() *Application {
	// Initialize managers
	sessionManager := session.NewManager()
	aiManager := ai.NewManager()
	pipelineManager := pipeline.NewManager(sessionManager)
	vimEngine := vim.NewEngine()

	// Register AI providers
	claudeProvider := ai.NewClaudeProvider()
	aiManager.RegisterProvider("claude", claudeProvider)

	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("Failed to get working directory: %v", err)
		workingDir = "/"
	}

	// Get allowed directory from environment variable
	allowedDir := os.Getenv("AICHAIN_ALLOWED_DIR")
	if allowedDir == "" {
		allowedDir = workingDir
	}

	app := &Application{
		sessionManager:  sessionManager,
		pipelineManager: pipelineManager,
		aiManager:       aiManager,
		vimEngine:       vimEngine,
		workingDir:      workingDir,
		config: &Config{
			DefaultModel:     "claude-opus-4-5-20251101",
			DefaultProvider:  "claude",
			AutoStartBackend: true,
			AllowedDirectory: allowedDir,
		},
	}

	return app
}

// NewApplicationWithConfig creates a new AIChain application with custom config
func NewApplicationWithConfig(config *Config) *Application {
	// Initialize managers
	sessionManager := session.NewManager()
	aiManager := ai.NewManager()
	pipelineManager := pipeline.NewManager(sessionManager)
	vimEngine := vim.NewEngine()

	// Register AI providers
	claudeProvider := ai.NewClaudeProvider()
	aiManager.RegisterProvider("claude", claudeProvider)

	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("Failed to get working directory: %v", err)
		workingDir = "/"
	}

	// Set defaults for missing config values
	if config.DefaultModel == "" {
		config.DefaultModel = "claude-opus-4-5-20251101"
	}
	if config.DefaultProvider == "" {
		config.DefaultProvider = "claude"
	}

	app := &Application{
		sessionManager:  sessionManager,
		pipelineManager: pipelineManager,
		aiManager:       aiManager,
		vimEngine:       vimEngine,
		workingDir:      workingDir,
		config:          config,
	}

	return app
}

// Initialize initializes the application
func (app *Application) Initialize() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	// Create default session if none exist
	if len(app.sessionManager.ListSessions()) == 0 {
		session, err := app.CreateSession("General", "claude", "assistant", "You are Claude, a helpful AI assistant integrated into a VIM-like terminal interface.")
		if err != nil {
			return fmt.Errorf("failed to create default session: %v", err)
		}
		app.activeSessionID = session.ID
	}

	return nil
}

// CreateSession creates a new AI session
func (app *Application) CreateSession(name, model, role, systemPrompt string) (*session.Session, error) {
	if model == "" {
		model = app.config.DefaultModel
	}

	session, err := app.sessionManager.CreateSession(name, model, role, systemPrompt)
	if err != nil {
		return nil, err
	}

	// Set as active if no active session
	if app.activeSessionID == "" {
		app.activeSessionID = session.ID
	}

	return session, nil
}

// GetActiveSession returns the currently active session
func (app *Application) GetActiveSession() (*session.Session, error) {
	if app.activeSessionID == "" {
		return nil, fmt.Errorf("no active session")
	}
	return app.sessionManager.GetSession(app.activeSessionID)
}

// SetActiveSession sets the active session
func (app *Application) SetActiveSession(sessionID string) error {
	_, err := app.sessionManager.GetSession(sessionID)
	if err != nil {
		return err
	}

	app.mu.Lock()
	app.activeSessionID = sessionID
	app.mu.Unlock()

	return nil
}

// SendMessage sends a message to the specified session
func (app *Application) SendMessage(sessionID, message string) (*ai.AIResponse, error) {
	session, err := app.sessionManager.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Add user message to session
	session.AddMessage("user", message, "user")
	session.SetStatus(StatusProcessing)

	// Prepare AI context
	aiContext := ai.AIContext{
		SystemPrompt:        session.SystemPrompt,
		ConversationHistory: app.convertMessages(session.Messages),
		SessionRole:         session.Role,
		Temperature:         0.7,
		MaxTokens:          4000,
	}

	// Send to AI provider
	provider := app.config.DefaultProvider
	response, err := app.aiManager.SendMessage(provider, context.Background(), message, aiContext)
	if err != nil {
		session.SetStatus(StatusError)
		return nil, fmt.Errorf("AI request failed: %v", err)
	}

	// Add AI response to session
	session.AddMessage("assistant", response.Content, provider)
	session.SetStatus(StatusActive)

	// Process through any connected pipelines
	pipelines := app.findPipelinesForSession(sessionID)
	for _, p := range pipelines {
		if err := p.ProcessMessage(sessionID, response.Content); err != nil {
			log.Printf("Pipeline processing failed: %v", err)
		}
	}

	return response, nil
}

// CreateDualAISession creates two AI sessions and connects them
func (app *Application) CreateDualAISession(name1, name2 string, bidirectional bool) (*pipeline.Pipeline, error) {
	// Create first session
	session1, err := app.CreateSession(name1, app.config.DefaultModel, "participant", "You are participating in an AI collaboration session.")
	if err != nil {
		return nil, fmt.Errorf("failed to create first session: %v", err)
	}

	// Create second session
	session2, err := app.CreateSession(name2, app.config.DefaultModel, "participant", "You are participating in an AI collaboration session.")
	if err != nil {
		return nil, fmt.Errorf("failed to create second session: %v", err)
	}

	// Create pipeline to connect them
	pipeline, err := app.pipelineManager.CreateDualAIPipeline(session1.ID, session2.ID, bidirectional)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %v", err)
	}

	pipeline.Start()
	app.activePipeline = pipeline.ID

	return pipeline, nil
}

// CreateDebateSession creates a debate between two AI sessions
func (app *Application) CreateDebateSession(participant1, participant2, topic string) (*pipeline.Pipeline, error) {
	// Create participant sessions with debate-specific prompts
	session1, err := app.CreateSession(participant1, app.config.DefaultModel, "debater", 
		fmt.Sprintf("You are participating in a debate about: %s. Present thoughtful arguments and engage constructively with opposing viewpoints.", topic))
	if err != nil {
		return nil, err
	}

	session2, err := app.CreateSession(participant2, app.config.DefaultModel, "debater", 
		fmt.Sprintf("You are participating in a debate about: %s. Present thoughtful counter-arguments and engage constructively with opposing viewpoints.", topic))
	if err != nil {
		return nil, err
	}

	// Create debate pipeline
	pipeline, err := app.pipelineManager.CreateDebatePipeline(session1.ID, session2.ID, "")
	if err != nil {
		return nil, err
	}

	pipeline.Start()
	app.activePipeline = pipeline.ID

	// Start the debate with the topic
	app.SendMessage(session1.ID, fmt.Sprintf("Let's debate: %s. Please present your opening position.", topic))

	return pipeline, nil
}

// ProcessKeypress processes a VIM keypress
func (app *Application) ProcessKeypress(key string) error {
	return app.vimEngine.ProcessKey(key)
}

// GetVIMMode returns the current VIM mode
func (app *Application) GetVIMMode() vim.VIMMode {
	return app.vimEngine.GetCurrentMode()
}

// SetVIMMode sets the VIM mode
func (app *Application) SetVIMMode(mode vim.VIMMode) error {
	return app.vimEngine.SetMode(mode)
}

// ListSessions returns all sessions
func (app *Application) ListSessions() []*session.Session {
	return app.sessionManager.ListSessions()
}

// GetSessionByName returns a session by name
func (app *Application) GetSessionByName(name string) (*session.Session, error) {
	return app.sessionManager.GetSessionByName(name)
}

// GetPipelines returns all pipelines
func (app *Application) GetPipelines() []*pipeline.Pipeline {
	// This would need to be implemented in the pipeline manager
	return []*pipeline.Pipeline{}
}

// GetStatus returns application status
func (app *Application) GetStatus() map[string]interface{} {
	app.mu.RLock()
	defer app.mu.RUnlock()

	sessions := app.sessionManager.ListSessions()
	sessionNames := make([]string, len(sessions))
	for i, s := range sessions {
		sessionNames[i] = s.Name
	}

	return map[string]interface{}{
		"active_session":    app.activeSessionID,
		"active_pipeline":   app.activePipeline,
		"working_directory": app.workingDir,
		"vim_mode":         app.vimEngine.GetCurrentMode().String(),
		"sessions":         sessionNames,
		"session_count":    len(sessions),
	}
}

// Helper functions

// convertMessages converts session messages to AI messages
func (app *Application) convertMessages(messages []session.Message) []ai.Message {
	aiMessages := make([]ai.Message, len(messages))
	for i, msg := range messages {
		aiMessages[i] = ai.Message{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}
	return aiMessages
}

// findPipelinesForSession finds pipelines that include a specific session
func (app *Application) findPipelinesForSession(sessionID string) []*pipeline.Pipeline {
	// This would search through pipelines to find ones containing the session
	// For now, return empty slice
	return []*pipeline.Pipeline{}
}

// Shutdown gracefully shuts down the application
func (app *Application) Shutdown() error {
	// Stop all pipelines
	// Save session state
	// Cleanup resources
	return nil
}

// GetConfig returns the application configuration
func (app *Application) GetConfig() *Config {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return app.config
}