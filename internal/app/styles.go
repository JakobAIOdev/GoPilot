package app

import "charm.land/lipgloss/v2"

var (
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6F6F6F")).
			Padding(0, 0).
			MarginBottom(1)

	userNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B7C9FF")).
			Bold(true)

	assistantNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8D8D8D")).
				Bold(true)

	userBubbleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F5F5F5")).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#565B66")).
			Padding(0, 1)

	assistantBubbleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EDEDED")).
				Padding(0, 0)

	splashStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EDEDED")).
			Padding(0, 0)

	assistantTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EDEDED"))

	assistantHeadingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F5F5F5")).
				Bold(true)

	codeBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EAEAEA")).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#27272A")).
			Padding(0, 1)

	plainHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A")).
			Padding(0, 0, 0, 0)

	plainTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EDEDED"))

	inputFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(0, 1).
			MarginTop(1)

	inputMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A")).
			Padding(0, 1, 0, 1)

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

	completionBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#3A3A3A")).
				Padding(0, 1).
				MarginTop(1)

	completionTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8A8A8A"))

	completionItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D0D0D0"))

	completionSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F5F5F5")).
				Bold(true)

	assistantBulletStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D0D0D0")).
				Bold(true)

	assistantLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8A8A8A"))

	codeLangStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A")).
			Bold(true)

	editProposalBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5B6B4A")).
				Padding(0, 1)

	editProposalTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EEF5DE")).
				Bold(true)

	editProposalPathStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#DCE8C2")).
				Bold(true)

	editProposalPreviewStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#A4AF97")).
					Padding(0, 0, 0, 2)

	editProposalHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8E9785"))

	inlineCodeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E0C097")).
				Background(lipgloss.Color("#2A2A2A"))

	inlineBoldStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F5F5F5")).
				Bold(true)
)
