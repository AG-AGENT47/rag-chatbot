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

// ErrRateLimit is returned when an embedding API responds with HTTP 429.
// Callers detect it with errors.As to surface a "retry shortly" message
// instead of a generic service error.
type ErrRateLimit struct{ msg string }

func (e *ErrRateLimit) Error() string { return "embedder: rate limited: " + e.msg }

// Embedder turns query text into a vector. The implementation MUST match the
// model that built the stored knowledge_base embeddings (same model, same
// dimensionality) or retrieval silently degrades to noise.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	// Name is the provider/model id, logged at startup.
	Name() string
}

// EmbedConfig selects and configures the embedding provider.
type EmbedConfig struct {
	Provider     string // "gemini" (default) | "voyage"
	GeminiAPIKey string
	VoyageAPIKey string
}

// NewEmbedder builds the configured Embedder.
//
// gemini (default): free tier is ~100 RPM — comfortably enough for a portfolio.
// voyage: legacy. voyage-3-lite's free tier is 3 RPM once the trial credit is
// spent, which is what took the chatbot down; kept only for rollback.
func NewEmbedder(cfg EmbedConfig) (Embedder, error) {
	switch cfg.Provider {
	case "", "gemini":
		if cfg.GeminiAPIKey == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is required for EMBED_PROVIDER=gemini")
		}
		return &geminiEmbedder{
			apiKey: cfg.GeminiAPIKey,
			client: &http.Client{Timeout: 15 * time.Second},
		}, nil
	case "voyage":
		if cfg.VoyageAPIKey == "" {
			return nil, fmt.Errorf("VOYAGE_API_KEY is required for EMBED_PROVIDER=voyage")
		}
		return &voyageEmbedder{
			apiKey: cfg.VoyageAPIKey,
			client: &http.Client{Timeout: 15 * time.Second},
		}, nil
	default:
		return nil, fmt.Errorf("unknown EMBED_PROVIDER %q (want \"gemini\" or \"voyage\")", cfg.Provider)
	}
}

// ---------------------------------------------------------------------------
// Gemini — gemini-embedding-001, 768 dimensions
// ---------------------------------------------------------------------------

const (
	geminiEmbedModel = "gemini-embedding-001"
	geminiEmbedURL   = "https://generativelanguage.googleapis.com/v1beta/models/" + geminiEmbedModel + ":embedContent"
	geminiEmbedDims  = 768
)

type geminiEmbedder struct {
	apiKey string
	client *http.Client
}

func (g *geminiEmbedder) Name() string { return "gemini-" + geminiEmbedModel }

type geminiEmbedRequest struct {
	Model                string             `json:"model"`
	Content              geminiEmbedContent `json:"content"`
	TaskType             string             `json:"taskType"`
	OutputDimensionality int                `json:"outputDimensionality"`
}

type geminiEmbedContent struct {
	Parts []geminiEmbedPart `json:"parts"`
}

type geminiEmbedPart struct {
	Text string `json:"text"`
}

type geminiEmbedResponse struct {
	Embedding struct {
		Values []float32 `json:"values"`
	} `json:"embedding"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed returns the query vector. taskType=RETRIEVAL_QUERY pairs with the
// RETRIEVAL_DOCUMENT embeddings stored by portfolio-store/scripts/embed.py.
func (g *geminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := geminiEmbedRequest{
		Model:                "models/" + geminiEmbedModel,
		Content:              geminiEmbedContent{Parts: []geminiEmbedPart{{Text: text}}},
		TaskType:             "RETRIEVAL_QUERY",
		OutputDimensionality: geminiEmbedDims,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedder: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiEmbedURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.apiKey)

	resp, err := g.client.Do(req)
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
		return nil, fmt.Errorf("embedder: gemini status %d: %s", resp.StatusCode, string(respBody))
	}

	var er geminiEmbedResponse
	if err := json.Unmarshal(respBody, &er); err != nil {
		return nil, fmt.Errorf("embedder: unmarshal: %w", err)
	}
	if er.Error != nil {
		return nil, fmt.Errorf("embedder: gemini api error: %s", er.Error.Message)
	}
	if len(er.Embedding.Values) == 0 {
		return nil, fmt.Errorf("embedder: empty gemini response")
	}
	return er.Embedding.Values, nil
}

// ---------------------------------------------------------------------------
// Voyage — voyage-3-lite, 512 dimensions (legacy, rollback only)
// ---------------------------------------------------------------------------

const (
	voyageURL   = "https://api.voyageai.com/v1/embeddings"
	voyageModel = "voyage-3-lite"
)

type voyageEmbedder struct {
	apiKey string
	client *http.Client
}

func (v *voyageEmbedder) Name() string { return "voyage-" + voyageModel }

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

func (v *voyageEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := voyageRequest{Model: voyageModel, Input: []string{text}, InputType: "query"}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedder: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, voyageURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	resp, err := v.client.Do(req)
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
		return nil, fmt.Errorf("embedder: voyage status %d: %s", resp.StatusCode, string(respBody))
	}

	var vr voyageResponse
	if err := json.Unmarshal(respBody, &vr); err != nil {
		return nil, fmt.Errorf("embedder: unmarshal: %w", err)
	}
	if vr.Error != nil {
		return nil, fmt.Errorf("embedder: voyage api error: %s", vr.Error.Message)
	}
	if len(vr.Data) == 0 {
		return nil, fmt.Errorf("embedder: empty voyage response")
	}
	return vr.Data[0].Embedding, nil
}
