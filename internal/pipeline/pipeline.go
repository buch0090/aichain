package pipeline

import (
	"fmt"
	"sync"
	"time"

	"github.com/aichain/aichain/internal/session"
)

// Pipeline represents an AI-to-AI communication pipeline
type Pipeline struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Sessions    []SessionNode          `json:"sessions"`
	Rules       []PipelineRule         `json:"rules"`
	Status      PipelineStatus         `json:"status"`
	CreatedAt   time.Time             `json:"created_at"`
	manager     *session.Manager       `json:"-"`
	mu          sync.RWMutex          `json:"-"`
}

// SessionNode represents a session in the pipeline
type SessionNode struct {
	SessionID   string                 `json:"session_id"`
	Role        NodeRole               `json:"role"`        // "input", "processor", "output"
	Triggers    []Trigger              `json:"triggers"`    // When this node activates
	Position    int                    `json:"position"`    // Order in pipeline
}

// NodeRole defines the role of a session in the pipeline
type NodeRole string

const (
	InputNode     NodeRole = "input"
	ProcessorNode NodeRole = "processor"
	OutputNode    NodeRole = "output"
)

// Trigger defines when a pipeline node should activate
type Trigger struct {
	Type      TriggerType            `json:"type"`
	Condition string                 `json:"condition"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// TriggerType defines types of triggers
type TriggerType string

const (
	ManualTrigger   TriggerType = "manual"
	AutoTrigger     TriggerType = "auto"
	KeywordTrigger  TriggerType = "keyword"
	ResponseTrigger TriggerType = "response"
)

// PipelineRule defines how messages flow through the pipeline
type PipelineRule struct {
	Condition   string                 `json:"condition"`   // "on_response", "on_keyword", "manual"
	Action      string                 `json:"action"`      // "forward", "broadcast", "chain"
	Transform   *MessageTransform      `json:"transform"`   // How to modify message before sending
	SourceNode  string                 `json:"source_node"`
	TargetNodes []string               `json:"target_nodes"`
}

// MessageTransform defines how to transform messages between sessions
type MessageTransform struct {
	Prefix    string            `json:"prefix,omitempty"`
	Suffix    string            `json:"suffix,omitempty"`
	Template  string            `json:"template,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// PipelineStatus tracks the current state of a pipeline
type PipelineStatus string

const (
	PipelineActive  PipelineStatus = "active"
	PipelineIdle    PipelineStatus = "idle"
	PipelineStopped PipelineStatus = "stopped"
	PipelineError   PipelineStatus = "error"
)

// Manager handles all pipelines
type Manager struct {
	pipelines      map[string]*Pipeline
	sessionManager *session.Manager
	mu             sync.RWMutex
}

// NewManager creates a new pipeline manager
func NewManager(sessionManager *session.Manager) *Manager {
	return &Manager{
		pipelines:      make(map[string]*Pipeline),
		sessionManager: sessionManager,
	}
}

// CreatePipeline creates a new AI collaboration pipeline
func (m *Manager) CreatePipeline(name string, sessionIDs []string) (*Pipeline, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("pipeline_%d", time.Now().UnixNano())

	// Create session nodes
	nodes := make([]SessionNode, 0, len(sessionIDs))
	for i, sessionID := range sessionIDs {
		// Verify session exists
		_, err := m.sessionManager.GetSession(sessionID)
		if err != nil {
			return nil, fmt.Errorf("session %s not found: %v", sessionID, err)
		}

		role := ProcessorNode
		if i == 0 {
			role = InputNode
		} else if i == len(sessionIDs)-1 {
			role = OutputNode
		}

		node := SessionNode{
			SessionID: sessionID,
			Role:      role,
			Position:  i,
			Triggers: []Trigger{
				{
					Type:      AutoTrigger,
					Condition: "on_message",
				},
			},
		}
		nodes = append(nodes, node)
	}

	pipeline := &Pipeline{
		ID:        id,
		Name:      name,
		Sessions:  nodes,
		Rules:     []PipelineRule{},
		Status:    PipelineIdle,
		CreatedAt: time.Now(),
		manager:   m.sessionManager,
	}

	m.pipelines[id] = pipeline
	return pipeline, nil
}

// GetPipeline retrieves a pipeline by ID
func (m *Manager) GetPipeline(id string) (*Pipeline, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pipeline, exists := m.pipelines[id]
	if !exists {
		return nil, fmt.Errorf("pipeline %s not found", id)
	}
	return pipeline, nil
}

// CreateDualAIPipeline creates a simple dual AI pipeline
func (m *Manager) CreateDualAIPipeline(session1ID, session2ID string, bidirectional bool) (*Pipeline, error) {
	pipeline, err := m.CreatePipeline("Dual AI", []string{session1ID, session2ID})
	if err != nil {
		return nil, err
	}

	// Add forward rule (session1 -> session2)
	pipeline.AddRule(PipelineRule{
		Condition:   "on_response",
		Action:      "forward",
		SourceNode:  session1ID,
		TargetNodes: []string{session2ID},
		Transform: &MessageTransform{
			Prefix: "[From AI Partner]: ",
		},
	})

	// Add backward rule if bidirectional (session2 -> session1)
	if bidirectional {
		pipeline.AddRule(PipelineRule{
			Condition:   "on_response",
			Action:      "forward",
			SourceNode:  session2ID,
			TargetNodes: []string{session1ID},
			Transform: &MessageTransform{
				Prefix: "[From AI Partner]: ",
			},
		})
	}

	return pipeline, nil
}

// CreateDebatePipeline creates a debate pipeline with moderator
func (m *Manager) CreateDebatePipeline(participant1ID, participant2ID, moderatorID string) (*Pipeline, error) {
	sessionIDs := []string{participant1ID, participant2ID}
	if moderatorID != "" {
		sessionIDs = append(sessionIDs, moderatorID)
	}

	pipeline, err := m.CreatePipeline("AI Debate", sessionIDs)
	if err != nil {
		return nil, err
	}

	// Participants talk to each other
	pipeline.AddRule(PipelineRule{
		Condition:   "on_response",
		Action:      "forward",
		SourceNode:  participant1ID,
		TargetNodes: []string{participant2ID},
		Transform: &MessageTransform{
			Prefix: "[Debate Opponent]: ",
		},
	})

	pipeline.AddRule(PipelineRule{
		Condition:   "on_response",
		Action:      "forward",
		SourceNode:  participant2ID,
		TargetNodes: []string{participant1ID},
		Transform: &MessageTransform{
			Prefix: "[Debate Opponent]: ",
		},
	})

	// Moderator receives all messages if present
	if moderatorID != "" {
		pipeline.AddRule(PipelineRule{
			Condition:   "on_response",
			Action:      "broadcast",
			SourceNode:  participant1ID,
			TargetNodes: []string{moderatorID},
			Transform: &MessageTransform{
				Prefix: "[Participant 1]: ",
			},
		})

		pipeline.AddRule(PipelineRule{
			Condition:   "on_response",
			Action:      "broadcast",
			SourceNode:  participant2ID,
			TargetNodes: []string{moderatorID},
			Transform: &MessageTransform{
				Prefix: "[Participant 2]: ",
			},
		})
	}

	return pipeline, nil
}

// AddRule adds a communication rule to the pipeline
func (p *Pipeline) AddRule(rule PipelineRule) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.Rules = append(p.Rules, rule)
}

