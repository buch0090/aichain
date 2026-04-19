package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aichain/aichain/internal/ai"
	"github.com/aichain/aichain/internal/app"
	"github.com/aichain/aichain/internal/chain"
	"github.com/aichain/aichain/internal/tools"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var debugLogger *log.Logger

func init() {
	// Create debug log file
	debugFile, err := os.OpenFile("claudevim-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to create debug log: %v", err)
		debugLogger = log.New(os.Stderr, "DEBUG: ", log.Ltime|log.Lshortfile)
	} else {
		debugLogger = log.New(debugFile, "DEBUG: ", log.Ltime|log.Lshortfile)
	}
}

// ChainExecutionModel handles the chain execution UI with agent panes
type ChainExecutionModel struct {
	app            *app.Application
	chain          *chain.Chain
	aiManager      *ai.Manager
	toolManager    *tools.ToolManager
	workingDir     string  // Directory where AIs can access files
	Width          int
	Height         int

	// Message input at bottom
	messageInput   textinput.Model
	inputFocused   bool

	// Agent panes
	AgentPanes     []*AgentPane
	activeAgent    int

	// Channel-based agent communication
	ChainAgents    map[string]*ChainAgent
	agentCtx       context.Context
	agentCancel    context.CancelFunc
	
	// UI message channel for agent responses
	uiMsgChan      chan AgentResponseMsg

	// UI State
	styles         ChainExecutionStyles
}

// AgentResponseMsg is sent when an agent provides a response
type AgentResponseMsg struct {
	AgentIndex int
	Message    AgentMessage
}

// RefreshUIMsg is sent periodically to refresh the UI
type RefreshUIMsg struct{}

// AgentPane represents a pane for a single agent
type AgentPane struct {
	Agent          *chain.ChainNode
	Messages       []AgentMessage
	Status         AgentStatus
	LastActivity   string
	
	// Viewport for scrolling content
	Viewport       viewport.Model
}

// AgentMessage represents a message in an agent pane
type AgentMessage struct {
	Role      string    // "user", "assistant", "system"
	Content   string
	Timestamp string
	Source    string    // Which agent/human sent this
}

// AgentStatus represents the current status of an agent
type AgentStatus int

const (
	AgentIdle AgentStatus = iota
	AgentThinking
	AgentResponding
	AgentError
)

// ChainAgent represents an agent with channel communication capabilities
type ChainAgent struct {
	ID         string
	Node       *chain.ChainNode
	Pane       *AgentPane
	AgentIndex int  // Index in AgentPanes slice for UI callbacks
	
	// Channels for inter-agent communication  
	InChan     chan AgentMessage
	OutChans   map[string]chan AgentMessage
	
	// Agent processing
	aiManager  *ai.Manager
	workingDir string
	
	// UI Communication - callback to send messages to TUI
	UICallback func(tea.Msg) tea.Cmd
	
	// Control
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
}

// ChainExecutionStyles holds styling for chain execution UI
type ChainExecutionStyles struct {
	Title          lipgloss.Style
	AgentPane      lipgloss.Style
	ActivePane     lipgloss.Style
	Message        lipgloss.Style
	UserMessage    lipgloss.Style          // Messages from human - bright green
	AssistantMessage lipgloss.Style        // Agent's own responses - blue
	SystemMessage  lipgloss.Style          // System messages - orange/red
	InterAgentMessage lipgloss.Style       // Messages FROM other agents - purple
	Input          lipgloss.Style
	StatusIdle     lipgloss.Style
	StatusThinking lipgloss.Style
	StatusError    lipgloss.Style
}

