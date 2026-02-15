package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aichain/aichain/internal/app"
	"github.com/aichain/aichain/internal/tui"
	
	"github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "aichain",
		Short: "AIChain - AI-collaborative terminal with VIM-like interface",
		Long: `AIChain is a standalone terminal application that provides VIM-like controls
for managing multiple AI agent chains and enabling AI-to-AI collaboration.

Features:
- Multiple AI sessions with real-time collaboration
- VIM-like keybindings throughout the interface
- AI debate and pipeline modes
- File explorer with VIM navigation
- Session templates for common workflows`,
		Run: runApplication,
	}

	// Global flags
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file (default: ~/.config/aichain/config.yaml)")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "enable debug mode")

	// Subcommands
	rootCmd.AddCommand(
		newVersionCmd(),
		newConfigCmd(),
		newSessionCmd(),
		newDebateCmd(),
		newPipelineCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runApplication(cmd *cobra.Command, args []string) {
	// Check for required environment variables
	if os.Getenv("CLAUDE_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "Error: CLAUDE_API_KEY environment variable is required")
		fmt.Fprintln(os.Stderr, "Please set your Claude API key:")
		fmt.Fprintln(os.Stderr, "  export CLAUDE_API_KEY=your-api-key-here")
		os.Exit(1)
	}

	// Check for allowed directory (optional for now)
	allowedDir := os.Getenv("AICHAIN_ALLOWED_DIR")
	if allowedDir == "" {
		// Default to current working directory if not set
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not get working directory: %v\n", err)
			allowedDir = "."
		} else {
			allowedDir = wd
		}
		fmt.Printf("Directory access: %s (set AICHAIN_ALLOWED_DIR to customize)\n", allowedDir)
	}

	// Create application with directory restrictions
	application := app.NewApplicationWithConfig(&app.Config{
		AllowedDirectory: allowedDir,
	})
	
	// Initialize application
	if err := application.Initialize(); err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	// Create TUI with chain setup mode
	tuiModel := tui.NewModelWithChainSetup(application)

	// Create Bubble Tea program
	program := tea.NewProgram(
		tuiModel,
		tea.WithAltScreen(),       // Use alternate screen buffer
		// tea.WithMouseCellMotion(), // Mouse support disabled for text selection
	)

	// Handle graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nShutting down AIChain...")
		application.Shutdown()
		program.Quit()
		cancel()
	}()

	// Start the TUI
	if _, err := program.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("AIChain %s\n", version)
			fmt.Printf("Commit: %s\n", commit)
			fmt.Printf("Built: %s\n", date)
		},
	}
}

func newConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration management",
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize configuration",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Initializing AIChain configuration...")
			// TODO: Implement config initialization
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Current AIChain configuration:")
			// TODO: Implement config display
		},
	})

	return configCmd
}

func newSessionCmd() *cobra.Command {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "Session management",
	}

	sessionCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all sessions",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Active sessions:")
			// TODO: Implement session listing
		},
	})

	sessionCmd.AddCommand(&cobra.Command{
		Use:   "create [name]",
		Short: "Create a new session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			fmt.Printf("Creating session: %s\n", name)
			// TODO: Implement session creation
		},
	})

	return sessionCmd
}

func newDebateCmd() *cobra.Command {
	debateCmd := &cobra.Command{
		Use:   "debate [topic]",
		Short: "Start an AI debate session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			topic := args[0]
			fmt.Printf("Starting AI debate on topic: %s\n", topic)
			
			// Create application for CLI command
			application := app.NewApplication()
			if err := application.Initialize(); err != nil {
				log.Fatalf("Failed to initialize: %v", err)
			}

			// Create debate session
			pipeline, err := application.CreateDebateSession("Claude", "GPT", topic)
			if err != nil {
				log.Fatalf("Failed to create debate: %v", err)
			}

			fmt.Printf("Debate pipeline created: %s\n", pipeline.ID)
			fmt.Println("Launch AIChain to participate in the debate.")
		},
	}

	return debateCmd
}

func newPipelineCmd() *cobra.Command {
	pipelineCmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Pipeline management",
	}

	pipelineCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List active pipelines",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Active pipelines:")
			// TODO: Implement pipeline listing
		},
	})

	pipelineCmd.AddCommand(&cobra.Command{
		Use:   "create [name] [session1] [session2]",
		Short: "Create a new pipeline",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			name, session1, session2 := args[0], args[1], args[2]
			fmt.Printf("Creating pipeline: %s (%s <-> %s)\n", name, session1, session2)
			// TODO: Implement pipeline creation
		},
	})

	return pipelineCmd
}