package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type providerConfig struct {
	Name    string
	Backend chatBackend
	Models  []string
}

type chatBackend interface {
	Generate(ctx context.Context, model string, prompt string) (string, error)
}

type geminiCLIBackend struct {
	command string
	timeout time.Duration
}

type codexCLIBackend struct {
	command string
	timeout time.Duration
}

func availableProviders() []providerConfig {
	return []providerConfig{
		{
			Name:    "gemini",
			Backend: newGeminiCLIBackend(),
			Models: []string{
				"gemini-2.5-flash",
				"gemini-2.5-pro",
				"gemini-3-flash-preview",
				"gemini-3-pro-preview",
			},
		},
		{
			Name:    "codex",
			Backend: newCodexCLIBackend(),
			Models: []string{
				"gpt-5.4",
				"gpt-5.4-mini",
			},
		},
	}
}

func newGeminiCLIBackend() chatBackend {
	return geminiCLIBackend{
		command: "gemini",
		timeout: 2 * time.Minute,
	}
}

func newCodexCLIBackend() chatBackend {
	return codexCLIBackend{
		command: "codex",
		timeout: 3 * time.Minute,
	}
}

func (b geminiCLIBackend) Generate(ctx context.Context, model string, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt is empty")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("model is empty")
	}

	if b.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, b.command, "-m", model, "-p", prompt)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = strings.TrimSpace(stdout.String())
		}
		if errText != "" {
			return "", fmt.Errorf("gemini command failed: %s", errText)
		}
		return "", fmt.Errorf("gemini command failed: %w", err)
	}

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		text = strings.TrimSpace(stderr.String())
	}
	if text == "" {
		return "", fmt.Errorf("gemini returned an empty response")
	}

	return text, nil
}

func (b codexCLIBackend) Generate(ctx context.Context, model string, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt is empty")
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("model is empty")
	}

	if b.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.timeout)
		defer cancel()
	}

	outputFile, err := os.CreateTemp("", "gopilot-codex-*.txt")
	if err != nil {
		return "", fmt.Errorf("create codex output file: %w", err)
	}
	outputPath := outputFile.Name()
	if closeErr := outputFile.Close(); closeErr != nil {
		return "", fmt.Errorf("close codex output file: %w", closeErr)
	}
	defer os.Remove(outputPath)

	cmd := exec.CommandContext(
		ctx,
		b.command,
		"exec",
		"-m", model,
		"--skip-git-repo-check",
		"--sandbox", "read-only",
		"--output-last-message", outputPath,
		prompt,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = strings.TrimSpace(stdout.String())
		}
		if errText != "" {
			return "", fmt.Errorf("codex command failed: %s", errText)
		}
		return "", fmt.Errorf("codex command failed: %w", err)
	}

	content, err := os.ReadFile(filepath.Clean(outputPath))
	if err != nil {
		return "", fmt.Errorf("read codex output: %w", err)
	}

	text := strings.TrimSpace(string(content))
	if text == "" {
		text = strings.TrimSpace(stdout.String())
	}
	if text == "" {
		text = strings.TrimSpace(stderr.String())
	}
	if text == "" {
		return "", fmt.Errorf("codex returned an empty response")
	}

	return text, nil
}

func buildPromptWithHistory(history []message, userPrompt string) string {
	const maxHistoryMessages = 12

	trimmedHistory := history
	if len(trimmedHistory) > maxHistoryMessages {
		trimmedHistory = trimmedHistory[len(trimmedHistory)-maxHistoryMessages:]
	}

	var b strings.Builder
	b.WriteString("You are in an ongoing chat. Continue the conversation consistently.\n")
	b.WriteString("Use the conversation history below as context. Reply to the latest user message.\n\n")
	b.WriteString("Conversation history:\n")

	if len(trimmedHistory) == 0 {
		b.WriteString("(no previous messages)\n")
	} else {
		for _, msg := range trimmedHistory {
			role := "Assistant"
			if msg.from == "User" {
				role = "User"
			}
			b.WriteString(role)
			b.WriteString(": ")
			b.WriteString(msg.content)
			b.WriteString("\n")
		}
	}

	b.WriteString("\nLatest user message:\n")
	b.WriteString(userPrompt)
	b.WriteString("\n")

	return b.String()
}
