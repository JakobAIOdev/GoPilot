package gemini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

func TestDecodeAPIErrorJSONPayload(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED"}}`)
	err := decodeAPIError("429 Too Many Requests", raw)
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	if !strings.Contains(text, "Resource exhausted") || !strings.Contains(text, "RESOURCE_EXHAUSTED") {
		t.Fatalf("unexpected error text: %q", text)
	}
}

func TestDecodeAPIErrorNonJSONPayload(t *testing.T) {
	t.Parallel()

	err := decodeAPIError("502 Bad Gateway", []byte("<html>bad gateway</html>"))
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	if !strings.Contains(text, "502 Bad Gateway") || !strings.Contains(text, "bad gateway") {
		t.Fatalf("unexpected error text: %q", text)
	}
}

func TestContextPromptPrefersFileBlocksForEdits(t *testing.T) {
	t.Parallel()

	text := contextPrompt(chat.Request{
		WorkspaceRoot:  "/tmp/project",
		AllowFileEdits: true,
	})

	if !strings.Contains(text, "Include the COMPLETE file content") {
		t.Fatalf("expected file-edit guidance, got %q", text)
	}
}

func TestSystemPromptIncludesProjectInstructions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "GOPILOT.md"), []byte("Always run tests before finishing."), 0o644); err != nil {
		t.Fatal(err)
	}

	text := systemPrompt(root)
	if !strings.Contains(text, "You are GoPilot") {
		t.Fatalf("expected base system prompt, got %q", text)
	}
	if !strings.Contains(text, "Always run tests before finishing.") {
		t.Fatalf("expected GOPILOT.md instructions, got %q", text)
	}
	if !strings.Contains(text, "<gopilot_instructions>") {
		t.Fatalf("expected GOPILOT.md wrapper, got %q", text)
	}
}

func TestFindProjectInstructionsPathSearchesUpToRepoRoot(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "GOPILOT.md"), []byte("Repo rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	workspace := filepath.Join(repoRoot, "internal", "pkg")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FindProjectInstructionsPath(workspace)
	want := filepath.Join(repoRoot, "GOPILOT.md")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSystemPromptWithoutProjectInstructionsUsesBasePromptOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	text := systemPrompt(root)
	if strings.Contains(text, "<gopilot_instructions>") {
		t.Fatalf("did not expect GOPILOT.md wrapper, got %q", text)
	}
}
