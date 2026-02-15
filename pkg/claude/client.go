package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client represents a Claude API client
type Client struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// Request represents a Claude API request
type ClaudeRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	Messages    []Message `json:"messages"`
}

// Message represents a message in the Claude API format
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response represents a Claude API response
type ClaudeResponse struct {
	Content []ContentBlock `json:"content"`
	Error   *APIError      `json:"error,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// NewClient creates a new Claude API client
func NewClient() *Client {
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		// Try to get from config file later
		// For now, use empty key (will cause error on first request)
	}

	return &Client{
		apiKey:  apiKey,
		baseURL: "https://api.anthropic.com/v1",
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendMessage sends a message to Claude and returns the response
func (c *Client) SendMessage(prompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("Claude API key not set. Set CLAUDE_API_KEY environment variable or run 'claudevim --setup'")
	}

	// Prepare the request
	req := ClaudeRequest{
		Model:       "claude-opus-4-5-20251101", // Use the model you specified
		MaxTokens:   4000,
		Temperature: 0.7,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	// Convert to JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", c.baseURL+"/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
	httpReq.Header.Set("Anthropic-Version", "2023-06-01")

	// Send request
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Check for API errors
	if claudeResp.Error != nil {
		return "", fmt.Errorf("Claude API error: %s", claudeResp.Error.Message)
	}

	// Extract text from response
	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude API")
	}

	return claudeResp.Content[0].Text, nil
}

// Status returns the client status
func (c *Client) Status() map[string]interface{} {
	status := map[string]interface{}{
		"api_key_set": c.apiKey != "",
		"base_url":    c.baseURL,
	}

	// Test connection if API key is set
	if c.apiKey != "" {
		_, err := c.SendMessage("Hello")
		status["connection"] = err == nil
		if err != nil {
			status["last_error"] = err.Error()
		}
	} else {
		status["connection"] = false
		status["last_error"] = "API key not set"
	}

	return status
}

// SetAPIKey sets the API key
func (c *Client) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// TestConnection tests the connection to Claude API
func (c *Client) TestConnection() error {
	_, err := c.SendMessage("Hello, this is a test message.")
	return err
}