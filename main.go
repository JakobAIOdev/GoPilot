package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const windowTitle = "GoPilot"

type message struct {
	from    string
	content string
}

type geminiResponseMsg struct {
	model string
	text  string
}

type geminiErrorMsg struct {
	model string
	err   error
}

var availableModels = []string{
	"gemini-2.5-flash",
	"gemini-2.5-pro",
	"gemini-3-flash-preview",
	"gemini-3-pro-preview",
}

type model struct {
	input          textinput.Model
	viewport       viewport.Model
	messages       []message
	sharedHistory  []message
	backend        chatBackend
	ready          bool
	waiting        bool
	choosingModel  bool
	modelMenuIndex int
	models         []string
	modelIndex     int
	width          int
	height         int
	panelW         int
}

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F5F5F5")).
			Padding(0, 0, 1, 0).
			Bold(true)

	headerMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6F6F6F")).
			Padding(0, 0, 1, 0)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(1, 1)

	userNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8D8D8D"))

	assistantNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8D8D8D"))

	userBubbleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F5F5F5")).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#454545")).
			Padding(0, 1)

	assistantBubbleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EDEDED")).
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#3A3A3A")).
				Padding(0, 1)

	inputFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(0, 1).
			MarginTop(1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6F6F6F"))

	inputMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A")).
			Padding(0, 0, 0, 1)

	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EDEDED")).
			Padding(1, 2)

	menuStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(0, 1).
			MarginTop(1)

	menuTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F5F5F5")).
			Bold(true)

	menuHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A"))

	menuItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D0D0D0"))

	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F5F5F5")).
				Bold(true)
)

func initialModel() model {
	ti := textinput.New()
	ti.Prompt = "ask > "
	ti.Placeholder = "Type a prompt and press Enter"
	ti.Focus()
	ti.CharLimit = 200
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
		backend:    newGeminiCLIBackend(),
		models:     availableModels,
		modelIndex: 0,
		messages: []message{
			{from: "GoPilot", content: "Ready for prompts. Responses now wait for Gemini to finish, and you can switch models quickly with ctrl+n / ctrl+p."},
		},
	}

	m.syncViewport()
	return m
}

func requestGeminiResponse(backend chatBackend, selectedModel string, prompt string, history []message) tea.Cmd {
	return func() tea.Msg {
		text, err := backend.Generate(context.Background(), selectedModel, buildPromptWithHistory(history, prompt))
		if err != nil {
			return geminiErrorMsg{model: selectedModel, err: err}
		}
		return geminiResponseMsg{model: selectedModel, text: text}
	}
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

func (m *model) addAssistantMessage(text string) {
	m.messages = append(m.messages, message{from: "GoPilot", content: text})
}

func (m *model) resetConversation() {
	m.sharedHistory = nil
	m.messages = []message{
		{
			from:    "GoPilot",
			content: "Conversation cleared. Shared context is empty now.",
		},
	}
}

func (m *model) setModelByName(name string) bool {
	for i, modelName := range m.models {
		if modelName == name {
			m.modelIndex = i
			return true
		}
	}
	return false
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

func cloneMessages(messages []message) []message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]message, len(messages))
	copy(cloned, messages)
	return cloned
}

func (m model) renderModelMenu(width int) string {
	var lines []string
	lines = append(lines, menuTitleStyle.Render("Model Selection"))
	lines = append(lines, menuHintStyle.Render("Up/Down choose  •  Enter confirm  •  Esc cancel"))
	lines = append(lines, "")

	for i, modelName := range m.models {
		prefix := "  "
		if i == m.modelMenuIndex {
			prefix = "> "
		}

		label := fmt.Sprintf("%s%s", prefix, modelName)
		if i == m.modelIndex {
			label += "  (active)"
		}

		if i == m.modelMenuIndex {
			lines = append(lines, menuSelectedStyle.Render(label))
			continue
		}

		lines = append(lines, menuItemStyle.Render(label))
	}

	return menuStyle.Width(width).Render(strings.Join(lines, "\n"))
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
		if m.setModelByName(selected) {
			m.addAssistantMessage(fmt.Sprintf("Active model switched to %s.", m.currentModel()))
			return true
		}

		m.addAssistantMessage(fmt.Sprintf("Unknown model %q.", selected))
		return true
	case "/clear":
		m.resetConversation()
		return true
	default:
		m.addAssistantMessage(fmt.Sprintf("Unknown command %q.", fields[0]))
		return true
	}
}

