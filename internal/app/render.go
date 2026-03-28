package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const appVerticalPadding = 2

func renderMessage(msg string, from string, width int) string {
	bubbleWidth := min(max(width-2, 28), 76)
	textWidth := max(bubbleWidth-4, 24)
	textContent := lipgloss.NewStyle().
		Width(textWidth).
		MaxWidth(textWidth).
		Render(msg)

	if from == "User" {
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

func (m *model) syncViewport() {
	var b strings.Builder
	for i, msg := range m.messages {
		b.WriteString(renderMessage(msg.Content, msg.From, m.viewport.Width()))
		if i < len(m.messages)-1 {
			b.WriteString("\n\n")
		}
	}
	m.viewport.SetContent(b.String())
}

func (m *model) refreshLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	panelWidth := max(m.panelW-4, 32)
	viewportWidth := max(m.panelW-6, 28)
	inputWidth := max(m.panelW-12, 20)

	m.viewport.SetWidth(viewportWidth)
	m.input.SetWidth(inputWidth)

	headerMeta := headerMetaStyle.Render("gemini subscription")
	headerText := lipgloss.JoinVertical(
		lipgloss.Left,
		fmt.Sprintf("%s  %s", windowTitle, headerMeta),
		fmt.Sprintf("Streaming terminal chat. Model: %s", m.currentModel()),
	)

	statusText := fmt.Sprintf("%d messages  •  Enter send  •  /model menu  •  Ctrl+N/Ctrl+P model  •  PgUp/PgDn scroll  •  Esc quit", len(m.messages))
	if m.waiting {
		statusText = fmt.Sprintf("%s  •  streaming from %s", statusText, m.currentModel())
	}
	if m.choosingModel {
		statusText = fmt.Sprintf("%s  •  selecting model", statusText)
	}

	headerHeight := lipgloss.Height(headerStyle.Width(panelWidth).Render(headerText))
	statusHeight := lipgloss.Height(statusStyle.Width(m.panelW - 2).Render(statusText))
	inputHeight := lipgloss.Height(inputFrameStyle.Width(panelWidth).Render(m.input.View()))
	metaHeight := lipgloss.Height(inputMetaStyle.Width(panelWidth).Render(fmt.Sprintf("Current model: %s", m.currentModel())))
	footerHeight := lipgloss.Height(footerStyle.Render("Google subscription backend with streaming output."))
	panelChromeHeight := lipgloss.Height(panelStyle.Width(panelWidth).Render("")) - 1

	menuHeight := 0
	if m.choosingModel {
		menuHeight = lipgloss.Height(m.renderMenu(
			panelWidth,
			"Model Selection",
			"Up/Down choose  •  Enter confirm  •  Esc cancel",
			m.models,
			m.modelIndex,
			m.modelMenuIndex,
		))
	}

	occupiedHeight := appVerticalPadding + headerHeight + statusHeight + menuHeight + inputHeight + metaHeight + footerHeight + panelChromeHeight
	viewportHeight := max(m.height-occupiedHeight, 4)
	m.viewport.SetHeight(viewportHeight)
}

func (m model) renderMenu(width int, title string, hint string, items []string, activeIndex int, menuIndex int) string {
	var lines []string
	lines = append(lines, menuTitleStyle.Render(title))
	lines = append(lines, menuHintStyle.Render(hint))
	lines = append(lines, "")

	for i, itemName := range items {
		prefix := "  "
		if i == menuIndex {
			prefix = "> "
		}

		label := fmt.Sprintf("%s%s", prefix, itemName)
		if i == activeIndex {
			label += "  (active)"
		}

		if i == menuIndex {
			lines = append(lines, menuSelectedStyle.Render(label))
			continue
		}

		lines = append(lines, menuItemStyle.Render(label))
	}

	return menuStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) View() tea.View {
	if !m.ready {
		v := tea.NewView(loadingStyle.Render("Loading GoPilot..."))
		v.AltScreen = true
		v.WindowTitle = windowTitle
		return v
	}

	headerMeta := headerMetaStyle.Render("gemini subscription")
	headerText := lipgloss.JoinVertical(
		lipgloss.Left,
		fmt.Sprintf("%s  %s", windowTitle, headerMeta),
		fmt.Sprintf("Streaming terminal chat. Model: %s", m.currentModel()),
	)

	statusText := fmt.Sprintf("%d messages  •  Enter send  •  /model menu  •  Ctrl+N/Ctrl+P model  •  PgUp/PgDn scroll  •  Esc quit", len(m.messages))
	if m.waiting {
		statusText = fmt.Sprintf("%s  •  streaming from %s", statusText, m.currentModel())
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
		menu = m.renderMenu(
			m.panelW-4,
			"Model Selection",
			"Up/Down choose  •  Enter confirm  •  Esc cancel",
			m.models,
			m.modelIndex,
			m.modelMenuIndex,
		)
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		headerStyle.Width(m.panelW-4).Render(headerText),
		status,
		conversation,
		menu,
		inputCard,
		inputMeta,
		footerStyle.Render("Google subscription backend with streaming output."),
	)

	v := tea.NewView(appStyle.Render(content))
	v.AltScreen = true
	v.WindowTitle = windowTitle
	return v
}
