package app

import (
	"encoding/json"
	"errors"
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
var errNoProposedFileEdits = errors.New("no `gopilot-file` blocks found in the last response")

type slashCommandSpec struct {
	Name        string
	Description string
	NeedsValue  bool
}

var supportedSlashCommands = []slashCommandSpec{
	{Name: "/add", Description: "Attach a file as context", NeedsValue: true},
	{Name: "/apply", Description: "Apply proposed file edits"},
	{Name: "/clear", Description: "Reset conversation"},
	{Name: "/codebase", Description: "Attach entire working directory"},
	{Name: "/copy", Description: "Copy last response to clipboard"},
	{Name: "/delete", Description: "Delete a saved session", NeedsValue: true},
	{Name: "/drop", Description: "Remove an attached file", NeedsValue: true},
	{Name: "/files", Description: "List attached files"},
	{Name: "/help", Description: "Show available commands"},
	{Name: "/load", Description: "Load a saved session", NeedsValue: true},
	{Name: "/model", Description: "Select a model", NeedsValue: true},
	{Name: "/new", Description: "Start a new session"},
	{Name: "/plain", Description: "Show last response as plain text"},
	{Name: "/sessions", Description: "List saved sessions"},
	{Name: "/undo", Description: "Revert last applied changes", NeedsValue: true},
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

	if strings.Contains(text, "RESOURCE_EXHAUSTED") || strings.Contains(text, "No capacity available") {
		return fmt.Sprintf("API error: %s\n\nThis model may be temporarily at capacity. Use `Ctrl+N` or `/model` to switch to a different model (e.g. a flash variant).", text)
	}

	if strings.Contains(text, "429") {
		return fmt.Sprintf("API error: %s\n\nRate limit hit. Please wait a moment or switch models with `/model`.", text)
	}

	if strings.Contains(text, "503") || strings.Contains(text, "UNAVAILABLE") {
		return fmt.Sprintf("API error: %s\n\nThe server is temporarily unavailable. Retrying automatically.", text)
	}

	return fmt.Sprintf("Request failed: %s", text)
}

func quotaRetryDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}

	text := strings.TrimSpace(err.Error())

	if strings.Contains(text, "RESOURCE_EXHAUSTED") || strings.Contains(text, "No capacity available") {
		return 10 * time.Second, true
	}

	if strings.Contains(text, "429") {
		return 8 * time.Second, true
	}

	if strings.Contains(text, "503") || strings.Contains(text, "UNAVAILABLE") {
		return 4 * time.Second, true
	}

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
	blocks := extractAllFencedCodeBlocks(text)
	if len(blocks) == 0 {
		return ""
	}
	return blocks[0]
}

func extractAllFencedCodeBlocks(text string) []string {
	lines := strings.Split(text, "\n")
	var blocks []string
	var codeLines []string
	inCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				blocks = append(blocks, strings.Join(codeLines, "\n"))
				inCode = false
				continue
			}
			// Skip gopilot-file blocks
			if strings.HasPrefix(strings.TrimPrefix(trimmed, "```"), "gopilot-file") {
				inCode = true
				codeLines = nil
				continue
			}
			inCode = true
			codeLines = nil
			continue
		}
		if inCode {
			codeLines = append(codeLines, line)
		}
	}
	return blocks
}

func formatHelpText() string {
	var lines []string
	lines = append(lines, "Available Commands")
	lines = append(lines, "")
	for _, cmd := range supportedSlashCommands {
		if cmd.NeedsValue {
			lines = append(lines, fmt.Sprintf("  %-16s %s", cmd.Name+" <arg>", cmd.Description))
		} else {
			lines = append(lines, fmt.Sprintf("  %-16s %s", cmd.Name, cmd.Description))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "Keyboard Shortcuts")
	lines = append(lines, "")
	lines = append(lines, "  Enter          Send prompt")
	lines = append(lines, "  Tab            Autocomplete")
	lines = append(lines, "  Ctrl+N/P       Cycle models")
	lines = append(lines, "  Esc            Quit")
	return strings.Join(lines, "\n")
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

	if isBinaryFile(relPath, data) {
		return chat.ContextFile{}, fmt.Errorf("%s appears to be a binary file and cannot be used as context", relPath)
	}

	return chat.ContextFile{
		Path:     relPath,
		Language: detectLanguage(relPath),
		Content:  string(data),
	}, nil
}

var binaryExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".ico": true, ".webp": true,
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true, ".mkv": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true, ".7z": true, ".rar": true,
	".wasm": true, ".o": true, ".a": true, ".lib": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ttf": true, ".otf": true, ".woff": true, ".woff2": true, ".eot": true,
	".class": true, ".pyc": true, ".pyo": true,
	".sqlite": true, ".db": true,
}

