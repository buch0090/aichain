# Pi Coding Agent vs AIChain: Architecture Comparison

A side-by-side analysis of how [pi](https://github.com/badlogic/pi-mono/tree/main/packages/coding-agent) (a production coding agent) and AIChain handle tool calling, and why AIChain hits "tool calling limit reached."

---

## The Core Difference: No Tool Round Limit vs Hard Cap

### Pi: No artificial limit

Pi's agent loop (`packages/agent/src/agent-loop.ts`) has **no tool round counter**. The loop structure is:

```
while (true):
    stream assistant response from LLM
    if response has tool calls:
        execute all tool calls (parallel or sequential)
        feed results back → continue loop
    else:
        agent is done → break
```

The loop runs until the **model itself decides to stop calling tools** and produces a final text response. There's no `maxToolRounds` variable anywhere. The model naturally converges because:
- It gets the information it needs from tool results
- It decides it has enough context to answer
- The `stop_reason` from the API comes back as `"end_turn"` instead of `"tool_use"`

### AIChain: Hard cap at 25 rounds

AIChain's tool loop (`internal/ai/claude.go`) has:

```go
maxToolRounds := 25
for {
    toolRounds++
    if toolRounds > maxToolRounds {
        finalContent.WriteString("\n[Tool calling limit reached - response may be incomplete]")
        break
    }
    // ... call API, handle tools ...
}
```

**Why 25 is still not enough:** A real coding task (explore a Laravel project, read 5-6 files, understand structure, write a migration) can easily take 15+ tool calls just for reading. If the agent then writes files and reads more for verification, 25 rounds is tight.

**The fix is simple: remove the cap**, or make it very high (200+) as a safety net, not a practical limit.

---

## Tool Richness: 7 Tools vs 3 Tools

### Pi's tools (`src/core/tools/`)

| Tool | What it does | Why it matters |
|------|-------------|----------------|
| **read** | Read files with `offset`/`limit` pagination | Agent can read large files in chunks without blowing up context |
| **bash** | Execute any shell command | Agent can run `grep`, `git`, `npm`, `go build`, tests, anything |
| **edit** | Targeted find-and-replace edits with diff | Precise edits without rewriting entire files |
| **write** | Create/overwrite files | For new files |
| **grep** | Search with regex, glob filters, context lines | Fast code search without bash overhead |
| **find** | Find files by name/pattern | Navigate unfamiliar projects |
| **ls** | List directory with entry limit | Quick directory overview |

### AIChain's tools (`internal/ai/claude.go`)

| Tool | What it does | What's missing |
|------|-------------|----------------|
| **list_files** | List a directory | No entry limit, no recursion |
| **read_file** | Read an entire file | No offset/limit — blows up context on large files |
| **write_file** | Write entire file | No targeted edit — must rewrite entire file to change one line |

### What this means in practice

**AIChain's agent exploring a project:**
1. `list_files(".")` → sees 50 files
2. `list_files("app")` → more files
3. `list_files("app/Models")` → more files
4. `read_file("app/Models/User.php")` → entire 500-line file dumped into context
5. `read_file("app/Models/Survey.php")` → another entire file
6. `read_file("database/migrations/...")` → another entire file

Each `read_file` dumps the **entire file** into the conversation. After 4-5 reads, the context is bloated with thousands of lines of code. The model then needs more tool calls because it's working with a massive, unfocused context.

**Pi's agent doing the same:**
1. `bash("find . -name '*.php' -path '*/Models/*'")` → targeted file list in one call
2. `read("app/Models/User.php", offset=1, limit=50)` → just the first 50 lines
3. `grep("survey", path="app", glob="*.php")` → finds relevant files instantly
4. `read("app/Models/Survey.php", offset=30, limit=20)` → just the relevant section
5. `edit("database/migrations/...", edits=[{oldText: "...", newText: "..."}])` → surgical change

Fewer calls, smaller context, more precise results.

---

## Output Truncation: Smart vs None

### Pi: Built-in truncation system

Pi has a dedicated `truncate.ts` module with two strategies:
- **`truncateHead`**: Keep first N lines/bytes (for file reads — you want to see the beginning)
- **`truncateTail`**: Keep last N lines/bytes (for bash output — you want to see errors at the end)

Defaults: **2000 lines or 50KB**, whichever hits first.

When truncation happens, the tool result tells the model exactly how to get more:
```
[Showing lines 1-2000 of 15000. Use offset=2001 to continue.]
```

The model can then decide if it needs more or has enough.

### AIChain: No truncation

`read_file` in AIChain returns the raw file content with no limits:

```go
content, err := os.ReadFile(fullPath)
return fmt.Sprintf("Content of %s:\n```\n%s\n```", path, string(content))
```

A 5000-line file gets dumped entirely into the API context. This:
- Wastes tokens (you're paying for all of it)
- Fills up `MaxTokens` budget faster
- Makes the model's job harder (finding relevant info in a sea of code)
- Can hit the model's context window limit on large files

---

## Edit Strategy: Surgical vs Full Rewrite

### Pi: Targeted find-and-replace

Pi's `edit` tool takes an array of `{oldText, newText}` pairs:

```json
{
  "path": "app/Models/User.php",
  "edits": [
    {"oldText": "protected $fillable = [", "newText": "protected $fillable = [\n        'survey_id',"}
  ]
}
```

This:
- Only touches the lines that need changing
- Produces a diff the user can review
- Doesn't require reading the entire file first
- Uses minimal tokens

### AIChain: Rewrite the entire file

AIChain's `write_file` is the only way to modify a file. To change one line in a 500-line file, the agent must:
1. `read_file` the entire file (500 lines into context)
2. `write_file` the entire file with the one-line change (500 lines sent back through the API)

That's 1000 lines of token usage for a 1-line change. And it forces an extra tool round.

---

## Bash: Full Shell vs No Shell

### Pi: Full bash access

Pi gives the agent a full shell. The agent can:
- `grep -r "pattern" --include="*.go"` — search the whole project
- `go test ./...` — run tests to verify changes
- `git diff` — see what changed
- `wc -l *.go` — quick file stats
- Pipe commands, use sed/awk for complex transforms

This drastically reduces tool rounds because one bash command can do what would take 5-10 `list_files`/`read_file` calls.

### AIChain: No shell access

AIChain has no bash tool. The agent can only list directories, read files, and write files. To search for a pattern across a project, it must:
1. `list_files(".")` 
2. `list_files("app")`
3. `list_files("app/Models")`
4. `read_file("app/Models/User.php")` — check manually
5. `read_file("app/Models/Survey.php")` — check manually
6. ... repeat for every file

This is why 25 tool rounds isn't enough.

---

## Context Management: Compaction vs Nothing

### Pi: Automatic context compaction

Pi tracks context size and automatically compacts when it gets too large:

```typescript
// From compaction.ts
shouldCompact(contextTokens, model.contextLength, threshold)
```

Compaction:
1. Summarizes the conversation so far
2. Tracks which files were read and modified
3. Replaces the full history with a compact summary
4. Lets the conversation continue with a fresh, smaller context

This means pi sessions can run indefinitely — the agent can do hundreds of tool calls across a long session without context overflow.

### AIChain: No context management

AIChain accumulates every message and tool result in the conversation history forever. After a complex task with many tool calls:
- The context grows huge
- The model slows down (more tokens to process per call)
- Eventually hits the model's context window limit
- No recovery mechanism

---

## MaxTokens: Generous vs Restrictive

### Pi: 8192+ output tokens

Pi uses model-appropriate max tokens (typically 8192 for Sonnet/Opus).

### AIChain: 4096 in chain, 1024 previously

AIChain's chain agents use `MaxTokens: 4096` (recently bumped from 1024). This is the **output** token limit per API call. With tool calling:
- Each API call needs output tokens for: thinking + tool call JSON + any text
- 4096 is workable but tight for complex multi-tool responses
- The model sometimes can't fit a complete code file in one tool call

---

## Summary: Why AIChain Hits the Limit

| Factor | Pi | AIChain | Impact |
|--------|------|---------|--------|
| Tool round limit | None (model decides) | 25 hard cap | **Primary cause** |
| Tools available | 7 (read, bash, edit, write, grep, find, ls) | 3 (list, read, write) | Agent needs 3-5x more calls to do the same work |
| Bash/shell access | Yes | No | Can't search, test, or run commands |
| File read pagination | offset/limit params | Reads entire file | Wastes context, forces more reads |
| Output truncation | 2000 lines / 50KB | None | Large files blow up context |
| Targeted edits | edit tool (find/replace) | write_file (full rewrite) | 2x more tool calls for edits |
| Context compaction | Automatic | None | Long sessions degrade |
| MaxTokens | 8192 | 4096 | Less room per API call |

## Recommended Fixes (Priority Order)

### 1. Remove the tool round cap (immediate fix)
Change `maxToolRounds` to something like 200 or remove the cap entirely. Let the model decide when it's done. Add a timeout instead (e.g., 5 minutes per agent turn).

### 2. Add a bash tool (biggest impact)
One `bash` tool call replaces 5-10 file-browsing calls. Let the agent run `grep`, `find`, `ls -la`, `go test`, etc.

### 3. Add offset/limit to read_file
Don't dump entire files. Let the agent read chunks:
```json
{"path": "big-file.go", "offset": 1, "limit": 100}
```

### 4. Add output truncation
Cap tool results at 2000 lines / 50KB. Tell the model how to get more.

### 5. Add an edit tool
Targeted find-and-replace instead of full file rewrites. Saves tokens and tool rounds.

### 6. Increase MaxTokens to 8192
Gives the model more room to work per API call.

### 7. Add context compaction (longer term)
Summarize old conversation history to keep context manageable across long sessions.
