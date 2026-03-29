package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProposedFileEditsRejectsConflictingDuplicatePaths(t *testing.T) {
	t.Parallel()

	text := "```gopilot-file path=a.txt\none\n```\n```gopilot-file path=a.txt\ntwo\n```"
	_, err := parseProposedFileEdits(text)
	if err == nil {
		t.Fatal("expected conflicting duplicate path to fail")
	}
}

func TestParseProposedFileEditsDeduplicatesIdenticalBlocks(t *testing.T) {
	t.Parallel()

	text := "```gopilot-file path=a.txt\nsame\n```\n```gopilot-file path=a.txt\nsame\n```"
	edits, err := parseProposedFileEdits(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("expected one deduplicated edit, got %d", len(edits))
	}
}

func TestApplyProposedFileEditsClassifiesResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	updatedPath := filepath.Join(root, "updated.txt")
	unchangedPath := filepath.Join(root, "unchanged.txt")
	if err := os.WriteFile(updatedPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unchangedPath, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	text := "" +
		"```gopilot-file path=created.txt\nnew file\n```\n" +
		"```gopilot-file path=updated.txt\nnew content\n```\n" +
		"```gopilot-file path=unchanged.txt\nsame\n```"

	results, _, err := applyProposedFileEdits(root, text)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	got := map[string]string{}
	for _, result := range results {
		got[result.Path] = result.Action
	}

	if got["created.txt"] != "created" {
		t.Fatalf("expected created.txt to be created, got %q", got["created.txt"])
	}
	if got["updated.txt"] != "updated" {
		t.Fatalf("expected updated.txt to be updated, got %q", got["updated.txt"])
	}
	if got["unchanged.txt"] != "unchanged" {
		t.Fatalf("expected unchanged.txt to be unchanged, got %q", got["unchanged.txt"])
	}
}

func TestRevertUndoBatchRestoresPreviousState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	updatedPath := filepath.Join(root, "updated.txt")
	if err := os.WriteFile(updatedPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	text := "" +
		"```gopilot-file path=created.txt\nnew file\n```\n" +
		"```gopilot-file path=updated.txt\nnew content\n```"

	_, undoBatch, err := applyProposedFileEdits(root, text)
	if err != nil {
		t.Fatal(err)
	}

	reverted, err := revertUndoBatch(root, undoBatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(reverted) != 2 {
		t.Fatalf("expected 2 reverted files, got %d", len(reverted))
	}

	if _, err := os.Stat(filepath.Join(root, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected created.txt to be removed, got err=%v", err)
	}
	data, err := os.ReadFile(updatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("expected updated.txt to be restored, got %q", string(data))
	}
}

func TestSplitInlineSlashCommands(t *testing.T) {
	t.Parallel()

	commands, prompt := splitInlineSlashCommands("kannst du bitte /codebase 3 ordner anlegen")
	if len(commands) != 1 || commands[0] != "/codebase" {
		t.Fatalf("unexpected commands: %#v", commands)
	}
	if prompt != "kannst du bitte 3 ordner anlegen" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}
