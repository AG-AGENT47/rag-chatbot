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
)

const (
	geminiModel   = "gemini-1.5-flash-latest"
	geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"
)

type geminiClient struct {
	apiKey string
	client *http.Client
}

func newGemini(apiKey string) (LLM, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required for gemini provider")
	}
	return &geminiClient{
		apiKey: apiKey,
		client: &http.Client{},
	}, nil
}

func (g *geminiClient) Name() string { return "gemini-1.5-flash-latest" }

// --- request types ---

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"system_instruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
}

// --- response types ---

type geminiStreamChunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (g *geminiClient) Stream(
	ctx context.Context,
	systemPrompt string,
	history []Message,
	userMsg string,
) (<-chan StreamEvent, error) {
	contents := make([]geminiContent, 0, len(history)+1)
	for _, m := range history {
		role := m.Role
		if role == "assistant" {
			role = "model" // Gemini uses "model" not "assistant"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: userMsg}},
	})

	reqBody := geminiRequest{Contents: contents}
	if systemPrompt != "" {
		reqBody.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s",
		geminiBaseURL, geminiModel, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gemini: status %d: %s", resp.StatusCode, string(respBody))
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
			if data == "" {
				continue
			}

			var chunk geminiStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				ch <- StreamEvent{Err: fmt.Errorf("gemini: parse chunk: %w", err)}
				return
			}
			if chunk.Error != nil {
				ch <- StreamEvent{Err: fmt.Errorf("gemini: api error: %s", chunk.Error.Message)}
				return
			}
			for _, candidate := range chunk.Candidates {
				for _, part := range candidate.Content.Parts {
					if part.Text == "" {
						continue
					}
					select {
					case ch <- StreamEvent{Text: part.Text}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			ch <- StreamEvent{Err: fmt.Errorf("gemini: read stream: %w", err)}
		}
	}()

	return ch, nil
}
