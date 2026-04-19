package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/aichain/aichain/internal/app"
	"github.com/aichain/aichain/internal/chain"
)

// buildTestModelWithMock creates a ChainExecutionModel wired with a mock AI
// provider instead of the real Claude client. It cancels goroutines on cleanup.
//
// Because NewChainExecutionModel starts agent goroutines immediately, and
// those goroutines block on InChan, swapping the provider on the shared
// ai.Manager after construction still works — the goroutines haven't called
// SendMessage yet (they're waiting for a message on InChan).
func buildTestModelWithMock(t *testing.T, dsl string, mock *MockProvider) *ChainExecutionModel {
	t.Helper()

	parser := chain.NewDSLParser()
	c, err := parser.ParseChainDSL(dsl)
	if err != nil {
		t.Fatalf("ParseChainDSL(%q): %v", dsl, err)
	}

	// Stamp minimal config so the model constructor doesn't choke.
	for i := range c.Nodes {
		if c.Nodes[i].Type == chain.NodeTypeAI {
			c.Nodes[i].Model = "mock-model"
			c.Nodes[i].Role = "test"
			c.Nodes[i].SystemPrompt = "You are test agent " + c.Nodes[i].ID
		}
	}

	application := app.NewApplicationWithConfig(&app.Config{
		AllowedDirectory: t.TempDir(),
	})

	m := NewChainExecutionModel(application, c)
	t.Cleanup(func() { m.agentCancel() })

	// Swap the real (possibly nil) Claude provider with our mock.
	// All ChainAgents share the same ai.Manager pointer, so this
	// takes effect for every agent goroutine.
	m.aiManager.RegisterProvider("claude", mock)

	return &m
}

