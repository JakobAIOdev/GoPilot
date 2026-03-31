package app

import (
	"testing"
	"time"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

func TestNewModelForRunLoadDoesNotCreateExtraSession(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HOME", configDir)

	session := storedSession{
		ID:            "existing-session",
		Title:         "existing",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		WorkspaceRoot: t.TempDir(),
		Model:         "gemini-3-flash-preview",
		Messages: []chat.Message{
			{From: "User", Content: "hello"},
			{From: "GoPilot", Content: "hi"},
		},
		SharedHistory: []chat.Message{
			{From: "User", Content: "hello"},
			{From: "GoPilot", Content: "hi"},
		},
	}
	if err := saveStoredSession(session); err != nil {
		t.Fatal(err)
	}

	before, err := listStoredSessions()
	if err != nil {
		t.Fatal(err)
	}

	m, err := newModelForRun(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.sessionID != session.ID {
		t.Fatalf("expected session %q, got %q", session.ID, m.sessionID)
	}

	after, err := listStoredSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("expected no extra session file, before=%d after=%d", len(before), len(after))
	}
}
