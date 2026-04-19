# AIChain

A terminal application for AI agent chaining — define a topology of Claude AI agents using a simple DSL, assign each a role, and watch them collaborate in real time.

```
    ┌─────┐      ┌─────┐      ┌─────┐
    │ AI  │─────▶│ AI  │─────▶│ AI  │
    │ 🤖  │      │ 🧠  │      │ ⚡  │
    └─────┘      └─────┘      └─────┘
        │            │            │
        └────────────┼────────────┘
                     │
               ┌─────▼─────┐
               │  AIChain  │
               │   🚀 TUI  │
               └───────────┘
```

## Quick Start

```bash
# 1. Set your API key
export CLAUDE_API_KEY=your-api-key

# 2. Build
make build

# 3. Run interactive setup
./bin/aichain --setup

# Or run a DSL file directly
./bin/aichain my-chain.dsl
```

## Usage

```
aichain [dsl-file]              Execute a DSL file directly
aichain --setup                 Interactive chain builder (2-step wizard)
aichain --setup --debug         Debug mode (no alt screen, logs to claudevim-debug.log)
aichain --server [--port 8747]  Start the HTTP backend server
aichain --version               Show version
```

### Running a DSL file

Create a file like `my-chain.dsl` containing just the topology:

```
A -> B -> C
```

Then run:

```bash
./bin/aichain my-chain.dsl
```

> **Note:** When running a DSL file directly, agent assignment uses defaults from the `.agents/` directory. For full control, use `--setup` to interactively assign agents to each node.

### Interactive setup

```bash
./bin/aichain --setup
```

**Step 1 — Enter topology:**
```
A -> B -> C
```

**Step 2 — Assign an agent to each node.** For each AI node, select from the pre-configured agents in your `.agents/` directory using `j`/`k` to navigate and `Enter` to confirm.

**Step 3 — Press Enter to start execution.** Each agent gets its own pane. Type a message at the bottom and press Enter to send it into the chain.

## DSL Syntax

| Operator | Meaning |
|----------|---------|
| `A -> B` | A sends output to B (one-way) |
| `A <- B` | B sends output to A (reverse one-way) |
| `A <> B` | A and B communicate bidirectionally |
| `*`      | Human interaction point (⚠️ not yet functional in execution — see Known Issues) |

These compose into chains:

```
A -> B -> C          # linear pipeline
A <> B <> C          # neighbors communicate bidirectionally
A -> B <- C          # B receives from both A and C
```

Node identifiers can be any letters or numbers (`A`, `B`, `C`, `1`, `2`, etc.).

## Agent Definitions

Agents are YAML files in `.agents/`. On first run, AIChain auto-creates defaults: `developer`, `architect`, `reviewer`, `debugger`, `security`, `tester`.

```yaml
# .agents/developer.yaml
id: developer
name: Software Developer
description: Expert at writing, reviewing, and debugging code
model: claude-3-5-sonnet-20241022
role: developer
temperature: 0.3
system_prompt: |
  You are an expert software developer with deep knowledge of multiple
  programming languages and best practices.
tags:
  - coding
  - development
```

Edit these or add your own `.yaml` files to customize agent behavior.

## Example Use Cases

### Code review pipeline: `A -> B -> C`
- **A** = developer (writes code)
- **B** = reviewer (critiques for bugs, style, security)
- **C** = tester (validates with test cases)

### Architecture review: `A <> B <> C`
- **A** = architect (designs systems)
- **B** = developer (implements)
- **C** = security (audits)

All three exchange messages bidirectionally with their neighbors.

## How It Works

Each AI agent runs in its own goroutine. The DSL topology determines which Go channels are wired together:

- `A -> B`: A's output channel → B's input channel
- `A <> B`: channels wired in both directions

When an agent responds, it includes a `<to_next_agent>` block with focused instructions for the next agent. Only this block is forwarded downstream (the full response stays in the agent's own pane).

Agents have access to tools during their turn:

| Tool | Description |
|------|-------------|
| `read_file(path)` | Read file contents |
| `write_file(path, content)` | Write/create a file |
| `list_files(dir)` | List directory contents |

Tool calls are limited to 10 rounds per agent turn to prevent infinite loops. All file operations are sandboxed to the working directory.

## Chain Execution Keybindings

| Key | Action |
|-----|--------|
| Type + `Enter` | Send message to the chain (when input is focused) |
| `Tab` | Cycle focus: input → agent panes → input |
| `←` `→` / `h` `l` | Navigate between agent panes |
| `↑` `↓` / `j` `k` | Scroll within focused agent pane |
| `PgUp` `PgDn` | Page scroll in agent pane |
| `Enter` (in pane mode) | Return focus to message input |
| `Ctrl+C` / `Esc` | Quit |

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `CLAUDE_API_KEY` | Yes | Anthropic API key |
| `AICHAIN_ALLOWED_DIR` | No | Directory agents can access (defaults to CWD) |

## Build & Test

```bash
make build          # Build binary to ./bin/aichain
make test           # Run all tests
make install        # Copy to /usr/local/bin
make clean          # Remove build artifacts
make fmt            # Format code
```

## Debug Mode

```bash
./bin/aichain --setup --debug
```

Debug mode disables the alternate screen buffer (so you can see normal terminal output) and writes detailed logs:

- `claudevim-debug.log` — TUI and chain execution debug logs
- `claude-sdk-debug.log` — Claude API request/response logs (tool calls, content blocks)

## Project Structure

```
cmd/aichain/main.go              # CLI entry point
internal/
├── app/app.go                   # Application orchestrator
├── chain/
│   ├── dsl.go                   # DSL parser (tokenizer-based)
│   ├── agents.go                # YAML agent loader (.agents/ directory)
│   └── setup.go                 # 2-step setup flow
├── tui/
│   ├── model.go                 # Top-level Bubble Tea model
│   ├── chain_setup.go           # Setup wizard UI (DSL input + agent selection)
│   ├── chain_execution.go       # Execution UI (agent panes, channels, goroutines)
│   ├── chain_execution_helpers.go
│   ├── panes.go                 # Explorer, Editor, Chat pane components
│   └── messages.go              # Bubble Tea message types
├── ai/
│   ├── provider.go              # AI provider interface
│   └── claude.go                # Claude SDK integration with tool calling
├── tools/tools.go               # Tool interface (read/write/list files)
├── session/session.go           # Session management
├── pipeline/pipeline.go         # Pipeline message routing
└── vim/keybindings.go           # VIM mode engine
```

## Known Issues

- **Human node (`*`) is not functional in chain execution.** The `*` node is parsed correctly in the DSL and skipped during agent assignment, but no channel wiring or UI exists for it during execution. Chains containing `*` will break at that point. See `CODE_WALKTHROUGH.md` for details.
- **Agent model selection is ignored** — all agents use the same Claude model regardless of YAML config.
- **No persistence** — sessions and message history exist only in memory.

See `CODE_WALKTHROUGH.md` for a complete bug inventory with code references.

## License

MIT
