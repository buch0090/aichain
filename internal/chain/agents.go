package chain

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentDefinition represents a pre-configured agent from .agents directory
type AgentDefinition struct {
	ID           string            `yaml:"id" json:"id"`
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description" json:"description"`
	Model        string            `yaml:"model" json:"model"`
	Role         string            `yaml:"role" json:"role"`
	SystemPrompt string            `yaml:"system_prompt" json:"system_prompt"`
	Temperature  float64           `yaml:"temperature" json:"temperature"`
	Config       map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	Tags         []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Examples     []string          `yaml:"examples,omitempty" json:"examples,omitempty"`
}

// AgentLoader loads pre-configured agents from .agents directory
type AgentLoader struct {
	agentsDir string
	agents    map[string]AgentDefinition
	loaded    bool
}

// NewAgentLoader creates a new agent loader
func NewAgentLoader(allowedDir string) *AgentLoader {
	agentsDir := filepath.Join(allowedDir, ".agents")
	return &AgentLoader{
		agentsDir: agentsDir,
		agents:    make(map[string]AgentDefinition),
		loaded:    false,
	}
}

// LoadAgents loads all agent definitions from .agents directory
func (a *AgentLoader) LoadAgents() error {
	// Check if .agents directory exists
	if _, err := os.Stat(a.agentsDir); os.IsNotExist(err) {
		// Create default agents directory with examples
		if err := a.createDefaultAgents(); err != nil {
			return fmt.Errorf("failed to create default agents: %w", err)
		}
	}

	// Walk through .agents directory
	err := filepath.WalkDir(a.agentsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process .yaml and .yml files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		// Load agent definition
		agent, err := a.loadAgentFile(path)
		if err != nil {
			fmt.Printf("Warning: Failed to load agent %s: %v\n", path, err)
			return nil // Continue with other agents
		}

		a.agents[agent.ID] = agent
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk agents directory: %w", err)
	}

	a.loaded = true
	return nil
}

