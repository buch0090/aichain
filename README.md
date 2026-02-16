# AIChain 🚀

A revolutionary VIM-like terminal application for AI agent chaining, featuring **multiple AI agents that can communicate with each other** in real-time.

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

## 🌟 Unique Features

### **AI-to-AI Communication** (Never Done Before!)
- **Multiple AI Agents**: Multiple AI models working together in chains
- **AI Debate Mode**: Watch different AI models debate topics and build on each other's ideas
- **Pipeline Workflows**: Chain multiple AI agents for complex problem-solving
- **Real-time Collaboration**: AI agents can see and respond to each other's outputs

### **True VIM Experience**
- Full VIM modal editing (Normal, Insert, Visual, Command modes)
- VIM keybindings throughout the entire interface
- No mouse required - pure keyboard workflow
- Familiar VIM commands (`:q`, `gt`, `gT`, `v`, etc.)

### **Professional Terminal Interface**
- Three-pane layout: File Explorer | Editor | AI Chat
- File explorer with VIM navigation (`j`/`k`, `h`/`l`)
- Real-time chat with multiple AI sessions
- Session templates for common workflows

## 🔧 Quick Start

### Prerequisites
```bash
# Required: Claude API key
export CLAUDE_API_KEY=your-claude-api-key-here
```

### Installation
```bash
# Build from source
make -f Makefile-standalone build

# Or use development mode
make -f Makefile-standalone dev
```

### First Launch
```bash
# Start AIChain
./bin/aichain

# Or start an AI debate immediately
./bin/aichain debate "Should AI development be regulated?"
```

## 🎮 Basic Usage

### VIM Keybindings
| Mode | Key | Action |
|------|-----|--------|
| Normal | `E` | Toggle file explorer |
| Normal | `Ctrl+t` | New AI session |
| Normal | `gt` / `gT` | Next/Previous session |
| Normal | `h`/`j`/`k`/`l` | Navigate panes |
| Normal | `i` | Enter insert mode |
| Normal | `v` | Enter visual mode |
| Normal | `:` | Command mode |
| Visual | `Enter` | Send selection to AI |
| Explorer | `j`/`k` | Navigate files |
| Explorer | `Enter` | Open file |

### Commands
```vim
:session new            # Create new AI session
:dual                   # Create dual AI session
:debate [topic]         # Start AI debate
:q                      # Quit
```

## 🤖 AI Collaboration Examples

### 1. Code Review Workflow
```bash
# Create two AI sessions - Developer and Reviewer
:session new Developer
:session new Reviewer
:dual

# Developer writes code, automatically sent to Reviewer
# Reviewer provides feedback, sent back to Developer
```

### 2. AI Debate
```bash
# Start a debate between two AI perspectives
./bin/aichain debate "Is functional programming better than OOP?"

# Watch the AIs build arguments and counter-arguments
# You can jump in at any time with your own input
```

### 3. Architecture Design Pipeline
```bash
# Chain AIs: Architect → Database Expert → Security Expert
:session new Architect
:session new Database
:session new Security
:pipeline chain Architect Database Security

# Each AI builds on the previous one's output
```

## 📁 Project Structure
```
aichain/
├── cmd/aichain-standalone/    # Main application
├── internal/
│   ├── app/                     # Core application logic
│   ├── ai/                      # AI provider interfaces
│   │   ├── provider.go          # Generic AI interface
│   │   └── claude.go            # Claude implementation
│   ├── pipeline/                # AI-to-AI communication
│   │   └── pipeline.go          # Pipeline engine
│   ├── session/                 # Session management
│   │   └── session.go           # Multi-session handling
│   ├── vim/                     # VIM keybinding engine
│   │   └── keybindings.go       # Modal editing system
│   ├── tui/                     # Terminal UI
│   │   ├── model.go             # Main TUI model
│   │   ├── panes.go             # Explorer, Editor, Chat panes
│   │   └── messages.go          # Event system
│   └── config/                  # Configuration system
│       └── config.go            # YAML configuration
├── configs/
│   └── default.yaml             # Default configuration
└── Makefile-standalone          # Build system
```

## ⚙️ Configuration

AIChain uses YAML configuration:

```yaml
# ~/.config/aichain/config.yaml
app:
  default_model: "claude-opus-4-5-20251101"
  default_provider: "claude"

ui:
  theme: "dark"
  default_layout: "triple"  # explorer | editor | chat
  triple_proportions: [0.2, 0.5, 0.3]

session_templates:
  code_review:
    sessions:
      - name: "Developer"
        model: "claude-opus-4-5-20251101"
        role: "developer"
      - name: "Reviewer" 
        model: "claude-sonnet-4-5-20251101"
        role: "reviewer"
    pipeline:
      type: "dual"
      auto_forward: true
```

## 🛠️ Development

### Build Commands
```bash
make -f Makefile-standalone build      # Build application
make -f Makefile-standalone test       # Run tests  
make -f Makefile-standalone dev        # Development mode
make -f Makefile-standalone build-all  # Multi-platform builds
```

### Architecture Highlights
- **Go + Bubble Tea**: Fast, lightweight TUI framework
- **Concurrent AI Sessions**: Thread-safe session management
- **Plugin Architecture**: Easy to add new AI providers
- **Event-Driven**: Reactive UI updates
- **VIM Engine**: Full modal editing implementation

## 🎯 What Makes This Special

### 1. **AI Collaboration** (Industry First)
- No other tool enables multiple AI models to collaborate in real-time
- Watch different AI personalities debate and build on each other's ideas
- Create custom AI workflows and pipelines

### 2. **True VIM Integration**
- Not just VIM-inspired - actual VIM modal editing
- Every feature accessible via keyboard
- Familiar workflow for VIM experts

### 3. **Terminal-Native**
- No browser, no Electron - pure terminal performance
- Lightweight and fast
- Works over SSH, in tmux, anywhere terminals work

### 4. **Professional Workflow**
- File explorer + editor + AI chat in one interface
- Session templates for repeatable workflows
- Configurable layouts and keybindings

## 🚀 Future Enhancements

- [ ] **Multi-Provider Support**: OpenAI GPT, local models via Ollama
- [ ] **Voice Integration**: Talk to your AI sessions
- [ ] **Code Execution**: Run code directly from AI suggestions
- [ ] **Team Collaboration**: Share AI sessions across team members
- [ ] **Plugin System**: Custom AI workflows and integrations
- [ ] **Git Integration**: AI-powered code reviews and commits

## 🤝 Contributing

This is a prototype showcasing the innovative AI collaboration concept. The core architecture is complete and functional.

## 📄 License

MIT License - Feel free to build upon this innovative foundation!

---

**AIChain**: Where VIM meets collaborative AI. The future of coding is here! 🌟