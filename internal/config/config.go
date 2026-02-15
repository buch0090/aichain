package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the complete application configuration
type Config struct {
	App          AppConfig                     `yaml:"app"`
	UI           UIConfig                      `yaml:"ui"`
	Providers    map[string]ProviderConfig     `yaml:"providers"`
	Templates    map[string]SessionTemplate    `yaml:"session_templates"`
	Keybindings  map[string]map[string]string  `yaml:"keybindings"`
	Commands     map[string]string             `yaml:"commands"`
	Performance  PerformanceConfig             `yaml:"performance"`
	Logging      LoggingConfig                 `yaml:"logging"`
}

// AppConfig holds general application settings
type AppConfig struct {
	DefaultModel       string `yaml:"default_model"`
	DefaultProvider    string `yaml:"default_provider"`
	AutoStartBackend   bool   `yaml:"auto_start_backend"`
	WorkingDirectory   string `yaml:"working_directory"`
}

// UIConfig holds UI-specific settings
type UIConfig struct {
	Theme             string    `yaml:"theme"`
	ShowLineNumbers   bool      `yaml:"show_line_numbers"`
	TabSize           int       `yaml:"tab_size"`
	WordWrap          bool      `yaml:"word_wrap"`
	DefaultLayout     string    `yaml:"default_layout"`
	TripleProportions []float64 `yaml:"triple_proportions"`
	DualProportions   []float64 `yaml:"dual_proportions"`
}

// ProviderConfig holds AI provider configuration
type ProviderConfig struct {
	APIKeyEnv string        `yaml:"api_key_env"`
	BaseURL   string        `yaml:"base_url"`
	Models    []ModelConfig `yaml:"models"`
}

// ModelConfig holds model-specific configuration
type ModelConfig struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	MaxTokens   int      `yaml:"max_tokens"`
	ContextSize int      `yaml:"context_size"`
	Strengths   []string `yaml:"strengths"`
}

// SessionTemplate holds session template configuration
type SessionTemplate struct {
	Description string                `yaml:"description"`
	Sessions    []SessionConfig       `yaml:"sessions"`
	Pipeline    PipelineConfig        `yaml:"pipeline"`
	Layout      string                `yaml:"layout"`
}

// SessionConfig holds individual session configuration
type SessionConfig struct {
	Name         string `yaml:"name"`
	Model        string `yaml:"model"`
	Role         string `yaml:"role"`
	SystemPrompt string `yaml:"system_prompt"`
}

// PipelineConfig holds pipeline configuration
type PipelineConfig struct {
	Type          string   `yaml:"type"`
	Bidirectional bool     `yaml:"bidirectional"`
	AutoForward   bool     `yaml:"auto_forward"`
	Flow          []string `yaml:"flow"`
}