// NewChainExecutionModel creates a new chain execution model
func NewChainExecutionModel(app *app.Application, completedChain *chain.Chain) ChainExecutionModel {
	// Get working directory from app config
	workingDir := "."
	if config := app.GetConfig(); config != nil && config.AllowedDirectory != "" {
		workingDir = config.AllowedDirectory
	}

	// Initialize AI manager and Claude provider
	aiManager := ai.NewManager()
	claudeProvider := ai.NewClaudeProvider()
	if claudeProvider != nil {
		aiManager.RegisterProvider("claude", claudeProvider)
	}

	// Initialize tool manager
	toolManager := tools.NewToolManager()

	// Create message input
	messageInput := textinput.New()
	messageInput.Placeholder = "Type your message here..."
	messageInput.Focus()
	messageInput.CharLimit = 1000
	messageInput.Width = 80

	// Create agent panes for each AI node
	var agentPanes []*AgentPane
	for i, node := range completedChain.Nodes {
		if node.Type == chain.NodeTypeAI {
			vp := viewport.New(30, 15) // Initial dimensions, will be updated later
			vp.SetContent("No messages yet...")
			
			pane := &AgentPane{
				Agent:        &completedChain.Nodes[i],
				Messages:     []AgentMessage{},
				Status:       AgentIdle,
				LastActivity: "Ready",
				Viewport:     vp,
			}
			agentPanes = append(agentPanes, pane)
		}
	}

	// Create agent context for managing goroutines
	agentCtx, agentCancel := context.WithCancel(context.Background())

	model := ChainExecutionModel{
		app:          app,
		chain:        completedChain,
		aiManager:    aiManager,
		toolManager:  toolManager,
		workingDir:   workingDir,
		messageInput: messageInput,
		inputFocused: true,
		AgentPanes:   agentPanes,
		activeAgent:  0,
		ChainAgents:  make(map[string]*ChainAgent),
		agentCtx:     agentCtx,
		agentCancel:  agentCancel,
		styles:       createChainExecutionStyles(),
		Width:        80,  // Default width
		Height:       24,  // Default height
	}
	
	// Setup channel-based agent communication
	model.setupAgentChannels()
	
	return model
}

// Update handles chain execution messages
func (m ChainExecutionModel) Update(msg tea.Msg) (ChainExecutionModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.updateLayout()

	case RefreshUIMsg:
		// Periodic UI refresh - update all pane content to reflect changes from goroutines
		for _, pane := range m.AgentPanes {
			m.updatePaneContent(pane)
		}
		// Schedule next refresh
		cmd = m.scheduleRefresh()

	case AgentResponseMsg:
		// Handle agent responses from background processing
		if msg.AgentIndex >= 0 && msg.AgentIndex < len(m.AgentPanes) {
			pane := m.AgentPanes[msg.AgentIndex]
			pane.Messages = append(pane.Messages, msg.Message)
			pane.Status = AgentIdle
			pane.LastActivity = "Ready"
			
			// Update viewport content
			m.updatePaneContent(pane)
		}

	case tea.KeyMsg:
		if m.inputFocused {
			return m.updateInput(msg)
		} else {
			// Handle viewport scrolling
			if m.activeAgent < len(m.AgentPanes) {
				vp := &m.AgentPanes[m.activeAgent].Viewport
				
				// Pass keys to active viewport first
				var vcmd tea.Cmd
				
				// Manual viewport scrolling
				switch msg.String() {
				case "up", "k":
					vp.LineUp(1)
				case "down", "j":
					vp.LineDown(1)
				case "pgup":
					vp.ViewUp()
				case "pgdown":
					vp.ViewDown()
				case "home":
					vp.GotoTop()
				case "end":
					vp.GotoBottom()
				default:
					// For other keys, try the viewport Update method
					*vp, vcmd = vp.Update(msg)
				}
				
				if vcmd != nil {
					cmd = vcmd
				}
			}
			
			// Handle navigation keys that viewport doesn't use
			navModel, navCmd := m.updateNavigation(msg)
			if navCmd != nil {
				cmd = navCmd
			}
			return navModel, cmd
		}
	
	case tea.MouseMsg:
		// Pass mouse events to the active viewport for scrolling
		if !m.inputFocused && m.activeAgent < len(m.AgentPanes) {
			var vcmd tea.Cmd
			m.AgentPanes[m.activeAgent].Viewport, vcmd = m.AgentPanes[m.activeAgent].Viewport.Update(msg)
			if vcmd != nil {
				cmd = vcmd
			}
		}
	
	default:
		// Pass other messages to the active viewport
		if !m.inputFocused && m.activeAgent < len(m.AgentPanes) {
			var vcmd tea.Cmd
			m.AgentPanes[m.activeAgent].Viewport, vcmd = m.AgentPanes[m.activeAgent].Viewport.Update(msg)
			if vcmd != nil {
				cmd = vcmd
			}
		}
	}

	return m, cmd
}

