package main

import "testing"

func TestParseArgsEmpty(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.showVersion {
		t.Fatal("did not expect showVersion")
	}
	if opts.loadSessionID != "" {
		t.Fatalf("expected empty loadSessionID, got %q", opts.loadSessionID)
	}
}

func TestParseArgsVersion(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs([]string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.showVersion {
		t.Fatal("expected showVersion")
	}
}

func TestParseArgsLoad(t *testing.T) {
	t.Parallel()

	opts, err := parseArgs([]string{"--load", "20260331-182153-c90a194e"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.loadSessionID != "20260331-182153-c90a194e" {
		t.Fatalf("unexpected loadSessionID: %q", opts.loadSessionID)
	}
}

func TestParseArgsLoadMissingID(t *testing.T) {
	t.Parallel()

	_, err := parseArgs([]string{"--load"})
	if err == nil {
		t.Fatal("expected error")
	}
}
