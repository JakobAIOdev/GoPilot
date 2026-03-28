package main

import (
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

type model struct {
	input    textinput.Model
	viewport viewport.Model
	messages []message
	ready    bool
	width    int
	height   int
	panelW   int
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

	loadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EDEDED")).
			Padding(1, 2)
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
		input:    ti,
		viewport: vp,
		messages: []message{
			{from: "GoPilot", content: "Ready for prompts. Clean terminal, stronger contrast, and a layout that feels more deliberate than the stock demo."},
		},
	}

	m.syncViewport()
	return m
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

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			userText := strings.TrimSpace(m.input.Value())
			if userText == "" {
				return m, nil
			}
			m.messages = append(m.messages, message{from: "User", content: userText})
			m.messages = append(m.messages, message{from: "GoPilot", content: fmt.Sprintf("Echo mode is still active. You said: %q", userText)})
			m.input.SetValue("")
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
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

func renderMessage(msg message, width int) string {
	maxContentWidth := min(max(width-6, 24), 72)
	textContent := lipgloss.NewStyle().
		MaxWidth(maxContentWidth).
		Render(msg.content)

	if msg.from == "User" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			userNameStyle.Render("you"),
			userBubbleStyle.MaxWidth(maxContentWidth).Render(textContent),
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		assistantNameStyle.Render("gopilot"),
		assistantBubbleStyle.MaxWidth(maxContentWidth).Render(textContent),
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
		"Simple terminal chat UI.",
	)

	status := statusStyle.Width(m.panelW - 2).Render(
		fmt.Sprintf("%d messages  •  Enter send  •  PgUp/PgDn scroll  •  Esc quit", len(m.messages)),
	)

	conversation := panelStyle.Width(m.panelW - 4).Render(m.viewport.View())
	inputCard := inputFrameStyle.Width(m.panelW - 4).Render(m.input.View())

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		headerStyle.Width(m.panelW-4).Render(headerText),
		status,
		conversation,
		inputCard,
		footerStyle.Render("Minimal layout, no extra noise."),
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
