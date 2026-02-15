package tui

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aichain/aichain/internal/app"
	"github.com/aichain/aichain/internal/chain"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var setupDebugLogger *log.Logger

func init() {
	// Create debug log file for setup
	debugFile, err := os.OpenFile("claudevim-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		setupDebugLogger = log.New(os.Stderr, "SETUP-DEBUG: ", log.Ltime|log.Lshortfile)
	} else {
		setupDebugLogger = log.New(debugFile, "SETUP-DEBUG: ", log.Ltime|log.Lshortfile)
	}
}

// ChainSetupMode represents the current setup mode
type ChainSetupMode int

const (
	SetupModeDSL      ChainSetupMode = iota // Step 1: DSL input
	SetupModeNodeConfig                      // Step 2: Node configuration
	SetupModeComplete                        // Setup finished
)

// ChainSetupModel handles the chain setup UI
type ChainSetupModel struct {
	app         *app.Application
	setupFlow   *chain.SetupFlow
	mode        ChainSetupMode
	width       int
	height      int

	// DSL Input (Step 1)
	dslInput    textinput.Model
	dslError    string

	// Node Configuration (Step 2)
	agentSelector   int    // Selected agent index

	// Available options
	availableAgents []chain.AgentDefinition

	// UI State
	activeField     int    // Which field is focused
	showHelp        bool
	styles          ChainSetupStyles
}

// ChainSetupStyles holds styling for setup UI
type ChainSetupStyles struct {
	Title       lipgloss.Style
	Subtitle    lipgloss.Style
	Help        lipgloss.Style
	Error       lipgloss.Style
	Success     lipgloss.Style
	Field       lipgloss.Style
	ActiveField lipgloss.Style
	Button      lipgloss.Style
	ActiveButton lipgloss.Style
}

// NewChainSetupModel creates a new chain setup model
func NewChainSetupModel(application *app.Application) ChainSetupModel {
	// Get allowed directory from app config if available
	allowedDir := "."
	if config := application.GetConfig(); config != nil && config.AllowedDirectory != "" {
		allowedDir = config.AllowedDirectory
	}
	
	setupFlow := chain.NewSetupFlowWithDir(allowedDir)
	
	// DSL input
	dslInput := textinput.New()
	dslInput.Placeholder = "Enter chain DSL (e.g., A -> B -> C)"
	dslInput.Focus()
	dslInput.CharLimit = 200
	dslInput.Width = 60

	return ChainSetupModel{
		app:             application,
		setupFlow:       setupFlow,
		mode:           SetupModeDSL,
		dslInput:       dslInput,
		styles:         createChainSetupStyles(),
		availableAgents: []chain.AgentDefinition{}, // Will be loaded after DSL parsing
	}
}

// NewModelWithChainSetup creates a new TUI model starting with chain setup
func NewModelWithChainSetup(application *app.Application) Model {
	setupModel := NewChainSetupModel(application)
	
	// Create a modified regular model that starts with chain setup
	model := NewModel(application)
	model.chainSetup = &setupModel
	model.mode = ModeChainSetup
	
	return model
}

// Update handles chain setup messages
func (m ChainSetupModel) Update(msg tea.Msg) (ChainSetupModel, tea.Cmd) {
	var cmd tea.Cmd

	// Check if setup flow is complete and mode needs updating
	if m.setupFlow.Complete && m.mode != SetupModeComplete {
		setupDebugLogger.Printf("Transitioning to SetupModeComplete (was mode %d)", int(m.mode))
		m.mode = SetupModeComplete
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		setupDebugLogger.Printf("ChainSetup received key: '%s', current mode: %d, setup complete: %v", 
			msg.String(), int(m.mode), m.setupFlow.Complete)
		
		switch m.mode {
		case SetupModeDSL:
			setupDebugLogger.Printf("Routing to updateDSLInput")
			return m.updateDSLInput(msg)
		case SetupModeNodeConfig:
			setupDebugLogger.Printf("Routing to updateNodeConfig")
			return m.updateNodeConfig(msg)
		case SetupModeComplete:
			setupDebugLogger.Printf("Routing to updateComplete")
			return m.updateComplete(msg)
		default:
			setupDebugLogger.Printf("Unknown mode %d, not routing", int(m.mode))
		}
	}

	return m, cmd
}

