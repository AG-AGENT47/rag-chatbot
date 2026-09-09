# rag-chatbot

Go-based RAG (Retrieval-Augmented Generation) service for [Avyakt Garg's portfolio](https://github.com/AG-AGENT47). Part 2 of a 3-repo system:

```
portfolio-website  →  rag-chatbot (you are here)  →  portfolio-store
```

Answers recruiter and visitor questions about Avyakt's background by retrieving semantically relevant chunks from a Neon PostgreSQL knowledge base and streaming responses via Groq Llama 3.3 70B over SSE.

---

## Status

| Feature | Status |
|---|---|
| Hybrid retrieval (vector + full-text search, RRF merge) | Done |
| SSE streaming responses | Done |
| Guardrails (injection detection, length limits) | Done |
| Topic filter (off-topic redirect without LLM call) | Done |
| Query contextualization (pronoun resolution) | Done |
| Metadata-grounded context (company, role, date per chunk) | Done |
| Rate limit error signalling (`rate_limited` SSE field) | Done |
| LLM provider switching (Gemini / Groq) | Done |
| Interaction logging + ratings | Done |
| Metrics endpoint | Done |
| Render deployment | Done |
| Embedding cache (absorb Voyage 3 RPM bursts) | Planned |
| Re-ranking retrieved chunks | Planned |
| Conversation memory compression | Planned |

---

## Architecture

```
POST /chat
  │
  ├─ Guardrails          — length check + injection pattern detection
  ├─ Query contextualization — prepend last user message (pronoun resolution)
  ├─ Voyage AI           — embed with voyage-3-lite
  ├─ Hybrid search       — vector (pgvector cosine) + full-text (tsvector/tsquery)
  │                         merged via Reciprocal Rank Fusion → top 5 chunks
  ├─ Topic filter        — cosine distance > 0.80 → redirect (no LLM call)
  ├─ Context builder     — format chunks with metadata headers (company/role/date)
  ├─ Groq Llama 3.3 70B  — stream response via SSE
  └─ Neon DB             — log interaction for metrics
```

---

## Project Layout

```
rag-chatbot/
├── cmd/server/main.go          # Entry point, router setup
├── internal/
│   ├── api/
│   │   ├── handlers.go         # /chat, /rating, /metrics, /health
│   │   ├── middleware.go       # CORS, IP rate limiting
│   │   └── models.go           # Request/response types (incl. ssePayload)
│   ├── db/
│   │   └── db.go               # Neon PostgreSQL pool + queries
│   ├── guardrails/
│   │   └── guardrails.go       # Input validation + injection detection
│   ├── llm/
│   │   ├── llm.go              # LLM interface (StreamEvent channel)
│   │   ├── gemini.go           # Gemini provider (fallback)
│   │   └── groq.go             # Groq Llama 3.3 70B provider (default)
│   └── rag/
│       ├── embedder.go         # Voyage AI embeddings + ErrRateLimit type
│       ├── retriever.go        # HybridTopK: vector + FTS + RRF merge
│       └── pipeline.go         # RAG pipeline orchestration + buildContext
├── frontend/
│   └── index.html              # Test chat UI (served at /)
├── render.yaml                 # Render deployment config
├── Makefile
└── .env.example
```

---

## Stack

- **Go 1.21** + **chi** router
- **pgx/v5** + **pgvector-go** — Neon PostgreSQL with pgvector + full-text search
- **Voyage AI** `voyage-3-lite` — query embeddings
- **Groq Llama 3.3 70B** (default) — 1,000 free req/day, 12K TPM
- **Gemini 1.5 Flash** (fallback) — swap with `LLM_PROVIDER=gemini`
- **Render** — free tier deployment via `render.yaml`

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/AG-AGENT47/rag-chatbot
cd rag-chatbot

# 2. Set up secrets
cp .env.example .env
# Fill in: NEON_DATABASE_URL, VOYAGE_API_KEY, GROQ_API_KEY, ALLOWED_ORIGINS

# 3. Install dependencies
make deps

# 4. Run
make run
# Open http://localhost:8080
```

---

## API Contract

### `POST /chat` — SSE streaming

**Request body** (`Content-Type: application/json`):

```json
{
  "message": "how is he with uber",
  "history": [
    {"role": "user",      "content": "tell me about his projects"},
    {"role": "assistant", "content": "Avyakt built ..."}
  ]
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `message` | `string` | yes | The current user message. **NOT** a `messages` array — this is a flat string. |
| `history` | `[]{"role", "content"}` | no | Prior turns. Omit or send `[]` for a fresh conversation. Max 5 turns kept (older are truncated). |

**Response**: `text/event-stream`. Each line is `data: <json>\n\n`.

```json
{"token": "Avyakt has a strong..."}
{"token": " connection with Uber"}
{"done": true, "id": "d883c97c-073b-4bfe-9b41-5650a022a566"}
```

All possible event shapes:

| Shape | When |
|---|---|
| `{"token": "..."}` | Streaming token from LLM |
| `{"done": true, "id": "<uuid>"}` | Stream complete — save `id` to submit a rating |
| `{"error": "..."}` | Infrastructure failure (embed/retrieve/LLM) |
| `{"error": "rate_limited", "rate_limited": true}` | Voyage AI 429 — retry after ~20 s |

Check `event.rate_limited === true` to show a retry prompt instead of a generic error.

---

### `POST /rating`

```json
{"interaction_id": "<uuid from done event>", "rating": 5}
```

`rating`: `1` = thumbs down, `5` = thumbs up. Returns `204 No Content`.

---

### `GET /metrics`

```json
{"total_conversations": 42, "avg_rating": 4.2, "recent_questions": ["..."]}
```

---

### `GET /health`

```json
{"status": "ok"}
```

---

### curl examples

```bash
# Health check
curl https://rag-chatbot-qge9.onrender.com/health

# Chat (fresh conversation)
curl -N -X POST https://rag-chatbot-qge9.onrender.com/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "how is he with uber", "history": []}'

# Chat (with history)
curl -N -X POST https://rag-chatbot-qge9.onrender.com/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "what tech did he use there",
    "history": [{"role":"user","content":"how is he with uber"},{"role":"assistant","content":"Avyakt has two Uber internships..."}]
  }'

# Submit rating
curl -X POST https://rag-chatbot-qge9.onrender.com/rating \
  -H "Content-Type: application/json" \
  -d '{"interaction_id": "<id from done event>", "rating": 5}'
```

---

## Rate Limits (free tier)

| Service | Limit | Impact |
|---|---|---|
| Voyage AI | 3 RPM | Embedding calls — one per message |
| Groq Llama 3.3 70B | 1K RPD / 12K TPM | LLM generation |

With hybrid retrieval the context is ~350–500 tokens (5 chunks) vs the previous 1,100-token full-resume stopgap — roughly 2× more TPM headroom per request.

**Planned**: in-memory embedding cache (5-min TTL) to absorb burst 429s without hitting the API.

---

## LLM Switching

Switch providers with zero code changes:

```bash
LLM_PROVIDER=groq   make run   # Groq Llama 3.3 70B (default)
LLM_PROVIDER=gemini make run   # Gemini 1.5 Flash (fallback)
```

---

## Deployment (Render)

Render reads `render.yaml` automatically on push.

Required env vars in the Render dashboard:

| Variable | Description |
|---|---|
| `NEON_DATABASE_URL` | Neon PostgreSQL connection string |
| `VOYAGE_API_KEY` | Voyage AI API key |
| `GROQ_API_KEY` | Groq API key |
| `GEMINI_API_KEY` | Google AI Studio key (optional fallback) |
| `ALLOWED_ORIGINS` | Portfolio website URL for CORS |
| `SIMILARITY_THRESHOLD` | Topic filter threshold (default: `0.80`) |

Push to `main` → auto-deploy. Health check at `GET /health`.

---

## Related Repos

- [`portfolio-website`](https://github.com/AG-AGENT47) — frontend that embeds this chatbot
- [`portfolio-store`](https://github.com/AG-AGENT47) — Neon DB schema + knowledge base ingestion
