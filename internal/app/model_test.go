package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	t.Setenv("XDG_CONFIG_HOME", configDir)

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
