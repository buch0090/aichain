package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aichain/aichain/internal/ai"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sendMessageToChain sends a message to the AI chain
func (m *ChainExecutionModel) sendMessageToChain(message string) tea.Cmd {
	debugLogger.Printf("sendMessageToChain called with message: %s", message)
	
	// Add user message to all agent panes that should receive it
	userMsg := AgentMessage{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "human",
	}

	// For now, add to all agents (later we'll implement proper chain routing)
	for _, pane := range m.AgentPanes {
		pane.Messages = append(pane.Messages, userMsg)
		pane.Status = AgentThinking
		pane.LastActivity = "Processing..."
		
		// Update viewport content
		m.updatePaneContent(pane)
	}

	// NUCLEAR OPTION: Execute the first agent immediately and return the response as a command
	if len(m.AgentPanes) > 0 {
		debugLogger.Printf("NUCLEAR: Executing agent 0 immediately")
		
		return func() tea.Msg {
			debugLogger.Printf("NUCLEAR: Command executing, calling agent 0")
			response := m.callAIAgent(0, message)
			debugLogger.Printf("NUCLEAR: Got response from agent 0: %v", response != nil)
			return response
		}
	}
	
	return nil
}

// sendToAIAgentsCmd sends the message to AI agents using real Claude API
func (m *ChainExecutionModel) sendToAIAgentsCmd(userMessage string) tea.Cmd {
	debugLogger.Printf("sendToAIAgentsCmd called with %d agent panes", len(m.AgentPanes))
	
	// Use tea.Sequence to ensure execution
	if len(m.AgentPanes) > 0 {
		debugLogger.Printf("Creating command for agent 0")
		return tea.Sequence(
			func() tea.Msg {
				debugLogger.Printf("COMMAND FUNCTION EXECUTING: Processing agent 0")
				return m.callAIAgent(0, userMessage)
			},
		)
	}
	
	return nil
}

// callAIAgent makes an actual Claude API call for a specific agent
func (m *ChainExecutionModel) callAIAgent(agentIndex int, userMessage string) tea.Msg {
	debugLogger.Printf("callAIAgent called for agent %d with message: %s", agentIndex, userMessage)
	if agentIndex >= len(m.AgentPanes) {
		debugLogger.Printf("Agent index %d out of bounds (have %d agents)", agentIndex, len(m.AgentPanes))
		return nil
	}
	
	pane := m.AgentPanes[agentIndex]
	
	// Build conversation history from pane messages
	var conversationHistory []ai.Message
	for _, msg := range pane.Messages {
		// Parse timestamp from message, use current time as fallback
		timestamp, err := time.Parse("15:04:05", msg.Timestamp)
		if err != nil {
			timestamp = time.Now()
		}
		
		conversationHistory = append(conversationHistory, ai.Message{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: timestamp,
		})
	}
	
	// Create enhanced system prompt with working directory context
	enhancedSystemPrompt := fmt.Sprintf(`%s

WORKING DIRECTORY: %s
You are currently operating in this directory and have full access to it via the tools provided to you.
When discussing file paths, use relative paths from this working directory.
`, pane.Agent.SystemPrompt, m.workingDir)

	// Debug: Log the enhanced system prompt
	debugLogger.Printf("Agent %d system prompt:\n%s", agentIndex, enhancedSystemPrompt)

	// Create AI context with agent's configuration
	aiContext := ai.AIContext{
		SystemPrompt:        enhancedSystemPrompt,
		ConversationHistory: conversationHistory,
		SessionRole:         pane.Agent.Role,
		Temperature:         0.7, // Default temperature
		MaxTokens:          4000, // Reasonable limit
		CodeContext: &ai.CodeContext{
			Directory: m.workingDir,
		},
	}
	
	// Get temperature from agent config if available
	if pane.Agent.Config != nil {
		if temp, ok := pane.Agent.Config["temperature"].(float64); ok {
			aiContext.Temperature = temp
		}
	}
	
	// Make the Claude API call
	ctx := context.Background()
	debugLogger.Printf("Making Claude API call for agent %d", agentIndex)
	response, err := m.aiManager.SendMessage("claude", ctx, userMessage, aiContext)
	debugLogger.Printf("Claude API call completed for agent %d, error: %v", agentIndex, err)
	
	if err != nil {
		// Return error message
		return AgentResponseMsg{
			AgentIndex: agentIndex,
			Message: AgentMessage{
				Role:      "system",
				Content:   fmt.Sprintf("Error: %v", err),
				Timestamp: time.Now().Format("15:04:05"),
				Source:    "system",
			},
		}
	}
	
	// Use the response content directly (tool calling is handled in the Claude provider)
	content := response.Content
	debugLogger.Printf("Claude response content: %s", content)

	// Return successful response
	return AgentResponseMsg{
		AgentIndex: agentIndex,
		Message: AgentMessage{
			Role:      "assistant",
			Content:   content,
			Timestamp: time.Now().Format("15:04:05"),
			Source:    pane.Agent.Name,
		},
	}
}


