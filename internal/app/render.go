package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const appVerticalPadding = 2
const classicLayoutWidth = 84

type markdownBlock struct {
	kind string
	lang string
	text string
}

func renderMessage(msg string, from string, width int) string {
	if from == "GoPilot" && msg == initialSplash {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			assistantNameStyle.Render("gopilot"),
			splashStyle.Render(msg),
		)
	}

	if from == "User" {
		bubbleWidth := min(max(width-2, 28), 76)
		textWidth := max(bubbleWidth-4, 24)
		textContent := lipgloss.NewStyle().
			Width(textWidth).
			MaxWidth(textWidth).
			Render(msg)

		return lipgloss.JoinVertical(
			lipgloss.Left,
			userNameStyle.Render("you"),
			userBubbleStyle.Width(bubbleWidth).MaxWidth(bubbleWidth).Render(textContent),
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		assistantNameStyle.Render("gopilot"),
		renderAssistantMarkdown(msg, min(max(width, 28), 76)),
	)
}

func renderAssistantMarkdown(msg string, width int) string {
	blocks := parseMarkdownBlocks(msg)
	rendered := make([]string, 0, len(blocks))

	for _, block := range blocks {
		switch block.kind {
		case "code":
			rendered = append(rendered, renderCodeBlock(block, width))
		default:
			rendered = append(rendered, renderMarkdownText(block.text, width))
		}
	}

	return strings.Join(rendered, "\n\n")
}

func parseMarkdownBlocks(msg string) []markdownBlock {
	lines := strings.Split(msg, "\n")
	blocks := make([]markdownBlock, 0, 4)
	var textLines []string
	var codeLines []string
	var codeLang string
	inCode := false

	flushText := func() {
		if len(textLines) == 0 {
			return
		}
		blocks = append(blocks, markdownBlock{
			kind: "text",
			text: strings.Join(textLines, "\n"),
		})
		textLines = nil
	}

	flushCode := func() {
		blocks = append(blocks, markdownBlock{
			kind: "code",
			lang: codeLang,
			text: strings.Join(codeLines, "\n"),
		})
		codeLines = nil
		codeLang = ""
	}

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCode {
				flushCode()
				inCode = false
				continue
			}

			flushText()
			inCode = true
			codeLang = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
			continue
		}

		if inCode {
			codeLines = append(codeLines, line)
			continue
		}

		textLines = append(textLines, line)
	}

	if inCode {
		flushCode()
	}
	flushText()

	return blocks
}

func renderMarkdownText(text string, width int) string {
	lines := strings.Split(text, "\n")
	renderedLines := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			renderedLines = append(renderedLines, "")
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			renderedLines = append(renderedLines, assistantHeadingStyle.Render(cleanInlineMarkdown(title)))
			continue
		}

		if strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "- ") {
			renderedLines = append(renderedLines, "• "+cleanInlineMarkdown(strings.TrimSpace(trimmed[2:])))
			continue
		}

		renderedLines = append(renderedLines, cleanInlineMarkdown(trimmed))
	}

	return assistantTextStyle.Width(width).Render(strings.Join(renderedLines, "\n"))
}

func renderCodeBlock(block markdownBlock, width int) string {
	content := strings.TrimRight(block.text, "\n")
	return codeBlockStyle.Width(width).Render(content)
}

func cleanInlineMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"**", "",
		"__", "",
		"`", "",
	)
	return replacer.Replace(text)
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

	statusText := fmt.Sprintf("%d msgs  •  Enter send  •  /model  •  /copy  •  /plain  •  Esc quit", len(m.messages))
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

	if m.plainView {
		plain := lastAssistantMessage(m.messages)
		if plain == "" {
			plain = "Nothing to show."
		}

		hint := plainHintStyle.Render("Plain view  •  Esc back")
		content := plainTextStyle.Render(plain)
		layout := lipgloss.JoinVertical(lipgloss.Left, hint, "", content)

		v := tea.NewView(layout)
		v.AltScreen = true
		v.WindowTitle = windowTitle
		return v
	}

	statusText := fmt.Sprintf("%d msgs  •  Enter send  •  /model  •  /copy  •  /plain  •  Esc quit", len(m.messages))
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