// PerformanceConfig holds performance-related settings
type PerformanceConfig struct {
	MaxMessagesPerSession  int `yaml:"max_messages_per_session"`
	MaxConcurrentRequests  int `yaml:"max_concurrent_requests"`
	RequestTimeout         int `yaml:"request_timeout"`
	AutoSaveInterval       int `yaml:"auto_save_interval"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level           string `yaml:"level"`
	File            string `yaml:"file"`
	Timestamps      bool   `yaml:"timestamps"`
	LogAIRequests   bool   `yaml:"log_ai_requests"`
}

// Manager handles configuration loading and management
type Manager struct {
	config     *Config
	configPath string
}

// NewManager creates a new configuration manager
func NewManager() *Manager {
	return &Manager{}
}

// LoadConfig loads configuration from file or creates default
func (m *Manager) LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		return m.createDefaultConfig(configPath)
	}

	// Load existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	m.config = &config
	m.configPath = configPath

	// Validate configuration
	if err := m.validateConfig(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return &config, nil
}

// SaveConfig saves the current configuration to file
func (m *Manager) SaveConfig() error {
	if m.config == nil {
		return fmt.Errorf("no configuration to save")
	}

	data, err := yaml.Marshal(m.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Ensure config directory exists
	configDir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// UpdateConfig updates the configuration
func (m *Manager) UpdateConfig(config *Config) error {
	m.config = config
	return m.validateConfig()
}

// createDefaultConfig creates a default configuration
func (m *Manager) createDefaultConfig(configPath string) (*Config, error) {
	config := &Config{
		App: AppConfig{
			DefaultModel:     "claude-opus-4-5-20251101",
			DefaultProvider:  "claude",
			AutoStartBackend: true,
		},
		UI: UIConfig{
			Theme:             "dark",
			ShowLineNumbers:   true,
			TabSize:           4,
			WordWrap:          true,
			DefaultLayout:     "triple",
			TripleProportions: []float64{0.2, 0.5, 0.3},
			DualProportions:   []float64{0.5, 0.5},
		},
		Providers: map[string]ProviderConfig{
			"claude": {
				APIKeyEnv: "CLAUDE_API_KEY",
				BaseURL:   "https://api.anthropic.com/v1",
				Models: []ModelConfig{
					{
						Name:        "claude-opus-4-5-20251101",
						DisplayName: "Claude Opus 4",
						MaxTokens:   4096,
						ContextSize: 200000,
						Strengths:   []string{"coding", "analysis", "writing", "reasoning"},
					},
				},
			},
		},
		Templates: map[string]SessionTemplate{
			"code_review": {
				Description: "Two-AI code review system",
				Sessions: []SessionConfig{
					{
						Name:         "Developer",
						Model:        "claude-opus-4-5-20251101",
						Role:         "developer",
						SystemPrompt: "You are a senior software developer focused on writing clean, efficient code.",
					},
					{
						Name:         "Reviewer",
						Model:        "claude-opus-4-5-20251101",
						Role:         "reviewer",
						SystemPrompt: "You are a code reviewer focused on security, performance, and best practices.",
					},
				},
				Pipeline: PipelineConfig{
					Type:          "dual",
					Bidirectional: false,
					AutoForward:   true,
				},
				Layout: "dual",
			},
		},
		Keybindings: map[string]map[string]string{
			"global": {
				"ctrl+c": "quit",
				":q":     "quit",
			},
			"normal": {
				"E":      "toggle_explorer",
				"ctrl+t": "new_session",
				"gt":     "next_session",
				"gT":     "prev_session",
				"i":      "insert_mode",
				"v":      "visual_mode",
				":":      "command_mode",
			},
			"visual": {
				"enter":  "send_selection_to_ai",
				"escape": "normal_mode",
			},
		},
		Commands: map[string]string{
			"s":        "session",
			"explain":  "ai explain",
			"fix":      "ai fix",
			"optimize": "ai optimize",
			"review":   "ai review",
		},
		Performance: PerformanceConfig{
			MaxMessagesPerSession: 1000,
			MaxConcurrentRequests: 3,
			RequestTimeout:        60,
			AutoSaveInterval:      300,
		},
		Logging: LoggingConfig{
			Level:         "info",
			Timestamps:    true,
			LogAIRequests: false,
		},
	}

	m.config = config
	m.configPath = configPath

	// Save default config to file
	if err := m.SaveConfig(); err != nil {
		return nil, fmt.Errorf("failed to save default config: %v", err)
	}

	return config, nil
}

// validateConfig validates the configuration
func (m *Manager) validateConfig() error {
	if m.config == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Validate app config
	if m.config.App.DefaultProvider == "" {
		return fmt.Errorf("default_provider cannot be empty")
	}

	// Validate that default provider exists
	if _, exists := m.config.Providers[m.config.App.DefaultProvider]; !exists {
		return fmt.Errorf("default provider '%s' not found in providers", m.config.App.DefaultProvider)
	}

	// Validate UI config
	if m.config.UI.TabSize <= 0 {
		m.config.UI.TabSize = 4
	}

	// Validate proportions
	if len(m.config.UI.TripleProportions) != 3 {
		m.config.UI.TripleProportions = []float64{0.2, 0.5, 0.3}
	}

	if len(m.config.UI.DualProportions) != 2 {
		m.config.UI.DualProportions = []float64{0.5, 0.5}
	}

	// Validate performance config
	if m.config.Performance.MaxMessagesPerSession <= 0 {
		m.config.Performance.MaxMessagesPerSession = 1000
	}

	if m.config.Performance.MaxConcurrentRequests <= 0 {
		m.config.Performance.MaxConcurrentRequests = 3
	}

	if m.config.Performance.RequestTimeout <= 0 {
		m.config.Performance.RequestTimeout = 60
	}

	return nil
}

// getDefaultConfigPath returns the default configuration path
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./claudevim-config.yaml"
	}

	return filepath.Join(homeDir, ".config", "claudevim", "config.yaml")
}

// GetConfigDir returns the configuration directory
func GetConfigDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "."
	}

	return filepath.Join(homeDir, ".config", "claudevim")
}

// InitializeConfig creates the configuration directory and default config
func InitializeConfig() error {
	configDir := GetConfigDir()
	
	// Create config directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Create default config if it doesn't exist
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		manager := NewManager()
		_, err := manager.createDefaultConfig(configPath)
		return err
	}

	return nil
}

// GetSessionTemplate returns a session template by name
func (c *Config) GetSessionTemplate(name string) (SessionTemplate, bool) {
	template, exists := c.Templates[name]
	return template, exists
}

// GetKeybinding returns a keybinding for a specific mode and key
func (c *Config) GetKeybinding(mode, key string) (string, bool) {
	modeBindings, exists := c.Keybindings[mode]
	if !exists {
		return "", false
	}

	action, exists := modeBindings[key]
	return action, exists
}

// GetCommand returns the full command for an alias
func (c *Config) GetCommand(alias string) string {
	if command, exists := c.Commands[alias]; exists {
		return command
	}
	return alias // Return the alias itself if no mapping found
}

// GetProviderConfig returns configuration for a specific provider
func (c *Config) GetProviderConfig(name string) (ProviderConfig, bool) {
	config, exists := c.Providers[name]
	return config, exists
}