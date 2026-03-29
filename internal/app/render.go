package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

const appVerticalPadding = 2
const classicLayoutWidth = 104

type markdownBlock struct {
	kind string
	lang string
	text string
}

type editProposalSummary struct {
	path      string
	lineCount int
	charCount int
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

	bubbleWidth := min(max(width, 30), 92)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		assistantNameStyle.Render("gopilot"),
		assistantBubbleStyle.Width(bubbleWidth).MaxWidth(bubbleWidth).Render(renderAssistantMarkdown(msg, bubbleWidth)),
	)
}

func renderAssistantMarkdown(msg string, width int) string {
	blocks := parseMarkdownBlocks(msg)
	rendered := make([]string, 0, len(blocks))
	editProposals := make([]editProposalSummary, 0, 4)

	for _, block := range blocks {
		switch block.kind {
		case "code":
			if proposal, ok := summarizeEditProposal(block); ok {
				editProposals = append(editProposals, proposal)
				continue
			}
			rendered = append(rendered, renderCodeBlock(block, width))
		default:
			rendered = append(rendered, renderMarkdownText(block.text, width))
		}
	}

	if len(editProposals) > 0 {
		rendered = append(rendered, renderEditProposalSummary(editProposals, width))
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
			renderedLines = append(renderedLines, assistantBulletStyle.Render("•")+" "+cleanInlineMarkdown(strings.TrimSpace(trimmed[2:])))
			continue
		}

		renderedLines = append(renderedLines, cleanInlineMarkdown(trimmed))
	}

	return assistantTextStyle.Width(width).Render(strings.Join(renderedLines, "\n"))
}

func renderCodeBlock(block markdownBlock, width int) string {
	content := strings.TrimRight(block.text, "\n")
	if strings.TrimSpace(block.lang) == "" {
		return codeBlockStyle.MaxWidth(width).Render(content)
	}

	label := codeLangStyle.Render(strings.ToLower(block.lang))
	return lipgloss.JoinVertical(
		lipgloss.Left,
		label,
		codeBlockStyle.MaxWidth(width).Render(content),
	)
}

func summarizeEditProposal(block markdownBlock) (editProposalSummary, bool) {
	if !strings.HasPrefix(strings.TrimSpace(block.lang), "gopilot-file path=") {
		return editProposalSummary{}, false
	}
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(block.lang), "gopilot-file path="))
	content := strings.TrimRight(block.text, "\n")
	lineCount := 0
	if content != "" {
		lineCount = len(strings.Split(content, "\n"))
	}
	charCount := len(content)
	return editProposalSummary{
		path:      path,
		lineCount: lineCount,
		charCount: charCount,
	}, true
}

func renderEditProposalSummary(proposals []editProposalSummary, width int) string {
	body := []string{
		editProposalTitleStyle.Render(fmt.Sprintf("Pending Changes  •  %d file(s)", len(proposals))),
		editProposalHintStyle.Render("/apply to accept  •  /undo to revert"),
		"",
	}
	for _, proposal := range proposals {
		body = append(body, editProposalPathStyle.Render(proposal.path))
		body = append(body, editProposalPreviewStyle.Render(fmt.Sprintf("%d lines  •  %d chars", proposal.lineCount, proposal.charCount)))
	}
	return editProposalBoxStyle.MaxWidth(width).Render(strings.Join(body, "\n"))
}

func pendingApplyHint(messages []chat.Message) string {
	last := lastAssistantMessage(messages)
	if last == "" {
		return ""
	}
	edits, err := parseProposedFileEdits(last)
	if err != nil || len(edits) == 0 {
		return ""
	}
	return fmt.Sprintf("pending edits: %d file(s)  •  /apply accept  •  /undo revert", len(edits))
}

