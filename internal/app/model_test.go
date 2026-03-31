package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

func TestIsPendingAssistantMessage(t *testing.T) {
	t.Parallel()

	if !isPendingAssistantMessage("`gemini-3-flash-preview`\nThinking...") {
		t.Fatal("expected Thinking... status to be treated as pending")
	}
	if !isPendingAssistantMessage("`gemini-3-flash-preview`\nSending request...\nUsing 2 attached file(s)") {
		t.Fatal("expected request status to be treated as pending")
	}
	if isPendingAssistantMessage("`gemini-3-flash-preview`\nHere is the actual answer") {
		t.Fatal("did not expect real assistant content to be treated as pending")
	}
}

func TestStoredSessionOmitsPendingAssistantPlaceholder(t *testing.T) {
	t.Parallel()

	m := model{
		sessionID:      "session-1",
		sessionCreated: time.Now(),
		models:         []string{"gemini-3-flash-preview"},
		messages: []chat.Message{
			{From: "User", Content: "hello"},
			{From: "GoPilot", Content: pendingAssistantMessage("gemini-3-flash-preview", 0)},
		},
		sharedHistory: []chat.Message{
			{From: "User", Content: "hello"},
		},
		waiting: true,
	}

	session := m.storedSession()
	if len(session.Messages) != 1 {
		t.Fatalf("expected pending assistant message to be omitted, got %d messages", len(session.Messages))
	}
	if session.Messages[0].From != "User" {
		t.Fatalf("expected remaining message to be the user message, got %q", session.Messages[0].From)
	}
}

