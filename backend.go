package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type chatBackend interface {
	Generate(ctx context.Context, model string, prompt string) (string, error)
}

type geminiCLIBackend struct {
	command string
	timeout time.Duration
}

func newGeminiCLIBackend() chatBackend {
	return geminiCLIBackend{
		command: "gemini",
		timeout: 2 * time.Minute,
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