// updateInput handles input when message input is focused
func (m ChainExecutionModel) updateInput(msg tea.KeyMsg) (ChainExecutionModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "enter":
		// Send message to chain
		message := strings.TrimSpace(m.messageInput.Value())
		debugLogger.Printf("Enter pressed in updateInput, message: '%s', empty: %v", message, message == "")
		if message != "" {
			debugLogger.Printf("Calling sendMessageToChain with message: '%s'", message)
			cmd = m.sendMessageToChain(message)
			m.messageInput.SetValue("")
		}
		return m, cmd

	case "tab":
		// Switch focus to agent panes
		m.inputFocused = false
		m.messageInput.Blur()
		return m, nil
	}

	// Update message input
	m.messageInput, cmd = m.messageInput.Update(msg)
	return m, cmd
}

// updateNavigation handles navigation when not in input mode
func (m ChainExecutionModel) updateNavigation(msg tea.KeyMsg) (ChainExecutionModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "enter":
		// Switch focus back to input
		m.inputFocused = true
		m.messageInput.Focus()
		return m, nil
	
	case "tab":
		// Tab cycles through agent panes (stay in pane mode)
		if len(m.AgentPanes) > 0 {
			m.activeAgent = (m.activeAgent + 1) % len(m.AgentPanes)
		}
		return m, nil

	case "left", "h":
		if m.activeAgent > 0 {
			m.activeAgent--
		}
		return m, nil

	case "right", "l":
		if m.activeAgent < len(m.AgentPanes)-1 {
			m.activeAgent++
		}
		return m, nil

	// Remove viewport scrolling keys - let viewport handle them directly
	// Only keep navigation keys that viewport doesn't use
	}

	return m, nil
}

// UpdateLayout updates the layout dimensions (public method for testing)
func (m *ChainExecutionModel) UpdateLayout() {
	m.updateLayout()
}

// View renders the chain execution UI
func (m ChainExecutionModel) View() string {
	if m.chain == nil {
		return "No chain loaded"
	}

	if m.Width == 0 || m.Height == 0 {
		return fmt.Sprintf("Initializing Chain Execution... (Width: %d, Height: %d)", m.Width, m.Height)
	}

	var b strings.Builder

	// Title
	title := m.styles.Title.Render("🔗 AIChain - AI Chain Execution")
	b.WriteString(title + "\n\n")

	// Chain info
	chainInfo := fmt.Sprintf("Chain: %s | Agents: %d | Status: %s", 
		m.chain.Name, len(m.AgentPanes), m.chain.Status)
	b.WriteString(chainInfo + "\n\n")

	// Agent panes
	if len(m.AgentPanes) > 0 {
		b.WriteString(m.renderAgentPanes())
	}

	// Message input area
	b.WriteString("\n" + m.renderMessageInput())

	// Footer
	footer := "Tab: Switch focus • ←→/h/l: Navigate agents • ↑↓/j/k/PgUp/PgDn: Scroll • Mouse: Scroll • Enter: Send message • Ctrl+C: Quit"
	b.WriteString("\n" + footer)

	return b.String()
}

// renderAgentPanes renders all agent panes
func (m ChainExecutionModel) renderAgentPanes() string {
	if len(m.AgentPanes) == 0 {
		return "No agents configured"
	}

	// Calculate pane dimensions - ensure they fit exactly
	availableWidth := m.Width - 2 // Leave small margin
	paneWidth := availableWidth / len(m.AgentPanes)
	
	// Ensure minimum width and account for borders
	if paneWidth < 25 {
		paneWidth = 25
	}
	
	// Adjust for border padding (each pane has 2 chars border + 2 chars padding = 4 total)
	contentWidth := paneWidth - 4
	if contentWidth < 15 {
		contentWidth = 15
	}

	paneHeight := m.Height - 10 // Leave space for title, input, footer
	if paneHeight < 8 {
		paneHeight = 8
	}

	// Update viewport dimensions for each pane
	for _, pane := range m.AgentPanes {
		pane.Viewport.Width = contentWidth
		pane.Viewport.Height = paneHeight - 4 // Account for header, separator, status
		
		// Don't refresh content here - it resets scroll position!
		// Only refresh content when messages actually change
	}

	// Render panes side by side with consistent sizing
	var paneContents []string
	for i, pane := range m.AgentPanes {
		isActive := i == m.activeAgent
		content := m.renderAgentPane(pane, isActive, paneWidth)
		paneContents = append(paneContents, content)
	}

	// Join panes horizontally with no spacing
	return lipgloss.JoinHorizontal(lipgloss.Top, paneContents...)
}

