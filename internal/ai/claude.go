package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"path/filepath"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ClaudeProvider implements the Provider interface for Anthropic's Claude
type ClaudeProvider struct {
	client *anthropic.Client
	sdkLogger *log.Logger
}

// NewClaudeProvider creates a new Claude provider
func NewClaudeProvider() *ClaudeProvider {
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		return nil
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	// Create SDK debug logger
	sdkLogFile, err := os.OpenFile("claude-sdk-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// Fall back to stderr if can't create log file
		sdkLogFile = os.Stderr
	}
	sdkLogger := log.New(sdkLogFile, "SDK-DEBUG: ", log.LstdFlags)

	return &ClaudeProvider{
		client: &client,
		sdkLogger: sdkLogger,
	}
}

// GetProviderName returns the provider name
func (c *ClaudeProvider) GetProviderName() string {
	return "claude"
}

// SendMessage sends a message to Claude using the official SDK with proper tool calling
func (c *ClaudeProvider) SendMessage(ctx context.Context, prompt string, aiContext AIContext) (*AIResponse, error) {
	// Build messages from conversation history and current prompt
	messages := make([]anthropic.MessageParam, 0, len(aiContext.ConversationHistory)+1)
	
	// Add conversation history
	for _, msg := range aiContext.ConversationHistory {
		if msg.Role != "system" { // System messages go in the system field
			if msg.Role == "user" {
				messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
			} else if msg.Role == "assistant" {
				messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
			}
		}
	}

	// Add current prompt
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)))

	// Set defaults if not provided
	temperature := aiContext.Temperature
	if temperature == 0 {
		temperature = 0.7
	}
	
	maxTokens := aiContext.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1024
	}

	// Define tools using proper SDK format
	listFilesSchema := anthropic.ToolInputSchemaParam{
		Type: "object",
		Properties: map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path to list (use '.' for current directory)",
			},
		},
		Required: []string{"path"},
	}
	
	readFileSchema := anthropic.ToolInputSchemaParam{
		Type: "object",
		Properties: map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		Required: []string{"path"},
	}
	
	writeFileSchema := anthropic.ToolInputSchemaParam{
		Type: "object",
		Properties: map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		Required: []string{"path", "content"},
	}

	// Convert to ToolUnionParam format
	tools := []anthropic.ToolUnionParam{
		anthropic.ToolUnionParamOfTool(listFilesSchema, "list_files"),
		anthropic.ToolUnionParamOfTool(readFileSchema, "read_file"), 
		anthropic.ToolUnionParamOfTool(writeFileSchema, "write_file"),
	}

	// Prepare the request params
	params := anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_5_20250929,
		MaxTokens: int64(maxTokens),
		Messages:  messages,
		Tools:     tools,
	}

	// Add system prompt properly
	if aiContext.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{
			Type: "text", 
			Text: aiContext.SystemPrompt,
		}}
	}

	// Add temperature if provided
	if temperature != 0 {
		params.Temperature = anthropic.Float(temperature)
	}

	// Tool calling loop - handle multiple rounds of tool use
	allMessages := messages
	var finalContent strings.Builder
	maxToolRounds := 25  // Increased from 10 to 25
	toolRounds := 0
	
	// Track repeated tool failures to break infinite loops
	var lastToolCall string
	var lastToolError string
	consecutiveFailures := 0
	
	for {
		toolRounds++
		if toolRounds > maxToolRounds {
			c.sdkLogger.Printf("Maximum tool calling rounds (%d) exceeded, breaking loop", maxToolRounds)
			finalContent.WriteString("\n[Tool calling limit reached - response may be incomplete]")
			break
		}
		// Make the API call
		message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:       params.Model,
			MaxTokens:   params.MaxTokens,
			Messages:    allMessages,
			Tools:       params.Tools,
			System:      params.System,
			Temperature: params.Temperature,
		})
		if err != nil {
			return nil, fmt.Errorf("Claude API request failed: %w", err)
		}

		// Process content blocks and look for tool uses
		var toolResults []anthropic.ContentBlockParamUnion
		hasTools := false
		
		c.sdkLogger.Printf("Processing %d content blocks", len(message.Content))
		
		for _, block := range message.Content {
			c.sdkLogger.Printf("Block type: %s", block.Type)
			if block.Type == "text" {
				textBlock := block.AsText()
				c.sdkLogger.Printf("Text block content: %q", textBlock.Text)
				finalContent.WriteString(textBlock.Text)
			} else if block.Type == "tool_use" {
				hasTools = true
				toolUseBlock := block.AsToolUse()
				
				c.sdkLogger.Printf("Tool use detected - Name: %s, ID: %s, Input: %+v", toolUseBlock.Name, toolUseBlock.ID, toolUseBlock.Input)
				
				// Execute the tool
				toolResult, toolError := c.executeToolFunction(toolUseBlock.Name, toolUseBlock.Input, aiContext)
				
				c.sdkLogger.Printf("Tool execution result - Name: %s, Error: %v, Result: %q", toolUseBlock.Name, toolError, toolResult)
				
				// Track consecutive failures to break infinite loops  
				inputBytes, _ := json.Marshal(toolUseBlock.Input)
				currentToolCall := fmt.Sprintf("%s:%s", toolUseBlock.Name, string(inputBytes))
				currentToolErrorStr := ""
				if toolError != nil {
					currentToolErrorStr = toolError.Error()
				}
				
				if currentToolCall == lastToolCall && currentToolErrorStr == lastToolError && toolError != nil {
					consecutiveFailures++
					c.sdkLogger.Printf("Consecutive failure #%d for tool %s with same error: %s", consecutiveFailures, toolUseBlock.Name, toolError)
					
					if consecutiveFailures >= 3 {
						c.sdkLogger.Printf("Breaking infinite loop - same tool call failed 3 times in a row")
						finalContent.WriteString(fmt.Sprintf("\n[Stopped infinite loop: %s failed repeatedly with: %s]", toolUseBlock.Name, toolError))
						hasTools = false // Force exit from tool calling loop
						break
					}
				} else {
					consecutiveFailures = 0 // Reset counter for different tool calls or successful calls
				}
				
				lastToolCall = currentToolCall
				lastToolError = currentToolErrorStr
				
				// Create tool result block
				if toolError != nil {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUseBlock.ID, fmt.Sprintf("Error: %v", toolError), true))
				} else {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUseBlock.ID, toolResult, false))
				}
			}
		}
		
		// If no tools were used, we're done
		if !hasTools {
			break
		}
		
		// Add assistant message and tool results, then continue the loop
		assistantBlocks := make([]anthropic.ContentBlockParamUnion, len(message.Content))
		for i, block := range message.Content {
			assistantBlocks[i] = block.ToParam()
		}
		allMessages = append(allMessages, anthropic.NewAssistantMessage(assistantBlocks...))
		allMessages = append(allMessages, anthropic.NewUserMessage(toolResults...))
	}

	return &AIResponse{
		Content:      strings.TrimSpace(finalContent.String()),
		TokensUsed:   0, // TODO: Get from response usage
		Model:        "claude-sonnet-4.5",
		Confidence:   1.0,
		InputTokens:  0,
		OutputTokens: 0,
	}, nil
}

