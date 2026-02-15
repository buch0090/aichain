# ClaudeVIM

**VIM with AI Superpowers** - The first truly VIM-native Claude AI integration.

## What Makes ClaudeVIM Different

Instead of adding chat windows or external interfaces, ClaudeVIM makes Claude feel like a **native VIM feature**:

- **Claude sessions ARE VIM buffers** - Show up in `:ls`, navigate with `:b Claude:Debug`
- **Pure VIM commands** - `:tabnew Claude:Review`, `:vs Claude:Docs` 
- **100% keyboard workflow** - Never need mouse, all via VIM commands
- **Multiple concurrent sessions** - Managed like VIM tabs with `gt`/`gT`

## Quick Start

```bash
# Install (single command)
curl -L https://github.com/claudevim/install.sh | bash

# First use (interactive setup)
vim myfile.py
:Claude explain
```

## Core Commands

```vim
:Claude explain          " Explain selected code
:Claude fix              " Fix bugs in selection
:Claude optimize         " Performance improvements
:Claude review           " Code review

:tabnew Claude:Debug     " New debugging session
:vs Claude:Review        " Split with review session
:b Claude:Architecture   " Switch to architecture discussion
```

## Architecture

```
VIM Plugin ←→ Go Backend ←→ Claude API
     ↓
  SQLite DB (sessions, history, config)
```

## Development Status

🚧 **Early Development** - Basic functionality being built

- [x] Project structure
- [ ] VIM plugin skeleton
- [ ] Go backend server
- [ ] Claude API integration
- [ ] Session management
- [ ] Setup wizard

## Contributing

This is the early stages - core architecture and basic commands are being developed first.

## License

MIT License - See LICENSE file