package app

import "charm.land/lipgloss/v2"

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6F6F6F")).
			Padding(0, 0, 1, 0)

	panelStyle = lipgloss.NewStyle().
			Padding(0, 0)

	userNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8D8D8D"))

	assistantNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8D8D8D"))

	userBubbleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F5F5F5")).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#454545")).
			Padding(0, 1)

	splashStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EDEDED"))

	assistantTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EDEDED"))

	assistantHeadingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F5F5F5")).
				Bold(true)

	codeBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EAEAEA"))

	plainHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8A8A8A")).
			Padding(1, 2, 0, 2)

	plainTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EDEDED")).
			Padding(0, 2, 1, 2)

	inputFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3A3A3A")).
			Padding(0, 1).
			MarginTop(1)

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
