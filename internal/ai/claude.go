package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	// Truncation limits for tool output (matches pi's defaults)
	maxOutputLines = 2000
	maxOutputBytes = 50 * 1024 // 50KB
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

	// Define tools
	tools := buildToolDefinitions()

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

	// Tool calling loop — no hard round limit. The model decides when to stop.
	// We only break on: (a) model stops calling tools, (b) repeated identical
	// failures (infinite loop), or (c) context timeout.
	allMessages := messages
	var finalContent strings.Builder
	toolRounds := 0
	
	// Track repeated tool failures to break infinite loops
	var lastToolCall string
	var lastToolError string
	consecutiveFailures := 0
	
	for {
		toolRounds++
		c.sdkLogger.Printf("Tool round %d", toolRounds)
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
		var offset, limit int
		if v, ok := inputMap["offset"]; ok {
			offset = toInt(v)
		}
		if v, ok := inputMap["limit"]; ok {
			limit = toInt(v)
		}
		return c.readFile(path, workingDir, offset, limit)

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

	case "edit_file":
		path, ok := inputMap["path"].(string)
		if !ok {
			return "", fmt.Errorf("edit_file requires 'path' parameter")
		}
		editsRaw, ok := inputMap["edits"]
		if !ok {
			return "", fmt.Errorf("edit_file requires 'edits' parameter")
		}
		return c.editFile(path, editsRaw, workingDir)

	case "bash":
		command, ok := inputMap["command"].(string)
		if !ok {
			return "", fmt.Errorf("bash requires 'command' parameter")
		}
		var timeout int
		if v, ok := inputMap["timeout"]; ok {
			timeout = toInt(v)
		}
		return c.bash(command, workingDir, timeout)

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Tool definitions
// ─────────────────────────────────────────────────────────────────────

func buildToolDefinitions() []anthropic.ToolUnionParam {
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
				"description": "Path to the file to read (relative or absolute)",
			},
			"offset": map[string]interface{}{
				"type":        "number",
				"description": "Line number to start reading from (1-indexed, optional)",
			},
			"limit": map[string]interface{}{
				"type":        "number",
				"description": "Maximum number of lines to read (optional)",
			},
		},
		Required: []string{"path"},
	}

	writeFileSchema := anthropic.ToolInputSchemaParam{
		Type: "object",
		Properties: map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to write (creates parent directories)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		Required: []string{"path", "content"},
	}

	editFileSchema := anthropic.ToolInputSchemaParam{
		Type: "object",
		Properties: map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to edit (relative or absolute)",
			},
			"edits": map[string]interface{}{
				"type": "array",
				"description": "One or more targeted replacements. Each edit is matched against the original file, not incrementally. Do not include overlapping or nested edits. If two changes touch the same block or nearby lines, merge them into one edit instead.",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"oldText": map[string]interface{}{
							"type":        "string",
							"description": "Exact text to find. Must be unique in the file and must not overlap with other edits.",
						},
						"newText": map[string]interface{}{
							"type":        "string",
							"description": "Replacement text.",
						},
					},
					"required": []string{"oldText", "newText"},
				},
			},
		},
		Required: []string{"path", "edits"},
	}

	bashSchema := anthropic.ToolInputSchemaParam{
		Type: "object",
		Properties: map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Bash command to execute",
			},
			"timeout": map[string]interface{}{
				"type":        "number",
				"description": "Timeout in seconds (optional, default 30)",
			},
		},
		Required: []string{"command"},
	}

	return []anthropic.ToolUnionParam{
		anthropic.ToolUnionParamOfTool(readFileSchema, "read_file"),
		anthropic.ToolUnionParamOfTool(bashSchema, "bash"),
		anthropic.ToolUnionParamOfTool(editFileSchema, "edit_file"),
		anthropic.ToolUnionParamOfTool(writeFileSchema, "write_file"),
		anthropic.ToolUnionParamOfTool(listFilesSchema, "list_files"),
	}
}

// ─────────────────────────────────────────────────────────────────────
// Tool implementations
// ─────────────────────────────────────────────────────────────────────

// resolvePath resolves and validates a path within the working directory.
func resolvePath(path, workingDir string) (string, error) {
	var fullPath string
	if filepath.IsAbs(path) {
		fullPath = filepath.Clean(path)
	} else {
		fullPath = filepath.Join(workingDir, path)
	}
	// Security: must be within working directory
	absWork, _ := filepath.Abs(workingDir)
	absFull, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absFull, absWork) {
		return "", fmt.Errorf("path %q is outside the allowed directory", path)
	}
	return fullPath, nil
}

