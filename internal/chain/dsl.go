package chain

import (
	"fmt"
	"regexp"
	"strings"
)

// ChainNode represents a single node in the AI chain
type ChainNode struct {
	ID          string            `json:"id"`           // A, B, C, *, etc.
	Name        string            `json:"name"`         // Human-readable name
	Type        NodeType          `json:"type"`         // AI, Human, Choice
	Model       string            `json:"model"`        // claude-3-5-sonnet, gpt-4, etc.
	Role        string            `json:"role"`         // developer, reviewer, architect, etc.
	SystemPrompt string           `json:"system_prompt"`
	Config      map[string]interface{} `json:"config"` // Node-specific configuration
}

// Connection represents a connection between nodes
type Connection struct {
	From        string         `json:"from"`         // Source node ID
	To          string         `json:"to"`           // Target node ID  
	Type        ConnectionType `json:"type"`         // OneWay, TwoWay, Choice
	Condition   string         `json:"condition"`    // When to trigger (optional)
}

// Chain represents the complete AI chain
type Chain struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	DSL         string       `json:"dsl"`          // Original DSL string
	Nodes       []ChainNode  `json:"nodes"`
	Connections []Connection `json:"connections"`
	Status      ChainStatus  `json:"status"`
}

// NodeType defines the type of node
type NodeType string

const (
	NodeTypeAI     NodeType = "ai"
	NodeTypeHuman  NodeType = "human"
	NodeTypeChoice NodeType = "choice"   // For A,B -> * choice scenarios
)

// ConnectionType defines the type of connection
type ConnectionType string

const (
	ConnOneWay ConnectionType = "one_way"   // A -> B
	ConnTwoWay ConnectionType = "two_way"   // A <> B  
	ConnChoice ConnectionType = "choice"    // A,B -> * (human chooses)
)

// ChainStatus represents the current state of the chain
type ChainStatus string

const (
	StatusCreating  ChainStatus = "creating"
	StatusConfiguring ChainStatus = "configuring" 
	StatusReady     ChainStatus = "ready"
	StatusRunning   ChainStatus = "running"
	StatusPaused    ChainStatus = "paused"
	StatusComplete  ChainStatus = "complete"
)

// DSLParser parses the AI chaining DSL
type DSLParser struct{}

// NewDSLParser creates a new DSL parser
func NewDSLParser() *DSLParser {
	return &DSLParser{}
}

// ParseChainDSL parses a DSL string and returns a Chain structure
func (p *DSLParser) ParseChainDSL(dsl string) (*Chain, error) {
	dsl = strings.TrimSpace(dsl)
	if dsl == "" {
		return nil, fmt.Errorf("empty DSL string")
	}

	chain := &Chain{
		DSL:         dsl,
		Nodes:       []ChainNode{},
		Connections: []Connection{},
		Status:      StatusCreating,
	}

	// Extract all unique node IDs
	nodeIDs := p.extractNodeIDs(dsl)
	
	// Create nodes
	for _, id := range nodeIDs {
		nodeType := NodeTypeAI
		if id == "*" {
			nodeType = NodeTypeHuman
		}
		
		chain.Nodes = append(chain.Nodes, ChainNode{
			ID:   id,
			Type: nodeType,
			Name: p.generateNodeName(id, nodeType),
		})
	}

	// Parse connections
	connections, err := p.parseConnections(dsl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connections: %w", err)
	}
	chain.Connections = connections

	return chain, nil
}

// extractNodeIDs finds all unique node identifiers in the DSL
func (p *DSLParser) extractNodeIDs(dsl string) []string {
	// Regex to match node identifiers (letters, *, numbers)
	re := regexp.MustCompile(`[A-Za-z0-9*]+`)
	matches := re.FindAllString(dsl, -1)
	
	// Remove duplicates and connection symbols
	seen := make(map[string]bool)
	var nodeIDs []string
	
	for _, match := range matches {
		// Skip connection symbols
		if match == "-" || match == "<" || match == ">" {
			continue
		}
		
		if !seen[match] {
			seen[match] = true
			nodeIDs = append(nodeIDs, match)
		}
	}
	
	return nodeIDs
}

// parseConnections extracts connections from DSL string
func (p *DSLParser) parseConnections(dsl string) ([]Connection, error) {
	// Tokenize into node IDs and operators. Using non-overlapping regex on pairs
	// fails for chains like "A <> B <> C" because "B" is consumed by the first
	// match and unavailable for the second. Tokenizing first solves this.
	tokenRe := regexp.MustCompile(`([A-Za-z0-9*]+|<>|->|<-)`)
	tokens := tokenRe.FindAllString(dsl, -1)

	// Walk token triples: node op node op node ...
	// tokens[0]=node, tokens[1]=op, tokens[2]=node, tokens[3]=op, tokens[4]=node ...
	var connections []Connection
	for i := 1; i+1 < len(tokens); i += 2 {
		op   := tokens[i]
		from := tokens[i-1]
		to   := tokens[i+1]

		switch op {
		case "<>":
			connections = append(connections, Connection{From: from, To: to, Type: ConnTwoWay})
		case "->":
			connections = append(connections, Connection{From: from, To: to, Type: ConnOneWay})
		case "<-":
			connections = append(connections, Connection{From: to, To: from, Type: ConnOneWay})
		default:
			return nil, fmt.Errorf("unknown operator %q between %q and %q", op, from, to)
		}
	}

	return connections, nil
}

// generateNodeName creates a human-readable name for a node
func (p *DSLParser) generateNodeName(id string, nodeType NodeType) string {
	if id == "*" {
		return "Human"
	}
	
	switch nodeType {
	case NodeTypeAI:
		return fmt.Sprintf("AI Agent %s", id)
	case NodeTypeChoice:
		return fmt.Sprintf("Choice Point %s", id)
	default:
		return fmt.Sprintf("Node %s", id)
	}
}

// ValidateDSL validates a DSL string for syntax errors
func (p *DSLParser) ValidateDSL(dsl string) error {
	if strings.TrimSpace(dsl) == "" {
		return fmt.Errorf("DSL cannot be empty")
	}
	
	// Check for valid connection patterns
	validPattern := regexp.MustCompile(`^[A-Za-z0-9*\s<>-]+$`)
	if !validPattern.MatchString(dsl) {
		return fmt.Errorf("DSL contains invalid characters")
	}
	
	// Ensure at least one connection exists
	hasConnection := regexp.MustCompile(`(<>|->|<-)`)
	if !hasConnection.MatchString(dsl) {
		return fmt.Errorf("DSL must contain at least one connection (-> or <> or <-)")
	}
	
	return nil
}

// Examples of valid DSL strings:
// "A -> B"                    // Simple chain
// "A <> B"                    // Two-way communication  
// "A -> B -> C"               // Linear chain
// "A <> B <> C <> D"          // Bi-directional chain
// "A -> B <- C"               // B receives from both A and C
// "A -> * <- B"               // Human in the middle
// "A <> *"                    // Human can communicate with A
// "A -> B, C -> *"            // A and C send to different targets