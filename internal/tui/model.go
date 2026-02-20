package tui

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/aichain/aichain/internal/app"
	"github.com/aichain/aichain/internal/chain"
	"github.com/aichain/aichain/internal/session"
	"github.com/aichain/aichain/internal/vim"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AppMode represents the current application mode
type AppMode int

const (
	ModeChainSetup    AppMode = iota // Chain setup mode
	ModeChainExecution               // Chain execution mode
	ModeNormal                       // Normal operation mode  
)

// Model represents the main TUI model
type Model struct {
	app            *app.Application
	width          int
	height         int
	layout         Layout
	activePane     int
	vimMode        vim.VIMMode
	statusLine     string
	commandInput   textinput.Model
	commandMode    bool
	mode           AppMode
	
	// Chain setup
	chainSetup     *ChainSetupModel
	
	// Chain execution
	chainExecution *ChainExecutionModel
	
	// Panes
	explorer       *Explorer
	editor         *Editor
	chat           *Chat
	
	// Styles
	styles         Styles
}

// Layout defines the TUI layout
type Layout struct {
	Type        LayoutType    `json:"type"`
	Panes       []PaneConfig  `json:"panes"`
	Proportions []float64     `json:"proportions"`
}

// LayoutType defines layout types
type LayoutType string

const (
	SingleLayout LayoutType = "single"
	DualLayout   LayoutType = "dual"
	TripleLayout LayoutType = "triple"
)

// PaneConfig defines a pane configuration
type PaneConfig struct {
	Type      PaneType `json:"type"`
	SessionID string   `json:"session_id"`
	FilePath  string   `json:"file_path"`
}

// PaneType defines pane types
type PaneType string

const (
	ExplorerPane PaneType = "explorer"
	EditorPane   PaneType = "editor"
	ChatPane     PaneType = "chat"
)

// Styles holds all styling information
type Styles struct {
	Border        lipgloss.Style
	ActiveBorder  lipgloss.Style
	StatusLine    lipgloss.Style
	VimMode       lipgloss.Style
	ChatUser      lipgloss.Style
	ChatAssistant lipgloss.Style
	Explorer      lipgloss.Style
}

// NewModel creates a new TUI model
func NewModel(application *app.Application) Model {
	// Create command input
	cmdInput := textinput.New()
	cmdInput.Prompt = ":"
	cmdInput.CharLimit = 256
	
	// Create default layout (triple: explorer + editor + chat)
	layout := Layout{
		Type: TripleLayout,
		Panes: []PaneConfig{
			{Type: ExplorerPane},
			{Type: EditorPane},
			{Type: ChatPane},
		},
		Proportions: []float64{0.2, 0.5, 0.3},
	}

	// Create panes
	explorer := NewExplorerPane()
	editor := NewEditorPane()
	chat := NewChatPane()

	// Set active session for chat pane
	if activeSession, err := application.GetActiveSession(); err == nil {
		chat.SetSession(activeSession)
	}

	return Model{
		app:          application,
		layout:       layout,
		activePane:   1, // Start with editor active
		vimMode:      vim.NormalMode,
		commandInput: cmdInput,
		mode:         ModeNormal, // Normal mode by default
		explorer:     explorer,
		editor:       editor,
		chat:         chat,
		styles:       createStyles(),
	}
}

