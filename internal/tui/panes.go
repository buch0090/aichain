package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aichain/aichain/internal/session"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// Explorer represents the file explorer pane
type Explorer struct {
	files       []FileInfo
	cursor      int
	currentDir  string
	height      int
	width       int
}

// FileInfo represents file information
type FileInfo struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// NewExplorerPane creates a new explorer pane
func NewExplorerPane() *Explorer {
	wd, err := os.Getwd()
	if err != nil {
		wd = "/"
	}

	pane := &Explorer{
		currentDir: wd,
	}
	pane.loadFiles()
	return pane
}

// Update handles messages for the explorer pane
func (p *Explorer) Update(msg tea.Msg) (Explorer, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if p.cursor < len(p.files)-1 {
				p.cursor++
			}
		case "k", "up":
			if p.cursor > 0 {
				p.cursor--
			}
		case "enter":
			if p.cursor < len(p.files) {
				file := p.files[p.cursor]
				if file.IsDir {
					p.enterDirectory(file.Name)
				} else {
					return *p, func() tea.Msg {
						return FileSelectedMsg{
							FilePath: filepath.Join(p.currentDir, file.Name),
						}
					}
				}
			}
		case "h", "backspace":
			p.goUp()
		}
	}
	return *p, nil
}

// View renders the explorer pane
func (p *Explorer) View() string {
	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("📁 %s\n", p.currentDir))
	
	// Safe width calculation for separator
	separatorWidth := p.width - 2
	if separatorWidth < 0 {
		separatorWidth = 10  // minimum width
	}
	b.WriteString(strings.Repeat("─", separatorWidth))
	b.WriteString("\n")

	// Files
	for i, file := range p.files {
		icon := "📄"
		if file.IsDir {
			icon = "📁"
		}

		line := fmt.Sprintf("%s %s", icon, file.Name)
		
		if i == p.cursor {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#444444")).
				Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// loadFiles loads files in the current directory
func (p *Explorer) loadFiles() {
	entries, err := os.ReadDir(p.currentDir)
	if err != nil {
		p.files = []FileInfo{}
		return
	}

	files := make([]FileInfo, 0, len(entries)+1)
	
	// Add parent directory if not root
	if p.currentDir != "/" {
		files = append(files, FileInfo{
			Name:  "..",
			IsDir: true,
		})
	}

	// Add entries
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	p.files = files
	p.cursor = 0
}

// enterDirectory enters a subdirectory
func (p *Explorer) enterDirectory(name string) {
	if name == ".." {
		p.goUp()
		return
	}

	newDir := filepath.Join(p.currentDir, name)
	if info, err := os.Stat(newDir); err == nil && info.IsDir() {
		p.currentDir = newDir
		p.loadFiles()
	}
}

// goUp goes to parent directory
func (p *Explorer) goUp() {
	parent := filepath.Dir(p.currentDir)
	if parent != p.currentDir {
		p.currentDir = parent
		p.loadFiles()
	}
}

// Editor represents the code editor pane
type Editor struct {
	filePath string
	content  []string
	cursor   Cursor
	height   int
	width    int
	scroll   int
}

// Cursor represents cursor position
type Cursor struct {
	Row int
	Col int
}

// NewEditorPane creates a new editor pane
func NewEditorPane() *Editor {
	return &Editor{
		content: []string{"Welcome to AIChain!", "", "Open a file to start editing."},
	}
}

// Update handles messages for the editor pane
func (p *Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	switch msg := msg.(type) {
	case FileSelectedMsg:
		p.loadFile(msg.FilePath)
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if p.cursor.Row < len(p.content)-1 {
				p.cursor.Row++
				p.adjustScroll()
			}
		case "k", "up":
			if p.cursor.Row > 0 {
				p.cursor.Row--
				p.adjustScroll()
			}
		case "h", "left":
			if p.cursor.Col > 0 {
				p.cursor.Col--
			}
		case "l", "right":
			if p.cursor.Row < len(p.content) && p.cursor.Col < len(p.content[p.cursor.Row]) {
				p.cursor.Col++
			}
		}
	}
	return *p, nil
}

// View renders the editor pane
func (p *Editor) View() string {
	var b strings.Builder

	// Header
	filename := "untitled"
	if p.filePath != "" {
		filename = filepath.Base(p.filePath)
	}
	b.WriteString(fmt.Sprintf("📝 %s\n", filename))
	b.WriteString(strings.Repeat("─", p.width-2))
	b.WriteString("\n")

	// Content
	visibleLines := p.height - 3 // Account for header and borders
	start := p.scroll
	end := start + visibleLines
	if end > len(p.content) {
		end = len(p.content)
	}

	for i := start; i < end; i++ {
		line := ""
		if i < len(p.content) {
			line = p.content[i]
		}

		// Add cursor if on this line
		if i == p.cursor.Row {
			if p.cursor.Col <= len(line) {
				if p.cursor.Col == len(line) {
					line += "█" // Cursor at end of line
				} else {
					line = line[:p.cursor.Col] + "█" + line[p.cursor.Col+1:]
				}
			}
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// loadFile loads a file into the editor
func (p *Editor) loadFile(filePath string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		p.content = []string{fmt.Sprintf("Error loading file: %v", err)}
		return
	}

	p.filePath = filePath
	p.content = strings.Split(string(content), "\n")
	p.cursor = Cursor{0, 0}
	p.scroll = 0
}

// adjustScroll adjusts scroll position to keep cursor visible
func (p *Editor) adjustScroll() {
	visibleLines := p.height - 3
	if p.cursor.Row < p.scroll {
		p.scroll = p.cursor.Row
	} else if p.cursor.Row >= p.scroll+visibleLines {
		p.scroll = p.cursor.Row - visibleLines + 1
	}
}

// Chat represents the AI chat pane
type Chat struct {
	session     *session.Session
	sessionID   string
	messages    []ChatMessage
	input       textinput.Model
	height      int
	width       int
	scroll      int
	inputMode   bool
}

// ChatMessage represents a chat message with styling
type ChatMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
	Style     lipgloss.Style
}

// NewChatPane creates a new chat pane
func NewChatPane() *Chat {
	input := textinput.New()
	input.Placeholder = "Type your message..."
	input.CharLimit = 1000

	return &Chat{
		input:    input,
		messages: []ChatMessage{},
	}
}

// Update handles messages for the chat pane
func (p *Chat) Update(msg tea.Msg) (Chat, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.inputMode {
			switch msg.Type {
			case tea.KeyEsc:
				p.inputMode = false
				p.input.Blur()
			case tea.KeyEnter:
				content := p.input.Value()
				if content != "" {
					p.input.SetValue("")
					return *p, p.sendMessage(content)
				}
			default:
				var cmd tea.Cmd
				p.input, cmd = p.input.Update(msg)
				return *p, cmd
			}
		} else {
			switch msg.String() {
			case "i", "a", "o":
				p.inputMode = true
				p.input.Focus()
			case "j", "down":
				if p.scroll < len(p.messages)-1 {
					p.scroll++
				}
			case "k", "up":
				if p.scroll > 0 {
					p.scroll--
				}
			}
		}
	}
	return *p, nil
}

// View renders the chat pane
func (p *Chat) View() string {
	var b strings.Builder

	// Header
	sessionName := "No Session"
	if p.session != nil {
		sessionName = p.session.Name
	}
	b.WriteString(fmt.Sprintf("💬 %s\n", sessionName))
	b.WriteString(strings.Repeat("─", p.width-2))
	b.WriteString("\n")

	// Messages
	visibleLines := p.height - 5 // Account for header, input, and borders
	start := 0
	if len(p.messages) > visibleLines {
		start = len(p.messages) - visibleLines
	}

	for i := start; i < len(p.messages); i++ {
		if i >= len(p.messages) {
			break
		}
		
		msg := p.messages[i]
		prefix := "You: "
		if msg.Role == "assistant" {
			prefix = "AI: "
		}
		
		// Word wrap the message content
		maxWidth := p.width - 4
		wrappedLines := p.wrapText(msg.Content, maxWidth-len(prefix))
		
		// Add the first line with prefix
		var line string
		if len(wrappedLines) > 0 {
			line = fmt.Sprintf("%s%s", prefix, wrappedLines[0])
			
			// Add additional wrapped lines with indentation
			for j := 1; j < len(wrappedLines); j++ {
				line += "\n" + strings.Repeat(" ", len(prefix)) + wrappedLines[j]
			}
		} else {
			line = prefix
		}
		
		if msg.Role == "assistant" {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ff8800")).
				Render(line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00aaff")).
				Render(line)
		}
		
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Input area
	b.WriteString(strings.Repeat("─", p.width-2))
	b.WriteString("\n")
	
	if p.inputMode {
		b.WriteString(p.input.View())
	} else {
		placeholder := "Press 'i' to start typing..."
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Render(placeholder))
	}

	return b.String()
}

// SetSession sets the active session for the chat pane
func (p *Chat) SetSession(sess *session.Session) {
	p.session = sess
	if sess != nil {
		p.sessionID = sess.ID
		p.loadMessages()
	}
}

// loadMessages loads messages from the session
func (p *Chat) loadMessages() {
	if p.session == nil {
		return
	}

	p.messages = make([]ChatMessage, len(p.session.Messages))
	for i, msg := range p.session.Messages {
		p.messages[i] = ChatMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}
}

// AddMessage adds a new message to the chat
func (p *Chat) AddMessage(msg session.Message) {
	chatMsg := ChatMessage{
		Role:      msg.Role,
		Content:   msg.Content,
		Timestamp: msg.Timestamp,
	}
	p.messages = append(p.messages, chatMsg)
}

// sendMessage sends a message to the AI
func (p *Chat) sendMessage(content string) tea.Cmd {
	if p.session == nil {
		return nil
	}

	// Add user message immediately
	userMsg := ChatMessage{
		Role:      "user",
		Content:   content,
		Timestamp: time.Now(),
	}
	p.messages = append(p.messages, userMsg)

	return func() tea.Msg {
		// This would send the message to the application
		return MessageSentMsg{
			SessionID: p.sessionID,
			Content:   content,
		}
	}
}

// wrapText wraps text to fit within the specified width
func (p *Chat) wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	
	var lines []string
	var currentLine string
	
	for _, word := range words {
		testLine := currentLine
		if testLine != "" {
			testLine += " "
		}
		testLine += word
		
		if len(testLine) <= maxWidth {
			currentLine = testLine
		} else {
			// Current line is full, start a new one
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			// Handle very long words that exceed maxWidth
			if len(word) > maxWidth {
				// Break long words
				for len(word) > maxWidth {
					lines = append(lines, word[:maxWidth])
					word = word[maxWidth:]
				}
				currentLine = word
			} else {
				currentLine = word
			}
		}
	}
	
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	
	return lines
}