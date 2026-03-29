package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

var resetAfterPattern = regexp.MustCompile(`(?i)reset after ([0-9]+[smh])`)
var gopilotFileBlockPattern = regexp.MustCompile("(?s)```gopilot-file\\s+path=([^\\n]+)\\n(.*?)```")

func cloneMessages(messages []chat.Message) []chat.Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]chat.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func cloneContextFiles(files []chat.ContextFile) []chat.ContextFile {
	if len(files) == 0 {
		return nil
	}

	cloned := make([]chat.ContextFile, len(files))
	copy(cloned, files)
	return cloned
}

func cloneRequest(req chat.Request) chat.Request {
	return chat.Request{
		Model:          req.Model,
		Messages:       cloneMessages(req.Messages),
		WorkspaceRoot:  req.WorkspaceRoot,
		ContextFiles:   cloneContextFiles(req.ContextFiles),
		AllowFileEdits: req.AllowFileEdits,
	}
}

func formatBackendError(err error) string {
	if err == nil {
		return ""
	}

	text := strings.TrimSpace(err.Error())

	if strings.Contains(text, "RESOURCE_EXHAUSTED") {
		if strings.Contains(strings.ToLower(text), "quota will reset after") {
			return fmt.Sprintf("Temporary model quota hit. %s Wait briefly, then retry, or use `Ctrl+N` or `/model` to switch models.", text)
		}
		return fmt.Sprintf("Model quota exhausted. %s Use `Ctrl+N` or `/model` to switch models and try again.", text)
	}

	if strings.Contains(text, "429") {
		return fmt.Sprintf("Request rate-limited. %s", text)
	}

	return fmt.Sprintf("Request failed. %s", text)
}

func quotaRetryDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}

	text := strings.TrimSpace(err.Error())
	if !strings.Contains(text, "RESOURCE_EXHAUSTED") {
		return 0, false
	}

	match := resetAfterPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0, false
	}

	delay, parseErr := time.ParseDuration(strings.ToLower(match[1]))
	if parseErr != nil || delay <= 0 {
		return 0, false
	}

	return delay, true
}

func formatRetryNotice(err error, delay time.Duration) string {
	text := strings.TrimSpace(err.Error())
	return fmt.Sprintf("Temporary model quota hit. %s Retrying automatically in %s.", text, delay.Round(time.Second))
}

func lastAssistantMessage(messages []chat.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].From != "GoPilot" {
			continue
		}

		text := strings.TrimSpace(messages[i].Content)
		if text == "" || text == initialSplash {
			continue
		}

		return messages[i].Content
	}

	return ""
}

func extractFirstFencedCodeBlock(text string) string {
	lines := strings.Split(text, "\n")
	var codeLines []string
	inCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				return strings.Join(codeLines, "\n")
			}

			inCode = true
			codeLines = nil
			continue
		}

		if inCode {
			codeLines = append(codeLines, line)
		}
	}

	return ""
}

func plainViewContent(text string) string {
	code := extractFirstFencedCodeBlock(text)
	if code != "" {
		return code
	}

	trimmed := strings.TrimSpace(text)
	if looksLikeJSON(trimmed) {
		return trimmed
	}

	return text
}

func looksLikeJSON(text string) bool {
	if text == "" {
		return false
	}

	if !(strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")) {
		return false
	}

	var parsed any
	return json.Unmarshal([]byte(text), &parsed) == nil
}

func normalizeWorkspacePath(workspaceRoot string, inputPath string) (string, string, error) {
	trimmed := strings.TrimSpace(inputPath)
	if trimmed == "" {
		return "", "", fmt.Errorf("path is empty")
	}

	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace root: %w", err)
	}

	cleanInput := filepath.Clean(trimmed)
	targetPath := cleanInput
	if !filepath.IsAbs(cleanInput) {
		targetPath = filepath.Join(rootAbs, cleanInput)
	}

	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}

	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", "", fmt.Errorf("compute relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path %q is outside the workspace", trimmed)
	}

	return filepath.ToSlash(rel), targetAbs, nil
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".sh":
		return "bash"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".txt":
		return "text"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func loadContextFile(workspaceRoot string, inputPath string) (chat.ContextFile, error) {
	relPath, absPath, err := normalizeWorkspacePath(workspaceRoot, inputPath)
	if err != nil {
		return chat.ContextFile{}, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return chat.ContextFile{}, fmt.Errorf("stat %s: %w", relPath, err)
	}
	if info.IsDir() {
		return chat.ContextFile{}, fmt.Errorf("%s is a directory", relPath)
	}
	if info.Size() > 256*1024 {
		return chat.ContextFile{}, fmt.Errorf("%s is too large for context (%d bytes)", relPath, info.Size())
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return chat.ContextFile{}, fmt.Errorf("read %s: %w", relPath, err)
	}

	return chat.ContextFile{
		Path:     relPath,
		Language: detectLanguage(relPath),
		Content:  string(data),
	}, nil
}

func contextFileList(files []chat.ContextFile) string {
	if len(files) == 0 {
		return "No files attached."
	}

	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return "Attached files:\n- " + strings.Join(paths, "\n- ")
}

type proposedFileEdit struct {
	Path    string
	Content string
}

func parseProposedFileEdits(text string) ([]proposedFileEdit, error) {
	matches := gopilotFileBlockPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no `gopilot-file` blocks found in the last response")
	}

	edits := make([]proposedFileEdit, 0, len(matches))
	for _, match := range matches {
		path := strings.TrimSpace(match[1])
		if path == "" {
			return nil, fmt.Errorf("found an edit block with an empty path")
		}

		content := strings.ReplaceAll(match[2], "\r\n", "\n")
		edits = append(edits, proposedFileEdit{
			Path:    path,
			Content: content,
		})
	}

	return edits, nil
}

func applyProposedFileEdits(workspaceRoot string, text string) ([]string, error) {
	edits, err := parseProposedFileEdits(text)
	if err != nil {
		return nil, err
	}

	written := make([]string, 0, len(edits))
	for _, edit := range edits {
		relPath, absPath, err := normalizeWorkspacePath(workspaceRoot, edit.Path)
		if err != nil {
			return nil, err
		}

		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, fmt.Errorf("create parent directory for %s: %w", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(edit.Content), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", relPath, err)
		}
		written = append(written, relPath)
	}

	sort.Strings(written)
	return written, nil
}
