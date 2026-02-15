package chain

import (
	"fmt"
	"strings"
)

// SetupFlow manages the two-step chain creation process
type SetupFlow struct {
	Step          int                 `json:"step"`           // 1 or 2
	DSLInput      string             `json:"dsl_input"`      // Step 1: Raw DSL string
	Chain         *Chain             `json:"chain"`          // Parsed chain from step 1
	NodeSetups    map[string]NodeSetup `json:"node_setups"`   // Step 2: Node configurations
	CurrentNodeID string             `json:"current_node_id"` // Which node we're configuring
	Complete      bool               `json:"complete"`       // Setup finished
	
	// Agent management
	AgentLoader   *AgentLoader       `json:"-"`              // Loads pre-configured agents
	AllowedDir    string             `json:"allowed_dir"`    // Directory for .agents
}

// NodeSetup holds configuration for a single node during setup
type NodeSetup struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	SelectedAgent string            `json:"selected_agent"` // ID of selected pre-configured agent
	AgentDef      *AgentDefinition  `json:"agent_def"`      // Full agent definition
	Configured    bool              `json:"configured"`     // Setup complete for this node
}

// AvailableModel represents an AI model option
type AvailableModel struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Provider    string   `json:"provider"`
	Strengths   []string `json:"strengths"`
	ContextSize int      `json:"context_size"`
}

// AvailableRole represents a pre-defined agent role
type AvailableRole struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"`
}

// NewSetupFlow creates a new setup flow
func NewSetupFlow() *SetupFlow {
	return NewSetupFlowWithDir(".")
}

// NewSetupFlowWithDir creates a new setup flow with specified directory
func NewSetupFlowWithDir(allowedDir string) *SetupFlow {
	agentLoader := NewAgentLoader(allowedDir)
	
	return &SetupFlow{
		Step:        1,
		NodeSetups:  make(map[string]NodeSetup),
		Complete:    false,
		AllowedDir:  allowedDir,
		AgentLoader: agentLoader,
	}
}

// ProcessStep1 handles DSL input and parsing
func (s *SetupFlow) ProcessStep1(dslInput string) error {
	s.DSLInput = strings.TrimSpace(dslInput)
	
	// Parse the DSL
	parser := NewDSLParser()
	
	// Validate first
	if err := parser.ValidateDSL(s.DSLInput); err != nil {
		return fmt.Errorf("invalid DSL: %w", err)
	}
	
	// Parse the chain
	chain, err := parser.ParseChainDSL(s.DSLInput)
	if err != nil {
		return fmt.Errorf("failed to parse DSL: %w", err)
	}
	
	s.Chain = chain
	
	// Load available agents
	if err := s.AgentLoader.LoadAgents(); err != nil {
		return fmt.Errorf("failed to load agents: %w", err)
	}

	// Initialize node setups for AI nodes (skip human nodes)
	for _, node := range chain.Nodes {
		if node.Type == NodeTypeAI {
			s.NodeSetups[node.ID] = NodeSetup{
				ID:         node.ID,
				Name:       node.Name,
				Configured: false,
			}
		}
	}
	
	// Move to step 2 and start with first AI node
	s.Step = 2
	s.CurrentNodeID = s.getFirstAINode()
	
	return nil
}

// ConfigureCurrentNode configures the current node with a selected agent
func (s *SetupFlow) ConfigureCurrentNode(agentID string) error {
	if s.Step != 2 || s.CurrentNodeID == "" {
		return fmt.Errorf("invalid setup state")
	}
	
	// Get the selected agent definition
	agent, exists := s.AgentLoader.GetAgent(agentID)
	if !exists {
		return fmt.Errorf("agent '%s' not found", agentID)
	}
	
	// Update node setup
	nodeSetup := s.NodeSetups[s.CurrentNodeID]
	nodeSetup.SelectedAgent = agentID
	nodeSetup.AgentDef = &agent
	nodeSetup.Configured = true
	s.NodeSetups[s.CurrentNodeID] = nodeSetup
	
	// Move to next unconfigured AI node
	s.CurrentNodeID = s.getNextUnconfiguredNode()
	
	// Check if all nodes are configured
	if s.CurrentNodeID == "" {
		s.Complete = true
		s.finalizeChain()
	}
	
	return nil
}