// executeToolFunction executes a tool function based on the tool name and input
func (c *ClaudeProvider) executeToolFunction(toolName string, input interface{}, aiContext AIContext) (string, error) {
	c.sdkLogger.Printf("Raw input type: %T, value: %+v", input, input)
	
	var inputMap map[string]interface{}
	
	// Handle different input formats
	switch v := input.(type) {
	case map[string]interface{}:
		// Already parsed JSON
		inputMap = v
	case []byte:
		// Raw JSON bytes
		if err := json.Unmarshal(v, &inputMap); err != nil {
			c.sdkLogger.Printf("Failed to unmarshal JSON bytes: %v", err)
			return "", fmt.Errorf("failed to parse JSON input for tool %s: %v", toolName, err)
		}
	case string:
		// JSON string
		if err := json.Unmarshal([]byte(v), &inputMap); err != nil {
			c.sdkLogger.Printf("Failed to unmarshal JSON string: %v", err)
			return "", fmt.Errorf("failed to parse JSON input for tool %s: %v", toolName, err)
		}
	case json.RawMessage:
		// JSON raw message
		if err := json.Unmarshal(v, &inputMap); err != nil {
			c.sdkLogger.Printf("Failed to unmarshal JSON raw message: %v", err)
			return "", fmt.Errorf("failed to parse JSON input for tool %s: %v", toolName, err)
		}
	default:
		c.sdkLogger.Printf("Unknown input type: %T", input)
		return "", fmt.Errorf("unsupported input format for tool %s: %T", toolName, input)
	}
	
	c.sdkLogger.Printf("Parsed input map: %+v", inputMap)

	workingDir := aiContext.CodeContext.Directory
	
	switch toolName {
	case "list_files":
		path, ok := inputMap["path"].(string)
		if !ok {
			return "", fmt.Errorf("list_files requires 'path' parameter")
		}
		return c.listFiles(path, workingDir)
		
	case "read_file":
		path, ok := inputMap["path"].(string)
		if !ok {
			return "", fmt.Errorf("read_file requires 'path' parameter")
		}
		return c.readFile(path, workingDir)
		
	case "write_file":
		path, ok := inputMap["path"].(string)
		if !ok {
			return "", fmt.Errorf("write_file requires 'path' parameter")
		}
		content, ok := inputMap["content"].(string)
		if !ok {
			return "", fmt.Errorf("write_file requires 'content' parameter")
		}
		return c.writeFile(path, content, workingDir)
		
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// listFiles implements the LIST_FILES tool
func (c *ClaudeProvider) listFiles(path, workingDir string) (string, error) {
	fullPath := filepath.Join(workingDir, path)
	if path == "." {
		fullPath = workingDir
	}
	
	// Security check
	if !strings.HasPrefix(fullPath, workingDir) {
		return "", fmt.Errorf("path must be within working directory")
	}
	
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to list directory %s: %v", path, err)
	}
	
	var fileList []string
	for _, entry := range entries {
		if entry.IsDir() {
			fileList = append(fileList, entry.Name()+"/")
		} else {
			fileList = append(fileList, entry.Name())
		}
	}
	
	return fmt.Sprintf("Files in %s:\n%s", path, strings.Join(fileList, "\n")), nil
}

// readFile implements the READ_FILE tool
func (c *ClaudeProvider) readFile(path, workingDir string) (string, error) {
	fullPath := filepath.Join(workingDir, path)
	
	// Security check
	if !strings.HasPrefix(fullPath, workingDir) {
		return "", fmt.Errorf("path must be within working directory")
	}
	
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", path, err)
	}
	
	return fmt.Sprintf("Content of %s:\n```\n%s\n```", path, string(content)), nil
}

