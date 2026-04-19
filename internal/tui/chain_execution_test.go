package tui

import (
	"testing"

	"github.com/aichain/aichain/internal/app"
	"github.com/aichain/aichain/internal/chain"
)

// buildTestModel parses dsl, stamps minimal config onto AI nodes, and returns
// a ChainExecutionModel with channels wired. A cleanup func cancels the agent
// goroutines when the test ends.
func buildTestModel(t *testing.T, dsl string) *ChainExecutionModel {
	t.Helper()

	parser := chain.NewDSLParser()
	c, err := parser.ParseChainDSL(dsl)
	if err != nil {
		t.Fatalf("ParseChainDSL(%q): %v", dsl, err)
	}

	for i := range c.Nodes {
		if c.Nodes[i].Type == chain.NodeTypeAI {
			c.Nodes[i].Model        = "claude-3-5-sonnet-20241022"
			c.Nodes[i].SystemPrompt = "test agent"
		}
	}

	application := app.NewApplicationWithConfig(&app.Config{
		AllowedDirectory: t.TempDir(),
	})

	m := NewChainExecutionModel(application, c)
	t.Cleanup(func() { m.agentCancel() })
	return &m
}

// assertWiring checks that agent's OutChans match the expected map of
// targetID → that agent's InChan, and that no unexpected entries exist.
func assertWiring(t *testing.T, agentID string, agent *ChainAgent, expected map[string]*ChainAgent) {
	t.Helper()

	if len(agent.OutChans) != len(expected) {
		t.Errorf("agent %s: OutChans len = %d, want %d", agentID, len(agent.OutChans), len(expected))
	}

	for targetID, targetAgent := range expected {
		ch, ok := agent.OutChans[targetID]
		if !ok {
			t.Errorf("agent %s: missing OutChan to %q", agentID, targetID)
			continue
		}
		if ch != targetAgent.InChan {
			t.Errorf("agent %s: OutChans[%q] is not %s.InChan (channel identity mismatch)", agentID, targetID, targetID)
		}
	}

	for targetID := range agent.OutChans {
		if _, ok := expected[targetID]; !ok {
			t.Errorf("agent %s: unexpected OutChan to %q", agentID, targetID)
		}
	}
}

// TestChannelWiring_SimpleOneWay: A -> B
func TestChannelWiring_SimpleOneWay(t *testing.T) {
	m := buildTestModel(t, "A -> B")

	if len(m.ChainAgents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(m.ChainAgents))
	}
	a, b := m.ChainAgents["A"], m.ChainAgents["B"]

	assertWiring(t, "A", a, map[string]*ChainAgent{"B": b})
	assertWiring(t, "B", b, map[string]*ChainAgent{})
}

// TestChannelWiring_Bidirectional: A <> B
func TestChannelWiring_Bidirectional(t *testing.T) {
	m := buildTestModel(t, "A <> B")

	if len(m.ChainAgents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(m.ChainAgents))
	}
	a, b := m.ChainAgents["A"], m.ChainAgents["B"]

	assertWiring(t, "A", a, map[string]*ChainAgent{"B": b})
	assertWiring(t, "B", b, map[string]*ChainAgent{"A": a})
}

// TestChannelWiring_LinearThreeHop: A -> B -> C
func TestChannelWiring_LinearThreeHop(t *testing.T) {
	m := buildTestModel(t, "A -> B -> C")

	if len(m.ChainAgents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(m.ChainAgents))
	}
	a, b, c := m.ChainAgents["A"], m.ChainAgents["B"], m.ChainAgents["C"]

	assertWiring(t, "A", a, map[string]*ChainAgent{"B": b})
	assertWiring(t, "B", b, map[string]*ChainAgent{"C": c})
	assertWiring(t, "C", c, map[string]*ChainAgent{})
}

// TestChannelWiring_BidirectionalThreeHop: A <> B <> C
// This was the reported bug — B<>C connection was silently dropped by the parser.
func TestChannelWiring_BidirectionalThreeHop(t *testing.T) {
	m := buildTestModel(t, "A <> B <> C")

	if len(m.ChainAgents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(m.ChainAgents))
	}
	a, b, c := m.ChainAgents["A"], m.ChainAgents["B"], m.ChainAgents["C"]

	assertWiring(t, "A", a, map[string]*ChainAgent{"B": b})
	assertWiring(t, "B", b, map[string]*ChainAgent{"A": a, "C": c})
	assertWiring(t, "C", c, map[string]*ChainAgent{"B": b})
}

// TestChannelWiring_FanIn: A -> * <- B (human node — neither A nor B should
// have an OutChan since * has no ChainAgent)
func TestChannelWiring_FanIn(t *testing.T) {
	m := buildTestModel(t, "A -> * <- B")

	if len(m.ChainAgents) != 2 {
		t.Fatalf("expected 2 AI agents (human node excluded), got %d", len(m.ChainAgents))
	}
	a, b := m.ChainAgents["A"], m.ChainAgents["B"]

	assertWiring(t, "A", a, map[string]*ChainAgent{})
	assertWiring(t, "B", b, map[string]*ChainAgent{})
}

// TestChannelWiring_InChanIsUnique verifies each agent gets its own InChan,
// not a shared channel reused across agents.
func TestChannelWiring_InChanIsUnique(t *testing.T) {
	m := buildTestModel(t, "A -> B -> C")

	a, b, c := m.ChainAgents["A"], m.ChainAgents["B"], m.ChainAgents["C"]

	if a.InChan == b.InChan {
		t.Error("A and B share the same InChan")
	}
	if b.InChan == c.InChan {
		t.Error("B and C share the same InChan")
	}
	if a.InChan == c.InChan {
		t.Error("A and C share the same InChan")
	}
}
