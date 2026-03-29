package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
	"github.com/JakobAIOdev/GoPilot/internal/gemini"
)

const windowTitle = "GoPilot"

var initialSplash = strings.Join([]string{
	"   ______      ____  _ __      __ ",
	"  / ____/___  / __ \\(_) /___  / /_",
	" / / __/ __ \\/ /_/ / / / __ \\/ __/",
	"/ /_/ / /_/ / ____/ / / /_/ / /_  ",
	"\\____/\\____/_/   /_/_/\\____/\\__/  ",
	"",
	"Ready for prompts.",
}, "\n")

func pendingAssistantMessage(modelName string, attachedCount int) string {
	if attachedCount > 0 {
		return fmt.Sprintf("`%s`\nThinking...\nUsing %d attached file(s)", modelName, attachedCount)
	}
	return fmt.Sprintf("`%s`\nThinking...", modelName)
}

func isPendingAssistantMessage(text string) bool {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		return false
	}
	first := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(first, "`") || !strings.HasSuffix(first, "`") {
		return false
	}
	switch strings.TrimSpace(lines[1]) {
	case "Thinking...", "Authenticating...", "Loading project...", "Preparing context...", "Sending request...", "Retrying request...":
		return true
	default:
		return false
	}
}

func formatAssistantStatus(modelName string, status string, attachedCount int) string {
	lines := []string{fmt.Sprintf("`%s`", modelName), strings.TrimSpace(status)}
	if attachedCount > 0 {
		lines = append(lines, fmt.Sprintf("Using %d attached file(s)", attachedCount))
	}
	return strings.Join(lines, "\n")
}

type streamMsg struct {
	event chat.StreamEvent
}

type streamStartedMsg struct {
	stream <-chan chat.StreamEvent
	cancel context.CancelFunc
	err    error
}

type streamFlushMsg struct{}

type retryStreamMsg struct{}

type model struct {
	input          textinput.Model
	viewport       viewport.Model
	messages       []chat.Message
	sharedHistory  []chat.Message
	sessionID      string
	sessionCreated time.Time
	sessionSaveErr string
	backend        chat.Backend
	models         []string
	modelIndex     int
	workspaceRoot  string
	contextFiles   []chat.ContextFile
	ready          bool
	waiting        bool
	choosingModel  bool
	plainView      bool
	modelMenuIndex int
	width          int
	height         int
	panelW         int
	stream         <-chan chat.StreamEvent
	cancelStream   context.CancelFunc
	streamBuffer   strings.Builder
	flushScheduled bool
	pendingRequest chat.Request
	retryCount     int
	completionBase string
	completions    []string
	completionIndex int
	completionStart int
	completionEnd   int
}

func newModel() model {
	ti := textinput.New()
	ti.Prompt = "ask > "
	ti.Placeholder = "Type a prompt and press Enter"
	ti.Focus()
	ti.CharLimit = 400
	tiStyles := ti.Styles()
	tiStyles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#DADADA")).Bold(true)
	tiStyles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F5F5"))
	tiStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070"))
	ti.SetStyles(tiStyles)

	vp := viewport.New()
	vp.Style = lipgloss.NewStyle()
	vp.SetHeight(10)
	vp.SetWidth(80)
	vp.SoftWrap = true

	m := model{
		input:         ti,
		viewport:      vp,
		backend:       gemini.NewBackend(),
		models:        availableModels(),
		modelIndex:    0,
		sessionID:     newSessionID(),
		sessionCreated: time.Now(),
		workspaceRoot: currentWorkingDir(),
		messages: []chat.Message{
			{From: "GoPilot", Content: initialSplash},
		},
	}
	m.saveSession()

	m.syncViewport()
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) currentModel() string {
	if len(m.models) == 0 {
		return ""
	}
	if m.modelIndex < 0 || m.modelIndex >= len(m.models) {
		return m.models[0]
	}
	return m.models[m.modelIndex]
}