// loadAgentFile loads a single agent definition file
func (a *AgentLoader) loadAgentFile(filePath string) (AgentDefinition, error) {
	var agent AgentDefinition

	data, err := os.ReadFile(filePath)
	if err != nil {
		return agent, fmt.Errorf("failed to read file: %w", err)
	}

	err = yaml.Unmarshal(data, &agent)
	if err != nil {
		return agent, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Set ID from filename if not specified
	if agent.ID == "" {
		filename := filepath.Base(filePath)
		agent.ID = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	// Validate required fields
	if agent.Name == "" {
		return agent, fmt.Errorf("agent name is required")
	}
	if agent.Model == "" {
		return agent, fmt.Errorf("agent model is required")
	}

	// Set defaults
	if agent.Temperature == 0 {
		agent.Temperature = 0.7
	}
	if agent.Role == "" {
		agent.Role = "assistant"
	}

	return agent, nil
}

// GetAgents returns all loaded agents
func (a *AgentLoader) GetAgents() map[string]AgentDefinition {
	if !a.loaded {
		// Try to load agents if not already loaded
		if err := a.LoadAgents(); err != nil {
			return make(map[string]AgentDefinition)
		}
	}
	return a.agents
}

// GetAgentsList returns agents as a sorted list
func (a *AgentLoader) GetAgentsList() []AgentDefinition {
	agents := a.GetAgents()
	var list []AgentDefinition
	
	for _, agent := range agents {
		list = append(list, agent)
	}
	
	return list
}

// GetAgent returns a specific agent by ID
func (a *AgentLoader) GetAgent(id string) (AgentDefinition, bool) {
	agents := a.GetAgents()
	agent, exists := agents[id]
	return agent, exists
}

// createDefaultAgents creates example agent definitions
func (a *AgentLoader) createDefaultAgents() error {
	// Create .agents directory
	if err := os.MkdirAll(a.agentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create agents directory: %w", err)
	}

	// Default agents to create
	defaultAgents := []AgentDefinition{
		{
			ID:          "developer",
			Name:        "Software Developer",
			Description: "Expert at writing, reviewing, and debugging code",
			Model:       "claude-3-5-sonnet-20241022",
			Role:        "developer",
			SystemPrompt: `You are an expert software developer with deep knowledge of multiple programming languages and best practices. 

Your responsibilities:
- Write clean, efficient, and well-documented code
- Follow established coding standards and conventions  
- Consider performance, security, and maintainability
- Provide clear explanations of your code decisions
- Suggest improvements and optimizations when appropriate

Focus on practical solutions that solve real problems effectively.`,
			Temperature: 0.3,
			Tags:        []string{"coding", "development", "debugging"},
		},
		{
			ID:          "architect",
			Name:        "System Architect", 
			Description: "Designs high-level system architecture and technical solutions",
			Model:       "claude-3-opus-20240229",
			Role:        "architect",
			SystemPrompt: `You are a senior system architect with extensive experience designing scalable, maintainable software systems.

Your expertise includes:
- High-level system design and architecture patterns
- Technology selection and trade-off analysis
- Scalability and performance considerations
- Security architecture and best practices
- Integration patterns and API design
- Cloud architecture and microservices

Provide comprehensive architectural guidance that balances technical requirements with business needs.`,
			Temperature: 0.4,
			Tags:        []string{"architecture", "design", "scalability"},
		},
		{
			ID:          "reviewer", 
			Name:        "Code Reviewer",
			Description: "Thorough code review focusing on quality, security, and best practices",
			Model:       "claude-3-5-sonnet-20241022",
			Role:        "reviewer",
			SystemPrompt: `You are a meticulous code reviewer with a keen eye for quality, security, and best practices.

Your review focuses on:
- Code correctness and logic errors
- Security vulnerabilities and potential exploits  
- Performance issues and optimization opportunities
- Adherence to coding standards and conventions
- Code maintainability and readability
- Test coverage and quality
- Documentation completeness

Provide constructive feedback with specific suggestions for improvement.`,
			Temperature: 0.2,
			Tags:        []string{"review", "quality", "security"},
		},
		{
			ID:          "debugger",
			Name:        "Debug Specialist",
			Description: "Expert at identifying and fixing bugs and issues",
			Model:       "claude-3-5-sonnet-20241022", 
			Role:        "debugger",
			SystemPrompt: `You are a debugging specialist with exceptional skills in identifying, analyzing, and resolving software issues.

Your debugging approach:
- Systematic analysis of error messages and stack traces
- Root cause analysis using logical reasoning
- Hypothesis-driven debugging strategies
- Knowledge of common bug patterns and pitfalls
- Proficiency with debugging tools and techniques
- Clear step-by-step troubleshooting guidance

Help developers understand not just how to fix issues, but why they occurred and how to prevent them.`,
			Temperature: 0.3,
			Tags:        []string{"debugging", "troubleshooting", "analysis"},
		},
		{
			ID:          "security",
			Name:        "Security Expert",
			Description: "Cybersecurity specialist focused on secure coding and vulnerability assessment",
			Model:       "claude-3-5-sonnet-20241022",
			Role:        "security",
			SystemPrompt: `You are a cybersecurity expert specializing in application security and secure coding practices.

Your security focus areas:
- Common vulnerabilities (OWASP Top 10)
- Secure coding practices and patterns
- Authentication and authorization mechanisms
- Data protection and encryption
- Input validation and sanitization
- Security testing and penetration testing
- Compliance and regulatory requirements

Provide actionable security recommendations that developers can implement effectively.`,
			Temperature: 0.2,
			Tags:        []string{"security", "vulnerability", "compliance"},
		},
		{
			ID:          "tester",
			Name:        "QA Tester", 
			Description: "Quality assurance expert focused on comprehensive testing strategies",
			Model:       "claude-3-haiku-20240307",
			Role:        "tester",
			SystemPrompt: `You are a QA expert with deep knowledge of software testing methodologies and best practices.

Your testing expertise covers:
- Test case design and test planning
- Unit, integration, and end-to-end testing
- Edge case identification and boundary testing
- Performance and load testing considerations
- Test automation strategies
- Bug reporting and defect tracking
- Quality metrics and testing standards

Create comprehensive testing strategies that ensure robust, reliable software delivery.`,
			Temperature: 0.4,
			Tags:        []string{"testing", "qa", "quality"},
		},
	}

	// Write each default agent to a file
	for _, agent := range defaultAgents {
		filePath := filepath.Join(a.agentsDir, agent.ID+".yaml")
		if err := a.writeAgentFile(filePath, agent); err != nil {
			return fmt.Errorf("failed to write default agent %s: %w", agent.ID, err)
		}
	}

	fmt.Printf("✅ Created default agents in %s\n", a.agentsDir)
	return nil
}

// writeAgentFile writes an agent definition to a YAML file
func (a *AgentLoader) writeAgentFile(filePath string, agent AgentDefinition) error {
	data, err := yaml.Marshal(agent)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// GetAgentsDir returns the agents directory path
func (a *AgentLoader) GetAgentsDir() string {
	return a.agentsDir
}