package tui

import (
	"fmt"
	"log"
	"os"
	"strings"

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

	// UI State
	styles         ChainExecutionStyles
}

// AgentResponseMsg is sent when an agent provides a response
type AgentResponseMsg struct {
	AgentIndex int
	Message    AgentMessage
}

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

// ChainExecutionStyles holds styling for chain execution UI
type ChainExecutionStyles struct {
	Title          lipgloss.Style
	AgentPane      lipgloss.Style
	ActivePane     lipgloss.Style
	Message        lipgloss.Style
	UserMessage    lipgloss.Style
	AssistantMessage lipgloss.Style
	SystemMessage  lipgloss.Style
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
		styles:       createChainExecutionStyles(),
		Width:        80,  // Default width
		Height:       24,  // Default height
	}
	
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
		var msgStyle lipgloss.Style
		switch msg.Role {
		case "user":
			msgStyle = m.styles.UserMessage
		case "assistant":
			msgStyle = m.styles.AssistantMessage
		case "system":
			msgStyle = m.styles.SystemMessage
		default:
			msgStyle = m.styles.Message
		}
		
		// Add message header
		msgHeader := fmt.Sprintf("[%s] %s", msg.Role, msg.Timestamp)
		contentLines = append(contentLines, msgStyle.Render(msgHeader))
		
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

// Helper methods continue in next part...