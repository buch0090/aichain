# Code Walkthrough: DSL Chain Execution

Two example DSLs traced end-to-end through the codebase, with bugs identified.

---

## Example 1: `A -> B -> C`

**Use case:** A = Architect/Planner, B = QA Reviewer, C = Senior Developer

### Phase 1: Startup

**Entry:** `cmd/aichain/main.go`

User runs: `aichain --setup` (or `aichain my-chain.dsl`)

With `--setup`:
1. `app.NewApplication()` creates the app with session/pipeline/AI managers
2. `tui.NewModelWithChainSetup(application)` creates a Bubble Tea `Model` in `ModeChainSetup`
3. TUI starts, user sees DSL input screen

### Phase 2: DSL Parsing

**File:** `internal/chain/dsl.go`

User types `A -> B -> C` and presses Enter.

**`chain_setup.go:updateDSLInput()`** calls **`setupFlow.ProcessStep1("A -> B -> C")`**

**Step 1 — Validation** (`ValidateDSL`):
- Checks non-empty ✅
- Regex `^[A-Za-z0-9*\s<>-]+$` matches ✅
- Regex `(<>|->|<-)` finds `->` ✅

**Step 2 — extractNodeIDs**:
- Regex `[A-Za-z0-9*]+` finds: `["A", "B", "C"]`
- All unique, none are `*`, so all get `NodeTypeAI`
- Result: 3 ChainNodes: `{ID:"A", Type:"ai"}`, `{ID:"B", Type:"ai"}`, `{ID:"C", Type:"ai"}`

**Step 3 — parseConnections**:
- Tokenizer regex `([A-Za-z0-9*]+|<>|->|<-)` produces: `["A", "->", "B", "->", "C"]`
- Loop walks `i=1,3`:
  - `i=1`: from=`A`, op=`->`, to=`B` → `Connection{From:"A", To:"B", Type:ConnOneWay}`
  - `i=3`: from=`B`, op=`->`, to=`C` → `Connection{From:"B", To:"C", Type:ConnOneWay}`

**Result chain:**
```
Nodes:       [A(ai), B(ai), C(ai)]
Connections: [A->B, B->C]
```

### Phase 3: Agent Assignment

**File:** `internal/chain/setup.go` + `internal/tui/chain_setup.go`

After DSL parsing, `ProcessStep1` also:
1. Calls `AgentLoader.LoadAgents()` — reads `.agents/*.yaml` files
2. Creates `NodeSetups` for each AI node: `{"A": NodeSetup{}, "B": NodeSetup{}, "C": NodeSetup{}}`
3. Sets `CurrentNodeID = "A"` (first AI node)
4. Moves to `SetupModeNodeConfig`

User sees agent selector. For `A -> B -> C` they'd pick:
- Node A → "architect" agent
- Node B → "reviewer" agent  
- Node C → "developer" agent

Each selection calls `setupFlow.ConfigureCurrentNode(agentID)`:
- Copies agent's model/role/systemPrompt/temperature onto the `NodeSetup`
- Advances `CurrentNodeID` to next unconfigured node
- When all done, sets `Complete = true` and calls `finalizeChain()`

**`finalizeChain()`** copies agent config onto the actual `Chain.Nodes`:
```go
s.Chain.Nodes[i].Model = agent.Model
s.Chain.Nodes[i].Role = agent.Role
s.Chain.Nodes[i].SystemPrompt = agent.SystemPrompt
```

### Phase 4: Transition to Execution

**File:** `internal/tui/chain_setup.go` → `internal/tui/model.go`

1. User presses Enter on "Setup Complete" screen
2. `updateComplete()` returns a `ChainCompleteMsg{Chain: setupFlow.Chain}`
3. Main `Model.Update()` catches `ChainCompleteMsg`:
   - Creates `NewChainExecutionModel(app, chain)`
   - Sets `m.mode = ModeChainExecution`

### Phase 5: Chain Execution Model Initialization

**File:** `internal/tui/chain_execution.go` → `NewChainExecutionModel()`

1. Creates a new `ai.Manager` with a fresh `ClaudeProvider` (reads `CLAUDE_API_KEY`)
2. Iterates `chain.Nodes`, creates an `AgentPane` for each **AI node only**:
   - For `A -> B -> C`: 3 panes (A, B, C)
3. Calls `setupAgentChannels()`

**`setupAgentChannels()`** — the critical wiring:

