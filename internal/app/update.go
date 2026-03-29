package app

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

const streamFlushInterval = 40 * time.Millisecond
const maxAutoRetries = 1

func waitForStreamEvent(stream <-chan chat.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-stream
		if !ok {
			return streamMsg{event: chat.StreamEvent{Done: true}}
		}
		return streamMsg{event: event}
	}
}

func startStreamCmd(backend chat.Backend, modelName string, history []chat.Message) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		req := chat.Request{
			Model:    modelName,
			Messages: history,
		}

		stream, err := backend.Stream(ctx, req)
		if err != nil {
			cancel()
			return streamStartedMsg{err: err}
		}

		return streamStartedMsg{
			stream: stream,
			cancel: cancel,
		}
	}
}

func scheduleStreamFlush() tea.Cmd {
	return tea.Tick(streamFlushInterval, func(time.Time) tea.Msg {
		return streamFlushMsg{}
	})
}

func scheduleRetry(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return retryStreamMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.panelW = min(max(msg.Width-6, 36), classicLayoutWidth)
		m.refreshLayout()
		m.ready = true
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, nil

	case streamMsg:
		if msg.event.Err != nil {
			m.waiting = false
			m.stream = nil
			m.flushPendingStreamText()
			if m.cancelStream != nil {
				m.cancelStream()
				m.cancelStream = nil
			}

			if delay, ok := quotaRetryDelay(msg.event.Err); ok && m.retryCount < maxAutoRetries && m.pendingModel != "" && len(m.pendingHistory) > 0 {
				m.retryCount++
				m.replaceLastAssistantMessage(formatRetryNotice(msg.event.Err, delay))
				m.refreshLayout()
				m.syncViewport()
				m.viewport.GotoBottom()
				return m, scheduleRetry(delay)
			}

			m.replaceLastAssistantMessage(formatBackendError(msg.event.Err))
			m.refreshLayout()
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}

		if msg.event.Text != "" {
			m.streamBuffer.WriteString(msg.event.Text)
			if !m.flushScheduled {
				m.flushScheduled = true
				return m, tea.Batch(waitForStreamEvent(m.stream), scheduleStreamFlush())
			}
		}

		if msg.event.Done {
			m.waiting = false
			m.stream = nil
			m.flushPendingStreamText()
			if m.cancelStream != nil {
				m.cancelStream()
				m.cancelStream = nil
			}
			m.sharedHistory = append(m.sharedHistory, chat.Message{From: "GoPilot", Content: m.messages[len(m.messages)-1].Content})
			m.pendingModel = ""
			m.pendingHistory = nil
			m.retryCount = 0
			m.refreshLayout()
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}

		return m, waitForStreamEvent(m.stream)

	case streamFlushMsg:
		m.flushScheduled = false
		m.flushPendingStreamText()
		if m.waiting && m.streamBuffer.Len() > 0 && !m.flushScheduled {
			m.flushScheduled = true
			return m, scheduleStreamFlush()
		}
		return m, nil

	case retryStreamMsg:
		if m.waiting || m.pendingModel == "" || len(m.pendingHistory) == 0 {
			return m, nil
		}

		m.waiting = true
		m.streamBuffer.Reset()
		m.flushScheduled = false
		m.replaceLastAssistantMessage("")
		m.refreshLayout()
		m.syncViewport()
		m.viewport.GotoBottom()
		return m, startStreamCmd(m.backend, m.pendingModel, cloneMessages(m.pendingHistory))

	case streamStartedMsg:
		if msg.err != nil {
			m.waiting = false
			m.flushPendingStreamText()

			if delay, ok := quotaRetryDelay(msg.err); ok && m.retryCount < maxAutoRetries && m.pendingModel != "" && len(m.pendingHistory) > 0 {
				m.retryCount++
				m.replaceLastAssistantMessage(formatRetryNotice(msg.err, delay))
				m.refreshLayout()
				m.syncViewport()
				m.viewport.GotoBottom()
				return m, scheduleRetry(delay)
			}

			m.replaceLastAssistantMessage(formatBackendError(msg.err))
			m.refreshLayout()
			m.syncViewport()
			m.viewport.GotoBottom()
			return m, nil
		}

		m.stream = msg.stream
		m.cancelStream = msg.cancel
		return m, waitForStreamEvent(msg.stream)

	case tea.KeyMsg:
		if m.plainView {
			switch msg.String() {
			case "ctrl+c":
				if m.cancelStream != nil {
					m.cancelStream()
				}
				return m, tea.Quit
			case "esc":
				m.plainView = false
				m.refreshLayout()
				m.syncViewport()
				return m, nil
			}

			return m, nil
		}

		if m.choosingModel {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.closeModelMenu()
				m.refreshLayout()
				m.syncViewport()
				return m, nil
			case "up", "ctrl+p":
				m.cycleModelMenu(-1)
				m.refreshLayout()
				return m, nil
			case "down", "ctrl+n":
				m.cycleModelMenu(1)
				m.refreshLayout()
				return m, nil
			case "enter":
				m.modelIndex = m.modelMenuIndex
				m.closeModelMenu()
				m.refreshLayout()
				m.syncViewport()
				return m, nil
			}

			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "esc":
			if m.cancelStream != nil {
				m.cancelStream()
			}
			return m, tea.Quit
		case "ctrl+n":
			if m.waiting {
				return m, nil
			}
			m.cycleModel(1)
			m.refreshLayout()
			m.syncViewport()
			return m, nil
		case "ctrl+p":
			if m.waiting {
				return m, nil
			}
			m.cycleModel(-1)
			m.refreshLayout()
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
				if m.handleSlashCommand(userText) {
					m.input.SetValue("")
				}
				m.refreshLayout()
				m.syncViewport()
				m.viewport.GotoBottom()
				return m, nil
			}

			m.messages = append(m.messages, chat.Message{From: "User", Content: userText})
			m.messages = append(m.messages, chat.Message{From: "GoPilot", Content: ""})
			m.sharedHistory = append(m.sharedHistory, chat.Message{From: "User", Content: userText})
			m.waiting = true
			m.streamBuffer.Reset()
			m.flushScheduled = false
			m.pendingModel = m.currentModel()
			m.pendingHistory = cloneMessages(m.sharedHistory)
			m.retryCount = 0
			m.input.SetValue("")
			m.refreshLayout()
			m.syncViewport()
			m.viewport.GotoBottom()
			cmd := startStreamCmd(m.backend, m.pendingModel, cloneMessages(m.pendingHistory))
			return m, cmd
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
