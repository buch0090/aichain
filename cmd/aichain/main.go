package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aichain/aichain/internal/app"
	"github.com/aichain/aichain/internal/tui"
	"github.com/aichain/aichain/pkg/server"
	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.1.0"

func main() {
	var (
		serverCmd   = flag.Bool("server", false, "Start the AIChain server")
		port        = flag.Int("port", 8747, "Server port")
		versionFlag = flag.Bool("version", false, "Show version")
		setupFlag   = flag.Bool("setup", false, "Run interactive setup")
		debugFlag   = flag.Bool("debug", false, "Enable debug mode (no alt screen, allows text selection)")
	)
	flag.Parse()

	if *versionFlag {
		fmt.Printf("AIChain v%s\n", version)
		os.Exit(0)
	}

	if *setupFlag {
		// Run chain setup TUI with stable configuration
		application := app.NewApplication()
		if err := application.Initialize(); err != nil {
			log.Fatalf("Failed to initialize application: %v", err)
		}
		
		tuiModel := tui.NewModelWithChainSetup(application)
		
		// Create Bubble Tea program with stable options
		var programOptions []tea.ProgramOption
		if !*debugFlag {
			programOptions = append(programOptions, 
				tea.WithAltScreen(),       // Use alternate screen buffer
				tea.WithMouseCellMotion(), // Enable mouse support
			)
		} else {
			// Debug mode: no alt screen, allows text selection
			programOptions = append(programOptions, tea.WithMouseCellMotion())
			fmt.Println("Debug mode enabled - check claudevim-debug.log for detailed logs")
		}
		
		program := tea.NewProgram(tuiModel, programOptions...)
		
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
		os.Exit(0)
	}

	if *serverCmd {
		log.Printf("Starting AIChain server on port %d", *port)
		server := server.New(*port)
		if err := server.Start(); err != nil {
			log.Fatal("Failed to start server:", err)
		}
		return
	}

	// Check for DSL file argument
	args := flag.Args()
	if len(args) > 0 {
		dslFile := args[0]
		
		// Initialize application
		application := app.NewApplication()
		if err := application.Initialize(); err != nil {
			log.Fatalf("Failed to initialize application: %v", err)
		}
		
		// Create TUI model directly with DSL file
		tuiModel := tui.NewModelWithDSLFile(application, dslFile)
		
		// Create Bubble Tea program
		var programOptions []tea.ProgramOption
		if !*debugFlag {
			programOptions = append(programOptions, 
				tea.WithAltScreen(),       // Use alternate screen buffer
				tea.WithMouseCellMotion(), // Enable mouse support
			)
		} else {
			// Debug mode: no alt screen, allows text selection
			programOptions = append(programOptions, tea.WithMouseCellMotion())
			fmt.Println("Debug mode enabled - check claudevim-debug.log for detailed logs")
		}
		
		program := tea.NewProgram(tuiModel, programOptions...)
		
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
		return
	}

	// Default: show help
	fmt.Printf("AIChain v%s - VIM with AI Superpowers\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  aichain [dsl-file]                  Execute a DSL file")
	fmt.Println("  aichain --setup                     Run interactive setup")
	fmt.Println("  aichain --server [--port 8747]      Start the backend server")
	fmt.Println("  aichain --version                   Show version")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  aichain my-chain.dsl                Run agents defined in my-chain.dsl")
	fmt.Println("  aichain --setup                     Interactive chain builder")
}