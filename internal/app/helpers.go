package app

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

var gopilotFileBlockPattern = regexp.MustCompile("(?s)```gopilot-file\\s+path=([^\\n]+)\\n(.*?)```")

type slashCommandSpec struct {
	Name        string
	Description string
	NeedsValue  bool
}

var supportedSlashCommands = []slashCommandSpec{
	{Name: "/add", Description: "Datei als Kontext anhängen", NeedsValue: true},
	{Name: "/apply", Description: "Letzten Dateivorschlag anwenden"},
	{Name: "/clear", Description: "Konversation zurücksetzen"},
	{Name: "/codebase", Description: "Ganzes Working Directory anhängen"},
	{Name: "/copy", Description: "Letzte Antwort kopieren"},
	{Name: "/drop", Description: "Angehängte Datei entfernen", NeedsValue: true},
	{Name: "/files", Description: "Angehängte Dateien anzeigen"},
	{Name: "/model", Description: "Modell auswählen", NeedsValue: true},
	{Name: "/plain", Description: "Letzte Antwort als Plaintext zeigen"},
}

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
		return fmt.Sprintf("API error: %s Please wait and try again later. You can also use `Ctrl+N` or `/model` to switch models.", text)
	}

	if strings.Contains(text, "429") {
		return fmt.Sprintf("API error: %s Please wait and try again later.", text)
	}

	return fmt.Sprintf("Request failed. %s", text)
}

func quotaRetryDelay(err error) (time.Duration, bool) {
	return 0, false
}

func formatRetryNotice(err error, delay time.Duration) string {
	text := strings.TrimSpace(err.Error())
	return fmt.Sprintf("API error: %s Retrying automatically in %s.", text, delay.Round(time.Second))
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

func loadContextTargets(workspaceRoot string, inputPath string) ([]chat.ContextFile, error) {
	relPath, absPath, err := normalizeWorkspacePath(workspaceRoot, inputPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", relPath, err)
	}

	if !info.IsDir() {
		file, err := loadContextFile(workspaceRoot, inputPath)
		if err != nil {
			return nil, err
		}
		return []chat.ContextFile{file}, nil
	}

	var files []chat.ContextFile
	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil || fileInfo.Size() > 256*1024 {
			return nil
		}

		rel, err := filepath.Rel(workspaceRoot, path)
		if err != nil {
			return nil
		}

		file, err := loadContextFile(workspaceRoot, filepath.ToSlash(rel))
		if err != nil {
			return nil
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("%s does not contain any attachable files", relPath)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func splitFirstArgument(input string) (string, string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", ""
	}

	for i, r := range trimmed {
		if r == ' ' || r == '\t' || r == '\n' {
			return trimmed[:i], strings.TrimSpace(trimmed[i+1:])
		}
	}

	return trimmed, ""
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

func activeCompletionSegment(input string, cursor int) (string, int, int, bool) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(input) {
		cursor = len(input)
	}

	start := -1
	for i := cursor - 1; i >= 0; i-- {
		if input[i] != '/' {
			continue
		}
		if i > 0 && !unicode.IsSpace(rune(input[i-1])) {
			continue
		}
		start = i
		break
	}
	if start == -1 {
		return "", 0, 0, false
	}

	end := cursor
	for end < len(input) && !unicode.IsSpace(rune(input[end])) {
		end++
	}

	segment := input[start:cursor]
	if strings.TrimSpace(segment) == "" || !strings.HasPrefix(strings.TrimLeft(segment, " "), "/") {
		return "", 0, 0, false
	}

	return segment, start, end, true
}

func autocompleteSuggestions(input string, cursor int, workspaceRoot string, models []string, attached []chat.ContextFile) ([]string, int, int) {
	segment, start, end, ok := activeCompletionSegment(input, cursor)
	if !ok {
		return nil, 0, 0
	}

	trimmedLeft := strings.TrimLeft(segment, " ")
	fields := strings.Fields(trimmedLeft)
	if len(fields) == 0 {
		return nil, start, end
	}

	command := fields[0]
	if trimmedLeft == "/model" || trimmedLeft == "/model " {
		return completeValues("/model ", "/model", models), start, end
	}
	if trimmedLeft == "/add" || trimmedLeft == "/add " {
		return completeWorkspaceFiles("/add ", "/add", workspaceRoot), start, end
	}
	if trimmedLeft == "/drop" || trimmedLeft == "/drop " {
		paths := make([]string, 0, len(attached))
		for _, file := range attached {
			paths = append(paths, file.Path)
		}
		return completeValues("/drop ", "/drop", paths), start, end
	}

	if len(fields) == 1 && !strings.HasSuffix(trimmedLeft, " ") {
		return completeCommands(trimmedLeft), start, end
	}

	switch command {
	case "/model":
		return completeValues(trimmedLeft, "/model", models), start, end
	case "/add":
		return completeWorkspaceFiles(trimmedLeft, "/add", workspaceRoot), start, end
	case "/drop":
		paths := make([]string, 0, len(attached))
		for _, file := range attached {
			paths = append(paths, file.Path)
		}
		return completeValues(trimmedLeft, "/drop", paths), start, end
	default:
		return nil, start, end
	}
}

func completeCommands(input string) []string {
	matches := make([]string, 0, len(supportedSlashCommands))
	for _, cmd := range supportedSlashCommands {
		if strings.HasPrefix(cmd.Name, input) {
			matches = append(matches, cmd.Name)
		}
	}
	sort.Strings(matches)
	return matches
}

func slashCommandDescription(command string) string {
	for _, cmd := range supportedSlashCommands {
		if cmd.Name == command {
			return cmd.Description
		}
	}
	return ""
}

func slashCommandNeedsValue(command string) bool {
	for _, cmd := range supportedSlashCommands {
		if cmd.Name == command {
			return cmd.NeedsValue
		}
	}
	return false
}

func completeValues(input string, command string, values []string) []string {
	prefix := strings.TrimSpace(strings.TrimPrefix(input, command))
	matches := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			matches = append(matches, command+" "+value)
		}
	}
	sort.Strings(matches)
	return matches
}

func completeWorkspaceFiles(input string, command string, workspaceRoot string) []string {
	prefix := strings.TrimSpace(strings.TrimPrefix(input, command))
	paths, err := listWorkspaceEntries(workspaceRoot)
	if err != nil {
		return nil
	}

	matches := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasPrefix(path, prefix) {
			matches = append(matches, command+" "+path)
		}
	}
	sort.Strings(matches)
	return matches
}

func listWorkspaceEntries(workspaceRoot string) ([]string, error) {
	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}

	var entries []string
	err = filepath.WalkDir(rootAbs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			if path != rootAbs {
				rel, err := filepath.Rel(rootAbs, path)
				if err == nil {
					entries = append(entries, filepath.ToSlash(rel)+"/")
				}
			}
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() > 256*1024 {
			return nil
		}

		rel, err := filepath.Rel(rootAbs, path)
		if err != nil {
			return nil
		}
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(entries)
	return entries, nil
}

func nextCompletion(current string, base string, candidates []string) string {
	if len(candidates) == 0 {
		return current
	}

	if current == base {
		return candidates[0]
	}

	for i, candidate := range candidates {
		if candidate == current {
			return candidates[(i+1)%len(candidates)]
		}
	}

	return candidates[0]
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func completionEqualsSegment(completion string, segment string) bool {
	if completion == segment {
		return true
	}
	if slashCommandNeedsValue(completion) && completion+" " == segment {
		return true
	}
	return false
}