// updateLayout updates the layout dimensions
func (m *ChainExecutionModel) updateLayout() {
	// Update message input width
	inputWidth := m.Width - 12 // Account for "Message: " prefix and padding
	if inputWidth < 20 {
		inputWidth = 20
	}
	m.messageInput.Width = inputWidth
}

// getStatusIcon returns an icon for the agent status
func (m *ChainExecutionModel) getStatusIcon(status AgentStatus) string {
	switch status {
	case AgentIdle:
		return "✅"
	case AgentThinking:
		return "🤔"
	case AgentResponding:
		return "💬"
	case AgentError:
		return "❌"
	default:
		return "⚪"
	}
}

// getStatusStyle returns the style for agent status
func (m *ChainExecutionModel) getStatusStyle(status AgentStatus) lipgloss.Style {
	switch status {
	case AgentIdle:
		return m.styles.StatusIdle
	case AgentThinking:
		return m.styles.StatusThinking
	case AgentResponding:
		return m.styles.StatusThinking
	case AgentError:
		return m.styles.StatusError
	default:
		return m.styles.StatusIdle
	}
}

// getMessageStyle returns the style for different message types
func (m *ChainExecutionModel) getMessageStyle(role string) lipgloss.Style {
	switch role {
	case "user":
		return m.styles.UserMessage
	case "assistant":
		return m.styles.AssistantMessage
	case "system":
		return m.styles.SystemMessage
	default:
		return m.styles.Message
	}
}

// wrapText wraps text to fit within specified width while preserving existing line breaks
func (m *ChainExecutionModel) wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	// Split by existing newlines first to preserve them
	existingLines := strings.Split(text, "\n")
	var wrappedLines []string

	for _, line := range existingLines {
		// If line is short enough, keep as-is
		if len(line) <= width {
			wrappedLines = append(wrappedLines, line)
			continue
		}

		// Wrap long lines by words
		words := strings.Fields(line)
		if len(words) == 0 {
			wrappedLines = append(wrappedLines, line) // Preserve empty lines
			continue
		}

		var currentLine strings.Builder
		for _, word := range words {
			if currentLine.Len() == 0 {
				currentLine.WriteString(word)
			} else if currentLine.Len()+1+len(word) <= width {
				currentLine.WriteString(" " + word)
			} else {
				wrappedLines = append(wrappedLines, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(word)
			}
		}

		if currentLine.Len() > 0 {
			wrappedLines = append(wrappedLines, currentLine.String())
		}
	}

	return strings.Join(wrappedLines, "\n")
}

// createChainExecutionStyles creates the styling for chain execution
func createChainExecutionStyles() ChainExecutionStyles {
	return ChainExecutionStyles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00ff88")).
			MarginBottom(1),

		AgentPane: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#555555")).
			Padding(1).
			Margin(0, 1),

		ActivePane: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00aaff")).
			Padding(1).
			Margin(0, 1).
			Bold(true),

		Message: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")),

		UserMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88ff88")),

		AssistantMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88aaff")),

		SystemMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffaa88")),

		Input: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#00aaff")).
			Padding(0, 1),

		StatusIdle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88ff88")),

		StatusThinking: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffaa00")),

		StatusError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff4444")),
	}
}