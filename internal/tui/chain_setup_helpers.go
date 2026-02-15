package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)


// viewComplete renders the completion screen
func (m ChainSetupModel) viewComplete() string {
	var b strings.Builder

	// Title
	title := m.styles.Success.Render("🎉 Chain Setup Complete!")
	b.WriteString(title + "\n\n")

	// Chain summary
	b.WriteString("Your AI Chain:\n")
	b.WriteString(m.setupFlow.GetChainVisualization() + "\n")

	// Start button
	startButton := m.styles.ActiveButton.Render("🚀 Start Chain Execution")
	b.WriteString(startButton + "\n\n")

	// Footer
	b.WriteString("Press Enter or Space to start • Ctrl+C to quit")

	return b.String()
}


// createChainSetupStyles creates the styling for chain setup
func createChainSetupStyles() ChainSetupStyles {
	return ChainSetupStyles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00ff88")).
			MarginBottom(1),
		
		Subtitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginBottom(1),
		
		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Border(lipgloss.RoundedBorder()).
			Padding(1).
			MarginTop(1),
		
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff4444")).
			Bold(true),
		
		Success: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00ff88")).
			Bold(true),
		
		Field: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")),
		
		ActiveField: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00aaff")).
			Bold(true),
		
		Button: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#333333")).
			Padding(0, 2).
			MarginTop(1),
		
		ActiveButton: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#00ff88")).
			Padding(0, 2).
			MarginTop(1).
			Bold(true),
	}
}

// getModelExplanation returns a user-friendly explanation for Claude models
func (m *ChainSetupModel) getModelExplanation(modelID string) string {
	switch modelID {
	case "claude-3-5-sonnet-20241022":
		return "Claude Sonnet 3.5 - Great for coding and general tasks"
	case "claude-3-opus-20240229":
		return "Claude Opus 3 - Best for complex reasoning and architecture"
	case "claude-3-haiku-20240307":
		return "Claude Haiku 3 - Fast and cost-effective for quick tasks"
	default:
		return fmt.Sprintf("Claude Model: %s", modelID)
	}
}