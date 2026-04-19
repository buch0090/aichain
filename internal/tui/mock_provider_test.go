package tui

import (
	"context"
	"fmt"
	"sync"

	"github.com/aichain/aichain/internal/ai"
)

// MockProvider implements ai.Provider for testing. Each call to SendMessage
// invokes the Handler function so tests can control responses and observe
// what prompt/context was sent.
type MockProvider struct {
	mu      sync.Mutex
	Calls   []MockCall
	Handler func(prompt string, ctx ai.AIContext) (*ai.AIResponse, error)
}

// MockCall records a single SendMessage invocation.
type MockCall struct {
	Prompt  string
	Context ai.AIContext
}

func NewMockProvider(handler func(string, ai.AIContext) (*ai.AIResponse, error)) *MockProvider {
	return &MockProvider{Handler: handler}
}

func (m *MockProvider) SendMessage(ctx context.Context, prompt string, aiCtx ai.AIContext) (*ai.AIResponse, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, MockCall{Prompt: prompt, Context: aiCtx})
	handler := m.Handler
	m.mu.Unlock()

	if handler == nil {
		return &ai.AIResponse{Content: "mock response"}, nil
	}
	return handler(prompt, aiCtx)
}

func (m *MockProvider) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

func (m *MockProvider) GetCall(i int) MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Calls[i]
}

func (m *MockProvider) GetModels() []ai.Model {
	return []ai.Model{{Name: "mock-model", DisplayName: "Mock"}}
}

func (m *MockProvider) GetCapabilities() ai.Capabilities {
	return ai.Capabilities{CodeGeneration: true}
}

func (m *MockProvider) GetRateLimits() ai.RateLimits {
	return ai.RateLimits{}
}

func (m *MockProvider) GetProviderName() string {
	return "mock"
}

// echoProvider returns a mock that echoes the prompt back, optionally wrapped
// in a <to_next_agent> block so forwarding works.
func echoProvider() *MockProvider {
	return NewMockProvider(func(prompt string, ctx ai.AIContext) (*ai.AIResponse, error) {
		return &ai.AIResponse{
			Content: fmt.Sprintf("processed: %s\n\n<to_next_agent>\n%s\n</to_next_agent>", prompt, prompt),
		}, nil
	})
}

// labelProvider returns a mock that prepends a label (the agent's role from
// the system prompt) so we can trace which agent produced which output.
func labelProvider(label string) *MockProvider {
	return NewMockProvider(func(prompt string, ctx ai.AIContext) (*ai.AIResponse, error) {
		content := fmt.Sprintf("[%s] saw: %s", label, prompt)
		forwarded := fmt.Sprintf("[%s] forwarding: %s", label, prompt)
		return &ai.AIResponse{
			Content: fmt.Sprintf("%s\n\n<to_next_agent>\n%s\n</to_next_agent>", content, forwarded),
		}, nil
	})
}