// updateDSLInput handles DSL input in step 1
func (m ChainSetupModel) updateDSLInput(msg tea.KeyMsg) (ChainSetupModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "enter":
		// Process DSL input
		dslText := strings.TrimSpace(m.dslInput.Value())
		if dslText == "" {
			m.dslError = "Please enter a chain DSL"
			return m, nil
		}

		err := m.setupFlow.ProcessStep1(dslText)
		if err != nil {
			m.dslError = err.Error()
			return m, nil
		}

		// Move to node configuration and load available agents
		m.mode = SetupModeNodeConfig
		m.dslError = ""
		m.availableAgents = m.setupFlow.GetAvailableAgents()
		m.agentSelector = 0
		m.activeField = 0
		return m, nil

	case "f1":
		m.showHelp = !m.showHelp
		return m, nil
	}

	// Update DSL input
	m.dslInput, cmd = m.dslInput.Update(msg)
	return m, cmd
}

// updateNodeConfig handles node configuration in step 2
func (m ChainSetupModel) updateNodeConfig(msg tea.KeyMsg) (ChainSetupModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "up", "k":
		if m.agentSelector > 0 {
			m.agentSelector--
		}
		return m, nil

	case "down", "j":
		if m.agentSelector < len(m.availableAgents)-1 {
			m.agentSelector++
		}
		return m, nil

	case "enter":
		// Save selected agent for current node
		if len(m.availableAgents) > 0 && m.agentSelector < len(m.availableAgents) {
			selectedAgent := m.availableAgents[m.agentSelector]
			err := m.setupFlow.ConfigureCurrentNode(selectedAgent.ID)
			if err != nil {
				// TODO: Show error
				return m, nil
			}

			if m.setupFlow.Complete {
				m.mode = SetupModeComplete
				return m, nil
			}
			
			// Move to next node and reset selector
			m.agentSelector = 0
			return m, nil
		}
		return m, nil

	case "f1":
		m.showHelp = !m.showHelp
		return m, nil
	}

	// No text input handling needed for agent selection

	return m, cmd
}

// ChainCompleteMsg signals that chain setup is complete
type ChainCompleteMsg struct {
	Chain *chain.Chain
}

// updateComplete handles completion state
func (m ChainSetupModel) updateComplete(msg tea.KeyMsg) (ChainSetupModel, tea.Cmd) {
	setupDebugLogger.Printf("updateComplete called with key: '%s'", msg.String())
	
	switch msg.String() {
	case "enter", " ":
		setupDebugLogger.Printf("Enter/Space pressed in completion state - sending ChainCompleteMsg")
		// Signal to main model to start chain execution
		return m, func() tea.Msg {
			setupDebugLogger.Printf("ChainCompleteMsg being sent with chain: %v", m.setupFlow.Chain != nil)
			return ChainCompleteMsg{Chain: m.setupFlow.Chain}
		}
	case "ctrl+c", "esc":
		setupDebugLogger.Printf("Ctrl+C/Esc pressed - quitting")
		return m, tea.Quit
	default:
		setupDebugLogger.Printf("Unhandled key in completion: '%s'", msg.String())
	}
	return m, nil
}

// View renders the chain setup UI
func (m ChainSetupModel) View() string {
	setupDebugLogger.Printf("View() called with mode: %d, setup complete: %v", int(m.mode), m.setupFlow.Complete)
	
	switch m.mode {
	case SetupModeDSL:
		setupDebugLogger.Printf("Rendering DSL input view")
		return m.viewDSLInput()
	case SetupModeNodeConfig:
		setupDebugLogger.Printf("Rendering node config view")
		return m.viewNodeConfig()
	case SetupModeComplete:
		setupDebugLogger.Printf("Rendering complete view")
		return m.viewComplete()
	}
	setupDebugLogger.Printf("No matching mode, returning empty")
	return ""
}

// GetMode returns the current setup mode (for debugging)
func (m ChainSetupModel) GetMode() ChainSetupMode {
	return m.mode
}