func (m model) contextFilesLen() int {
	return len(m.contextFiles)
}

func (m model) completionStatus() string {
	if len(m.completions) > 0 {
		return fmt.Sprintf("Tab autocomplete (%d)", len(m.completions))
	}
	return "Tab autocomplete"
}

func (m *model) resetCompletions() {
	m.completionBase = ""
	m.completions = nil
	m.completionIndex = 0
	m.completionStart = 0
	m.completionEnd = 0
}

func (m *model) refreshCompletions() {
	current := m.input.Value()
	cursor := m.input.Position()
	completions, start, end := autocompleteSuggestions(current, cursor, m.workspaceRoot, m.models, m.contextFiles)
	segment := ""
	if start >= 0 && start <= cursor && cursor <= len(current) {
		segment = current[start:cursor]
		m.completionBase = segment
	} else {
		m.completionBase = ""
	}
	m.completionStart = start
	m.completionEnd = end
	if len(completions) > 0 && containsString(completions, strings.TrimSpace(segment)) {
		completions = nil
	}
	if len(completions) > 0 {
		for _, completion := range completions {
			if completionEqualsSegment(completion, segment) {
				completions = nil
				break
			}
		}
	}
	m.completions = completions
	if len(m.completions) == 0 {
		m.completionIndex = 0
		return
	}
	if m.completionIndex < 0 || m.completionIndex >= len(m.completions) {
		m.completionIndex = 0
	}
}

func (m *model) hasCompletions() bool {
	return len(m.completions) > 0
}

func (m *model) applySelectedCompletion() bool {
	if !m.hasCompletions() {
		return false
	}
	if m.completionIndex < 0 || m.completionIndex >= len(m.completions) {
		m.completionIndex = 0
	}
	selected := m.completions[m.completionIndex]
	if slashCommandNeedsValue(selected) {
		selected += " "
	}
	current := m.input.Value()
	if m.completionStart < 0 || m.completionStart > len(current) {
		m.completionStart = 0
	}
	if m.completionEnd < m.completionStart || m.completionEnd > len(current) {
		m.completionEnd = len(current)
	}
	next := current[:m.completionStart] + selected + current[m.completionEnd:]
	m.input.SetValue(next)
	m.input.SetCursor(m.completionStart + len(selected))
	m.refreshCompletions()
	return true
}

func (m *model) cycleCompletion(step int) {
	if !m.hasCompletions() {
		return
	}
	m.completionIndex = (m.completionIndex + step + len(m.completions)) % len(m.completions)
}

func (m *model) shouldApplyCompletionOnEnter() bool {
	if !m.hasCompletions() {
		return false
	}
	if m.completionIndex < 0 || m.completionIndex >= len(m.completions) {
		return false
	}

	current := m.input.Value()
	cursor := m.input.Position()
	if m.completionStart < 0 || m.completionStart > cursor || cursor > len(current) {
		return false
	}

	segment := current[m.completionStart:cursor]
	selected := m.completions[m.completionIndex]
	return !completionEqualsSegment(selected, segment)
}

func (m *model) cycleModel(step int) {
	if len(m.models) == 0 {
		return
	}
	m.modelIndex = (m.modelIndex + step + len(m.models)) % len(m.models)
}

func (m *model) openModelMenu() {
	m.choosingModel = true
	m.modelMenuIndex = m.modelIndex
}

func (m *model) closeModelMenu() {
	m.choosingModel = false
	m.input.SetValue("")
}

func (m *model) togglePlainView() {
	m.plainView = !m.plainView
}

func (m *model) cycleModelMenu(step int) {
	if len(m.models) == 0 {
		return
	}
	m.modelMenuIndex = (m.modelMenuIndex + step + len(m.models)) % len(m.models)
}

