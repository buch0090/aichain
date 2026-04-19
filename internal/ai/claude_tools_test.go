package ai

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestProvider(t *testing.T) *ClaudeProvider {
	t.Helper()
	// Create a provider with nil client — we only test tool functions, not API calls
	return &ClaudeProvider{
		sdkLogger: log.New(os.Stderr, "TEST: ", 0),
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ─────────────────────────────────────────────────
// read_file with offset/limit
// ─────────────────────────────────────────────────

func TestReadFile_FullFile(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "hello.txt", "line1\nline2\nline3\nline4\nline5")

	result, err := p.readFile("hello.txt", dir, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line5") {
		t.Errorf("expected full file content, got: %s", result)
	}
	if !strings.Contains(result, "5 lines") {
		t.Errorf("expected line count in output, got: %s", result)
	}
}

func TestReadFile_WithOffset(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "hello.txt", "line1\nline2\nline3\nline4\nline5")

	result, err := p.readFile("hello.txt", dir, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "line1") || strings.Contains(result, "line2") {
		t.Errorf("offset=3 should skip first 2 lines, got: %s", result)
	}
	if !strings.Contains(result, "line3") {
		t.Errorf("offset=3 should start at line3, got: %s", result)
	}
}

func TestReadFile_WithLimit(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "hello.txt", "line1\nline2\nline3\nline4\nline5")

	result, err := p.readFile("hello.txt", dir, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line2") {
		t.Errorf("limit=2 should show first 2 lines, got: %s", result)
	}
	if !strings.Contains(result, "more lines") || !strings.Contains(result, "offset=3") {
		t.Errorf("should show continuation hint, got: %s", result)
	}
}

func TestReadFile_OffsetBeyondEnd(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "hello.txt", "line1\nline2")

	_, err := p.readFile("hello.txt", dir, 100, 0)
	if err == nil {
		t.Error("expected error for offset beyond end of file")
	}
}

func TestReadFile_SecurityCheck(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)

	_, err := p.readFile("../../etc/passwd", dir, 0, 0)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

// ─────────────────────────────────────────────────
// edit_file
// ─────────────────────────────────────────────────

func TestEditFile_SingleEdit(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "code.go", "func hello() {\n\tfmt.Println(\"hello\")\n}\n")

	edits := []interface{}{
		map[string]interface{}{
			"oldText": "\"hello\"",
			"newText": "\"world\"",
		},
	}
	result, err := p.editFile("code.go", edits, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "1 edit(s)") {
		t.Errorf("expected success message, got: %s", result)
	}

	// Verify file was changed
	content, _ := os.ReadFile(filepath.Join(dir, "code.go"))
	if !strings.Contains(string(content), "\"world\"") {
		t.Error("file should contain 'world' after edit")
	}
	if strings.Contains(string(content), "\"hello\"") {
		t.Error("file should not contain 'hello' after edit")
	}
}

func TestEditFile_MultipleEdits(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "code.go", "aaa\nbbb\nccc\n")

	edits := []interface{}{
		map[string]interface{}{"oldText": "aaa", "newText": "AAA"},
		map[string]interface{}{"oldText": "ccc", "newText": "CCC"},
	}
	result, err := p.editFile("code.go", edits, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "2 edit(s)") {
		t.Errorf("expected 2 edits applied, got: %s", result)
	}

	content, _ := os.ReadFile(filepath.Join(dir, "code.go"))
	if string(content) != "AAA\nbbb\nCCC\n" {
		t.Errorf("unexpected content: %q", string(content))
	}
}

func TestEditFile_OldTextNotFound(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "code.go", "hello world\n")

	edits := []interface{}{
		map[string]interface{}{"oldText": "xyz", "newText": "abc"},
	}
	_, err := p.editFile("code.go", edits, dir)
	if err == nil {
		t.Error("expected error when oldText not found")
	}
}

func TestEditFile_DuplicateMatch(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "code.go", "hello\nhello\n")

	edits := []interface{}{
		map[string]interface{}{"oldText": "hello", "newText": "world"},
	}
	_, err := p.editFile("code.go", edits, dir)
	if err == nil {
		t.Error("expected error when oldText matches multiple locations")
	}
}

// ─────────────────────────────────────────────────
// bash
// ─────────────────────────────────────────────────

func TestBash_SimpleCommand(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)

	result, err := p.bash("echo hello", dir, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", result)
	}
}

func TestBash_WorkingDirectory(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "testfile.txt", "content")

	result, err := p.bash("ls", dir, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "testfile.txt") {
		t.Errorf("expected 'testfile.txt' in ls output, got: %s", result)
	}
}

func TestBash_FailingCommand(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)

	result, err := p.bash("exit 42", dir, 5)
	if err != nil {
		t.Fatal(err) // bash tool should not return an error — it returns exit code in output
	}
	if !strings.Contains(result, "42") {
		t.Errorf("expected exit code 42 in output, got: %s", result)
	}
}

func TestBash_Timeout(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)

	result, err := p.bash("sleep 10", dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "timed out") {
		t.Errorf("expected timeout message, got: %s", result)
	}
}

func TestBash_Grep(t *testing.T) {
	p := newTestProvider(t)
	dir := tempDir(t)
	writeTestFile(t, dir, "code.go", "func main() {\n\tfmt.Println(\"hello\")\n}\n")
	writeTestFile(t, dir, "other.go", "package other\n")

	result, err := p.bash("grep -rn 'Println' .", dir, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "code.go") {
		t.Errorf("grep should find match in code.go, got: %s", result)
	}
}

// ─────────────────────────────────────────────────
// truncation
// ─────────────────────────────────────────────────

func TestTruncateOutput_SmallInput(t *testing.T) {
	result := truncateOutput("hello\nworld", false)
	if result != "hello\nworld" {
		t.Errorf("small input should pass through unchanged, got: %q", result)
	}
}

func TestTruncateOutput_ManyLines_Head(t *testing.T) {
	var lines []string
	for i := 0; i < 3000; i++ {
		lines = append(lines, "line")
	}
	input := strings.Join(lines, "\n")

	result := truncateOutput(input, false)
	if !strings.Contains(result, "Truncated") {
		t.Error("expected truncation notice for 3000 lines")
	}
	if !strings.Contains(result, "offset=") {
		t.Error("expected continuation hint with offset")
	}
}

func TestTruncateOutput_ManyLines_Tail(t *testing.T) {
	var lines []string
	for i := 0; i < 3000; i++ {
		lines = append(lines, "line")
	}
	input := strings.Join(lines, "\n")

	result := truncateOutput(input, true)
	if !strings.Contains(result, "truncated") || !strings.Contains(result, "last") {
		t.Error("expected tail truncation notice for 3000 lines")
	}
}

// ─────────────────────────────────────────────────
// resolvePath
// ─────────────────────────────────────────────────

func TestResolvePath_RelativePath(t *testing.T) {
	dir := tempDir(t)
	path, err := resolvePath("sub/file.txt", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("resolved path should be within dir: %s", path)
	}
}

func TestResolvePath_Traversal(t *testing.T) {
	dir := tempDir(t)
	_, err := resolvePath("../../etc/passwd", dir)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}
