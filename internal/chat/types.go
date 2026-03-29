package chat

import "context"

type Message struct {
	From    string
	Content string
}

type ContextFile struct {
	Path     string
	Language string
	Content  string
}

type Request struct {
	Model          string
	Messages       []Message
	WorkspaceRoot  string
	ContextFiles   []ContextFile
	AllowFileEdits bool
}

type StreamEvent struct {
	Status string
	Text string
	Done bool
	Err  error
}

type Backend interface {
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}
