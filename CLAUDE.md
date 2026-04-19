# CLAUDE.md — AIChain Project Guide

## What This App Does

AIChain is a **terminal-based AI agent chaining tool** built in Go. It lets you define chains of Claude AI agents using a simple DSL (Domain Specific Language), configure each agent with a role/persona, then execute the chain — where agents process messages sequentially or bidirectionally, passing outputs between each other through Go channels. The human user can inject messages into the chain and watch agents collaborate in real time through a terminal UI.

**Core concept:** You describe a topology of AI agents (e.g., `A -> B -> C` or `A <> B`), assign each node a pre-configured agent persona (developer, reviewer, architect, etc.), and the system orchestrates Claude API calls between them with tool access (file read/write/list).

## Two Entry Points

There are **two separate `main.go` binaries** — this is important:

1. **`cmd/aichain/main.go`** — The primary binary. Uses `flag` package. Supports:
   - `aichain <file.dsl>` — Parse and execute a DSL file directly
   - `aichain --setup` — Interactive 2-step chain builder TUI
   - `aichain --server` — HTTP backend server (legacy, for VIM integration)
   - `--debug` flag disables alt screen for easier debugging

2. **`cmd/aichain-standalone/main.go`** — Uses `cobra` for CLI. More subcommands (`debate`, `pipeline`, `session`, `config`, `version`). Requires `CLAUDE_API_KEY` env var explicitly. Many subcommands are TODOs/stubs.

## Project Structure

```
├── cmd/
│   ├── aichain/main.go              # Primary entry point (flag-based)
│   └── aichain-standalone/main.go   # Cobra-based entry point
├── internal/
│   ├── app/app.go                   # Application orchestrator (sessions, pipelines, AI manager)
│   ├── chain/
│   │   ├── dsl.go                   # DSL parser (A -> B, A <> B syntax)
│   │   ├── agents.go                # YAML agent loader from .agents/ directory
│   │   └── setup.go                 # 2-step setup flow (DSL input → agent assignment)
│   ├── ai/
│   │   ├── provider.go              # AI provider interface + Manager
│   │   └── claude.go                # Claude API implementation with tool calling loop
│   ├── tui/
│   │   ├── model.go                 # Main Bubble Tea model (3-pane: explorer/editor/chat)
│   │   ├── chain_setup.go           # Chain setup TUI (Step 1: DSL, Step 2: agent selection)
│   │   ├── chain_setup_helpers.go   # Setup view helpers and styles
│   │   ├── chain_execution.go       # Chain execution TUI with agent panes + goroutines
│   │   ├── chain_execution_helpers.go # Execution helpers, AI calls, styles
│   │   ├── panes.go                 # Explorer, Editor, Chat pane components
│   │   └── messages.go              # Bubble Tea message types
│   ├── pipeline/pipeline.go         # Pipeline system for session-to-session message routing
│   ├── session/session.go           # Session management (message history, links, status)
│   ├── tools/tools.go               # Tool interface + READ_FILE, WRITE_FILE, LIST_FILES
│   └── vim/keybindings.go           # VIM mode engine (Normal/Insert/Visual/Command)
├── pkg/
│   ├── claude/client.go             # Simple HTTP Claude client (used by server)
│   ├── server/server.go             # HTTP server for VIM plugin integration
│   └── session/manager.go           # Simpler session manager (used by server)
├── .agents/                         # YAML agent definitions (auto-created if missing)
├── configs/default.yaml             # Default configuration
├── Makefile                         # Build targets
└── test-chain.dsl                   # Example DSL file
```

## Key Architecture Concepts

### DSL (Domain Specific Language)
Defined in `internal/chain/dsl.go`. Syntax:
- `A -> B` — one-way: A sends to B
- `A <> B` — two-way: A and B communicate bidirectionally
- `A -> B -> C` — linear chain
- `A <> B <> C` — bidirectional chain
- `*` — human interaction point

The parser tokenizes the DSL string and extracts nodes + connections. Node IDs are single letters/numbers. `*` is always NodeTypeHuman.

### Agent Definitions (`.agents/` directory)
YAML files in `.agents/` define reusable agent personas. Each has: `id`, `name`, `description`, `model`, `role`, `system_prompt`, `temperature`, `tags`. Six defaults are auto-created: `developer`, `architect`, `reviewer`, `debugger`, `security`, `tester`.

