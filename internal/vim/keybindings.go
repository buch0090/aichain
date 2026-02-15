package vim

import (
	"fmt"
	"strings"
)

// VIMMode represents different VIM modes
type VIMMode int

const (
	NormalMode VIMMode = iota
	InsertMode
	VisualMode
	CommandMode
	ExplorerMode
	ChatMode
)

// String returns the string representation of a VIM mode
func (m VIMMode) String() string {
	switch m {
	case NormalMode:
		return "NORMAL"
	case InsertMode:
		return "INSERT"
	case VisualMode:
		return "VISUAL"
	case CommandMode:
		return "COMMAND"
	case ExplorerMode:
		return "EXPLORER"
	case ChatMode:
		return "CHAT"
	default:
		return "UNKNOWN"
	}
}

// KeyBinding represents a VIM keybinding
type KeyBinding struct {
	Mode        VIMMode `json:"mode"`
	Key         string  `json:"key"`
	Action      string  `json:"action"`
	Context     string  `json:"context"`    // "global", "editor", "chat", "explorer"
	Description string  `json:"description"`
	Handler     KeyHandler `json:"-"`
}

// KeyHandler is a function that handles a key press
type KeyHandler func(args ...interface{}) error

// Engine manages VIM keybindings and mode transitions
type Engine struct {
	currentMode  VIMMode
	bindings     map[string]KeyBinding
	modeHandlers map[VIMMode]ModeHandler
	context      string
	buffer       string // For multi-key sequences
}

// ModeHandler handles mode-specific behavior
type ModeHandler interface {
	OnEnter(engine *Engine) error
	OnExit(engine *Engine) error
	OnKey(engine *Engine, key string) error
}

// NewEngine creates a new VIM keybinding engine
func NewEngine() *Engine {
	engine := &Engine{
		currentMode:  NormalMode,
		bindings:     make(map[string]KeyBinding),
		modeHandlers: make(map[VIMMode]ModeHandler),
		context:      "global",
	}

	// Register default keybindings
	engine.registerDefaultBindings()
	
	return engine
}

// GetCurrentMode returns the current VIM mode
func (e *Engine) GetCurrentMode() VIMMode {
	return e.currentMode
}

// SetMode changes the current VIM mode
func (e *Engine) SetMode(mode VIMMode) error {
	if e.currentMode == mode {
		return nil
	}

	// Exit current mode
	if handler, exists := e.modeHandlers[e.currentMode]; exists {
		if err := handler.OnExit(e); err != nil {
			return fmt.Errorf("failed to exit mode %s: %v", e.currentMode, err)
		}
	}

	// Enter new mode
	oldMode := e.currentMode
	e.currentMode = mode

	if handler, exists := e.modeHandlers[mode]; exists {
		if err := handler.OnEnter(e); err != nil {
			e.currentMode = oldMode // Revert on error
			return fmt.Errorf("failed to enter mode %s: %v", mode, err)
		}
	}

	return nil
}

// SetContext changes the current context
func (e *Engine) SetContext(context string) {
	e.context = context
}

// GetContext returns the current context
func (e *Engine) GetContext() string {
	return e.context
}

// RegisterBinding registers a new keybinding
func (e *Engine) RegisterBinding(binding KeyBinding) {
	key := e.makeBindingKey(binding.Mode, binding.Key, binding.Context)
	e.bindings[key] = binding
}

// makeBindingKey creates a unique key for a binding
func (e *Engine) makeBindingKey(mode VIMMode, key, context string) string {
	return fmt.Sprintf("%d:%s:%s", mode, key, context)
}

// ProcessKey processes a key press
func (e *Engine) ProcessKey(key string) error {
	// Try mode-specific handler first
	if handler, exists := e.modeHandlers[e.currentMode]; exists {
		if err := handler.OnKey(e, key); err == nil {
			return nil // Key was handled by mode handler
		}
	}

	// Try specific keybinding
	bindingKey := e.makeBindingKey(e.currentMode, key, e.context)
	if binding, exists := e.bindings[bindingKey]; exists {
		if binding.Handler != nil {
			return binding.Handler()
		}
	}

	// Try global keybinding
	globalKey := e.makeBindingKey(e.currentMode, key, "global")
	if binding, exists := e.bindings[globalKey]; exists {
		if binding.Handler != nil {
			return binding.Handler()
		}
	}

	// Handle built-in VIM keys
	return e.handleBuiltinKey(key)
}

