package chain

import (
	"testing"
)

func TestParseDSL_LinearThreeHop(t *testing.T) {
	parser := NewDSLParser()
	c, err := parser.ParseChainDSL("A -> B -> C")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Nodes
	if len(c.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(c.Nodes))
	}
	for i, id := range []string{"A", "B", "C"} {
		if c.Nodes[i].ID != id {
			t.Errorf("node %d: ID = %q, want %q", i, c.Nodes[i].ID, id)
		}
		if c.Nodes[i].Type != NodeTypeAI {
			t.Errorf("node %s: Type = %q, want %q", id, c.Nodes[i].Type, NodeTypeAI)
		}
	}

	// Connections
	if len(c.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(c.Connections))
	}
	assertConn(t, c.Connections[0], "A", "B", ConnOneWay)
	assertConn(t, c.Connections[1], "B", "C", ConnOneWay)
}

func TestParseDSL_WithHumanNode(t *testing.T) {
	parser := NewDSLParser()
	c, err := parser.ParseChainDSL("A -> B -> * -> D")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(c.Nodes))
	}

	// Check types
	expectedTypes := map[string]NodeType{
		"A": NodeTypeAI,
		"B": NodeTypeAI,
		"*": NodeTypeHuman,
		"D": NodeTypeAI,
	}
	for _, node := range c.Nodes {
		want, ok := expectedTypes[node.ID]
		if !ok {
			t.Errorf("unexpected node ID %q", node.ID)
			continue
		}
		if node.Type != want {
			t.Errorf("node %s: Type = %q, want %q", node.ID, node.Type, want)
		}
	}

	// Connections
	if len(c.Connections) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(c.Connections))
	}
	assertConn(t, c.Connections[0], "A", "B", ConnOneWay)
	assertConn(t, c.Connections[1], "B", "*", ConnOneWay)
	assertConn(t, c.Connections[2], "*", "D", ConnOneWay)
}

func TestParseDSL_Bidirectional(t *testing.T) {
	parser := NewDSLParser()
	c, err := parser.ParseChainDSL("A <> B <> C")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(c.Connections))
	}
	assertConn(t, c.Connections[0], "A", "B", ConnTwoWay)
	assertConn(t, c.Connections[1], "B", "C", ConnTwoWay)
}

func TestParseDSL_ReverseArrow(t *testing.T) {
	parser := NewDSLParser()
	c, err := parser.ParseChainDSL("A -> * <- B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(c.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(c.Connections))
	}
	assertConn(t, c.Connections[0], "A", "*", ConnOneWay)
	// <- reverses: from=B, to=*
	assertConn(t, c.Connections[1], "B", "*", ConnOneWay)
}

func TestValidateDSL_Errors(t *testing.T) {
	parser := NewDSLParser()

	tests := []struct {
		name string
		dsl  string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"no connection", "A B C"},
		{"invalid chars", "A -> B; C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := parser.ValidateDSL(tt.dsl); err == nil {
				t.Errorf("ValidateDSL(%q) = nil, want error", tt.dsl)
			}
		})
	}
}

func TestValidateDSL_Valid(t *testing.T) {
	parser := NewDSLParser()

	dsls := []string{
		"A -> B",
		"A <> B",
		"A -> B -> C",
		"A -> B -> * -> D",
		"A <> B <> C <> D",
		"A -> * <- B",
	}

	for _, dsl := range dsls {
		t.Run(dsl, func(t *testing.T) {
			if err := parser.ValidateDSL(dsl); err != nil {
				t.Errorf("ValidateDSL(%q) = %v, want nil", dsl, err)
			}
		})
	}
}

func TestExtractNodeIDs_NoDuplicates(t *testing.T) {
	parser := NewDSLParser()
	// "A -> B -> A" should only produce [A, B], not [A, B, A]
	ids := parser.extractNodeIDs("A -> B -> A")
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique IDs, got %d: %v", len(ids), ids)
	}
}

// helper
func assertConn(t *testing.T, conn Connection, from, to string, typ ConnectionType) {
	t.Helper()
	if conn.From != from || conn.To != to || conn.Type != typ {
		t.Errorf("connection = {From:%q To:%q Type:%q}, want {From:%q To:%q Type:%q}",
			conn.From, conn.To, conn.Type, from, to, typ)
	}
}