**Creates ChainAgents** (one per AI node):
```
ChainAgent "A": InChan(cap=10), OutChans={}
ChainAgent "B": InChan(cap=10), OutChans={}
ChainAgent "C": InChan(cap=10), OutChans={}
```

**Wires connections:**
- Connection `A->B` (OneWay): `A.OutChans["B"] = B.InChan` ✅
- Connection `B->C` (OneWay): `B.OutChans["C"] = C.InChan` ✅

**Starts goroutines:** Each `ChainAgent.Run()` blocks on `<-InChan`

**Final wiring:**
```
Human → A.InChan → [A processes] → A.OutChans["B"] → B.InChan → [B processes] → B.OutChans["C"] → C.InChan → [C processes] → (end, no OutChans)
```

### Phase 6: Human Sends a Message

**File:** `internal/tui/chain_execution_helpers.go` → `sendMessageToChain()`

User types "Design a REST API for user management" and presses Enter.

1. Creates `AgentMessage{Role:"user", Content:"...", Source:"human"}`
2. Finds the first AI node by iterating `m.chain.Nodes` — finds `A`
3. Sends to `A.InChan` via non-blocking select
4. Returns `RefreshUIMsg{}` to start the 500ms refresh cycle

### Phase 7: Agent A Processes (Architect)

**File:** `internal/tui/chain_execution.go` → `ChainAgent.Run()` → `processMessage()`

1. **Receives** from `InChan`
2. **Appends** incoming message to `A.Pane.Messages` (shows the human's message in A's pane)
3. **Sets** status to "🤔 Thinking..."
4. **Builds system prompt**: Architect's system prompt + chain forwarding instructions:
   ```
   You are part of an agent chain. After completing your work, you MUST end your 
   response with a <to_next_agent> block containing a focused prompt for the next 
   agent(s) (B). This tells them exactly what you need.
   ```
5. **Calls** `getConversationHistory()` — excludes the last message (current prompt), remaps inter-agent messages to "user" role
6. **Calls** Claude API via `provider.SendMessage()` — this enters the tool-calling loop in `claude.go`
7. **Appends** Claude's response to `A.Pane.Messages`
8. **Extracts** `<to_next_agent>` block from response
9. **Forwards** to `B.InChan` via `A.OutChans["B"]`

### Phase 8: Agent B Processes (QA Reviewer)

Same flow as A. B receives forwarded content, processes it, adds chain instructions targeting `C`, forwards `<to_next_agent>` to `C.InChan`.

### Phase 9: Agent C Processes (Developer)

