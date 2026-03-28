package app

import (
	"fmt"
	"strings"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

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
			return fmt.Sprintf("Model capacity exhausted. %s Use `Ctrl+N` or `/model` to switch to another model and try again.", text)
		}
		return fmt.Sprintf("Model capacity exhausted. %s Use `Ctrl+N` or `/model` to switch to another model and try again.", text)
	}

	if strings.Contains(text, "429") {
		return fmt.Sprintf("Request rate-limited. %s", text)
	}

	return fmt.Sprintf("Request failed. %s", text)
}
