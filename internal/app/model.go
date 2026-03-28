package app

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
	"github.com/JakobAIOdev/GoPilot/internal/gemini"
)

const windowTitle = "GoPilot"

type streamMsg struct {
	event chat.StreamEvent
}

type streamStartedMsg struct {
	stream <-chan chat.StreamEvent
	cancel context.CancelFunc
	err    error
}

type streamFlushMsg struct{}

type model struct {
	input          textinput.Model
	viewport       viewport.Model
	messages       []chat.Message
	sharedHistory  []chat.Message
	backend        chat.Backend
	models         []string
	modelIndex     int
	ready          bool
	waiting        bool
	choosingModel  bool
	modelMenuIndex int
	width          int
	height         int
	panelW         int
	stream         <-chan chat.StreamEvent
	cancelStream   context.CancelFunc
	streamBuffer   strings.Builder
	flushScheduled bool
}

func newModel() model {
	ti := textinput.New()
	ti.Prompt = "ask > "
	ti.Placeholder = "Type a prompt and press Enter"
	ti.Focus()
	ti.CharLimit = 400
	tiStyles := ti.Styles()
	tiStyles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#DADADA")).Bold(true)
	tiStyles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F5F5"))
	tiStyles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070"))
	ti.SetStyles(tiStyles)

	vp := viewport.New()
	vp.Style = lipgloss.NewStyle()
	vp.SetHeight(10)
	vp.SetWidth(80)
	vp.SoftWrap = true

	m := model{
		input:      ti,
		viewport:   vp,
		backend:    gemini.NewBackend(),
		models:     availableModels(),
		modelIndex: 0,
		messages: []chat.Message{
			{From: "GoPilot", Content: "Ready for prompts. Gemini now streams over your local Google subscription sign-in from ~/.gemini."},
		},
	}

	m.syncViewport()
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) currentModel() string {
	if len(m.models) == 0 {
		return ""
	}
	if m.modelIndex < 0 || m.modelIndex >= len(m.models) {
		return m.models[0]
	}
	return m.models[m.modelIndex]
}

func (m *model) cycleModel(step int) {
	if len(m.models) == 0 {
		return
	}
	m.modelIndex = (m.modelIndex + step + len(m.models)) % len(m.models)
}

func (m *model) openModelMenu() {
	m.choosingModel = true
	m.modelMenuIndex = m.modelIndex
}

func (m *model) closeModelMenu() {
	m.choosingModel = false
	m.input.SetValue("")
}

func (m *model) cycleModelMenu(step int) {
	if len(m.models) == 0 {
		return
	}
	m.modelMenuIndex = (m.modelMenuIndex + step + len(m.models)) % len(m.models)
}

func (m *model) resetConversation() {
	if m.cancelStream != nil {
		m.cancelStream()
		m.cancelStream = nil
	}
	m.stream = nil
	m.waiting = false
	m.sharedHistory = nil
	m.messages = []chat.Message{
		{From: "GoPilot", Content: "Conversation cleared. Shared context is empty now."},
	}
}

func (m *model) replaceLastAssistantMessage(text string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].From == "GoPilot" {
			m.messages[i].Content = text
			return
		}
	}

	m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: text})
}

func (m *model) appendToLastAssistantMessage(text string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].From == "GoPilot" {
			m.messages[i].Content += text
			return
		}
	}

	m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: text})
}

func (m *model) flushPendingStreamText() {
	if m.streamBuffer.Len() == 0 {
		return
	}

	m.appendToLastAssistantMessage(m.streamBuffer.String())
	m.streamBuffer.Reset()
	m.syncViewport()
	m.viewport.GotoBottom()
}

func (m *model) handleSlashCommand(input string) bool {
	if !strings.HasPrefix(input, "/") {
		return false
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return false
	}

	switch fields[0] {
	case "/model":
		if len(fields) == 1 {
			m.openModelMenu()
			return true
		}

		selected := strings.TrimSpace(fields[1])
		for i, modelName := range m.models {
			if modelName == selected {
				m.modelIndex = i
				m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Active model switched to %s.", m.currentModel())})
				return true
			}
		}

		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Unknown model %q.", selected)})
		return true
	case "/clear":
		m.resetConversation()
		return true
	default:
		m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: fmt.Sprintf("Unknown command %q.", fields[0])})
		return true
	}
}