func (m *model) resetConversation() {
	if m.cancelStream != nil {
		m.cancelStream()
		m.cancelStream = nil
	}
	m.stream = nil
	m.waiting = false
	m.sharedHistory = nil
	m.pendingRequest = chat.Request{}
	m.retryCount = 0
	m.messages = []chat.Message{
		{From: "GoPilot", Content: fmt.Sprintf("Conversation cleared. %d file(s) still attached.", len(m.contextFiles))},
	}
	m.saveSession()
}

func (m *model) applyStoredSession(session storedSession) {
	m.sessionID = session.ID
	m.sessionCreated = session.CreatedAt
	m.messages = cloneMessages(session.Messages)
	m.sharedHistory = cloneMessages(session.SharedHistory)
	m.contextFiles = cloneContextFiles(session.ContextFiles)
	m.pendingRequest = chat.Request{}
	m.retryCount = 0
	m.waiting = false
	m.stream = nil
	m.cancelStream = nil
	m.streamBuffer.Reset()
	m.flushScheduled = false
	if len(m.messages) == 0 {
		m.messages = []chat.Message{{From: "GoPilot", Content: initialSplash}}
	}
	for i, modelName := range m.models {
		if modelName == session.Model {
			m.modelIndex = i
			break
		}
	}
}

func (m model) storedSession() storedSession {
	messages := cloneMessages(m.messages)
	if m.waiting && len(messages) > 0 && messages[len(messages)-1].From == "GoPilot" && isPendingAssistantMessage(messages[len(messages)-1].Content) {
		messages = messages[:len(messages)-1]
	}
	return storedSession{
		ID:            m.sessionID,
		Title:         deriveSessionTitle(m.messages),
		CreatedAt:     m.sessionCreated,
		WorkspaceRoot: m.workspaceRoot,
		Model:         m.currentModel(),
		Messages:      messages,
		SharedHistory: cloneMessages(m.sharedHistory),
		ContextFiles:  cloneContextFiles(m.contextFiles),
	}
}

func (m *model) saveSession() {
	if strings.TrimSpace(m.sessionID) == "" {
		m.sessionID = newSessionID()
	}
	if m.sessionCreated.IsZero() {
		m.sessionCreated = time.Now()
	}
	if err := saveStoredSession(m.storedSession()); err != nil {
		m.sessionSaveErr = err.Error()
		return
	}
	m.sessionSaveErr = ""
}

func (m *model) startNewSession() {
	if m.cancelStream != nil {
		m.cancelStream()
		m.cancelStream = nil
	}
	m.stream = nil
	m.waiting = false
	m.sharedHistory = nil
	m.pendingRequest = chat.Request{}
	m.retryCount = 0
	m.contextFiles = nil
	m.messages = []chat.Message{
		{From: "GoPilot", Content: initialSplash},
	}
	m.sessionID = newSessionID()
	m.sessionCreated = time.Now()
	m.resetCompletions()
	m.input.SetValue("")
	m.saveSession()
}

func (m *model) loadSessionCommand(target string) error {
	var session storedSession
	var err error
	if strings.TrimSpace(target) == "" || strings.EqualFold(strings.TrimSpace(target), "latest") {
		session, err = loadLatestStoredSession()
	} else {
		session, err = loadStoredSession(strings.TrimSpace(target))
	}
	if err != nil {
		return err
	}
	sessionRoot := strings.TrimSpace(session.WorkspaceRoot)
	m.applyStoredSession(session)
	currentRoot := currentWorkingDir()
	if currentRoot != "" {
		m.workspaceRoot = currentRoot
	}
	if sessionRoot != "" && currentRoot != "" && sessionRoot != currentRoot {
		m.contextFiles = nil
		m.messages = append(m.messages, chat.Message{
			From:    "GoPilot",
			Content: fmt.Sprintf("Session `%s` was created in `%s`. Keeping the current workspace `%s`, so attached files were cleared.", session.ID, sessionRoot, currentRoot),
		})
	}
	m.resetCompletions()
	m.input.SetValue("")
	m.syncViewport()
	return nil
}