// renderAgentPane renders a single agent pane using viewport
func (m ChainExecutionModel) renderAgentPane(pane *AgentPane, isActive bool, fixedWidth int) string {
	var b strings.Builder

	// Pane header
	status := m.getStatusIcon(pane.Status)
	header := fmt.Sprintf("%s %s", status, pane.Agent.Name)
	b.WriteString(header + "\n")

	// Agent info
	agentInfo := fmt.Sprintf("Model: %s | Role: %s", pane.Agent.Model, pane.Agent.Role)
	b.WriteString(agentInfo + "\n")
	b.WriteString(strings.Repeat("─", fixedWidth-4) + "\n")

	// Viewport content (handles scrolling automatically)
	viewportContent := pane.Viewport.View()
	b.WriteString(viewportContent)
	
	// Status line with scroll indicators from viewport
	b.WriteString("\n" + strings.Repeat("─", fixedWidth-4) + "\n")
	statusStyle := m.getStatusStyle(pane.Status)
	
	// Create status line with viewport scroll info
	scrollInfo := ""
	if pane.Viewport.TotalLineCount() > pane.Viewport.Height {
		scrollPercent := int((float64(pane.Viewport.YOffset) / float64(pane.Viewport.TotalLineCount() - pane.Viewport.Height)) * 100)
		scrollInfo = fmt.Sprintf(" [%d%%]", scrollPercent)
		
		// Add scroll indicators
		if pane.Viewport.YOffset > 0 {
			scrollInfo = "↑" + scrollInfo
		}
		if pane.Viewport.YOffset < pane.Viewport.TotalLineCount() - pane.Viewport.Height {
			scrollInfo += "↓"
		}
	}
	
	statusText := pane.LastActivity + scrollInfo
	statusLine := statusStyle.Render(statusText)
	b.WriteString(statusLine)

	// Apply pane styling with fixed width
	content := b.String()
	if isActive {
		return m.styles.ActivePane.Width(fixedWidth).Render(content)
	}
	return m.styles.AgentPane.Width(fixedWidth).Render(content)
}

// renderMessageInput renders the message input area
func (m ChainExecutionModel) renderMessageInput() string {
	var b strings.Builder

	b.WriteString(strings.Repeat("═", m.Width-2) + "\n")
	b.WriteString("Message: ")
	
	if m.inputFocused {
		b.WriteString(m.styles.Input.Render(m.messageInput.View()))
	} else {
		b.WriteString(m.messageInput.View())
	}

	return b.String()
}