// writeFile implements the WRITE_FILE tool
func (c *ClaudeProvider) writeFile(path, content, workingDir string) (string, error) {
	fullPath := filepath.Join(workingDir, path)
	
	// Security check
	if !strings.HasPrefix(fullPath, workingDir) {
		return "", fmt.Errorf("path must be within working directory")
	}
	
	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}
	
	err := os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", path, err)
	}
	
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// GetModels returns available Claude models
func (c *ClaudeProvider) GetModels() []Model {
	return []Model{
		{
			Name:        string(anthropic.ModelClaudeSonnet4_5_20250929),
			DisplayName: "Claude Sonnet 4.5",
			ContextSize: 200000,
			MaxTokens:   8192,
			Strengths:   []string{"reasoning", "coding", "analysis", "writing"},
			Pricing: &Pricing{
				InputCost:  0.000003, // $0.003 per 1K tokens
				OutputCost: 0.000015, // $0.015 per 1K tokens
				Currency:   "USD",
			},
		},
		{
			Name:        string(anthropic.ModelClaudeOpus4_5_20251101),
			DisplayName: "Claude Opus 4.5",
			ContextSize: 200000,
			MaxTokens:   8192,
			Strengths:   []string{"complex reasoning", "creative writing", "advanced analysis"},
			Pricing: &Pricing{
				InputCost:  0.000015, // $0.015 per 1K tokens
				OutputCost: 0.000075, // $0.075 per 1K tokens
				Currency:   "USD",
			},
		},
		{
			Name:        "claude-3-haiku-20240307",
			DisplayName: "Claude Haiku",
			ContextSize: 200000,
			MaxTokens:   8192,
			Strengths:   []string{"speed", "simple tasks", "cost-effective"},
			Pricing: &Pricing{
				InputCost:  0.00000025, // $0.00025 per 1K tokens
				OutputCost: 0.00000125, // $0.00125 per 1K tokens
				Currency:   "USD",
			},
		},
	}
}

// GetCapabilities returns the provider's capabilities
func (c *ClaudeProvider) GetCapabilities() Capabilities {
	return Capabilities{
		CodeGeneration:   true,
		CodeReview:      true,
		TextGeneration:  true,
		ImageAnalysis:   true,
		FileUpload:      false,
		FunctionCalling: true,
		Languages:       []string{"go", "python", "javascript", "typescript", "rust", "java", "c++"},
	}
}

// GetRateLimits returns the provider's rate limits
func (c *ClaudeProvider) GetRateLimits() RateLimits {
	return RateLimits{
		RequestsPerMinute: 60,
		RequestsPerHour:   3600,
		RequestsPerDay:    1000,
		TokensPerMinute:   200000,
	}
}