// ProcessMessage processes a message through the pipeline
func (p *Pipeline) ProcessMessage(sourceSessionID, message string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Find applicable rules for this source session
	for _, rule := range p.Rules {
		if rule.SourceNode == sourceSessionID {
			err := p.executeRule(rule, message)
			if err != nil {
				return fmt.Errorf("failed to execute rule: %v", err)
			}
		}
	}

	return nil
}

// executeRule executes a specific pipeline rule
func (p *Pipeline) executeRule(rule PipelineRule, message string) error {
	// Transform message if transform is specified
	transformedMessage := message
	if rule.Transform != nil {
		transformedMessage = p.applyTransform(rule.Transform, message)
	}

	// Execute action based on rule type
	switch rule.Action {
	case "forward":
		return p.forwardMessage(rule.TargetNodes, transformedMessage)
	case "broadcast":
		return p.broadcastMessage(rule.TargetNodes, transformedMessage)
	case "chain":
		return p.chainMessage(rule.TargetNodes, transformedMessage)
	default:
		return fmt.Errorf("unknown action: %s", rule.Action)
	}
}

// applyTransform applies a message transform
func (p *Pipeline) applyTransform(transform *MessageTransform, message string) string {
	result := message

	if transform.Prefix != "" {
		result = transform.Prefix + result
	}

	if transform.Suffix != "" {
		result = result + transform.Suffix
	}

	if transform.Template != "" {
		// Simple template substitution - could be enhanced
		result = fmt.Sprintf(transform.Template, message)
	}

	return result
}

// forwardMessage forwards a message to target sessions
func (p *Pipeline) forwardMessage(targetSessionIDs []string, message string) error {
	for _, sessionID := range targetSessionIDs {
		session, err := p.manager.GetSession(sessionID)
		if err != nil {
			return fmt.Errorf("target session %s not found: %v", sessionID, err)
		}

		session.AddMessage("user", message, "pipeline")
	}
	return nil
}

// broadcastMessage broadcasts a message to all target sessions
func (p *Pipeline) broadcastMessage(targetSessionIDs []string, message string) error {
	return p.forwardMessage(targetSessionIDs, message)
}

// chainMessage sends a message through sessions in sequence
func (p *Pipeline) chainMessage(targetSessionIDs []string, message string) error {
	// For now, just forward to first session
	// Could be enhanced to wait for response and chain
	if len(targetSessionIDs) > 0 {
		return p.forwardMessage(targetSessionIDs[:1], message)
	}
	return nil
}

// Start activates the pipeline
func (p *Pipeline) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.Status = PipelineActive
}

// Stop deactivates the pipeline
func (p *Pipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.Status = PipelineStopped
}

// GetStatus returns the current pipeline status
func (p *Pipeline) GetStatus() PipelineStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return p.Status
}