// updatePaneContent updates a pane's viewport with current messages
func (m *ChainExecutionModel) updatePaneContent(pane *AgentPane) {
	// Remember the current scroll position
	oldYOffset := pane.Viewport.YOffset
	wasAtBottom := oldYOffset >= pane.Viewport.TotalLineCount() - pane.Viewport.Height

	if len(pane.Messages) == 0 {
		pane.Viewport.SetContent("No messages yet...")
		return
	}

	var contentLines []string
	for _, msg := range pane.Messages {
		// Format message with role indicator and styling
		msgStyle := m.getMessageStyle(msg.Role, msg.Source, pane.Agent.Name)

		// Render header — inter-agent messages get a separator line, others get a bracketed label
		if msg.Role == "assistant" && msg.Source != pane.Agent.Name && msg.Source != "human" {
			label := fmt.Sprintf(" from %s ", msg.Source)
			dashCount := pane.Viewport.Width - 4 - len(label) - len(msg.Timestamp)
			if dashCount < 2 {
				dashCount = 2
			}
			separator := "──" + label + strings.Repeat("─", dashCount) + " " + msg.Timestamp
			contentLines = append(contentLines, msgStyle.Render(separator))
		} else {
			var roleDisplay string
			if msg.Role == "user" {
				roleDisplay = "👤 Human"
			} else {
				roleDisplay = msg.Role
			}
			contentLines = append(contentLines, msgStyle.Render(fmt.Sprintf("[%s] %s", roleDisplay, msg.Timestamp)))
		}
		
		// Wrap and add message content with styling and code block detection
		content := m.wrapText(msg.Content, pane.Viewport.Width-2)
		lines := strings.Split(content, "\n")
		
		inCodeBlock := false
		for _, line := range lines {
			// Detect code block markers
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inCodeBlock = !inCodeBlock
				// Style code block delimiters differently
				codeStyle := msgStyle.Copy().Foreground(lipgloss.Color("#888888")).Italic(true)
				contentLines = append(contentLines, codeStyle.Render(line))
			} else if inCodeBlock {
				// Style code content
				codeStyle := msgStyle.Copy().Foreground(lipgloss.Color("#00ff88")).Background(lipgloss.Color("#1a1a1a"))
				contentLines = append(contentLines, codeStyle.Render(line))
			} else {
				// Regular content
				contentLines = append(contentLines, msgStyle.Render(line))
			}
		}
		
		// Add separator between messages
		contentLines = append(contentLines, "")
	}

	// Set viewport content
	pane.Viewport.SetContent(strings.Join(contentLines, "\n"))
	
	// Only auto-scroll to bottom if we were already at the bottom (preserve user scroll position)
	if wasAtBottom {
		pane.Viewport.GotoBottom()
	} else {
		// Try to restore the previous scroll position
		if oldYOffset <= pane.Viewport.TotalLineCount() - pane.Viewport.Height {
			pane.Viewport.YOffset = oldYOffset
		}
	}
}

// setupAgentChannels creates ChainAgents with channels based on DSL connections
func (m *ChainExecutionModel) setupAgentChannels() {
	debugLogger.Printf("Setting up agent channels for chain: %s", m.chain.DSL)
	
	// Create ChainAgents for each AI node
	for i, node := range m.chain.Nodes {
		if node.Type == chain.NodeTypeAI {
			ctx, cancel := context.WithCancel(m.agentCtx)
			
			// Find the corresponding AgentPane and its index
			var pane *AgentPane
			var agentIndex int = -1
			for j, ap := range m.AgentPanes {
				if ap.Agent.ID == node.ID {
					pane = ap
					agentIndex = j
					break
				}
			}
			
			agent := &ChainAgent{
				ID:         node.ID,
				Node:       &m.chain.Nodes[i],
				Pane:       pane,
				AgentIndex: agentIndex,
				InChan:     make(chan AgentMessage, 10),  // Buffered channel
				OutChans:   make(map[string]chan AgentMessage),
				aiManager:  m.aiManager,
				workingDir: m.workingDir,
				UICallback: func(msg tea.Msg) tea.Cmd {
					// Simple callback that returns the message as a command
					return func() tea.Msg { return msg }
				},
				ctx:        ctx,
				cancel:     cancel,
			}
			
			m.ChainAgents[node.ID] = agent
			debugLogger.Printf("Created ChainAgent: %s", node.ID)
		}
	}
	
	// Wire up channels based on DSL connections  
	for _, conn := range m.chain.Connections {
		fromAgent := m.ChainAgents[conn.From]
		toAgent := m.ChainAgents[conn.To]
		
		if fromAgent != nil && toAgent != nil {
			// Connect output of fromAgent to input of toAgent
			fromAgent.OutChans[conn.To] = toAgent.InChan
			debugLogger.Printf("Connected %s -> %s", conn.From, conn.To)
			
			// If bidirectional, also connect reverse  
			if conn.Type == chain.ConnTwoWay {
				toAgent.OutChans[conn.From] = fromAgent.InChan
				debugLogger.Printf("Connected %s <-> %s (bidirectional)", conn.From, conn.To)
			}
		}
	}
	
	// Start agent goroutines
	for _, agent := range m.ChainAgents {
		go agent.Run()
		debugLogger.Printf("Started goroutine for agent: %s", agent.ID)
	}
}