func isBinaryFile(path string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if binaryExtensions[ext] {
		return true
	}
	checkLen := len(data)
	if checkLen > 512 {
		checkLen = 512
	}
	for _, b := range data[:checkLen] {
		if b == 0 {
			return true
		}
	}
	return false
}

var skipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"node_modules": true, "vendor": true, ".venv": true, "venv": true,
	"__pycache__": true, ".mypy_cache": true,
	".next": true, ".nuxt": true, "dist": true, "build": true, "target": true,
	".gopilot": true, ".gopilot-backup": true,
	".idea": true, ".vscode": true,
}

func shouldSkipDir(name string) bool {
	if skipDirs[name] {
		return true
	}
	// Skip hidden directories (dotfiles) except "." itself
	if strings.HasPrefix(name, ".") && len(name) > 1 {
		return true
	}
	return false
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
			if shouldSkipDir(d.Name()) {
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

type appliedFileEdit struct {
	Path   string
	Action string
}

type undoEntry struct {
	Path         string `json:"path"`
	ExistedBefore bool   `json:"existed_before"`
	PreviousContent string `json:"previous_content"`
}

type undoBatch struct {
	AppliedAt time.Time   `json:"applied_at"`
	Entries   []undoEntry `json:"entries"`
}

func parseProposedFileEdits(text string) ([]proposedFileEdit, error) {
	matches := gopilotFileBlockPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, errNoProposedFileEdits
	}

	edits := make([]proposedFileEdit, 0, len(matches))
	seen := make(map[string]string, len(matches))
	for _, match := range matches {
		path := strings.TrimSpace(match[1])
		if path == "" {
			return nil, fmt.Errorf("found an edit block with an empty path")
		}

		content := strings.ReplaceAll(match[2], "\r\n", "\n")
		if existing, ok := seen[path]; ok {
			if existing != content {
				return nil, fmt.Errorf("found multiple edit blocks for %s with different contents", path)
			}
			continue
		}
		seen[path] = content
		edits = append(edits, proposedFileEdit{
			Path:    path,
			Content: content,
		})
	}

	return edits, nil
}

func applyProposedFileEdits(workspaceRoot string, text string) ([]appliedFileEdit, undoBatch, error) {
	edits, err := parseProposedFileEdits(text)
	if err != nil {
		return nil, undoBatch{}, err
	}

	applied := make([]appliedFileEdit, 0, len(edits))
	undo := undoBatch{AppliedAt: time.Now(), Entries: make([]undoEntry, 0, len(edits))}
	for _, edit := range edits {
		relPath, absPath, err := normalizeWorkspacePath(workspaceRoot, edit.Path)
		if err != nil {
			return nil, undoBatch{}, err
		}

		action := "created"
		var previousContent string
		existedBefore := false
		if existing, err := os.ReadFile(absPath); err == nil {
			existedBefore = true
			previousContent = string(existing)
			if string(existing) == edit.Content {
				applied = append(applied, appliedFileEdit{Path: relPath, Action: "unchanged"})
				continue
			}
			action = "updated"
		} else if !os.IsNotExist(err) {
			return nil, undoBatch{}, fmt.Errorf("read existing %s: %w", relPath, err)
		}

		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, undoBatch{}, fmt.Errorf("create parent directory for %s: %w", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(edit.Content), 0o644); err != nil {
			return nil, undoBatch{}, fmt.Errorf("write %s: %w", relPath, err)
		}
		applied = append(applied, appliedFileEdit{Path: relPath, Action: action})
		undo.Entries = append(undo.Entries, undoEntry{
			Path:            relPath,
			ExistedBefore:   existedBefore,
			PreviousContent: previousContent,
		})
	}

	sort.Slice(applied, func(i, j int) bool {
		return applied[i].Path < applied[j].Path
	})
	sort.Slice(undo.Entries, func(i, j int) bool {
		return undo.Entries[i].Path < undo.Entries[j].Path
	})
	return applied, undo, nil
}

func revertUndoBatch(workspaceRoot string, batch undoBatch) ([]string, error) {
	reverted := make([]string, 0, len(batch.Entries))
	for i := len(batch.Entries) - 1; i >= 0; i-- {
		entry := batch.Entries[i]
		relPath, absPath, err := normalizeWorkspacePath(workspaceRoot, entry.Path)
		if err != nil {
			return nil, err
		}
		if entry.ExistedBefore {
			if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
				return nil, fmt.Errorf("create parent directory for %s: %w", relPath, err)
			}
			if err := os.WriteFile(absPath, []byte(entry.PreviousContent), 0o644); err != nil {
				return nil, fmt.Errorf("restore %s: %w", relPath, err)
			}
		} else {
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("remove %s: %w", relPath, err)
			}
		}
		reverted = append(reverted, relPath)
	}
	sort.Strings(reverted)
	return reverted, nil
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
	if trimmedLeft == "/load" || trimmedLeft == "/load " {
		summaries, err := listStoredSessions()
		if err != nil {
			return nil, start, end
		}
		values := make([]string, 0, len(summaries)+1)
		values = append(values, "latest")
		for _, session := range summaries {
			values = append(values, session.ID)
		}
		return completeValues("/load ", "/load", values), start, end
	}
	if trimmedLeft == "/undo" || trimmedLeft == "/undo " {
		return completeValues("/undo ", "/undo", []string{"session"}), start, end
	}
	if trimmedLeft == "/delete" || trimmedLeft == "/delete " {
		summaries, err := listStoredSessions()
		if err != nil {
			return nil, start, end
		}
		values := make([]string, 0, len(summaries)+1)
		values = append(values, "all")
		for _, session := range summaries {
			values = append(values, session.ID)
		}
		return completeValues("/delete ", "/delete", values), start, end
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
	case "/load":
		summaries, err := listStoredSessions()
		if err != nil {
			return nil, start, end
		}
		values := make([]string, 0, len(summaries)+1)
		values = append(values, "latest")
		for _, session := range summaries {
			values = append(values, session.ID)
		}
		return completeValues(trimmedLeft, "/load", values), start, end
	case "/undo":
		return completeValues(trimmedLeft, "/undo", []string{"session"}), start, end
	case "/delete":
		summaries, err := listStoredSessions()
		if err != nil {
			return nil, start, end
		}
		values := make([]string, 0, len(summaries)+1)
		values = append(values, "all")
		for _, session := range summaries {
			values = append(values, session.ID)
		}
		return completeValues(trimmedLeft, "/delete", values), start, end
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

func splitInlineSlashCommands(input string) ([]string, string) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil, ""
	}

	var commands []string
	var promptWords []string

	consumeArg := func(i int, command string) (string, int, bool) {
		if i+1 >= len(fields) {
			return command, i, false
		}
		next := fields[i+1]
		if strings.HasPrefix(next, "/") {
			return command, i, false
		}
		return command + " " + next, i + 1, true
	}

	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if !strings.HasPrefix(field, "/") {
			promptWords = append(promptWords, field)
			continue
		}

		switch field {
		case "/codebase", "/files", "/apply", "/new", "/clear", "/plain", "/sessions", "/help":
			commands = append(commands, field)
		case "/add", "/drop", "/load", "/model", "/delete":
			command, nextIndex, ok := consumeArg(i, field)
			if !ok {
				promptWords = append(promptWords, field)
				continue
			}
			commands = append(commands, command)
			i = nextIndex
		case "/undo", "/copy":
			command, nextIndex, ok := consumeArg(i, field)
			if ok {
				commands = append(commands, command)
				i = nextIndex
			} else {
				commands = append(commands, field)
			}
		default:
			promptWords = append(promptWords, field)
		}
	}

	return commands, strings.Join(promptWords, " ")
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
			if shouldSkipDir(name) {
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