func (m model) renderInputPreview(width int) string {
	value := strings.TrimRight(m.input.Value(), "\n")
	if value == "" {
		return ""
	}
	if len([]rune(value)) <= max(width-12, 24) && !strings.Contains(value, "\n") {
		return ""
	}

	body := []string{
		editProposalTitleStyle.Render("Prompt Preview"),
		editProposalPreviewStyle.Width(max(width-4, 20)).Render(value),
	}
	return editProposalBoxStyle.Width(width).Render(strings.Join(body, "\n\n"))
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

	contentWidth := min(max(m.panelW, 36), classicLayoutWidth)
	inputWidth := max(contentWidth-4, 20)

	m.viewport.SetWidth(max(contentWidth-4, 28))
	m.input.SetWidth(inputWidth)

	statusText := fmt.Sprintf("%d msgs  •  Enter send  •  /model  •  /add  •  /files  •  /apply  •  /copy  •  /plain  •  Esc quit", len(m.messages))
	if m.waiting {
		statusText = fmt.Sprintf("%s  •  streaming from %s", statusText, m.currentModel())
	}
	if m.choosingModel {
		statusText = fmt.Sprintf("%s  •  selecting model", statusText)
	}
	if strings.TrimSpace(m.sessionSaveErr) != "" {
		statusText = fmt.Sprintf("%s  •  session save failed", statusText)
	}
	if hint := pendingApplyHint(m.messages); hint != "" {
		statusText = fmt.Sprintf("%s  •  %s", statusText, hint)
	}

	statusHeight := lipgloss.Height(statusStyle.Width(contentWidth).Render(statusText))
	inputHeight := lipgloss.Height(inputFrameStyle.Width(contentWidth).Render(m.input.View()))
	metaHeight := lipgloss.Height(inputMetaStyle.Width(contentWidth).Render(fmt.Sprintf("Current model: %s", m.currentModel())))

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
	if m.choosingSession {
		menuHeight = lipgloss.Height(m.renderSessionMenu(contentWidth))
	}
	completionHeight := 0
	if m.hasCompletions() {
		completionHeight = lipgloss.Height(m.renderCompletions(contentWidth))
	}
	previewHeight := 0
	if preview := m.renderInputPreview(contentWidth); preview != "" {
		previewHeight = lipgloss.Height(preview)
	}

	occupiedHeight := appVerticalPadding + statusHeight + menuHeight + previewHeight + inputHeight + completionHeight + metaHeight
	viewportHeight := max(m.height-occupiedHeight, 4)
	m.viewport.SetHeight(viewportHeight)
}