// Run starts the ChainAgent goroutine for processing messages
func (a *ChainAgent) Run() {
	debugLogger.Printf("ChainAgent %s: Starting goroutine", a.ID)
	
	for {
		select {
		case msg := <-a.InChan:
			debugLogger.Printf("ChainAgent %s: Received message | role=%s source=%s len=%d preview=%q",
				a.ID, msg.Role, msg.Source, len(msg.Content), msg.Content[:min(80, len(msg.Content))])

			// Show the incoming message immediately, before the API call
			if a.Pane != nil {
				a.Pane.Messages = append(a.Pane.Messages, msg)
				a.Pane.Status = AgentThinking
				a.Pane.LastActivity = "🤔 Thinking..."
			}

			// Process message with Claude AI
			response := a.processMessage(msg)

			debugLogger.Printf("ChainAgent %s: Response ready | len=%d outbound_agents=%d preview=%q",
				a.ID, len(response.Content), len(a.OutChans), response.Content[:min(80, len(response.Content))])

			// Update agent status back to idle with completion indicator
			if a.Pane != nil {
				a.Pane.Status = AgentIdle
				if strings.Contains(response.Content, "[Tool calling limit reached") {
					a.Pane.LastActivity = "⚠️ Stopped (tool loop)"
				} else if strings.Contains(response.Content, "[Stopped infinite loop") {
					a.Pane.LastActivity = "🛑 Stopped (infinite loop)"
				} else {
					a.Pane.LastActivity = "✅ Completed"
				}
			}

			// Forward only the <to_next_agent> block to connected agents; fall back to
			// full content if the agent didn't follow the structured output format.
			forwardContent := extractForwardContent(response.Content)
			usedStructured := forwardContent != response.Content
			debugLogger.Printf("ChainAgent %s: Forward content | structured=%v len=%d preview=%q",
				a.ID, usedStructured, len(forwardContent), forwardContent[:min(80, len(forwardContent))])

			for targetID, outChan := range a.OutChans {
				select {
				case outChan <- AgentMessage{
					Role:      "assistant",
					Content:   forwardContent,
					Timestamp: response.Timestamp,
					Source:    a.ID,
				}:
					debugLogger.Printf("ChainAgent %s: Sent to %s | len=%d", a.ID, targetID, len(forwardContent))
				default:
					debugLogger.Printf("ChainAgent %s: DROP to %s — channel full (cap=10)", a.ID, targetID)
				}
			}
			
		case <-a.ctx.Done():
			debugLogger.Printf("ChainAgent %s: Shutting down", a.ID)
			return
		}
	}
}