Same flow. C has **no OutChans** (it's the end of the chain), so:
- No chain forwarding instructions are added to its system prompt
- No `<to_next_agent>` extraction or forwarding happens
- Chain execution is complete

### Phase 10: UI Updates

The `RefreshUIMsg` tick fires every 500ms. Each tick:
1. Calls `updatePaneContent()` for each pane
2. Renders messages with role-appropriate styling
3. Auto-scrolls to bottom if user was already at bottom
4. Schedules next refresh

---

## Example 2: `A -> B -> * -> D`

**Use case:** A = Architect, B = QA, * = Human review point, D = Senior Developer

### DSL Parsing Differences

**extractNodeIDs:** `["A", "B", "*", "D"]`
- A, B, D → `NodeTypeAI`
- `*` → `NodeTypeHuman`

**parseConnections:** Tokens: `["A", "->", "B", "->", "*", "->", "D"]`
- `A->B` (OneWay)
- `B->*` (OneWay)
- `*->D` (OneWay)

### Agent Assignment Differences

**`ProcessStep1()`** creates NodeSetups only for AI nodes:
```go
for _, node := range chain.Nodes {
    if node.Type == NodeTypeAI {  // ← skips * (human)
        s.NodeSetups[node.ID] = NodeSetup{...}
    }
}
```

So user configures 3 agents: A, B, D (not `*`). This is correct.

### Chain Execution — ⚠️ HERE'S WHERE BUGS LIVE

**`NewChainExecutionModel()` — Pane creation:**
```go
for i, node := range completedChain.Nodes {
    if node.Type == chain.NodeTypeAI {  // ← only AI nodes get panes
```
Result: 3 AgentPanes (A, B, D). No pane for `*`. This is fine for display.

**`setupAgentChannels()` — Agent creation:**
```go
for i, node := range m.chain.Nodes {
    if node.Type == chain.NodeTypeAI {  // ← only AI nodes get ChainAgents
```
Result: ChainAgents for A, B, D. No ChainAgent for `*`.

**`setupAgentChannels()` — Wiring connections:**
```go
for _, conn := range m.chain.Connections {
    fromAgent := m.ChainAgents[conn.From]  
    toAgent := m.ChainAgents[conn.To]
    
    if fromAgent != nil && toAgent != nil {  // ← BOTH must exist
        fromAgent.OutChans[conn.To] = toAgent.InChan
```

For `A -> B -> * -> D`:
- Connection `A->B`: fromAgent=A ✅, toAgent=B ✅ → **wired** ✅
- Connection `B->*`: fromAgent=B ✅, toAgent=`*` ❌ (nil, no ChainAgent for `*`) → **SILENTLY SKIPPED** 🐛
- Connection `*->D`: fromAgent=`*` ❌ (nil), toAgent=D ✅ → **SILENTLY SKIPPED** 🐛

**Result: The chain is broken. B has no OutChans. D has no InChan connections. The `*` node is completely ignored in execution.**

**What actually happens at runtime:**
1. Human types message → sent to A (first AI node)
2. A processes → forwards to B via `A.OutChans["B"]`
3. B processes → has **no OutChans** → response is a dead end
4. D never receives anything
5. The human `*` node never participates

---

## Bug Report

### 🐛 BUG 1 (Critical): Human (`*`) node in chain is completely non-functional

**Location:** `internal/tui/chain_execution.go` — `setupAgentChannels()` and `NewChainExecutionModel()`

**Problem:** The `*` (human) node is parsed correctly in the DSL but completely ignored during execution. No `ChainAgent` is created for it, no channels are wired through it, and there's no mechanism for the human to receive output from agent B and then manually send input to agent D.

**Impact:** Any DSL containing `*` (like `A -> B -> * -> D`) will have a broken chain. The agents before `*` will process normally but their output will dead-end. Agents after `*` will never receive input.

**What's needed:** 
- A special "human agent" that receives messages on its InChan and displays them in a dedicated UI area (or a special pane) 
- A way for the human to type a message that gets sent specifically to the next agent(s) after `*` (D in this case)
- Currently `sendMessageToChain()` always sends to the **first** AI node in the chain — there's no way to inject a message mid-chain

### 🐛 BUG 2 (Medium): `sendMessageToChain` always targets the first AI node

**Location:** `internal/tui/chain_execution_helpers.go:30-37`

```go
for _, node := range m.chain.Nodes {
    if node.Type == "ai" {
        if agent, exists := m.ChainAgents[node.ID]; exists {
            firstAgent = agent
            break  // ← always picks the first AI node
        }
    }
}
```

**Problem:** Every human message goes to the first AI node. In `A -> B -> C`, if you type a second message while C is still processing, it goes to A again — starting a new full chain run. This is arguably correct for that topology, but for `A -> B -> * -> D` the human's typed message should go to D (the node after `*`), not A.

**Impact:** Even if Bug 1 were fixed to make `*` a passthrough/display node, there's no mechanism to send the human's typed message to D specifically.

### 🐛 BUG 3 (Medium): No concurrency protection on `AgentPane.Messages`

**Location:** `internal/tui/chain_execution.go` — `Run()`, `processMessage()`, and `updatePaneContent()`

**Problem:** `ChainAgent.Run()` runs in a goroutine and appends to `a.Pane.Messages` without holding any lock. Meanwhile, the Bubble Tea update loop reads `pane.Messages` in `updatePaneContent()` on the main goroutine. This is a data race.

```go
// In goroutine (Run):
a.Pane.Messages = append(a.Pane.Messages, msg)

// In main goroutine (updatePaneContent via RefreshUIMsg):
for _, msg := range pane.Messages {
```

**Impact:** Potential panics, garbled messages, or slice corruption under concurrent access. Especially likely with `A -> B -> C` where agents A, B, C may all be appending to their panes simultaneously while the UI reads them.

### 🐛 BUG 4 (Medium): Model from agent YAML is ignored — hardcoded in claude.go

**Location:** `internal/ai/claude.go:100`

```go
params := anthropic.MessageNewParams{
    Model:     anthropic.ModelClaudeSonnet4_5_20250929,  // ← hardcoded
```

**Problem:** The user carefully selects agents with different models (e.g., architect uses Opus, developer uses Sonnet), and those models are stored on `ChainNode.Model`. But `processMessage()` in `chain_execution.go` never passes the model to the provider — it just calls `provider.SendMessage()` which always uses the hardcoded Sonnet 4.5.

**Impact:** All agents use the same model regardless of their YAML configuration. The architect agent set to `claude-3-opus-20240229` still gets Sonnet 4.5.

### 🐛 BUG 5 (Low): `MaxTokens` hardcoded to 1024 in chain agents

**Location:** `internal/tui/chain_execution.go` — `processMessage()`

```go
aiContext := ai.AIContext{
    ...
    MaxTokens: 1024,
```

**Problem:** For a senior developer agent writing substantial code, 1024 output tokens is very limiting. The `callAIAgent` fallback in `chain_execution_helpers.go` uses 4000 tokens, but the channel-based chain path (the one actually used) uses 1024.

**Impact:** Agent responses get truncated, especially for code-heavy tasks. The developer agent (C or D) would frequently run out of output tokens mid-response.

### 🐛 BUG 6 (Low): Non-blocking channel send can silently drop messages

**Location:** `internal/tui/chain_execution.go` — `Run()` forwarding loop

```go
for targetID, outChan := range a.OutChans {
    select {
    case outChan <- AgentMessage{...}:
        // sent
    default:
        debugLogger.Printf("ChainAgent %s: DROP to %s — channel full (cap=10)", ...)
    }
}
```

**Problem:** If the target agent's channel is full (10 messages buffered), the message is **silently dropped** with only a debug log. In a `A -> B -> C` chain, if B is slow and A sends multiple forwarded messages, they could be lost.

**Impact:** Chain messages can be lost without any user-visible indication. The user sees agent B or C stuck with no explanation.

### 🐛 BUG 7 (Low): `sendMessageToChain` uses `node.Type == "ai"` string literal

**Location:** `internal/tui/chain_execution_helpers.go:32`

```go
if node.Type == "ai" {
```

But the constant is `chain.NodeTypeAI` which equals `NodeType("ai")`. This works because Go string comparison, but it's inconsistent with the rest of the codebase which uses `chain.NodeTypeAI`. A refactor could introduce a mismatch.

### 🐛 BUG 8 (Low): Chain status never updates from "creating"

**Location:** `internal/chain/setup.go` → `finalizeChain()` sets `StatusReady`, but nothing ever sets `StatusRunning`. The execution UI shows the status from `m.chain.Status` but it stays at `"ready"` throughout execution.

### 🐛 BUG 9 (Low): Conversation history can violate API alternation rules

**Location:** `internal/tui/chain_execution.go` — `getConversationHistory()`

**Problem:** The role remapping logic handles the common case well, but consider this sequence in agent B's pane:
1. Message from A (role=assistant, source=A) → remapped to "user" ✅
2. B's own response (role=assistant, source=B) → stays "assistant" ✅  
3. Human sends a second message → goes to A again → A responds → forwarded to B
4. Message from A (role=assistant, source=A) → remapped to "user" ✅
5. B's own response → "assistant" ✅

This works for the linear case. But if you had a topology where **two different agents** both send to B simultaneously, you could get:
1. Message from A → remapped to "user"
2. Message from C → remapped to "user"  ← two "user" messages in a row

The Anthropic API accepts consecutive same-role messages in some cases but it's fragile and could cause unexpected behavior.

---

## Summary: What Works vs What Doesn't

| DSL | Status | Notes |
|-----|--------|-------|
| `A -> B -> C` | ✅ Works | Linear chain, all AI nodes. Main path is solid. |
| `A <> B` | ✅ Works | Bidirectional wiring works. Potential for infinite ping-pong but agents eventually stop. |
| `A -> B -> * -> D` | ❌ Broken | Human node `*` is completely non-functional. Chain breaks at `*`. |
| `A -> * <- B` | ❌ Broken | Same issue — `*` is ignored, no messages reach it. |
| `A <> *` | ❌ Broken | Same issue. |
| Any DSL with `*` | ❌ Broken | The `*` node is only supported at the parsing and setup UI levels, not in execution. |

### Priority Fix Order

1. **Add mutex protection** for `AgentPane.Messages` (Bug 3) — quick fix, prevents crashes
2. **Increase MaxTokens** to 4000+ (Bug 5) — one-line fix, big impact on response quality
3. **Pass model from ChainNode** to Claude provider (Bug 4) — requires adding model to `AIContext` or `SendMessage` params
4. **Implement human node** in chain execution (Bugs 1 & 2) — significant feature work, core to your use case