// handleBuiltinKey handles built-in VIM keys
func (e *Engine) handleBuiltinKey(key string) error {
	switch e.currentMode {
	case NormalMode:
		return e.handleNormalModeKey(key)
	case InsertMode:
		return e.handleInsertModeKey(key)
	case VisualMode:
		return e.handleVisualModeKey(key)
	case CommandMode:
		return e.handleCommandModeKey(key)
	}
	return fmt.Errorf("unhandled key: %s in mode %s", key, e.currentMode)
}

// handleNormalModeKey handles keys in normal mode
func (e *Engine) handleNormalModeKey(key string) error {
	switch key {
	case "i":
		return e.SetMode(InsertMode)
	case "a":
		return e.SetMode(InsertMode)
	case "o":
		return e.SetMode(InsertMode)
	case "v":
		return e.SetMode(VisualMode)
	case ":":
		return e.SetMode(CommandMode)
	case "Escape":
		return nil // Already in normal mode
	default:
		return fmt.Errorf("unknown normal mode key: %s", key)
	}
}

// handleInsertModeKey handles keys in insert mode
func (e *Engine) handleInsertModeKey(key string) error {
	switch key {
	case "Escape":
		return e.SetMode(NormalMode)
	default:
		// In a real implementation, this would insert the character
		return nil
	}
}

// handleVisualModeKey handles keys in visual mode
func (e *Engine) handleVisualModeKey(key string) error {
	switch key {
	case "Escape":
		return e.SetMode(NormalMode)
	case ":":
		return e.SetMode(CommandMode)
	default:
		return nil
	}
}

// handleCommandModeKey handles keys in command mode
func (e *Engine) handleCommandModeKey(key string) error {
	switch key {
	case "Escape":
		return e.SetMode(NormalMode)
	case "Enter":
		// Execute command and return to normal mode
		return e.SetMode(NormalMode)
	default:
		return nil
	}
}

// registerDefaultBindings registers the default AIChain keybindings
func (e *Engine) registerDefaultBindings() {
	bindings := []KeyBinding{
		// File Explorer
		{NormalMode, "E", "toggle_explorer", "global", "Toggle file explorer", e.handleToggleExplorer},
		{ExplorerMode, "j", "cursor_down", "explorer", "Move cursor down", e.handleCursorDown},
		{ExplorerMode, "k", "cursor_up", "explorer", "Move cursor up", e.handleCursorUp},
		{ExplorerMode, "Enter", "open_file", "explorer", "Open file", e.handleOpenFile},
		{ExplorerMode, "q", "close_explorer", "explorer", "Close explorer", e.handleCloseExplorer},

		// AI Sessions
		{NormalMode, "Ctrl+t", "new_ai_session", "global", "Create new AI session", e.handleNewAISession},
		{NormalMode, "gt", "next_session", "global", "Next session", e.handleNextSession},
		{NormalMode, "gT", "prev_session", "global", "Previous session", e.handlePrevSession},

		// AI Communication
		{VisualMode, "Enter", "send_to_ai", "editor", "Send selection to AI", e.handleSendToAI},
		{NormalMode, "Space", "send_line_to_ai", "chat", "Send current line to AI", e.handleSendLineToAI},

		// Layout Management
		{NormalMode, "Ctrl+w", "window_command", "global", "Window command prefix", e.handleWindowCommand},
		{NormalMode, "Ctrl+w+h", "focus_left", "global", "Focus left pane", e.handleFocusLeft},
		{NormalMode, "Ctrl+w+j", "focus_down", "global", "Focus down pane", e.handleFocusDown},
		{NormalMode, "Ctrl+w+k", "focus_up", "global", "Focus up pane", e.handleFocusUp},
		{NormalMode, "Ctrl+w+l", "focus_right", "global", "Focus right pane", e.handleFocusRight},

		// Code Actions
		{VisualMode, "ge", "ai_explain_code", "editor", "Explain selected code", e.handleExplainCode},
		{VisualMode, "gf", "ai_fix_code", "editor", "Fix selected code", e.handleFixCode},
		{VisualMode, "go", "ai_optimize_code", "editor", "Optimize selected code", e.handleOptimizeCode},
		{VisualMode, "gr", "ai_review_code", "editor", "Review selected code", e.handleReviewCode},
	}

	for _, binding := range bindings {
		e.RegisterBinding(binding)
	}
}

