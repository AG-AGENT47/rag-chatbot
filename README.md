# rag-chatbot

> **Work in Progress** — core RAG pipeline is functional and deployed; advanced retrieval methods (HyDE and others) are actively being integrated.

Go-based RAG (Retrieval-Augmented Generation) service for [Avyakt Garg's portfolio](https://github.com/AG-AGENT47). Part 2 of a 3-repo system:

```
portfolio-website  →  rag-chatbot (you are here)  →  portfolio-store
```

Answers recruiter and visitor questions about Avyakt's background by retrieving semantically relevant chunks from a Neon PostgreSQL knowledge base (pgvector) and streaming responses via Gemini 2.5 Flash over SSE.

---

## Status

| Feature | Status |
|---|---|
| Core RAG pipeline (embed → retrieve → generate) | Done |
| SSE streaming responses | Done |
| Guardrails (injection detection, length limits) | Done |
| Topic filter (off-topic redirect without LLM call) | Done |
| Query contextualization (pronoun resolution) | Done |
| LLM provider switching (Gemini / Groq) | Done |
| Interaction logging + ratings | Done |
| Metrics endpoint | Done |
| Render deployment | Done |
| **HyDE** (Hypothetical Document Embeddings) | In Progress |
| Re-ranking retrieved chunks | Planned |
| Conversation memory compression | Planned |

---

## What is HyDE?

**HyDE (Hypothetical Document Embeddings)** is an advanced retrieval technique that improves semantic search quality:

1. Instead of embedding the raw user query, the LLM first generates a *hypothetical answer* to the query
2. That hypothetical answer is embedded — it sits closer in vector space to real answers than the raw question does
3. The embedding of the hypothetical answer is used to retrieve chunks from the knowledge base

This dramatically improves retrieval recall for questions phrased very differently from how the knowledge base documents are written (e.g. "tell me about his projects" vs a stored chunk that begins "Avyakt built...").

---

## Architecture

```
POST /chat
  │
  ├─ Guardrails          — length check + injection pattern detection
  ├─ Query Context       — prepend last user message (pronoun resolution)
  ├─ [HyDE - WIP]        — generate hypothetical answer, embed that instead
  ├─ Voyage AI           — embed with voyage-3-lite (512 dims)
  ├─ pgvector            — cosine search → top 5 chunks
  ├─ Topic Filter        — cosine distance > 0.75 → redirect (no LLM call)
  ├─ Gemini 2.5 Flash    — stream response via SSE
  └─ Neon DB             — log interaction for metrics
```

## Project Layout

```
rag-chatbot/
├── cmd/server/main.go          # Entry point, router setup
├── internal/
│   ├── api/
│   │   ├── handlers.go         # /chat, /rating, /metrics, /health
│   │   ├── middleware.go       # CORS, rate limiting
│   │   └── models.go           # Request/response types
│   ├── db/
│   │   └── db.go               # Neon PostgreSQL pool + queries
│   ├── guardrails/
│   │   └── guardrails.go       # Input validation + injection detection
│   ├── llm/
│   │   ├── llm.go              # LLM interface (StreamEvent channel)
│   │   ├── gemini.go           # Gemini 2.5 Flash provider
│   │   └── groq.go             # Groq Llama 3.3 70B provider
│   └── rag/
│       ├── embedder.go         # Voyage AI voyage-3-lite embeddings
│       ├── retriever.go        # pgvector cosine search
│       └── pipeline.go         # Full RAG pipeline orchestration
├── frontend/
│   └── index.html              # Chat UI (served at /)
├── render.yaml                 # Render deployment config
├── Makefile
└── .env.example
```

---

## Stack

- **Go 1.21** + **chi** router
- **pgx/v5** + **pgvector-go** — Neon PostgreSQL with pgvector extension
- **Voyage AI** `voyage-3-lite` — 512-dim query embeddings
- **Gemini 2.5 Flash** (default) — 1500 free req/day via AI Studio
- **Groq Llama 3.3 70B** (fallback) — swap with `LLM_PROVIDER=groq`
- **Render** — free tier deployment via `render.yaml`

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/AG-AGENT47/rag-chatbot
cd rag-chatbot

# 2. Set up secrets
cp .env.example .env
# Fill in: NEON_DATABASE_URL, VOYAGE_API_KEY, GEMINI_API_KEY, ALLOWED_ORIGINS

# 3. Install dependencies
make deps

# 4. Run
make run
# Open http://localhost:8080
```

---

## Endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/chat` | SSE stream — body: `{message, history[]}` |
| `POST` | `/rating` | Submit rating — body: `{interaction_id, rating}` (1 or 5) |
| `GET` | `/metrics` | `{total_conversations, avg_rating, recent_questions}` |
| `GET` | `/health` | `{"status":"ok"}` — Render health check |

---

## LLM Switching

Switch providers with zero code changes via the `LLM_PROVIDER` env var:

```bash
LLM_PROVIDER=gemini make run   # Gemini 2.5 Flash (default)
LLM_PROVIDER=groq   make run   # Groq Llama 3.3 70B
```

The `LLM` interface in `internal/llm/llm.go` makes adding new providers straightforward.

---

## Deployment (Render)

Render reads `render.yaml` automatically on push.

Required env vars in the Render dashboard:

| Variable | Description |
|---|---|
| `NEON_DATABASE_URL` | Neon PostgreSQL connection string |
| `VOYAGE_API_KEY` | Voyage AI API key |
| `GEMINI_API_KEY` | Google AI Studio key |
| `ALLOWED_ORIGINS` | Your portfolio website URL (CORS) |

Push to `main` → auto-deploy. Health check at `GET /health`.

---

## Related Repos

- [`portfolio-website`](https://github.com/AG-AGENT47) — frontend that embeds this chatbot
- [`portfolio-store`](https://github.com/AG-AGENT47) — Neon DB + knowledge base ingestion scripts
