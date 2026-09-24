// Package inference contains no shell or process execution capabilities.
package inference

import "context"

type Request struct {
	Model, System, Prompt string
	Temperature           float64
	MaxTokens             int
}
type Delta struct {
	Text     string
	Thinking bool
}
type Backend interface {
	Models(context.Context) ([]string, error)
	Generate(context.Context, Request, func(Delta) error) error
}