### Chain Execution Flow
1. User defines DSL → parser creates `Chain` with `ChainNode`s and `Connection`s
2. User assigns an agent persona to each AI node
3. `ChainExecutionModel` creates:
   - An `AgentPane` (UI) per AI node
   - A `ChainAgent` (goroutine) per AI node with buffered Go channels
   - Channels wired according to DSL connections
4. Human types a message → sent to first agent's `InChan`
5. Agent goroutine receives message, calls Claude API, appends response to pane
6. Agent extracts `<to_next_agent>` block from response and forwards to connected agents via their channels
7. UI refreshes every 500ms via `RefreshUIMsg` tick

### Tool Calling
The Claude provider (`internal/ai/claude.go`) defines 3 tools passed to the API:
- `list_files` — list directory contents
- `read_file` — read a file
- `write_file` — write/create a file

Tool calling runs in a loop (max 10 rounds) with infinite-loop detection (breaks after 3 consecutive identical failures). All file operations are sandboxed to the working directory.

### Inter-Agent Communication Protocol
When an agent has downstream connections, its system prompt is augmented to require a `<to_next_agent>...</to_next_agent>` block. Only this block is forwarded to the next agent. If the agent doesn't use the format, the full response is forwarded as fallback.

### Conversation History for API Calls
In `getConversationHistory()`: messages from other agents (which have `role="assistant"` in the UI) are remapped to `role="user"` for the API to maintain proper user/assistant alternation. The most recent message (currently being processed) is excluded from history since it's passed as the prompt.

## TUI Framework

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) + [Bubbles](https://github.com/charmbracelet/bubbles).

Three app modes (`AppMode`):
- `ModeChainSetup` — 2-step wizard (DSL input → agent selection per node)
- `ModeChainExecution` — Side-by-side agent panes with message input at bottom
- `ModeNormal` — 3-pane layout (explorer | editor | chat) with VIM keybindings

The chain execution mode is the primary working mode. Agent panes use `viewport.Model` for scrollable content with scroll position preservation.

## Environment Variables

- **`CLAUDE_API_KEY`** (required) — Anthropic API key
- **`AICHAIN_ALLOWED_DIR`** (optional) — Directory agents can access; defaults to CWD

## Building & Running

```bash
make build              # builds to bin/aichain
make install            # copies to /usr/local/bin
make test               # runs go test ./...

# Run with interactive setup
./bin/aichain --setup

# Run a DSL file directly
./bin/aichain test-chain.dsl

# Debug mode (no alt screen, logs to claudevim-debug.log)
./bin/aichain --setup --debug
```

## Debug Logging

Two log files are written to the project root:
- `claudevim-debug.log` — TUI setup/execution debug logs
- `claude-sdk-debug.log` — Claude API request/response debug logs (tool calls, content blocks)

## Known Quirks & Gotchas

1. **Two binaries exist** — `cmd/aichain` is the one actually used. `cmd/aichain-standalone` is an alternate Cobra-based version with many TODO stubs.

2. **`pkg/` vs `internal/`** — `pkg/server/` and `pkg/claude/` are a simpler HTTP-based server+client pair (legacy VIM plugin backend). The real AI logic lives in `internal/ai/claude.go` using the official Anthropic SDK with tool calling.

3. **`internal/tools/tools.go`** exists as a standalone tool interface but isn't actually used in the chain execution path — the Claude provider has its own tool implementations inline in `claude.go`.

4. **`internal/pipeline/`** is the older pipeline system for session-to-session message routing. The chain execution system (`chain_execution.go`) uses Go channels directly and is the actively used approach.

5. **Model names are scattered** — default model is set in multiple places (`app.go`, `claude.go`, `configs/default.yaml`, agent YAML files). The Claude provider hardcodes `ModelClaudeSonnet4_5_20250929` regardless of what model is configured on the agent node.

6. **The VIM engine** (`internal/vim/`) is largely decorative — it tracks modes and has handler stubs but most key handling is done directly in the Bubble Tea update functions.

7. **`test-chain.dsl`** uses an older DSL format (agent declarations + flow) that differs from the simple arrow syntax the parser actually handles. The parser only handles the `A -> B -> C` style.

8. **Token usage is not tracked** — `AIResponse.TokensUsed`, `InputTokens`, `OutputTokens` are always 0.

9. **No persistence** — sessions, chains, and message history exist only in memory.

10. **The `session/manager.go` in `pkg/`** is a completely separate, simpler session manager used only by the HTTP server; don't confuse it with `internal/session/session.go`.
