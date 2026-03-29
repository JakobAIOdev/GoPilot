package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

var resetAfterPattern = regexp.MustCompile(`(?i)reset after ([0-9]+[smh])`)

func cloneMessages(messages []chat.Message) []chat.Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]chat.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func formatBackendError(err error) string {
	if err == nil {
		return ""
	}

	text := strings.TrimSpace(err.Error())

	if strings.Contains(text, "RESOURCE_EXHAUSTED") {
		if strings.Contains(strings.ToLower(text), "quota will reset after") {
			return fmt.Sprintf("Temporary model quota hit. %s Wait briefly, then retry, or use `Ctrl+N` or `/model` to switch models.", text)
		}
		return fmt.Sprintf("Model quota exhausted. %s Use `Ctrl+N` or `/model` to switch models and try again.", text)
	}

	if strings.Contains(text, "429") {
		return fmt.Sprintf("Request rate-limited. %s", text)
	}

	return fmt.Sprintf("Request failed. %s", text)
}

func quotaRetryDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}

	text := strings.TrimSpace(err.Error())
	if !strings.Contains(text, "RESOURCE_EXHAUSTED") {
		return 0, false
	}

	match := resetAfterPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0, false
	}

	delay, parseErr := time.ParseDuration(strings.ToLower(match[1]))
	if parseErr != nil || delay <= 0 {
		return 0, false
	}

	return delay, true
}

func formatRetryNotice(err error, delay time.Duration) string {
	text := strings.TrimSpace(err.Error())
	return fmt.Sprintf("Temporary model quota hit. %s Retrying automatically in %s.", text, delay.Round(time.Second))
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
	lines := strings.Split(text, "\n")
	var codeLines []string
	inCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				return strings.Join(codeLines, "\n")
			}

			inCode = true
			codeLines = nil
			continue
		}

		if inCode {
			codeLines = append(codeLines, line)
		}
	}

	return ""
}