func (m *model) replaceLastAssistantMessage(text string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].From == "GoPilot" {
			m.messages[i].Content = text
			return
		}
	}

	m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: text})
}

func (m *model) appendToLastAssistantMessage(text string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].From == "GoPilot" {
			m.messages[i].Content += text
			return
		}
	}

	m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: text})
}

func (m *model) flushPendingStreamText() {
	if m.streamBuffer.Len() == 0 {
		return
	}

	last := lastAssistantMessage(m.messages)
	if isPendingAssistantMessage(last) {
		m.replaceLastAssistantMessage(m.streamBuffer.String())
	} else {
		m.appendToLastAssistantMessage(m.streamBuffer.String())
	}
	m.streamBuffer.Reset()
	m.syncViewport()
	m.viewport.GotoBottom()
}

func (m *model) submitPrompt(userText string) {
	m.messages = append(m.messages, chat.Message{From: "User", Content: userText})
	m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: pendingAssistantMessage(m.currentModel(), len(m.contextFiles))})
	m.sharedHistory = append(m.sharedHistory, chat.Message{From: "User", Content: userText})
	m.waiting = true
	m.streamBuffer.Reset()
	m.flushScheduled = false
	m.pendingRequest = chat.Request{
		Model:          m.currentModel(),
		Messages:       cloneMessages(m.sharedHistory),
		WorkspaceRoot:  m.workspaceRoot,
		ContextFiles:   cloneContextFiles(m.contextFiles),
		AllowFileEdits: true,
	}
	m.retryCount = 0
	m.resetCompletions()
	m.input.SetValue("")
	m.saveSession()
}

