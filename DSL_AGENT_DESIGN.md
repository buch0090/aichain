# DSL Agent Design - Multi-Agent Communication System

## Overview
ClaudeVIM uses a Domain Specific Language (DSL) to define AI agent chains that can collaborate on software development tasks using Go channels for inter-agent communication and the basic Anthropic Go SDK.

## DSL Syntax
```
A -> B          Simple chain: A sends to B
A <> B          Two-way: A and B communicate both ways  
A -> B -> C     Linear chain: A to B to C
A <> B <> C     All nodes communicate with neighbors
A -> * <- B     Human (*) receives from both A and B
A <> *          Human can communicate with A
```

## Architecture Components

### 1. ChainAgent Structure
```go
type ChainAgent struct {
    ID         string
    Node       *chain.ChainNode
    provider   *ai.ClaudeProvider
    
    // Channels for inter-agent communication
    InChan     chan AgentMessage
    OutChans   map[string]chan AgentMessage
    
    // Tools the agent can use
    tools      map[string]Tool
    workingDir string
}
```

### 2. Agent Communication Flow
```go
func (a *ChainAgent) Run(ctx context.Context) {
    for {
        select {
        case msg := <-a.InChan:
            // Agent receives message from another agent or human
            response := a.processMessage(msg)
            
            // Send response to appropriate targets based on DSL
            a.routeResponse(response)
            
        case <-ctx.Done():
            return
        }
    }
}
```

### 3. DSL-Based Channel Routing
```go
func (c *ChainExecutionModel) setupAgentChannels() {
    // Parse DSL topology: "A -> B -> C"
    // Create channels: A.OutChans["B"] = B.InChan
    
    for _, connection := range c.chain.Connections {
        fromAgent := c.agents[connection.From]
        toAgent := c.agents[connection.To]
        
        if connection.Type == chain.OneWay {
            fromAgent.OutChans[connection.To] = toAgent.InChan
        } else if connection.Type == chain.TwoWay {
            fromAgent.OutChans[connection.To] = toAgent.InChan  
            toAgent.OutChans[connection.From] = fromAgent.InChan
        }
    }
}
```

### 4. Tool Integration with Basic Anthropic SDK
```go
func (a *ChainAgent) processMessage(msg AgentMessage) AgentMessage {
    // Build context with conversation history + tool capabilities
    systemPrompt := fmt.Sprintf(`%s

Available tools:
- read_file(path): Read file contents
- write_file(path, content): Write file
- run_command(cmd): Execute shell command
- list_files(dir): List directory contents

When you need to use a tool, respond with: TOOL_CALL:tool_name:args
`, a.Node.SystemPrompt)

    // Use basic Anthropic SDK
    response, err := a.provider.SendMessage(ctx, msg.Content, ai.AIContext{
        SystemPrompt: systemPrompt,
        ConversationHistory: a.getHistory(),
        // ... other context
    })
    
    // Parse response for tool calls
    if strings.HasPrefix(response.Content, "TOOL_CALL:") {
        return a.executeTool(response.Content)
    }
    
    return AgentMessage{Content: response.Content}
}
```

## Implementation Plan

### Phase 1: Core Architecture
1. **DSL-based routing system** - Parse chain topology and create channel connections
2. **ChainAgent struct** - Agent with channels, tools, and Claude provider
3. **Agent Communication Flow** - Go channels for inter-agent messaging

### Phase 2: Concurrent Processing
4. **Agent goroutines** - Each agent runs independently, processing messages
5. **Tool Integration** - Simple text-based tool calling with Basic Anthropic SDK

### Phase 3: Tool Framework
6. **Tool framework** - File operations, shell commands, git integration

## Example Workflows

### Software Development Chain
```
DSL: Developer -> Reviewer -> Tester -> *

Flow:
1. Developer: Reads requirements → writes code → sends to Reviewer
2. Reviewer: Gets code → analyzes → suggests changes → sends feedback
3. Tester: Gets final code → runs tests → reports to human
4. Human: Reviews results → gives new instructions
```

### Collaborative Architecture Design
```
DSL: Architect <> Developer <> Security

Flow:
1. Architect: Designs system architecture
2. Developer: Implements based on architecture, asks questions
3. Security: Reviews for vulnerabilities, suggests improvements
4. All agents can communicate bidirectionally for discussion
```

## Tool Categories

### File Operations
- `read_file(path)` - Read file contents
- `write_file(path, content)` - Write/modify files
- `list_files(dir)` - Directory listing
- `create_directory(path)` - Create directories

### Development Tools
- `run_command(cmd)` - Execute shell commands
- `run_tests()` - Execute test suite
- `build_project()` - Build/compile project
- `lint_code()` - Run code linting

### Git Operations
- `git_status()` - Check git status
- `git_commit(message)` - Create commits
- `git_diff()` - Show changes
- `git_branch()` - Branch operations

## Benefits

1. **True Multi-Agent Collaboration**: Agents work together, not just in parallel
2. **DSL-Driven**: Topology defined by simple, readable DSL
3. **Go Native**: Leverages Go's excellent concurrency primitives
4. **Tool-Enabled**: Agents can actually modify code and run commands
5. **Scalable**: Easy to add new agents and connections
6. **Simple**: Uses basic Anthropic SDK without complex frameworks

## Current Status

- ✅ Basic Claude API integration
- ✅ Chain setup and execution UI
- ✅ Agent panes with scrolling
- 🔄 **Next**: Implement multi-agent channel communication system