func TestLoadSessionCommandKeepsCurrentWorkspaceAndClearsForeignAttachments(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HOME", configDir)

	currentRoot := filepath.Join(t.TempDir(), "current")
	otherRoot := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(currentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(currentRoot); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()

	session := storedSession{
		ID:            "foreign-session",
		Title:         "foreign",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		WorkspaceRoot: otherRoot,
		Model:         "gemini-3-flash-preview",
		Messages: []chat.Message{
			{From: "User", Content: "old prompt"},
			{From: "GoPilot", Content: "old answer"},
		},
		SharedHistory: []chat.Message{
			{From: "User", Content: "old prompt"},
			{From: "GoPilot", Content: "old answer"},
		},
		ContextFiles: []chat.ContextFile{
			{Path: "go.mod", Language: "go", Content: "module example"},
		},
	}
	if err := saveStoredSession(session); err != nil {
		t.Fatal(err)
	}

	m := model{
		models:        []string{"gemini-3-flash-preview"},
		workspaceRoot: currentRoot,
	}
	if err := m.loadSessionCommand(session.ID); err != nil {
		t.Fatal(err)
	}

	if m.workspaceRoot != currentRoot {
		expectedRoot, err := filepath.EvalSymlinks(currentRoot)
		if err != nil {
			expectedRoot = currentRoot
		}
		actualRoot, err := filepath.EvalSymlinks(m.workspaceRoot)
		if err != nil {
			actualRoot = m.workspaceRoot
		}
		if actualRoot != expectedRoot {
			t.Fatalf("expected workspace root %q, got %q", expectedRoot, actualRoot)
		}
	}
	if len(m.contextFiles) != 0 {
		t.Fatalf("expected foreign context files to be cleared, got %d", len(m.contextFiles))
	}
	last := m.messages[len(m.messages)-1].Content
	if !strings.Contains(last, "Keeping the current workspace") {
		t.Fatalf("expected workspace warning message, got %q", last)
	}
}

func TestSaveSessionStoresErrorState(t *testing.T) {
	configParent := t.TempDir()
	t.Setenv("HOME", configParent)
	blockingPath := filepath.Join(configParent, "Library", "Application Support")
	if err := os.MkdirAll(filepath.Dir(blockingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blockingPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := model{
		sessionID:      "broken-session",
		sessionCreated: time.Now(),
		models:         []string{"gemini-3-flash-preview"},
		messages:       []chat.Message{{From: "GoPilot", Content: initialSplash}},
	}
	m.saveSession()

	if strings.TrimSpace(m.sessionSaveErr) == "" {
		t.Fatal("expected session save error to be captured")
	}
}

func TestStoredSessionIncludesUndoHistory(t *testing.T) {
	t.Parallel()

	m := model{
		sessionID:      "session-undo",
		sessionCreated: time.Now(),
		models:         []string{"gemini-3-flash-preview"},
		undoHistory: []undoBatch{
			{
				AppliedAt: time.Now(),
				Entries: []undoEntry{
					{Path: "a.txt", ExistedBefore: true, PreviousContent: "old"},
				},
			},
		},
		messages: []chat.Message{{From: "GoPilot", Content: initialSplash}},
	}

	session := m.storedSession()
	if len(session.UndoHistory) != 1 {
		t.Fatalf("expected undo history to be persisted, got %d entries", len(session.UndoHistory))
	}
}

func TestApplyEditsFromTextCreatesFilesAndUndoHistory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := model{
		workspaceRoot:  root,
		sessionID:      "session-apply",
		sessionCreated: time.Now(),
		models:         []string{"gemini-3-flash-preview"},
		messages:       []chat.Message{{From: "GoPilot", Content: initialSplash}},
	}

	text := "```gopilot-file path=1/info.txt\nhello\n```"
	if err := m.applyEditsFromText(text, true); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "1", "info.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
	if len(m.undoHistory) != 1 {
		t.Fatalf("expected undo history entry, got %d", len(m.undoHistory))
	}
	last := m.messages[len(m.messages)-1].Content
	if !strings.Contains(last, "Applied generated changes") {
		t.Fatalf("unexpected apply message: %q", last)
	}
}

func TestFilteredSessionSummariesUsesIDAndTitle(t *testing.T) {
	t.Parallel()

	m := model{
		sessionSummaries: []sessionSummary{
			{ID: "20260101-aaaa", Title: "alpha task"},
			{ID: "20260102-bbbb", Title: "beta task"},
		},
	}

	m.sessionFilter = "beta"
	items := m.filteredSessionSummaries()
	if len(items) != 1 || items[0].ID != "20260102-bbbb" {
		t.Fatalf("unexpected filtered sessions by title: %#v", items)
	}

	m.sessionFilter = "aaaa"
	items = m.filteredSessionSummaries()
	if len(items) != 1 || items[0].ID != "20260101-aaaa" {
		t.Fatalf("unexpected filtered sessions by id: %#v", items)
	}
}

func TestEnterSubmitsPromptWithInlineCodebaseCommand(t *testing.T) {
	t.Parallel()

	m := newModel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.workspaceRoot = root
	m.ready = true
	m.width = 120
	m.height = 40
	m.panelW = classicLayoutWidth
	m.input.SetValue("please review this repo /codebase")
	m.input.SetCursor(len(m.input.Value()))

	updatedModel, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	updated := updatedModel.(model)

	if cmd == nil {
		t.Fatal("expected stream command to be returned")
	}
	if !updated.waiting {
		t.Fatal("expected model to enter waiting state")
	}
	if len(updated.contextFiles) == 0 {
		t.Fatal("expected /codebase to attach workspace files before sending")
	}
	if got := updated.messages[len(updated.messages)-2].Content; got != "please review this repo" {
		t.Fatalf("expected cleaned user prompt, got %q", got)
	}
}

func TestRenderInputPreviewShowsLongPrompt(t *testing.T) {
	t.Parallel()

	m := newModel()
	m.input.SetValue(strings.Repeat("a", 120))

	rendered := m.renderInputPreview(80)
	if !strings.Contains(rendered, "Prompt Preview") {
		t.Fatalf("expected prompt preview to render, got %q", rendered)
	}
	if !strings.Contains(rendered, strings.Repeat("a", 40)) {
		t.Fatalf("expected preview to include prompt content, got %q", rendered)
	}
}

func TestApplyCommandShowsHelpfulMessageWithoutFileBlocks(t *testing.T) {
	t.Parallel()

	m := model{
		messages: []chat.Message{
			{From: "GoPilot", Content: initialSplash},
			{From: "GoPilot", Content: "Regular markdown answer without editable file blocks."},
		},
	}

	m.handleSlashCommand("/apply")

	last := m.messages[len(m.messages)-1].Content
	if !strings.Contains(last, "Nothing to apply from the last response") {
		t.Fatalf("unexpected apply guidance: %q", last)
	}
}

func TestProjectInstructionsStatusShowsRecognizedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "GOPILOT.md"), []byte("repo rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newModel()
	m.workspaceRoot = root
	m.refreshProjectInstructions()

	status := m.projectInstructionsStatus()
	if !strings.Contains(status, "GOPILOT.md") {
		t.Fatalf("expected GOPILOT.md path, got %q", status)
	}
}

func TestConversationMessageCountIgnoresSplashAndLocalNotices(t *testing.T) {
	t.Parallel()

	m := model{
		messages: []chat.Message{
			{From: "GoPilot", Content: initialSplash},
			{From: "GoPilot", Content: "Loaded session `abc`."},
			{From: "User", Content: "hello"},
			{From: "GoPilot", Content: "hi"},
		},
		sharedHistory: []chat.Message{
			{From: "User", Content: "hello"},
			{From: "GoPilot", Content: "hi"},
		},
	}

	if got := m.conversationMessageCount(); got != 2 {
		t.Fatalf("expected 2 chat messages, got %d", got)
	}
}
