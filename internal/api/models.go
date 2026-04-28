package api

import "github.com/AG-AGENT47/rag-chatbot/internal/llm"

// ChatRequest is the POST /chat request body.
type ChatRequest struct {
	Message string        `json:"message"`
	History []llm.Message `json:"history"`
}

// RatingRequest is the POST /rating request body.
type RatingRequest struct {
	InteractionID string `json:"interaction_id"`
	Rating        int    `json:"rating"` // 1 = thumbs down, 5 = thumbs up
}

// MetricsResponse is the GET /metrics response body.
type MetricsResponse struct {
	TotalConversations int      `json:"total_conversations"`
	AvgRating          *float64 `json:"avg_rating"`       // nil if no ratings yet
	RecentQuestions    []string `json:"recent_questions"` // last 5
}

// ssePayload is the JSON payload of each SSE data event.
type ssePayload struct {
	Token       string `json:"token,omitempty"`
	Done        bool   `json:"done,omitempty"`
	ID          string `json:"id,omitempty"`
	Error       string `json:"error,omitempty"`
	RateLimited bool   `json:"rate_limited,omitempty"`
}
