package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"github.com/AG-AGENT47/rag-chatbot/internal/api"
	"github.com/AG-AGENT47/rag-chatbot/internal/db"
	"github.com/AG-AGENT47/rag-chatbot/internal/llm"
	"github.com/AG-AGENT47/rag-chatbot/internal/rag"
)

func main() {
	// Load .env in development (no-op in production where env vars are set directly).
	_ = godotenv.Load()

	ctx := context.Background()

	// --- Config ---
	databaseURL := mustEnv("NEON_DATABASE_URL")
	voyageAPIKey := mustEnv("VOYAGE_API_KEY")
	llmProvider := envOr("LLM_PROVIDER", "groq")
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	groqAPIKey := os.Getenv("GROQ_API_KEY")
	allowedOrigins := envOr("ALLOWED_ORIGINS", "http://localhost:3000")
	port := envOr("PORT", "8080")
	threshold := parseFloatOr(envOr("SIMILARITY_THRESHOLD", "0.75"), 0.75)

	// --- Database ---
	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	log.Println("Database connected")

	database := db.New(pool)

	// --- LLM ---
	l, err := llm.New(llm.Config{
		Provider:     llmProvider,
		GeminiAPIKey: geminiAPIKey,
		GroqAPIKey:   groqAPIKey,
	})
	if err != nil {
		log.Fatalf("llm: %v", err)
	}
	log.Printf("LLM provider: %s", l.Name())

	// --- RAG Pipeline ---
	embedder := rag.NewEmbedder(voyageAPIKey)
	retriever := rag.NewRetriever(pool)
	pipeline := rag.NewPipeline(embedder, retriever, l, threshold)

	// --- Handlers ---
	h := api.NewHandler(pipeline, database)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(api.CORSMiddleware(allowedOrigins))

	r.Get("/health", h.Health)
	r.Get("/metrics", h.Metrics)
	r.Post("/rating", h.Rating)
	r.With(api.RateLimitMiddleware()).Post("/chat", h.Chat)

	// Serve test UI from working directory (run from repo root).
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "frontend/index.html")
	})

	log.Printf("Server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %q is not set", key)
	}
	return v
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseFloatOr(s string, fallback float64) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return f
}