// getFirstAINode returns the ID of the first AI node to configure
func (s *SetupFlow) getFirstAINode() string {
	if s.Chain == nil {
		return ""
	}
	
	for _, node := range s.Chain.Nodes {
		if node.Type == NodeTypeAI {
			return node.ID
		}
	}
	return ""
}

// getNextUnconfiguredNode finds the next AI node that needs configuration
func (s *SetupFlow) getNextUnconfiguredNode() string {
	for _, node := range s.Chain.Nodes {
		if node.Type == NodeTypeAI {
			if setup, exists := s.NodeSetups[node.ID]; exists && !setup.Configured {
				return node.ID
			}
		}
	}
	return ""
}

// GetCurrentNodeSetup returns the setup for the currently configuring node
func (s *SetupFlow) GetCurrentNodeSetup() *NodeSetup {
	if s.CurrentNodeID == "" {
		return nil
	}
	
	if setup, exists := s.NodeSetups[s.CurrentNodeID]; exists {
		return &setup
	}
	return nil
}

// GetProgressInfo returns setup progress information
func (s *SetupFlow) GetProgressInfo() (int, int) {
	total := len(s.NodeSetups)
	configured := 0
	
	for _, setup := range s.NodeSetups {
		if setup.Configured {
			configured++
		}
	}
	
	return configured, total
}

// finalizeChain applies all node configurations to the chain
func (s *SetupFlow) finalizeChain() {
	if s.Chain == nil {
		return
	}
	
	// Update chain nodes with agent configurations
	for i, node := range s.Chain.Nodes {
		if node.Type == NodeTypeAI {
			if setup, exists := s.NodeSetups[node.ID]; exists && setup.AgentDef != nil {
				agent := setup.AgentDef
				s.Chain.Nodes[i].Model = agent.Model
				s.Chain.Nodes[i].Role = agent.Role
				s.Chain.Nodes[i].SystemPrompt = agent.SystemPrompt
				s.Chain.Nodes[i].Name = agent.Name
				s.Chain.Nodes[i].Config = map[string]interface{}{
					"temperature":  agent.Temperature,
					"agent_id":     agent.ID,
					"description":  agent.Description,
					"tags":         agent.Tags,
				}
				// Merge any additional config from agent
				if agent.Config != nil {
					for k, v := range agent.Config {
						s.Chain.Nodes[i].Config[k] = v
					}
				}
			}
		}
	}
	
	s.Chain.Status = StatusReady
}

// GetAvailableModels returns the list of available Claude AI models
func (s *SetupFlow) GetAvailableModels() []AvailableModel {
	return []AvailableModel{
		{
			ID:          "claude-3-5-sonnet-20241022",
			DisplayName: "Claude Sonnet 3.5",
			Provider:    "anthropic",
			Strengths:   []string{"coding", "analysis", "general tasks"},
			ContextSize: 200000,
		},
		{
			ID:          "claude-3-opus-20240229",
			DisplayName: "Claude Opus 3",
			Provider:    "anthropic",
			Strengths:   []string{"complex reasoning", "architecture", "creative tasks"},
			ContextSize: 200000,
		},
		{
			ID:          "claude-3-haiku-20240307",
			DisplayName: "Claude Haiku 3",
			Provider:    "anthropic",
			Strengths:   []string{"speed", "cost-effective", "quick tasks"},
			ContextSize: 200000,
		},
	}
}