func (m *model) handleSlashCommand(input string) tea.Cmd {
	if !strings.HasPrefix(input, "/") {
		return nil
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "/add":
		pathArg, trailingPrompt := splitFirstArgument(strings.TrimSpace(strings.TrimPrefix(input, "/add")))
		if pathArg == "" {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: "Usage: /add <file>"})
			return nil
		}

		files, err := loadContextTargets(m.workspaceRoot, pathArg)
		if err != nil {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Attach failed: %v", err)})
			return nil
		}

		addedCount := 0
		updatedCount := 0
		for _, file := range files {
			replaced := false
			for i, existing := range m.contextFiles {
				if existing.Path == file.Path {
					m.contextFiles[i] = file
					updatedCount++
					replaced = true
					break
				}
			}
			if replaced {
				continue
			}
			m.contextFiles = append(m.contextFiles, file)
			addedCount++
		}

		switch {
		case len(files) == 1 && addedCount == 1:
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Attached `%s` as file context.", files[0].Path)})
		case len(files) == 1 && updatedCount == 1:
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Updated attached file `%s`.", files[0].Path)})
		default:
			messages := fmt.Sprintf("Attached %d file(s) from `%s`.", addedCount, pathArg)
			if updatedCount > 0 {
				messages = fmt.Sprintf("%s Updated %d existing attachment(s).", messages, updatedCount)
			}
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: messages})
		}
		m.saveSession()

		if trailingPrompt != "" {
			m.submitPrompt(trailingPrompt)
			return startStreamCmd(m.backend, cloneRequest(m.pendingRequest))
		}
		return nil
	case "/codebase":
		files, err := loadContextTargets(m.workspaceRoot, ".")
		if err != nil {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Attach failed: %v", err)})
			return nil
		}

		addedCount := 0
		updatedCount := 0
		for _, file := range files {
			replaced := false
			for i, existing := range m.contextFiles {
				if existing.Path == file.Path {
					m.contextFiles[i] = file
					updatedCount++
					replaced = true
					break
				}
			}
			if replaced {
				continue
			}
			m.contextFiles = append(m.contextFiles, file)
			addedCount++
		}

		message := fmt.Sprintf("Attached %d file(s) from the current codebase.", addedCount)
		if updatedCount > 0 {
			message = fmt.Sprintf("%s Updated %d existing attachment(s).", message, updatedCount)
		}
		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: message})
		m.saveSession()
		return nil
	case "/drop":
		pathArg, _ := splitFirstArgument(strings.TrimSpace(strings.TrimPrefix(input, "/drop")))
		if pathArg == "" {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: "Usage: /drop <file>"})
			return nil
		}

		relPath, _, err := normalizeWorkspacePath(m.workspaceRoot, pathArg)
		if err != nil {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Drop failed: %v", err)})
			return nil
		}

		next := m.contextFiles[:0]
		found := false
		for _, file := range m.contextFiles {
			if file.Path == relPath {
				found = true
				continue
			}
			next = append(next, file)
		}
		m.contextFiles = next
		if !found {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("`%s` is not attached.", relPath)})
			return nil
		}

		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Removed `%s` from file context.", relPath)})
		m.saveSession()
		return nil
	case "/files":
		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: contextFileList(m.contextFiles)})
		m.saveSession()
		return nil
	case "/apply":
		last := lastAssistantMessage(m.messages)
		if last == "" {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: "Nothing to apply yet."})
			return nil
		}

		written, err := applyProposedFileEdits(m.workspaceRoot, last)
		if err != nil {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Apply failed: %v", err)})
			return nil
		}

		for i, file := range m.contextFiles {
			for _, path := range written {
				if file.Path != path {
					continue
				}
				updated, loadErr := loadContextFile(m.workspaceRoot, path)
				if loadErr == nil {
					m.contextFiles[i] = updated
				}
				break
			}
		}

		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Applied %d file(s):\n- %s", len(written), strings.Join(written, "\n- "))})
		m.saveSession()
		return nil
	case "/load":
		target := ""
		if len(fields) > 1 {
			target = fields[1]
		}
		if err := m.loadSessionCommand(target); err != nil {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Load failed: %v", err)})
			m.saveSession()
			return nil
		}
		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Loaded session `%s`.", m.sessionID)})
		m.saveSession()
		return nil
	case "/model":
		if len(fields) == 1 {
			m.openModelMenu()
			return nil
		}

		selected := strings.TrimSpace(fields[1])
		for i, modelName := range m.models {
			if modelName == selected {
				m.modelIndex = i
				m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Active model switched to %s.", m.currentModel())})
				m.saveSession()
				return nil
			}
		}

		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Unknown model %q.", selected)})
		m.saveSession()
		return nil
	case "/new":
		m.startNewSession()
		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Started new session `%s`.", m.sessionID)})
		m.saveSession()
		return nil
	case "/clear":
		m.resetConversation()
		return nil
	case "/copy":
		last := lastAssistantMessage(m.messages)
		if last == "" {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: "Nothing to copy yet."})
			return nil
		}

		copyText := last
		label := "last response"
		if len(fields) > 1 && strings.TrimSpace(fields[1]) == "code" {
			code := extractFirstFencedCodeBlock(last)
			if code == "" {
				m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: "No fenced code block found in the last response."})
				return nil
			}
			copyText = code
			label = "first code block"
		}

		if err := clipboard.WriteAll(copyText); err != nil {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Copy failed: %v", err)})
			return nil
		}

		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Copied %s to clipboard.", label)})
		m.saveSession()
		return nil
	case "/plain":
		if lastAssistantMessage(m.messages) == "" {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: "Nothing to show in plain view yet."})
			m.saveSession()
			return nil
		}
		m.togglePlainView()
		return nil
	case "/sessions":
		summaries, err := listStoredSessions()
		if err != nil {
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Sessions failed: %v", err)})
			m.saveSession()
			return nil
		}
		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: formatSessionList(summaries)})
		m.saveSession()
		return nil
	default:
		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Unknown command %q.", fields[0])})
		m.saveSession()
		return nil
	}
}

func currentWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