// truncateOutput truncates text to maxOutputLines / maxOutputBytes (whichever
// hits first), keeping from the tail for bash and from the head for reads.
func truncateOutput(text string, keepTail bool) string {
	if len(text) <= maxOutputBytes {
		lines := strings.Split(text, "\n")
		if len(lines) <= maxOutputLines {
			return text
		}
	}

	lines := strings.Split(text, "\n")
	totalLines := len(lines)

	if keepTail {
		// Keep the last N lines (for bash output — errors are at the end)
		start := 0
		if totalLines > maxOutputLines {
			start = totalLines - maxOutputLines
		}
		selected := lines[start:]
		result := strings.Join(selected, "\n")
		if len(result) > maxOutputBytes {
			result = result[len(result)-maxOutputBytes:]
		}
		if start > 0 || len(result) < len(text) {
			return fmt.Sprintf("[Output truncated: showing last %d of %d lines]\n%s", len(selected), totalLines, result)
		}
		return result
	}

	// Keep the first N lines (for file reads)
	end := totalLines
	if end > maxOutputLines {
		end = maxOutputLines
	}
	selected := lines[:end]
	result := strings.Join(selected, "\n")
	if len(result) > maxOutputBytes {
		result = result[:maxOutputBytes]
	}
	if end < totalLines || len(result) < len(text) {
		return fmt.Sprintf("%s\n\n[Truncated: showing %d of %d lines. Use offset=%d to continue.]", result, end, totalLines, end+1)
	}
	return result
}

func (c *ClaudeProvider) listFiles(path, workingDir string) (string, error) {
	fullPath, err := resolvePath(path, workingDir)
	if err != nil {
		return "", err
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

	result := fmt.Sprintf("Files in %s:\n%s", path, strings.Join(fileList, "\n"))
	return truncateOutput(result, false), nil
}

func (c *ClaudeProvider) readFile(path, workingDir string, offset, limit int) (string, error) {
	fullPath, err := resolvePath(path, workingDir)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", path, err)
	}

	allLines := strings.Split(string(content), "\n")
	totalLines := len(allLines)

	// Apply offset (1-indexed)
	startLine := 0
	if offset > 0 {
		startLine = offset - 1
	}
	if startLine >= totalLines {
		return "", fmt.Errorf("offset %d is beyond end of file (%d lines total)", offset, totalLines)
	}

	// Apply limit
	endLine := totalLines
	if limit > 0 && startLine+limit < totalLines {
		endLine = startLine + limit
	}

	selected := allLines[startLine:endLine]
	text := strings.Join(selected, "\n")

	// Truncate if still too large
	text = truncateOutput(text, false)

	// Build result with continuation hint
	var result string
	if startLine > 0 || endLine < totalLines {
		result = fmt.Sprintf("Content of %s (lines %d-%d of %d):\n%s", path, startLine+1, endLine, totalLines, text)
		if endLine < totalLines {
			result += fmt.Sprintf("\n\n[%d more lines. Use offset=%d to continue.]", totalLines-endLine, endLine+1)
		}
	} else {
		result = fmt.Sprintf("Content of %s (%d lines):\n%s", path, totalLines, text)
	}

	return result, nil
}

func (c *ClaudeProvider) writeFile(path, content, workingDir string) (string, error) {
	fullPath, err := resolvePath(path, workingDir)
	if err != nil {
		return "", err
	}

	// Create directory if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	err = os.WriteFile(fullPath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", path, err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

func (c *ClaudeProvider) editFile(path string, editsRaw interface{}, workingDir string) (string, error) {
	fullPath, err := resolvePath(path, workingDir)
	if err != nil {
		return "", err
	}

	// Read current file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", path, err)
	}

	// Parse edits array
	editsJSON, err := json.Marshal(editsRaw)
	if err != nil {
		return "", fmt.Errorf("failed to marshal edits: %v", err)
	}

	var edits []struct {
		OldText string `json:"oldText"`
		NewText string `json:"newText"`
	}
	if err := json.Unmarshal(editsJSON, &edits); err != nil {
		return "", fmt.Errorf("failed to parse edits: %v", err)
	}

	if len(edits) == 0 {
		return "", fmt.Errorf("edits array is empty")
	}

	// Apply each edit
	result := string(content)
	var applied []string
	for i, edit := range edits {
		if edit.OldText == "" {
			return "", fmt.Errorf("edit %d: oldText is empty", i)
		}
		if !strings.Contains(result, edit.OldText) {
			return "", fmt.Errorf("edit %d: oldText not found in file (ensure exact match including whitespace)", i)
		}
		count := strings.Count(result, edit.OldText)
		if count > 1 {
			return "", fmt.Errorf("edit %d: oldText matches %d locations (must be unique)", i, count)
		}
		result = strings.Replace(result, edit.OldText, edit.NewText, 1)
		applied = append(applied, fmt.Sprintf("edit %d: replaced %d chars with %d chars", i+1, len(edit.OldText), len(edit.NewText)))
	}

	// Write the result
	if err := os.WriteFile(fullPath, []byte(result), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", path, err)
	}

	return fmt.Sprintf("Successfully applied %d edit(s) to %s:\n%s", len(edits), path, strings.Join(applied, "\n")), nil
}

func (c *ClaudeProvider) bash(command, workingDir string, timeout int) (string, error) {
	if timeout <= 0 {
		timeout = 30
	}

	c.sdkLogger.Printf("Executing bash command: %q (timeout: %ds)", command, timeout)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workingDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine stdout and stderr
	var output strings.Builder
	if stdout.Len() > 0 {
		output.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("STDERR:\n")
		output.WriteString(stderr.String())
	}

	result := truncateOutput(output.String(), true) // keep tail for bash

	if ctx.Err() == context.DeadlineExceeded {
		return result + fmt.Sprintf("\n[Command timed out after %ds]", timeout), nil
	}

	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return fmt.Sprintf("%s\n[Exit code: %d]", result, exitCode), nil
	}

	return result, nil
}

// toInt converts a JSON number (float64) or string to int.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
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