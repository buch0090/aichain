package tools

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// Tool represents a tool that agents can use
type Tool interface {
	Name() string
	Description() string
	Execute(args map[string]string, workingDir string) (string, error)
}

// ToolManager manages available tools for agents
type ToolManager struct {
	tools map[string]Tool
}

// NewToolManager creates a new tool manager
func NewToolManager() *ToolManager {
	tm := &ToolManager{
		tools: make(map[string]Tool),
	}
	
	// Register built-in tools
	tm.RegisterTool(&ReadFileTool{})
	tm.RegisterTool(&WriteFileTool{})
	tm.RegisterTool(&ListFilesTool{})
	
	return tm
}

// RegisterTool registers a tool
func (tm *ToolManager) RegisterTool(tool Tool) {
	tm.tools[tool.Name()] = tool
}

// GetTool gets a tool by name
func (tm *ToolManager) GetTool(name string) (Tool, bool) {
	tool, exists := tm.tools[name]
	return tool, exists
}

// ListTools returns all available tools
func (tm *ToolManager) ListTools() []Tool {
	tools := make([]Tool, 0, len(tm.tools))
	for _, tool := range tm.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolDescriptions returns descriptions of all tools for AI context
func (tm *ToolManager) GetToolDescriptions() string {
	var descriptions []string
	for _, tool := range tm.tools {
		descriptions = append(descriptions, fmt.Sprintf("- %s: %s", tool.Name(), tool.Description()))
	}
	return strings.Join(descriptions, "\n")
}

// ReadFileTool reads file contents
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string {
	return "READ_FILE"
}

func (t *ReadFileTool) Description() string {
	return "Read contents of a file. Usage: READ_FILE:path/to/file.go"
}

func (t *ReadFileTool) Execute(args map[string]string, workingDir string) (string, error) {
	filePath, ok := args["path"]
	if !ok {
		return "", fmt.Errorf("READ_file requires 'path' argument")
	}
	
	// Ensure path is within working directory
	fullPath := filepath.Join(workingDir, filePath)
	if !strings.HasPrefix(fullPath, workingDir) {
		return "", fmt.Errorf("file path must be within working directory")
	}
	
	content, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", filePath, err)
	}
	
	return fmt.Sprintf("File: %s\n```\n%s\n```", filePath, string(content)), nil
}

// WriteFileTool writes content to a file
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string {
	return "WRITE_FILE"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a file. Usage: WRITE_FILE:path/to/file.go:content here"
}

func (t *WriteFileTool) Execute(args map[string]string, workingDir string) (string, error) {
	filePath, ok := args["path"]
	if !ok {
		return "", fmt.Errorf("write_file requires 'path' argument")
	}
	
	content, ok := args["content"]
	if !ok {
		return "", fmt.Errorf("write_file requires 'content' argument")
	}
	
	// Ensure path is within working directory
	fullPath := filepath.Join(workingDir, filePath)
	if !strings.HasPrefix(fullPath, workingDir) {
		return "", fmt.Errorf("file path must be within working directory")
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %v", dir, err)
	}
	
	// Write file
	if err := ioutil.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", filePath, err)
	}
	
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), filePath), nil
}

// ListFilesTool lists files in a directory
type ListFilesTool struct{}

func (t *ListFilesTool) Name() string {
	return "LIST_FILES"
}

func (t *ListFilesTool) Description() string {
	return "List files in a directory. Usage: LIST_FILES:path/to/directory"
}

func (t *ListFilesTool) Execute(args map[string]string, workingDir string) (string, error) {
	dirPath := args["path"]
	if dirPath == "" {
		dirPath = "."
	}
	
	// Ensure path is within working directory
	fullPath := filepath.Join(workingDir, dirPath)
	if !strings.HasPrefix(fullPath, workingDir) {
		return "", fmt.Errorf("directory path must be within working directory")
	}
	
	files, err := ioutil.ReadDir(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to list directory %s: %v", dirPath, err)
	}
	
	var fileList []string
	for _, file := range files {
		if file.IsDir() {
			fileList = append(fileList, file.Name()+"/")
		} else {
			fileList = append(fileList, file.Name())
		}
	}
	
	return fmt.Sprintf("Files in %s:\n%s", dirPath, strings.Join(fileList, "\n")), nil
}