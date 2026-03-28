package app

import tea "charm.land/bubbletea/v2"

func Run() error {
	p := tea.NewProgram(newModel())
	_, err := p.Run()
	return err
}