func (m *model) syncViewport() {
	var b strings.Builder
	for i, msg := range m.messages {
		b.WriteString(renderMessage(msg, m.viewport.Width()))
		if i < len(m.messages)-1 {
			b.WriteString("\n\n")
		}
	}
	m.viewport.SetContent(b.String())
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.panelW = max(msg.Width-10, 36)
		viewportHeight := max(msg.Height-14, 6)
		m.viewport.SetWidth(max(m.panelW-6, 28))
		m.viewport.SetHeight(viewportHeight)
		m.input.SetWidth(max(m.panelW-12, 20))
		m.ready = true
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	case geminiResponseMsg:
		m.waiting = false
		m.replaceLastAssistantMessage(msg.text)
		m.sharedHistory = append(m.sharedHistory, message{from: "GoPilot", content: msg.text})
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	case geminiErrorMsg:
		m.waiting = false
		m.replaceLastAssistantMessage(fmt.Sprintf("Gemini request failed: %v", msg.err))
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		if m.choosingModel {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.closeModelMenu()
				m.syncViewport()
				return m, nil
			case "up", "ctrl+p":
				m.cycleModelMenu(-1)
				return m, nil
			case "down", "ctrl+n":
				m.cycleModelMenu(1)
				return m, nil
			case "enter":
				m.modelIndex = m.modelMenuIndex
				m.closeModelMenu()
				m.syncViewport()
				return m, nil
			}

			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "ctrl+n":
			if m.waiting {
				return m, nil
			}
			m.cycleModel(1)
			m.syncViewport()
			return m, nil
		case "ctrl+p":
			if m.waiting {
				return m, nil
			}
			m.cycleModel(-1)
			m.syncViewport()
			return m, nil
		case "enter":
			if m.waiting {
				return m, nil
			}

			userText := strings.TrimSpace(m.input.Value())
			if userText == "" {
				return m, nil
			}

			if strings.HasPrefix(userText, "/") {
				handled := m.handleSlashCommand(userText)
				if handled {
					m.input.SetValue("")
				}
				m.syncViewport()
				m.viewport.GotoBottom()
				return m, nil
			}

			m.messages = append(m.messages, message{from: "User", content: userText})
			m.messages = append(m.messages, message{
				from:    "GoPilot",
				content: fmt.Sprintf("Waiting for %s...", m.currentModel()),
			})
			currentModel := m.currentModel()
			history := cloneMessages(m.sharedHistory)
			m.sharedHistory = append(m.sharedHistory, message{from: "User", content: userText})
			m.waiting = true
			m.input.SetValue("")
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, requestGeminiResponse(m.backend, currentModel, userText, history)
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *model) replaceLastAssistantMessage(text string) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].from == "GoPilot" {
			m.messages[i].content = text
			return
		}
	}

	m.messages = append(m.messages, message{from: "GoPilot", content: text})
}

func renderMessage(msg message, width int) string {
	bubbleWidth := min(max(width-2, 28), 76)
	textWidth := max(bubbleWidth-4, 24)
	textContent := lipgloss.NewStyle().
		Width(textWidth).
		MaxWidth(textWidth).
		Render(msg.content)

	if msg.from == "User" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			userNameStyle.Render("you"),
			userBubbleStyle.Width(bubbleWidth).MaxWidth(bubbleWidth).Render(textContent),
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		assistantNameStyle.Render("gopilot"),
		assistantBubbleStyle.Width(bubbleWidth).MaxWidth(bubbleWidth).Render(textContent),
	)
}

func (m model) View() tea.View {
	if !m.ready {
		return tea.NewView(loadingStyle.Render("Loading GoPilot..."))
	}

	headerMeta := headerMetaStyle.Render("terminal copilot")
	headerText := lipgloss.JoinVertical(
		lipgloss.Left,
		fmt.Sprintf("%s  %s", windowTitle, headerMeta),
		fmt.Sprintf("Simple terminal chat UI. Active model: %s", m.currentModel()),
	)

	statusText := fmt.Sprintf("%d messages  •  Enter send  •  /model menu  •  Ctrl+N/Ctrl+P model  •  PgUp/PgDn scroll  •  Esc quit", len(m.messages))
	if m.waiting {
		statusText = fmt.Sprintf("%s  •  waiting for %s", statusText, m.currentModel())
	}
	if m.choosingModel {
		statusText = fmt.Sprintf("%s  •  selecting model", statusText)
	}

	status := statusStyle.Width(m.panelW - 2).Render(statusText)

	conversation := panelStyle.Width(m.panelW - 4).Render(m.viewport.View())
	inputCard := inputFrameStyle.Width(m.panelW - 4).Render(m.input.View())
	inputMeta := inputMetaStyle.Width(m.panelW - 4).Render(fmt.Sprintf("Current model: %s", m.currentModel()))
	menu := ""
	if m.choosingModel {
		menu = m.renderModelMenu(m.panelW - 4)
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		headerStyle.Width(m.panelW-4).Render(headerText),
		status,
		conversation,
		menu,
		inputCard,
		inputMeta,
		footerStyle.Render("Gemini CLI backend with quick model switching."),
	)

	return tea.NewView(appStyle.Render(content))
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
