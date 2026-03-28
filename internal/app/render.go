package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const appVerticalPadding = 2
const classicLayoutWidth = 84

func renderMessage(msg string, from string, width int) string {
	if from == "GoPilot" && msg == initialSplash {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			assistantNameStyle.Render("gopilot"),
			splashStyle.Render(msg),
		)
	}

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

	contentWidth := max(m.panelW, 36)
	viewportWidth := max(contentWidth-4, 28)
	inputWidth := max(contentWidth-4, 20)

	m.viewport.SetWidth(viewportWidth)
	m.input.SetWidth(inputWidth)

	statusText := fmt.Sprintf("%d msgs  •  Enter send  •  /model menu  •  Ctrl+N/Ctrl+P model  •  Esc quit", len(m.messages))
	if m.waiting {
		statusText = fmt.Sprintf("%s  •  streaming from %s", statusText, m.currentModel())
	}
	if m.choosingModel {
		statusText = fmt.Sprintf("%s  •  selecting model", statusText)
	}

	statusHeight := lipgloss.Height(statusStyle.Width(contentWidth).Render(statusText))
	inputHeight := lipgloss.Height(inputFrameStyle.Width(contentWidth).Render(m.input.View()))
	metaHeight := lipgloss.Height(inputMetaStyle.Width(contentWidth).Render(fmt.Sprintf("Current model: %s", m.currentModel())))

	panelChromeHeight := lipgloss.Height(panelStyle.Width(contentWidth).Render("")) - 1

	menuHeight := 0
	if m.choosingModel {
		menuHeight = lipgloss.Height(m.renderMenu(
			contentWidth,
			"Model Selection",
			"Up/Down choose  •  Enter confirm  •  Esc cancel",
			m.models,
			m.modelIndex,
			m.modelMenuIndex,
		))
	}

	occupiedHeight := appVerticalPadding + statusHeight + menuHeight + inputHeight + metaHeight + panelChromeHeight
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

	statusText := fmt.Sprintf("%d msgs  •  Enter send  •  /model menu  •  Ctrl+N/Ctrl+P model  •  Esc quit", len(m.messages))
	if m.waiting {
		statusText = fmt.Sprintf("%s  •  streaming from %s", statusText, m.currentModel())
	}
	if m.choosingModel {
		statusText = fmt.Sprintf("%s  •  selecting model", statusText)
	}

	contentWidth := max(m.panelW, 36)
	status := statusStyle.Width(contentWidth).Render(statusText)
	conversation := panelStyle.Width(contentWidth).Render(m.viewport.View())
	inputCard := inputFrameStyle.Width(contentWidth).Render(m.input.View())
	inputMeta := inputMetaStyle.Width(contentWidth).Render(fmt.Sprintf("Current model: %s", m.currentModel()))

	menu := ""
	if m.choosingModel {
		menu = m.renderMenu(
			contentWidth,
			"Model Selection",
			"Up/Down choose  •  Enter confirm  •  Esc cancel",
			m.models,
			m.modelIndex,
			m.modelMenuIndex,
		)
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		status,
		conversation,
		menu,
		inputCard,
		inputMeta,
	)

	layout := appStyle.Render(content)
	if m.width > 0 {
		layout = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, layout)
	}

	v := tea.NewView(layout)
	v.AltScreen = true
	v.WindowTitle = windowTitle
	return v
}