// viewDSLInput renders step 1: DSL input
func (m ChainSetupModel) viewDSLInput() string {
	var b strings.Builder

	// Title
	title := m.styles.Title.Render("🔗 AIChain AI Chain Setup")
	subtitle := m.styles.Subtitle.Render("Step 1 of 2: Define Your Claude AI Chain")
	
	b.WriteString(title + "\n")
	b.WriteString(subtitle + "\n\n")

	// Instructions
	instructions := `Create a chain of Claude AI agents using our DSL (Domain Specific Language):

Examples:
  A -> B            Simple chain: A sends to B
  A <> B            Two-way: A and B communicate both ways  
  A -> B -> C       Linear chain: A to B to C
  A <> B <> C       All nodes communicate with neighbors
  A -> * <- B       Human (*) receives from both A and B
  A <> *            Human can communicate with A

Symbols:
  ->     One-way communication
  <-     Reverse one-way communication  
  <>     Two-way communication
  *      Human interaction point
  A,B,C  Claude AI agent nodes (any letter/number)

Each AI node will use Claude models (Sonnet, Opus, or Haiku) based on the agent you select.`

	b.WriteString(instructions + "\n\n")

	// DSL Input
	b.WriteString("Chain DSL:\n")
	b.WriteString(m.dslInput.View() + "\n\n")

	// Error display
	if m.dslError != "" {
		b.WriteString(m.styles.Error.Render("Error: " + m.dslError) + "\n\n")
	}

	// Help
	if m.showHelp {
		help := `Keyboard shortcuts:
  Enter      Process DSL and continue to step 2
  F1         Toggle this help
  Ctrl+C     Quit
  Esc        Quit`
		b.WriteString(m.styles.Help.Render(help) + "\n\n")
	}

	// Footer
	b.WriteString("Press F1 for help • Enter to continue • Ctrl+C to quit")

	return b.String()
}

// viewNodeConfig renders step 2: agent selection
func (m ChainSetupModel) viewNodeConfig() string {
	// Check if setup is complete first
	if m.setupFlow.Complete {
		// Setup is complete, show completion screen
		return m.viewComplete()
	}
	
	currentNode := m.setupFlow.GetCurrentNodeSetup()
	if currentNode == nil {
		return "Error: No current node"
	}

	configured, total := m.setupFlow.GetProgressInfo()

	var b strings.Builder

	// Title and progress
	title := m.styles.Title.Render("🔗 AIChain AI Chain Setup")
	subtitle := m.styles.Subtitle.Render(fmt.Sprintf("Step 2 of 2: Select Agent for Node %s (%d/%d)", currentNode.ID, configured+1, total))
	
	b.WriteString(title + "\n")
	b.WriteString(subtitle + "\n\n")

	// Chain visualization
	b.WriteString("Chain Overview:\n")
	b.WriteString(m.setupFlow.GetChainVisualization() + "\n")

	// Current node configuration
	b.WriteString(fmt.Sprintf("Select an agent for Node %s:\n\n", currentNode.ID))

	// Agent selection
	if len(m.availableAgents) == 0 {
		b.WriteString(m.styles.Error.Render("No agents found in .agents directory"))
		b.WriteString("\n\nCreate agent files in: " + m.setupFlow.GetAgentsDirectory())
		return b.String()
	}

	// Render available agents
	for i, agent := range m.availableAgents {
		indicator := "   "
		if i == m.agentSelector {
			indicator = " ▶ "
		}
		
		agentInfo := fmt.Sprintf("%s%s - %s", indicator, agent.Name, agent.Description)
		
		// Get model explanation
		modelExplanation := m.getModelExplanation(agent.Model)
		modelInfo := fmt.Sprintf("     %s | Role: %s | Temp: %.1f", modelExplanation, agent.Role, agent.Temperature)
		
		if i == m.agentSelector {
			agentInfo = m.styles.ActiveField.Render(agentInfo)
			modelInfo = m.styles.ActiveField.Render(modelInfo)
		}
		
		b.WriteString(agentInfo + "\n")
		b.WriteString(modelInfo + "\n")
		
		// Show tags if available
		if len(agent.Tags) > 0 {
			tagsInfo := fmt.Sprintf("     Tags: %s", strings.Join(agent.Tags, ", "))
			if i == m.agentSelector {
				tagsInfo = m.styles.ActiveField.Render(tagsInfo)
			}
			b.WriteString(tagsInfo + "\n")
		}
		
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("↑↓ or j/k Select agent • Enter to confirm • F1 help • Ctrl+C quit")

	return b.String()
}

// Helper methods for chain setup UI continue in next part...