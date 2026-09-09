package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// groqDefaultModel: Groq rotates its catalogue and drops models without notice
// (llama-3.3-70b-versatile 404'd here). Override with GROQ_MODEL; check the live
// list at GET https://api.groq.com/openai/v1/models.
const (
	groqDefaultModel = "openai/gpt-oss-120b"
	groqURL          = "https://api.groq.com/openai/v1/chat/completions"
)

type groqClient struct {
	apiKey string
	model  string
	client *http.Client
}

func newGroq(apiKey, model string) (LLM, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("GROQ_API_KEY is required for groq provider")
	}
	if model == "" {
		model = groqDefaultModel
	}
	return &groqClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (g *groqClient) Name() string { return "groq/" + g.model }

// --- request types ---

type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type groqRequest struct {
	Model    string        `json:"model"`
	Messages []groqMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// --- response types ---

type groqStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (g *groqClient) Stream(
	ctx context.Context,
	systemPrompt string,
	history []Message,
	userMsg string,
) (<-chan StreamEvent, error) {
	messages := make([]groqMessage, 0, len(history)+2)
	if systemPrompt != "" {
		messages = append(messages, groqMessage{Role: "system", Content: systemPrompt})
	}
	for _, m := range history {
		messages = append(messages, groqMessage{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, groqMessage{Role: "user", Content: userMsg})

	reqBody := groqRequest{
		Model:    g.model,
		Messages: messages,
		Stream:   true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("groq: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, groqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("groq: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("groq: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("groq: status %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" || data == "" {
				return
			}

			var chunk groqStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				ch <- StreamEvent{Err: fmt.Errorf("groq: parse chunk: %w", err)}
				return
			}
			if chunk.Error != nil {
				ch <- StreamEvent{Err: fmt.Errorf("groq: api error: %s", chunk.Error.Message)}
				return
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content == "" {
					continue
				}
				select {
				case ch <- StreamEvent{Text: choice.Delta.Content}:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			ch <- StreamEvent{Err: fmt.Errorf("groq: read stream: %w", err)}
		}
	}()

	return ch, nil
}