// NewModelWithDSLFile creates a new TUI model and directly executes a DSL file
func NewModelWithDSLFile(application *app.Application, dslFile string) Model {
	// Create basic model
	model := NewModel(application)
	
	// Read DSL file content
	dslContent, err := ioutil.ReadFile(dslFile)
	if err != nil {
		log.Fatalf("Failed to read DSL file %s: %v", dslFile, err)
	}
	
	// Parse DSL and create chain
	parser := chain.NewDSLParser()
	completedChain, err := parser.ParseChainDSL(string(dslContent))
	if err != nil {
		log.Fatalf("Failed to parse DSL file %s: %v", dslFile, err)
	}
	
	// Create chain execution model directly
	chainExecModel := NewChainExecutionModel(application, completedChain)
	model.chainExecution = &chainExecModel
	model.mode = ModeChainExecution
	
	return model
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	
	// Handle ChainCompleteMsg first, regardless of mode
	if completeMsg, ok := msg.(ChainCompleteMsg); ok {
		if completeMsg.Chain != nil {
			executionModel := NewChainExecutionModel(m.app, completeMsg.Chain)
			
			// Use main model dimensions if available, otherwise use reasonable defaults
			if m.width > 0 && m.height > 0 {
				executionModel.Width = m.width
				executionModel.Height = m.height
			} else {
				// Use default dimensions that will work until WindowSizeMsg arrives
				executionModel.Width = 120
				executionModel.Height = 30
			}
			
			executionModel.UpdateLayout()
			m.chainExecution = &executionModel
			m.mode = ModeChainExecution
		}
		return m, nil
	}

	// Handle chain setup mode
	if m.mode == ModeChainSetup && m.chainSetup != nil {
		updatedSetup, cmd := m.chainSetup.Update(msg)
		m.chainSetup = &updatedSetup
		
		return m, cmd
	}

	// Handle chain execution mode
	if m.mode == ModeChainExecution && m.chainExecution != nil {
		updatedExecution, cmd := m.chainExecution.Update(msg)
		m.chainExecution = &updatedExecution
		
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		
		// Also update chain setup if active
		if m.chainSetup != nil {
			updatedSetup, _ := m.chainSetup.Update(msg)
			m.chainSetup = &updatedSetup
		}
		
		// Also update chain execution if active
		if m.chainExecution != nil {
			updatedExecution, _ := m.chainExecution.Update(msg)
			m.chainExecution = &updatedExecution
		}

	case tea.KeyMsg:
		if m.commandMode {
			return m.updateCommandMode(msg)
		}
		
		// Check for global navigation keys first (but not when chat is in input mode)
		chatInInputMode := (m.activePane == 2 && m.chat != nil && m.chat.inputMode)
		
		if !chatInInputMode {
			switch msg.String() {
			case "h", "left":
				// Focus left pane
				if m.activePane > 0 {
					m.activePane--
				}
				return m, nil
			case "l", "right":
				// Focus right pane
				if m.activePane < len(m.layout.Panes)-1 {
					m.activePane++
				}
				return m, nil
			case ":", "ctrl+c", "q":
				// Global commands
				return m.updateNormalMode(msg)
			}
		}
		
		// Handle pane-specific keys
		switch m.activePane {
		case 0: // Explorer
			if msg.String() == "j" || msg.String() == "down" || msg.String() == "k" || msg.String() == "up" || 
			   msg.String() == "enter" || msg.String() == "backspace" {
				var cmd tea.Cmd
				*m.explorer, cmd = m.explorer.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		case 1: // Editor
			var cmd tea.Cmd
			*m.editor, cmd = m.editor.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case 2: // Chat
			var cmd tea.Cmd
			*m.chat, cmd = m.chat.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case SessionUpdateMsg:
		if m.chat.sessionID == msg.SessionID {
			m.chat.AddMessage(msg.Message)
		}
	
	case MessageSentMsg:
		// Handle user message sent to AI
		return m, m.handleAIMessage(msg.SessionID, msg.Content)
		
	case FileSelectedMsg:
		// Handle file selection from explorer
		if m.editor != nil {
			var cmd tea.Cmd
			*m.editor, cmd = m.editor.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// updateNormalMode handles key presses in normal mode
func (m Model) updateNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.String() {
	case ":":
		// Enter command mode
		m.commandMode = true
		m.commandInput.Focus()
		return m, nil

	case "ctrl+c", "q":
		// Quit application
		return m, tea.Quit

	case "E":
		// Toggle explorer
		m.activePane = 0
		return m, nil

	case "h", "left":
		// Focus left pane
		if m.activePane > 0 {
			m.activePane--
		}
		return m, nil

	case "l", "right":
		// Focus right pane
		if m.activePane < len(m.layout.Panes)-1 {
			m.activePane++
		}
		return m, nil

	case "ctrl+t":
		// New session
		return m, m.createNewSession()

	case "gt":
		// Next session
		return m, m.nextSession()

	case "gT":
		// Previous session
		return m, m.previousSession()
	}

	// Send key to VIM engine
	if err := m.app.ProcessKeypress(msg.String()); err == nil {
		m.vimMode = m.app.GetVIMMode()
	}

	return m, tea.Batch(cmds...)
}

// updateCommandMode handles key presses in command mode
func (m Model) updateCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Exit command mode
		m.commandMode = false
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		return m, nil

	case tea.KeyEnter:
		// Execute command
		command := m.commandInput.Value()
		m.commandMode = false
		m.commandInput.Blur()
		m.commandInput.SetValue("")
		
		return m, m.executeCommand(command)
	}

	// Update command input
	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	return m, cmd
}

// View renders the TUI
func (m Model) View() string {
	// Show chain setup if in setup mode, even before window size is set
	if m.mode == ModeChainSetup && m.chainSetup != nil {
		return m.chainSetup.View()
	}

	// Show chain execution if in execution mode
	if m.mode == ModeChainExecution && m.chainExecution != nil {
		return m.chainExecution.View()
	}

	if m.width == 0 || m.height == 0 {
		return "Initializing AIChain..."
	}

	// Calculate pane dimensions
	contentHeight := m.height - 1 // Reserve space for status line
	var views []string

	switch m.layout.Type {
	case TripleLayout:
		// Three panes: explorer | editor | chat
		explorerWidth := int(float64(m.width) * m.layout.Proportions[0])
		editorWidth := int(float64(m.width) * m.layout.Proportions[1])
		chatWidth := m.width - explorerWidth - editorWidth

		explorerView := m.renderPane(m.explorer.View(), explorerWidth, contentHeight, 0)
		editorView := m.renderPane(m.editor.View(), editorWidth, contentHeight, 1)
		chatView := m.renderPane(m.chat.View(), chatWidth, contentHeight, 2)

		views = []string{explorerView, editorView, chatView}
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, views...)
	statusLine := m.renderStatusLine()

	if m.commandMode {
		commandLine := m.commandInput.View()
		return lipgloss.JoinVertical(lipgloss.Left, content, commandLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, content, statusLine)
}

// renderPane renders a pane with borders
func (m Model) renderPane(content string, width, height, paneIndex int) string {
	style := m.styles.Border
	if paneIndex == m.activePane {
		style = m.styles.ActiveBorder
	}

	return style.
		Width(width - 2).
		Height(height - 2).
		Render(content)
}

// renderStatusLine renders the status line
func (m Model) renderStatusLine() string {
	// VIM mode indicator
	modeStyle := m.styles.VimMode
	var modeColor lipgloss.Color
	switch m.vimMode {
	case vim.NormalMode:
		modeColor = "#00ff00"
	case vim.InsertMode:
		modeColor = "#ffff00"
	case vim.VisualMode:
		modeColor = "#ff8800"
	case vim.CommandMode:
		modeColor = "#0088ff"
	}
	modeStyle = modeStyle.Background(modeColor)
	modeText := modeStyle.Render(fmt.Sprintf(" %s ", m.vimMode.String()))

	// Session info
	sessionInfo := ""
	if activeSession, err := m.app.GetActiveSession(); err == nil {
		sessionInfo = fmt.Sprintf(" %s ", activeSession.Name)
	}

	// Status info
	status := m.app.GetStatus()
	statusText := fmt.Sprintf("Sessions: %d", status["session_count"])

	// Build status line
	leftSide := lipgloss.JoinHorizontal(lipgloss.Left, modeText, sessionInfo)
	rightSide := statusText

	padding := m.width - lipgloss.Width(leftSide) - lipgloss.Width(rightSide)
	if padding < 0 {
		padding = 0
	}

	return m.styles.StatusLine.Render(
		leftSide + strings.Repeat(" ", padding) + rightSide,
	)
}

// updateLayout updates pane dimensions based on terminal size
func (m *Model) updateLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	// Calculate pane dimensions based on layout
	totalWidth := m.width
	totalHeight := m.height - 2 // Reserve space for status line

	switch m.layout.Type {
	case TripleLayout:
		// Explorer | Editor | Chat layout
		explorerWidth := int(float64(totalWidth) * m.layout.Proportions[0])
		editorWidth := int(float64(totalWidth) * m.layout.Proportions[1])
		chatWidth := totalWidth - explorerWidth - editorWidth

		// Update pane dimensions
		m.explorer.width = explorerWidth
		m.explorer.height = totalHeight
		
		m.editor.width = editorWidth
		m.editor.height = totalHeight
		
		m.chat.width = chatWidth
		m.chat.height = totalHeight

	case DualLayout:
		// Two panes side by side
		leftWidth := int(float64(totalWidth) * 0.5)
		rightWidth := totalWidth - leftWidth

		if len(m.layout.Panes) >= 2 {
			if m.layout.Panes[0].Type == ExplorerPane {
				m.explorer.width = leftWidth
				m.explorer.height = totalHeight
			}
			if m.layout.Panes[1].Type == ChatPane {
				m.chat.width = rightWidth
				m.chat.height = totalHeight
			}
		}

	case SingleLayout:
		// Single pane takes full space
		if len(m.layout.Panes) > 0 {
			switch m.layout.Panes[0].Type {
			case ExplorerPane:
				m.explorer.width = totalWidth
				m.explorer.height = totalHeight
			case EditorPane:
				m.editor.width = totalWidth
				m.editor.height = totalHeight
			case ChatPane:
				m.chat.width = totalWidth
				m.chat.height = totalHeight
			}
		}
	}
}

// Command execution
func (m Model) createNewSession() tea.Cmd {
	return func() tea.Msg {
		_, err := m.app.CreateSession("New Session", "", "assistant", "")
		if err != nil {
			return ErrorMsg{Error: err}
		}
		return SessionCreatedMsg{}
	}
}

func (m Model) nextSession() tea.Cmd {
	return func() tea.Msg {
		sessions := m.app.ListSessions()
		if len(sessions) <= 1 {
			return nil
		}

		// Find current session index and move to next
		current := -1
		activeSession, _ := m.app.GetActiveSession()
		if activeSession != nil {
			for i, s := range sessions {
				if s.ID == activeSession.ID {
					current = i
					break
				}
			}
		}

		nextIndex := (current + 1) % len(sessions)
		m.app.SetActiveSession(sessions[nextIndex].ID)
		m.chat.SetSession(sessions[nextIndex])

		return SessionSwitchedMsg{SessionID: sessions[nextIndex].ID}
	}
}

func (m Model) previousSession() tea.Cmd {
	return func() tea.Msg {
		sessions := m.app.ListSessions()
		if len(sessions) <= 1 {
			return nil
		}

		// Find current session index and move to previous
		current := -1
		activeSession, _ := m.app.GetActiveSession()
		if activeSession != nil {
			for i, s := range sessions {
				if s.ID == activeSession.ID {
					current = i
					break
				}
			}
		}

		prevIndex := current - 1
		if prevIndex < 0 {
			prevIndex = len(sessions) - 1
		}
		
		m.app.SetActiveSession(sessions[prevIndex].ID)
		m.chat.SetSession(sessions[prevIndex])

		return SessionSwitchedMsg{SessionID: sessions[prevIndex].ID}
	}
}

func (m Model) executeCommand(command string) tea.Cmd {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "q", "quit":
		return tea.Quit
	case "session", "s":
		if len(args) > 0 {
			return m.switchToSession(args[0])
		}
	case "debate":
		if len(args) > 0 {
			return m.startDebate(strings.Join(args, " "))
		}
	case "dual":
		return m.createDualSession()
	}

	return nil
}

func (m Model) switchToSession(name string) tea.Cmd {
	return func() tea.Msg {
		session, err := m.app.GetSessionByName(name)
		if err != nil {
			return ErrorMsg{Error: err}
		}
		
		m.app.SetActiveSession(session.ID)
		m.chat.SetSession(session)
		return SessionSwitchedMsg{SessionID: session.ID}
	}
}

func (m Model) startDebate(topic string) tea.Cmd {
	return func() tea.Msg {
		pipeline, err := m.app.CreateDebateSession("Claude", "GPT", topic)
		if err != nil {
			return ErrorMsg{Error: err}
		}
		return DebateStartedMsg{PipelineID: pipeline.ID, Topic: topic}
	}
}

func (m Model) createDualSession() tea.Cmd {
	return func() tea.Msg {
		pipeline, err := m.app.CreateDualAISession("AI-1", "AI-2", true)
		if err != nil {
			return ErrorMsg{Error: err}
		}
		return DualSessionCreatedMsg{PipelineID: pipeline.ID}
	}
}

// createStyles creates the TUI styles
func createStyles() Styles {
	return Styles{
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#666666")),
		ActiveBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00ff88")),
		StatusLine: lipgloss.NewStyle().
			Background(lipgloss.Color("#333333")).
			Foreground(lipgloss.Color("#ffffff")).
			Width(0), // Will be set dynamically
		VimMode: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#000000")),
		ChatUser: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00aaff")),
		ChatAssistant: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff8800")),
		Explorer: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ff88")),
	}
}

// handleAIMessage sends a message to the AI and returns a command to get the response
func (m Model) handleAIMessage(sessionID, content string) tea.Cmd {
	return func() tea.Msg {
		// Send message to AI through the application
		response, err := m.app.SendMessage(sessionID, content)
		if err != nil {
			return ErrorMsg{Error: err}
		}
		
		return SessionUpdateMsg{
			SessionID: sessionID,
			Message: session.Message{
				ID:        "ai-" + fmt.Sprintf("%d", time.Now().UnixNano()),
				Role:      "assistant", 
				Content:   response.Content,
				Timestamp: time.Now(),
				Source:    "ai",
			},
		}
	}
}