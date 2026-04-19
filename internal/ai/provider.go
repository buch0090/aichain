package ai

import (
	"context"
	"fmt"
	"time"
)

// StreamCallback is called with each text delta as it streams from the API.
type StreamCallback func(delta string)

// Provider defines the interface for AI providers
type Provider interface {
	SendMessage(ctx context.Context, prompt string, context AIContext) (*AIResponse, error)
	SendMessageStreaming(ctx context.Context, prompt string, context AIContext, onDelta StreamCallback) (*AIResponse, error)
	GetModels() []Model
	GetCapabilities() Capabilities
	GetRateLimits() RateLimits
	GetProviderName() string
}

// AIContext provides context for AI requests
type AIContext struct {
	SystemPrompt        string              `json:"system_prompt"`
	ConversationHistory []Message           `json:"conversation_history"`
	CodeContext         *CodeContext        `json:"code_context"`
	SessionRole         string              `json:"session_role"`
	Temperature         float64             `json:"temperature"`
	MaxTokens          int                 `json:"max_tokens"`
}

// Message represents a conversation message
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// CodeContext provides code-specific context
type CodeContext struct {
	Language     string            `json:"language"`
	Filename     string            `json:"filename"`
	Selection    *CodeSelection    `json:"selection"`
	ProjectFiles []string          `json:"project_files"`
	Directory    string            `json:"directory"`
}

// CodeSelection represents selected code
type CodeSelection struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
}

// AIResponse represents the response from an AI provider
type AIResponse struct {
	Content          string              `json:"content"`
	TokensUsed       int                 `json:"tokens_used"`
	InputTokens      int                 `json:"input_tokens"`
	OutputTokens     int                 `json:"output_tokens"`
	Model            string              `json:"model"`
	Confidence       float64             `json:"confidence"`
	SuggestedActions []SuggestedAction   `json:"suggested_actions"`
	ProcessingTime   time.Duration       `json:"processing_time"`
	Provider         string              `json:"provider"`
}

// SuggestedAction represents an action the AI suggests
type SuggestedAction struct {
	Type        string                 `json:"type"`        // "edit", "create", "run", "explain"
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data"`
}

// Model represents an AI model
type Model struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	MaxTokens    int      `json:"max_tokens"`
	ContextSize  int      `json:"context_size"`
	Strengths    []string `json:"strengths"`
	Pricing      *Pricing `json:"pricing,omitempty"`
}

// Pricing represents model pricing information
type Pricing struct {
	InputCost  float64 `json:"input_cost"`   // Cost per token
	OutputCost float64 `json:"output_cost"`  // Cost per token
	Currency   string  `json:"currency"`
}

// Capabilities represents what an AI provider can do
type Capabilities struct {
	CodeGeneration   bool     `json:"code_generation"`
	CodeReview      bool     `json:"code_review"`
	TextGeneration  bool     `json:"text_generation"`
	ImageAnalysis   bool     `json:"image_analysis"`
	FileUpload      bool     `json:"file_upload"`
	FunctionCalling bool     `json:"function_calling"`
	Languages       []string `json:"languages"`
}

// RateLimits represents API rate limits
type RateLimits struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	RequestsPerHour   int `json:"requests_per_hour"`
	RequestsPerDay    int `json:"requests_per_day"`
	TokensPerMinute   int `json:"tokens_per_minute"`
}

// Manager manages multiple AI providers
type Manager struct {
	providers map[string]Provider
}

// NewManager creates a new AI provider manager
func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]Provider),
	}
}

// RegisterProvider registers a new AI provider
func (m *Manager) RegisterProvider(name string, provider Provider) {
	m.providers[name] = provider
}

// GetProvider gets a provider by name
func (m *Manager) GetProvider(name string) (Provider, error) {
	provider, exists := m.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}
	return provider, nil
}

// ListProviders returns all available providers
func (m *Manager) ListProviders() []string {
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// SendMessage sends a message using the specified provider
func (m *Manager) SendMessage(providerName string, ctx context.Context, prompt string, context AIContext) (*AIResponse, error) {
	provider, err := m.GetProvider(providerName)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	response, err := provider.SendMessage(ctx, prompt, context)
	if err != nil {
		return nil, err
	}

	// Add processing time and provider info
	response.ProcessingTime = time.Since(startTime)
	response.Provider = providerName

	return response, nil
}

// GetAllModels returns models from all providers
func (m *Manager) GetAllModels() map[string][]Model {
	models := make(map[string][]Model)
	for name, provider := range m.providers {
		models[name] = provider.GetModels()
	}
	return models
}

// FindBestProvider finds the best provider for a given task
func (m *Manager) FindBestProvider(task string) (string, error) {
	// Simple heuristic - could be enhanced with more sophisticated logic
	for name, provider := range m.providers {
		capabilities := provider.GetCapabilities()
		
		switch task {
		case "code":
			if capabilities.CodeGeneration {
				return name, nil
			}
		case "review":
			if capabilities.CodeReview {
				return name, nil
			}
		case "text":
			if capabilities.TextGeneration {
				return name, nil
			}
		default:
			// Return first available provider
			return name, nil
		}
	}
	
	return "", fmt.Errorf("no suitable provider found for task: %s", task)
}