// GetAvailableRoles returns pre-defined agent roles
func (s *SetupFlow) GetAvailableRoles() []AvailableRole {
	return []AvailableRole{
		{
			ID:          "developer",
			DisplayName: "Software Developer", 
			Description: "Writes and reviews code, implements features",
			SystemPrompt: "You are an expert software developer. Focus on writing clean, efficient, and well-documented code. Consider best practices, performance, and maintainability.",
		},
		{
			ID:          "architect",
			DisplayName: "System Architect",
			Description: "Designs system architecture and high-level solutions", 
			SystemPrompt: "You are a system architect. Focus on high-level design, scalability, and system integration. Consider trade-offs between different architectural approaches.",
		},
		{
			ID:          "reviewer",
			DisplayName: "Code Reviewer",
			Description: "Reviews code for quality, security, and best practices",
			SystemPrompt: "You are a code reviewer. Carefully analyze code for bugs, security issues, performance problems, and adherence to best practices. Provide constructive feedback.",
		},
		{
			ID:          "security",
			DisplayName: "Security Expert",
			Description: "Focuses on security vulnerabilities and best practices",
			SystemPrompt: "You are a cybersecurity expert. Analyze code and systems for security vulnerabilities, recommend secure coding practices, and ensure data protection.",
		},
		{
			ID:          "debugger",
			DisplayName: "Debug Specialist", 
			Description: "Identifies and fixes bugs and issues",
			SystemPrompt: "You are a debugging specialist. Focus on identifying root causes of issues, analyzing error logs, and providing step-by-step debugging approaches.",
		},
		{
			ID:          "optimizer",
			DisplayName: "Performance Optimizer",
			Description: "Optimizes code and system performance",
			SystemPrompt: "You are a performance optimization expert. Focus on identifying bottlenecks, improving algorithm efficiency, and optimizing system resource usage.",
		},
		{
			ID:          "tester",
			DisplayName: "QA Tester",
			Description: "Creates tests and ensures quality assurance",
			SystemPrompt: "You are a QA expert. Focus on creating comprehensive test cases, identifying edge cases, and ensuring software quality and reliability.",
		},
		{
			ID:          "custom",
			DisplayName: "Custom Role",
			Description: "Define your own custom agent role and instructions",
			SystemPrompt: "",
		},
	}
}

// GetChainVisualization returns a text representation of the chain
func (s *SetupFlow) GetChainVisualization() string {
	if s.Chain == nil {
		return "No chain configured"
	}
	
	var viz strings.Builder
	viz.WriteString(fmt.Sprintf("Chain: %s\n", s.DSLInput))
	viz.WriteString(fmt.Sprintf("Nodes: %d | Connections: %d\n", len(s.Chain.Nodes), len(s.Chain.Connections)))
	viz.WriteString("\nNodes:\n")
	
	for _, node := range s.Chain.Nodes {
		status := "⭕"
		if node.Type == NodeTypeHuman {
			status = "👤"
		} else if setup, exists := s.NodeSetups[node.ID]; exists && setup.Configured {
			status = "✅"
		} else if s.CurrentNodeID == node.ID {
			status = "🔄"
		}
		
		viz.WriteString(fmt.Sprintf("  %s %s (%s) - %s\n", status, node.ID, string(node.Type), node.Name))
	}
	
	viz.WriteString("\nConnections:\n")
	for _, conn := range s.Chain.Connections {
		symbol := "→"
		if conn.Type == ConnTwoWay {
			symbol = "↔"
		}
		viz.WriteString(fmt.Sprintf("  %s %s %s\n", conn.From, symbol, conn.To))
	}
	
	return viz.String()
}

// GetAvailableAgents returns the list of available pre-configured agents
func (s *SetupFlow) GetAvailableAgents() []AgentDefinition {
	if s.AgentLoader == nil {
		return []AgentDefinition{}
	}
	return s.AgentLoader.GetAgentsList()
}

// GetAgentsDirectory returns the path to the .agents directory
func (s *SetupFlow) GetAgentsDirectory() string {
	if s.AgentLoader == nil {
		return ""
	}
	return s.AgentLoader.GetAgentsDir()
}