package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/AG-AGENT47/rag-chatbot/internal/db"
	"github.com/AG-AGENT47/rag-chatbot/internal/guardrails"
	"github.com/AG-AGENT47/rag-chatbot/internal/llm"
	"github.com/AG-AGENT47/rag-chatbot/internal/rag"
)

// Handler holds all HTTP handler dependencies.
type Handler struct {
	pipeline *rag.Pipeline
	db       *db.DB
}

// NewHandler creates a Handler.
func NewHandler(pipeline *rag.Pipeline, database *db.DB) *Handler {
	return &Handler{pipeline: pipeline, db: database}
}

// Health handles GET /health — used by Render for health checks.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// Metrics handles GET /metrics — returns aggregated interaction stats.
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.db.GetMetrics(r.Context())
	if err != nil {
		log.Printf("metrics: %v", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		TotalConversations: metrics.TotalConversations,
		AvgRating:          metrics.AvgRating,
		RecentQuestions:    metrics.RecentQuestions,
	})
}

// Rating handles POST /rating — stores thumbs up/down for an interaction.
func (h *Handler) Rating(w http.ResponseWriter, r *http.Request) {
	var req RatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Rating != 1 && req.Rating != 5 {
		http.Error(w, `{"error":"rating must be 1 (thumbs down) or 5 (thumbs up)"}`, http.StatusBadRequest)
		return
	}
	if req.InteractionID == "" {
		http.Error(w, `{"error":"interaction_id is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.UpdateRating(r.Context(), req.InteractionID, req.Rating); err != nil {
		http.Error(w, `{"error":"interaction not found"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Chat handles POST /chat — the main RAG endpoint with SSE streaming.
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Step 1: Guardrails — validate before any API calls.
	if err := guardrails.Validate(req.Message); err != nil {
		http.Error(w, `{"error":"Invalid input."}`, http.StatusBadRequest)
		return
	}

	// Truncate history: keep last 5 messages, cap total content at 2000 chars.
	history := truncateHistory(req.History, 5, 2000)

	// Ensure the response writer supports flushing (required for SSE).
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	start := time.Now()

	// Run the RAG pipeline (embed → retrieve → filter → start LLM stream).
	result, err := h.pipeline.Run(ctx, req.Message, history)
	if err != nil {
		log.Printf("pipeline error: %v", err)
		// Set SSE headers before writing — can't use http.Error after this point.
		setSSSEHeaders(w)
		writeSSEAndFlush(w, flusher, ssePayload{Error: "Service temporarily unavailable. Please try again."})
		return
	}

	// Set SSE headers — must happen before any body writes.
	setSSSEHeaders(w)

	// Topic filter fired: no LLM call, send redirect message and log it.
	if result.IsFiltered {
		writeSSEAndFlush(w, flusher, ssePayload{Token: result.FilterMsg})
		writeSSEAndFlush(w, flusher, ssePayload{Done: true})
		latencyMs := time.Since(start).Milliseconds()
		if _, err := h.db.InsertInteraction(ctx, req.Message, result.FilterMsg, latencyMs); err != nil {
			log.Printf("log filtered interaction: %v", err)
		}
		return
	}

	// Stream tokens from the LLM.
	var sb strings.Builder
	for event := range result.TokenCh {
		if event.Err != nil {
			log.Printf("stream error: %v", event.Err)
			writeSSEAndFlush(w, flusher, ssePayload{Error: "Stream interrupted. Please try again."})
			return
		}
		sb.WriteString(event.Text)
		writeSSEAndFlush(w, flusher, ssePayload{Token: event.Text})
	}

	// All tokens received — log the interaction synchronously (ctx is still valid).
	answer := sb.String()
	latencyMs := time.Since(start).Milliseconds()

	interactionID, err := h.db.InsertInteraction(ctx, req.Message, answer, latencyMs)
	if err != nil {
		log.Printf("log interaction: %v", err)
		// Don't fail the response — just send Done without an ID.
	}

	// Send Done event with the interaction ID so the frontend can submit ratings.
	writeSSEAndFlush(w, flusher, ssePayload{Done: true, ID: interactionID})
}

// --- SSE helpers ---

func setSSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeSSEAndFlush(w http.ResponseWriter, f http.Flusher, payload ssePayload) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", data)
	f.Flush()
}

// --- History helpers ---

// truncateHistory keeps the last maxMessages turns and caps total char length.
func truncateHistory(history []llm.Message, maxMessages, maxChars int) []llm.Message {
	if len(history) > maxMessages {
		history = history[len(history)-maxMessages:]
	}

	// Walk backwards and drop oldest messages if total exceeds maxChars.
	total := 0
	for _, m := range history {
		total += len(m.Content)
	}
	for total > maxChars && len(history) > 0 {
		total -= len(history[0].Content)
		history = history[1:]
	}

	return history
}
