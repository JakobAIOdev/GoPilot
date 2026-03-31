package app

import tea "charm.land/bubbletea/v2"

func Run(loadSessionID string) (string, error) {
	m, err := newModelForRun(loadSessionID)
	if err != nil {
		return "", err
	}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	m, ok := finalModel.(model)
	if !ok {
		return "", nil
	}

	return m.sessionID, nil
}

func newModelForRun(loadSessionID string) (model, error) {
	if loadSessionID == "" {
		return newModel(), nil
	}

	m := newModelBase()
	if err := m.loadSessionCommand(loadSessionID); err != nil {
		return model{}, err
	}
	return m, nil
}
