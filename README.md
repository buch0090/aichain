# AIChain

A revolutionary VIM-like terminal application for AI agent chaining, featuring **multiple AI agents that
-can communicate with each other** in real-time.

```
    ┌─────┐      ┌─────┐      ┌─────┐      ┌─────┐
    │ AI  │──────│ AI  │──────│ AI  │──────│ AI  │
    │ 🤖  │ ⟸──⟹ │ 🧠  │ ⟸──⟹ │ ⚡  │ ⟸──⟹ │ 🎯  │
    └─────┘      └─────┘      └─────┘      └─────┘
        │            │            │            │
        └────────────┼────────────┼────────────┘
                     │            │
               ┌─────▼────────────▼─────┐
               │      AIChain 🚀       │
               │   VIM + AI Agents     │
               └───────────────────────┘
```

## DSL Overview

AIChain uses a topology DSL to define how agents communicate. You enter this in the interactive setup screen.

### Topology Syntax

| Operator | Meaning |
|----------|---------|
| `A -> B` | A sends output to B (one-way) |
| `A <- B` | A receives from B (one-way, reversed) |
| `A <> B` | A and B communicate bidirectionally |
| `*`      | Human node — a pane where you interact directly |

These compose into chains:

```
A -> B -> C          # linear pipeline
A <> B <> C          # all neighbors communicate bidirectionally
A -> * <- B          # human receives from both A and B
A <> *               # human and A communicate bidirectionally
```

Node identifiers can be any letters or numbers (`A`, `B`, `dev`, `review`, etc.).

## Interactive Setup

Run the setup screen:

```bash
./bin/aichain --setup
```

**Step 1 — Enter topology:**
```
A -> B -> C
```

**Step 2 — Assign an agent to each node.** For each node (`A`, `B`, `C`), you select from the pre-configured agents in your `.agents/` directory.

## Agent Definitions

Agents are YAML files stored in `.agents/`:

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

On first run, AIChain creates default agents in `.agents/`: `developer`, `architect`, `reviewer`, `debugger`, `security`, `tester`. You can edit these or add your own.

## Example Topologies

### Linear code review pipeline
```
A -> B -> C
```
Assign: A = developer, B = reviewer, C = tester. Developer writes, reviewer critiques, tester validates.

### Bidirectional collaboration
```
A <> B <> C
```
Assign: A = architect, B = developer, C = security. All three can exchange messages with their neighbors.

### Fan-in to human
```
A -> * <- B
```
Both agents send output to a human pane for review and decision.

### Human in the loop
```
A -> B -> *
```
Output flows through two agents before reaching you.

## How It Works

Each agent in the `flow` runs in its own goroutine and communicates via Go channels. The DSL topology determines which channels are wired together:

- `A -> B`: A's output channel connects to B's input channel
- `A <> B`: channels are connected in both directions
- `*`: a human-controlled pane that sends/receives from connected agents

Agents can use tools (file read/write, shell commands) during their turns.

## Setup

```bash
export CLAUDE_API_KEY=your-api-key

make -f Makefile-standalone build
./bin/aichain --setup
```

## Tools Available to Agents

| Tool | Description |
|------|-------------|
| `read_file(path)` | Read file contents |
| `write_file(path, content)` | Write to a file |
| `list_files(dir)` | List directory contents |
| `run_command(cmd)` | Execute a shell command |

Tool calls are limited to 5 rounds per agent turn to prevent infinite loops.

## Project Structure

```
internal/
├── chain/
│   ├── dsl.go        # DSL parser — connection types, node extraction
│   ├── agents.go     # ChainAgent struct, goroutine execution
│   └── setup.go      # Chain initialization
├── tui/
│   ├── model.go      # Top-level Bubble Tea model
│   ├── chain_setup.go         # Chain configuration UI
│   └── chain_execution.go     # Running chain with agent panes
├── ai/
│   └── claude.go     # Claude API integration
└── tools/
    └── tools.go      # Tool implementations
```

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down in focused pane |
| `Tab` | Switch focus between agent panes |
| `i` | Enter insert mode (type to agent) |
| `Esc` | Return to normal mode |
| `Enter` | Send message to focused agent |
| `:q` | Quit |

## Build

```bash
make -f Makefile-standalone build      # build binary to ./bin/aichain
make -f Makefile-standalone test       # run tests
make -f Makefile-standalone dev        # build and run
```

## License

MIT
