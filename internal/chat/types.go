package chat

import "context"

type Message struct {
	From    string
	Content string
}

type Request struct {
	Model    string
	Messages []Message
}

type StreamEvent struct {
	Text string
	Done bool
	Err  error
}

type Backend interface {
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}