func (m model) renderConversation(width int) string {
	var b strings.Builder
	for i, msg := range m.messages {
		b.WriteString(renderMessage(msg.Content, msg.From, width))
		if i < len(m.messages)-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func joinSections(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		nonEmpty = append(nonEmpty, part)
	}
	return strings.Join(nonEmpty, "\n")
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

func (m model) renderCompletions(width int) string {
	if !m.hasCompletions() {
		return ""
	}

	limit := min(len(m.completions), 8)
	start := 0
	if len(m.completions) > limit {
		start = m.completionIndex - limit/2
		if start < 0 {
			start = 0
		}
		maxStart := len(m.completions) - limit
		if start > maxStart {
			start = maxStart
		}
	}
	end := min(start+limit, len(m.completions))
	var lines []string
	lines = append(lines, completionTitleStyle.Render("Suggestions  •  Tab apply  •  Up/Down select"))
	lines = append(lines, "")

	for i := start; i < end; i++ {
		prefix := "  "
		style := completionItemStyle
		line := m.completions[i]
		if description := slashCommandDescription(m.completions[i]); description != "" {
			line = fmt.Sprintf("%-12s %s", m.completions[i], description)
		}
		if i == m.completionIndex {
			prefix = "> "
			style = completionSelectedStyle
		}
		lines = append(lines, style.Render(prefix+line))
	}

	if len(m.completions) > limit {
		lines = append(lines, "")
		lines = append(lines, completionTitleStyle.Render(fmt.Sprintf("%d-%d of %d", start+1, end, len(m.completions))))
	}

	return completionBoxStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderSessionMenu(width int) string {
	items := m.filteredSessionSummaries()
	var lines []string
	lines = append(lines, menuTitleStyle.Render("Load Session"))
	lines = append(lines, menuHintStyle.Render("Type to filter  •  Up/Down choose  •  Enter load  •  Esc cancel"))
	filter := strings.TrimSpace(m.sessionFilter)
	if filter == "" {
		filter = "all"
	}
	lines = append(lines, menuHintStyle.Render(fmt.Sprintf("Filter: %s  •  %d result(s)", filter, len(items))))
	lines = append(lines, "")

	if len(items) == 0 {
		lines = append(lines, menuItemStyle.Render("  No sessions match the current filter"))
		return menuStyle.Width(width).Render(strings.Join(lines, "\n"))
	}

	limit := min(len(items), 10)
	start := 0
	if len(items) > limit {
		start = m.sessionMenuIndex - limit/2
		if start < 0 {
			start = 0
		}
		maxStart := len(items) - limit
		if start > maxStart {
			start = maxStart
		}
	}
	end := min(start+limit, len(items))

	for i := start; i < end; i++ {
		prefix := "  "
		style := menuItemStyle
		if i == m.sessionMenuIndex {
			prefix = "> "
			style = menuSelectedStyle
		}
		label := fmt.Sprintf("%s%s  •  %s  •  %s", prefix, items[i].ID, items[i].UpdatedAt.Format("2006-01-02 15:04"), items[i].Title)
		lines = append(lines, style.Render(label))
	}

	if len(items) > limit {
		lines = append(lines, "")
		lines = append(lines, menuHintStyle.Render(fmt.Sprintf("%d-%d of %d", start+1, end, len(items))))
	}

	return menuStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) View() tea.View {
	if !m.ready {
		v := tea.NewView(loadingStyle.Render("Loading GoPilot..."))
		v.WindowTitle = windowTitle
		return v
	}

	if m.plainView {
		plain := lastAssistantMessage(m.messages)
		if plain == "" {
			plain = "Nothing to show."
		} else {
			plain = plainViewContent(plain)
		}

		content := plainTextStyle.Render(plain)
		hint := plainHintStyle.Render("Esc back")
		layout := joinSections(content, "", hint)

		v := tea.NewView(layout)
		v.WindowTitle = windowTitle
		return v
	}

	statusText := fmt.Sprintf("%d msgs  •  Enter send  •  /model  •  /add  •  /files  •  /apply  •  /copy  •  /plain  •  Esc quit", len(m.messages))
	if m.waiting {
		statusText = fmt.Sprintf("%s  •  streaming from %s", statusText, m.currentModel())
	}
	if m.choosingModel {
		statusText = fmt.Sprintf("%s  •  selecting model", statusText)
	}
	if strings.TrimSpace(m.sessionSaveErr) != "" {
		statusText = fmt.Sprintf("%s  •  session save failed", statusText)
	}
	if hint := pendingApplyHint(m.messages); hint != "" {
		statusText = fmt.Sprintf("%s  •  %s", statusText, hint)
	}

	contentWidth := max(m.panelW, 36)
	status := statusStyle.Width(contentWidth).Render(statusText)
	conversation := m.renderConversation(max(contentWidth-2, 28))
	inputPreview := m.renderInputPreview(contentWidth)
	inputCard := inputFrameStyle.Width(contentWidth).Render(m.input.View())
	completionBox := m.renderCompletions(contentWidth)
	metaText := fmt.Sprintf("%s  •  %s  •  %s  •  %d attached  •  %s", assistantLabelStyle.Render("model"), m.currentModel(), assistantLabelStyle.Render("workspace"), m.contextFilesLen(), m.completionStatus())
	if strings.TrimSpace(m.sessionSaveErr) != "" {
		metaText = fmt.Sprintf("%s  •  %s", metaText, assistantLabelStyle.Render("session save failed"))
	}
	if hint := pendingApplyHint(m.messages); hint != "" {
		metaText = fmt.Sprintf("%s  •  %s", metaText, assistantLabelStyle.Render(hint))
	}
	inputMeta := inputMetaStyle.Width(contentWidth).Render(metaText)

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
	if m.choosingSession {
		menu = m.renderSessionMenu(contentWidth)
	}

	layout := joinSections(
		status,
		conversation,
		menu,
		inputPreview,
		inputCard,
		completionBox,
		inputMeta,
	)
	v := tea.NewView(layout)
	v.WindowTitle = windowTitle
	return v
}