// processMessage processes a message using Claude AI with tools
func (a *ChainAgent) processMessage(msg AgentMessage) AgentMessage {
	// Build system prompt, adding structured output instructions when this agent
	// has downstream connections to route output to.
	systemPrompt := a.Node.SystemPrompt
	if len(a.OutChans) > 0 {
		targets := make([]string, 0, len(a.OutChans))
		for id := range a.OutChans {
			targets = append(targets, id)
		}
		systemPrompt += fmt.Sprintf(`

You are part of an agent chain. After completing your work, you MUST end your response with a <to_next_agent> block containing a focused prompt for the next agent(s) (%s). This tells them exactly what you need.

Format:
[Your full work, reasoning, code, analysis, etc.]

<to_next_agent>
[A clear, specific prompt for the next agent — what you need them to do, with only the relevant context they require]
</to_next_agent>`, strings.Join(targets, ", "))
	}

	history := a.getConversationHistory()

	// Debug: log the role sequence we are about to send so API errors are easy
	// to diagnose.
	if len(history) == 0 {
		debugLogger.Printf("ChainAgent %s: history empty (first-turn call), prompt role=user", a.ID)
	} else {
		roles := make([]string, len(history))
		for i, h := range history {
			roles[i] = h.Role
		}
		debugLogger.Printf("ChainAgent %s: history roles=%v len=%d, adding prompt role=user", a.ID, roles, len(history))
	}

	// Build AI context with working directory
	aiContext := ai.AIContext{
		SystemPrompt:        systemPrompt,
		ConversationHistory: history,
		Temperature:         0.7,
		MaxTokens:           8192,
		CodeContext: &ai.CodeContext{
			Directory: a.workingDir,
		},
	}

	// Get Claude provider
	provider, err := a.aiManager.GetProvider("claude")
	if err != nil || provider == nil {
		debugLogger.Printf("ChainAgent %s: No Claude provider available: %v", a.ID, err)
		return AgentMessage{
			Role:      "assistant", 
			Content:   "Error: No AI provider available",
			Timestamp: msg.Timestamp,
			Source:    a.ID,
		}
	}
	
	// Send message to Claude
	debugLogger.Printf("ChainAgent %s: calling API | history_len=%d prompt_preview=%q",
		a.ID, len(history), msg.Content[:min(80, len(msg.Content))])
	response, err := provider.SendMessage(context.Background(), msg.Content, aiContext)
	if err != nil {
		debugLogger.Printf("ChainAgent %s: API ERROR history_len=%d: %v", a.ID, len(history), err)
		return AgentMessage{
			Role:      "assistant",
			Content:   fmt.Sprintf("Error: %v", err),
			Timestamp: msg.Timestamp,
			Source:    a.ID,
		}
	}
	debugLogger.Printf("ChainAgent %s: API success | response_len=%d", a.ID, len(response.Content))
	
	// Add response to pane (incoming message was already appended in Run before the API call)
	if a.Pane != nil {
		a.Pane.Messages = append(a.Pane.Messages, AgentMessage{
			Role:      "assistant",
			Content:   response.Content,
			Timestamp: msg.Timestamp,
			Source:    a.ID,
		})
	}
	
	return AgentMessage{
		Role:      "assistant",
		Content:   response.Content,
		Timestamp: msg.Timestamp,
		Source:    a.ID,
	}
}

// getConversationHistory gets conversation history for AI context.
//
// Two invariants are enforced here so the Anthropic API never rejects the request:
//
//  1. The most-recent message in the pane is the *current* incoming prompt, which
//     is already passed separately to provider.SendMessage as the `prompt`
//     argument.  Including it in the history too would (a) duplicate it in the
//     conversation and (b) cause the history to start with an "assistant" role
//     when the message came from another agent — which the API rejects.
//
//  2. Messages received FROM other agents have role="assistant" in the pane (for
//     display purposes), but from this agent's perspective they are *input* — the
//     equivalent of a user turn.  We re-map them to "user" here so that the
//     history always alternates user / assistant as the API requires.
func (a *ChainAgent) getConversationHistory() []ai.Message {
	if a.Pane == nil {
		return []ai.Message{}
	}

	// Exclude the last entry — it is the message currently being processed.
	msgs := a.Pane.Messages
	if len(msgs) == 0 {
		return []ai.Message{}
	}
	msgs = msgs[:len(msgs)-1]

	var history []ai.Message
	for _, msg := range msgs {
		// Parse timestamp, use current time as fallback
		timestamp, err := time.Parse("15:04:05", msg.Timestamp)
		if err != nil {
			timestamp = time.Now()
		}

		// Re-map inter-agent messages to "user" role for the API.
		apiRole := msg.Role
		if msg.Role == "assistant" && msg.Source != a.ID {
			apiRole = "user"
		}

		history = append(history, ai.Message{
			Role:      apiRole,
			Content:   msg.Content,
			Timestamp: timestamp,
		})
	}
	return history
}

// scheduleRefresh creates a command to refresh the UI after a delay
func (m *ChainExecutionModel) scheduleRefresh() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return RefreshUIMsg{}
	})
}

// extractForwardContent returns the content of the <to_next_agent> block if
// present, or the full content as a fallback when the agent didn't use the
// structured output format.
func extractForwardContent(content string) string {
	const openTag = "<to_next_agent>"
	const closeTag = "</to_next_agent>"

	start := strings.Index(content, openTag)
	if start == -1 {
		return content
	}
	end := strings.Index(content, closeTag)
	if end == -1 {
		return content
	}
	return strings.TrimSpace(content[start+len(openTag) : end])
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