// waitForMessages polls a pane's Messages slice until it reaches `count`
// entries or the timeout expires. Returns the actual count.
func waitForMessages(pane *AgentPane, count int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(pane.Messages) >= count {
			return len(pane.Messages)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return len(pane.Messages)
}

// ─────────────────────────────────────────────────────────────────────
// Unit tests for extractForwardContent
// ─────────────────────────────────────────────────────────────────────

func TestExtractForwardContent_WithBlock(t *testing.T) {
	content := `Here is my analysis of the code.

<to_next_agent>
Please review the following function for bugs.
</to_next_agent>`

	got := extractForwardContent(content)
	want := "Please review the following function for bugs."
	if got != want {
		t.Errorf("extractForwardContent = %q, want %q", got, want)
	}
}

func TestExtractForwardContent_WithoutBlock(t *testing.T) {
	content := "Just a plain response with no tags."
	got := extractForwardContent(content)
	if got != content {
		t.Errorf("expected full content as fallback, got %q", got)
	}
}

func TestExtractForwardContent_OpenTagOnly(t *testing.T) {
	content := "some text <to_next_agent> but no closing tag"
	got := extractForwardContent(content)
	if got != content {
		t.Errorf("expected full content as fallback, got %q", got)
	}
}

func TestExtractForwardContent_EmptyBlock(t *testing.T) {
	content := "intro <to_next_agent>\n</to_next_agent> outro"
	got := extractForwardContent(content)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Integration test: A -> B -> C message flow
// ─────────────────────────────────────────────────────────────────────

func TestChainFlow_ABC_MessagePropagates(t *testing.T) {
	mock := echoProvider()
	m := buildTestModelWithMock(t, "A -> B -> C", mock)

	agentA := m.ChainAgents["A"]
	agentC := m.ChainAgents["C"]

	if agentA == nil || agentC == nil {
		t.Fatal("expected agents A and C to exist")
	}

	// Send a message into the chain entry point (A).
	agentA.InChan <- AgentMessage{
		Role:      "user",
		Content:   "design a REST API",
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "human",
	}

	// Wait for agent C to receive and process the message.
	// C should have 2 messages: the forwarded input + its own response.
	got := waitForMessages(agentC.Pane, 2, 10*time.Second)
	if got < 2 {
		t.Fatalf("agent C pane: expected >= 2 messages, got %d (chain did not propagate)", got)
	}

	// Verify A processed the message (should have input + response = 2 msgs).
	if len(agentA.Pane.Messages) < 2 {
		t.Errorf("agent A pane: expected >= 2 messages, got %d", len(agentA.Pane.Messages))
	}

	// Verify the mock was called 3 times (once per agent).
	callCount := mock.GetCallCount()
	if callCount != 3 {
		t.Errorf("expected 3 API calls (one per agent), got %d", callCount)
	}
}

func TestChainFlow_ABC_ContentFlowsThrough(t *testing.T) {
	// Each agent prefixes its response so we can trace the flow.
	mock := echoProvider()
	m := buildTestModelWithMock(t, "A -> B -> C", mock)

	agentA := m.ChainAgents["A"]
	agentC := m.ChainAgents["C"]

	agentA.InChan <- AgentMessage{
		Role:      "user",
		Content:   "hello",
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "human",
	}

	// Wait for C to finish
	waitForMessages(agentC.Pane, 2, 10*time.Second)

	// C's input message (forwarded from B) should contain "hello" somewhere
	// because echoProvider echoes the prompt through.
	foundHello := false
	for _, msg := range agentC.Pane.Messages {
		if strings.Contains(msg.Content, "hello") {
			foundHello = true
			break
		}
	}
	if !foundHello {
		t.Error("agent C never received content containing 'hello' — chain broken")
		for i, msg := range agentC.Pane.Messages {
			t.Logf("  C.Messages[%d] role=%s source=%s content=%q", i, msg.Role, msg.Source, msg.Content)
		}
	}
}

func TestChainFlow_ABC_SourceAttribution(t *testing.T) {
	mock := echoProvider()
	m := buildTestModelWithMock(t, "A -> B -> C", mock)

	agentA := m.ChainAgents["A"]
	agentB := m.ChainAgents["B"]
	agentC := m.ChainAgents["C"]

	agentA.InChan <- AgentMessage{
		Role:      "user",
		Content:   "test",
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "human",
	}

	waitForMessages(agentC.Pane, 2, 10*time.Second)

	// A's first message should be from "human"
	if len(agentA.Pane.Messages) > 0 && agentA.Pane.Messages[0].Source != "human" {
		t.Errorf("A's first message source = %q, want 'human'", agentA.Pane.Messages[0].Source)
	}

	// B's first message should be from "A" (forwarded)
	if len(agentB.Pane.Messages) > 0 && agentB.Pane.Messages[0].Source != "A" {
		t.Errorf("B's first message source = %q, want 'A'", agentB.Pane.Messages[0].Source)
	}

	// C's first message should be from "B" (forwarded)
	if len(agentC.Pane.Messages) > 0 && agentC.Pane.Messages[0].Source != "B" {
		t.Errorf("C's first message source = %q, want 'B'", agentC.Pane.Messages[0].Source)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Integration test: end-of-chain agent has no forwarding instructions
// ─────────────────────────────────────────────────────────────────────

func TestChainFlow_ABC_LastAgentNoForwardPrompt(t *testing.T) {
	mock := echoProvider()
	m := buildTestModelWithMock(t, "A -> B -> C", mock)

	agentA := m.ChainAgents["A"]
	agentC := m.ChainAgents["C"]

	agentA.InChan <- AgentMessage{
		Role:      "user",
		Content:   "test",
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "human",
	}

	waitForMessages(agentC.Pane, 2, 10*time.Second)

	// Check the system prompts that were sent.
	// Call 0 = agent A, Call 1 = agent B, Call 2 = agent C
	if mock.GetCallCount() < 3 {
		t.Fatalf("expected 3 calls, got %d", mock.GetCallCount())
	}

	// A and B should have the <to_next_agent> instruction in their system prompt.
	callA := mock.GetCall(0)
	if !strings.Contains(callA.Context.SystemPrompt, "to_next_agent") {
		t.Error("agent A system prompt should contain <to_next_agent> instructions")
	}

	callB := mock.GetCall(1)
	if !strings.Contains(callB.Context.SystemPrompt, "to_next_agent") {
		t.Error("agent B system prompt should contain <to_next_agent> instructions")
	}

	// C (last agent) should NOT have the forwarding instruction.
	callC := mock.GetCall(2)
	if strings.Contains(callC.Context.SystemPrompt, "to_next_agent") {
		t.Error("agent C (last in chain) should NOT have <to_next_agent> instructions")
	}
}

// ─────────────────────────────────────────────────────────────────────
// Integration test: A -> B (simpler, 2-hop sanity check)
// ─────────────────────────────────────────────────────────────────────

func TestChainFlow_AB_SimpleForward(t *testing.T) {
	mock := echoProvider()
	m := buildTestModelWithMock(t, "A -> B", mock)

	agentA := m.ChainAgents["A"]
	agentB := m.ChainAgents["B"]

	agentA.InChan <- AgentMessage{
		Role:      "user",
		Content:   "ping",
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "human",
	}

	got := waitForMessages(agentB.Pane, 2, 10*time.Second)
	if got < 2 {
		t.Fatalf("agent B: expected >= 2 messages, got %d", got)
	}

	if mock.GetCallCount() != 2 {
		t.Errorf("expected 2 API calls, got %d", mock.GetCallCount())
	}
}

// ─────────────────────────────────────────────────────────────────────
// Test: A -> B -> * -> D — confirms the human node bug
// ─────────────────────────────────────────────────────────────────────

func TestChainFlow_HumanNode_ChainBreaks(t *testing.T) {
	// This test documents the known bug: the * node breaks the chain.
	// Agent D should never receive a message because B->* and *->D are
	// not wired. When this bug is fixed, this test should be updated
	// to expect D to eventually receive a message.
	mock := echoProvider()
	m := buildTestModelWithMock(t, "A -> B -> * -> D", mock)

	agentA := m.ChainAgents["A"]
	agentB := m.ChainAgents["B"]
	agentD := m.ChainAgents["D"]

	if agentA == nil || agentB == nil || agentD == nil {
		t.Fatal("expected agents A, B, D to exist")
	}

	// Verify the wiring gap: B should have NO OutChans
	if len(agentB.OutChans) != 0 {
		t.Errorf("expected B to have 0 OutChans (bug: * not wired), got %d", len(agentB.OutChans))
	}

	// D should have no way to receive messages
	agentA.InChan <- AgentMessage{
		Role:      "user",
		Content:   "test",
		Timestamp: time.Now().Format("15:04:05"),
		Source:    "human",
	}

	// Wait for B to finish processing
	waitForMessages(agentB.Pane, 2, 10*time.Second)

	// Give D a bit of extra time — it should still have 0 messages
	time.Sleep(500 * time.Millisecond)

	if len(agentD.Pane.Messages) != 0 {
		t.Errorf("agent D received %d messages — expected 0 (chain should be broken at *)",
			len(agentD.Pane.Messages))
	}
}

// ─────────────────────────────────────────────────────────────────────
// Test: conversation history role remapping
// ─────────────────────────────────────────────────────────────────────

func TestGetConversationHistory_RemapsInterAgentMessages(t *testing.T) {
	// Simulate agent B's pane with messages from agent A and from B itself.
	pane := &AgentPane{
		Messages: []AgentMessage{
			{Role: "assistant", Content: "from A", Source: "A", Timestamp: "10:00:00"},
			{Role: "assistant", Content: "B's reply", Source: "B", Timestamp: "10:00:01"},
			{Role: "assistant", Content: "from A again", Source: "A", Timestamp: "10:00:02"},
			// The last message is the "current" one being processed — should be excluded.
		},
	}

	agent := &ChainAgent{
		ID:   "B",
		Pane: pane,
	}

	history := agent.getConversationHistory()

	// Should exclude the last message, so 2 entries.
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}

	// First entry: from A → should be remapped to "user"
	if history[0].Role != "user" {
		t.Errorf("history[0].Role = %q, want 'user' (inter-agent remap)", history[0].Role)
	}

	// Second entry: from B itself → should stay "assistant"
	if history[1].Role != "assistant" {
		t.Errorf("history[1].Role = %q, want 'assistant' (own message)", history[1].Role)
	}
}

func TestGetConversationHistory_ExcludesLastMessage(t *testing.T) {
	pane := &AgentPane{
		Messages: []AgentMessage{
			{Role: "user", Content: "hello", Source: "human", Timestamp: "10:00:00"},
		},
	}

	agent := &ChainAgent{
		ID:   "A",
		Pane: pane,
	}

	history := agent.getConversationHistory()

	// Only 1 message in pane, and it's the "current" one → history should be empty.
	if len(history) != 0 {
		t.Errorf("expected 0 history entries (last msg excluded), got %d", len(history))
	}
}

func TestGetConversationHistory_EmptyPane(t *testing.T) {
	pane := &AgentPane{
		Messages: []AgentMessage{},
	}

	agent := &ChainAgent{
		ID:   "A",
		Pane: pane,
	}

	history := agent.getConversationHistory()
	if len(history) != 0 {
		t.Errorf("expected 0 history entries, got %d", len(history))
	}
}
