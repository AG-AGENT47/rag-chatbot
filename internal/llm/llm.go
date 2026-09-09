package llm

import (
	"context"
	"fmt"
)

// Message is a single turn in the conversation history.
type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// StreamEvent carries either a text token or a terminal error — never both.
// Err != nil signals mid-stream failure; the channel is then closed.
type StreamEvent struct {
	Text string
	Err  error
}

// LLM is the interface all providers must implement.
type LLM interface {
	// Stream sends the prompt and returns a channel of events.
	// The channel closes when generation completes or an error occurs.
	Stream(ctx context.Context, systemPrompt string, history []Message, userMsg string) (<-chan StreamEvent, error)
	Name() string
}

// Config holds provider configuration sourced from environment.
type Config struct {
	Provider     string
	GeminiAPIKey string
	GroqAPIKey   string
	GroqModel    string // optional; defaults to groqDefaultModel
}

// New returns the LLM implementation for the configured provider.
func New(cfg Config) (LLM, error) {
	switch cfg.Provider {
	case "gemini":
		return newGemini(cfg.GeminiAPIKey)
	case "groq":
		return newGroq(cfg.GroqAPIKey, cfg.GroqModel)
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", cfg.Provider)
	}
}
