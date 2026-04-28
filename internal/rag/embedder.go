package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	voyageURL   = "https://api.voyageai.com/v1/embeddings"
	voyageModel = "voyage-3-lite"
)

// ErrRateLimit is returned when the embedding API responds with HTTP 429.
// Callers can detect this with errors.As to surface a user-friendly message
// instead of a generic service error.
type ErrRateLimit struct{ msg string }

func (e *ErrRateLimit) Error() string { return "embedder: rate limited: " + e.msg }

// Embedder embeds query text using Voyage AI.
type Embedder struct {
	apiKey string
	client *http.Client
}

// NewEmbedder creates an Embedder backed by Voyage AI.
func NewEmbedder(apiKey string) *Embedder {
	return &Embedder{
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type voyageRequest struct {
	Model     string   `json:"model"`
	Input     []string `json:"input"`
	InputType string   `json:"input_type"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed returns the 512-dimensional vector for the given query text.
// input_type="query" is required — it differs from "document" normalization.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := voyageRequest{
		Model:     voyageModel,
		Input:     []string{text},
		InputType: "query",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedder: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, voyageURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedder: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embedder: read response: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &ErrRateLimit{msg: string(respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedder: status %d: %s", resp.StatusCode, string(respBody))
	}

	var vr voyageResponse
	if err := json.Unmarshal(respBody, &vr); err != nil {
		return nil, fmt.Errorf("embedder: unmarshal: %w", err)
	}
	if vr.Error != nil {
		return nil, fmt.Errorf("embedder: api error: %s", vr.Error.Message)
	}
	if len(vr.Data) == 0 {
		return nil, fmt.Errorf("embedder: empty response")
	}

	return vr.Data[0].Embedding, nil
}