// Default key handlers (these would integrate with the actual application)
func (e *Engine) handleToggleExplorer(args ...interface{}) error {
	fmt.Println("Toggle explorer")
	return nil
}

func (e *Engine) handleCursorDown(args ...interface{}) error {
	fmt.Println("Cursor down")
	return nil
}

func (e *Engine) handleCursorUp(args ...interface{}) error {
	fmt.Println("Cursor up")
	return nil
}

func (e *Engine) handleOpenFile(args ...interface{}) error {
	fmt.Println("Open file")
	return nil
}

func (e *Engine) handleCloseExplorer(args ...interface{}) error {
	e.SetContext("global")
	return nil
}

func (e *Engine) handleNewAISession(args ...interface{}) error {
	fmt.Println("New AI session")
	return nil
}

func (e *Engine) handleNextSession(args ...interface{}) error {
	fmt.Println("Next session")
	return nil
}

func (e *Engine) handlePrevSession(args ...interface{}) error {
	fmt.Println("Previous session")
	return nil
}

func (e *Engine) handleSendToAI(args ...interface{}) error {
	fmt.Println("Send to AI")
	return nil
}

func (e *Engine) handleSendLineToAI(args ...interface{}) error {
	fmt.Println("Send line to AI")
	return nil
}

func (e *Engine) handleWindowCommand(args ...interface{}) error {
	fmt.Println("Window command")
	return nil
}

func (e *Engine) handleFocusLeft(args ...interface{}) error {
	fmt.Println("Focus left")
	return nil
}

func (e *Engine) handleFocusDown(args ...interface{}) error {
	fmt.Println("Focus down")
	return nil
}

func (e *Engine) handleFocusUp(args ...interface{}) error {
	fmt.Println("Focus up")
	return nil
}

func (e *Engine) handleFocusRight(args ...interface{}) error {
	fmt.Println("Focus right")
	return nil
}

func (e *Engine) handleExplainCode(args ...interface{}) error {
	fmt.Println("Explain code")
	return nil
}

func (e *Engine) handleFixCode(args ...interface{}) error {
	fmt.Println("Fix code")
	return nil
}

func (e *Engine) handleOptimizeCode(args ...interface{}) error {
	fmt.Println("Optimize code")
	return nil
}

func (e *Engine) handleReviewCode(args ...interface{}) error {
	fmt.Println("Review code")
	return nil
}

// GetBindings returns all registered keybindings
func (e *Engine) GetBindings() map[string]KeyBinding {
	return e.bindings
}

// GetBindingsForMode returns keybindings for a specific mode
func (e *Engine) GetBindingsForMode(mode VIMMode) []KeyBinding {
	var bindings []KeyBinding
	for _, binding := range e.bindings {
		if binding.Mode == mode {
			bindings = append(bindings, binding)
		}
	}
	return bindings
}

// FormatHelp returns a formatted help string for keybindings
func (e *Engine) FormatHelp(mode VIMMode) string {
	bindings := e.GetBindingsForMode(mode)
	if len(bindings) == 0 {
		return "No keybindings available for this mode"
	}

	var help strings.Builder
	help.WriteString(fmt.Sprintf("=== %s MODE ===\n", mode.String()))
	
	for _, binding := range bindings {
		help.WriteString(fmt.Sprintf("%-15s %s\n", binding.Key, binding.Description))
	}
	
	return help.String()
}