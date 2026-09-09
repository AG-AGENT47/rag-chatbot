# rag-chatbot

Go RAG (Retrieval-Augmented Generation) service for Avyakt Garg's portfolio. Part 2 of a 3-repo system:

```
portfolio-website  →  rag-chatbot (you are here)  →  portfolio-store
   (Next.js/Vercel)      (Go / Render)                (Neon Postgres + pgvector)
```

It answers visitor questions about Avyakt by embedding the query, retrieving the
most relevant chunks from the `knowledge_base` table with hybrid vector +
full-text search, and streaming an LLM answer back over SSE.

---

## Stack (as built)

| Concern | Choice |
|---|---|
| Language / router | Go 1.21, `chi/v5` |
| DB driver | `pgx/v5` + `pgvector-go` (Neon Postgres, `knowledge_base` = 46 chunks, `embedding VECTOR(768)`) |
| Query embeddings | **Google Gemini `gemini-embedding-001` @ 768-dim** (`taskType=RETRIEVAL_QUERY`). `EMBED_PROVIDER=gemini` (default). |
| Embedding rollback | Voyage `voyage-3-lite` @ 512-dim — `EMBED_PROVIDER=voyage`, kept only for rollback (see [Rate limits](#rate-limits-free-tier)) |
| LLM | **Groq `openai/gpt-oss-120b`** (`LLM_PROVIDER=groq`, default). Model overridable with `GROQ_MODEL`. |
| LLM fallback | Google `gemini-1.5-flash-latest` — `LLM_PROVIDER=gemini` |
| Hosting | Render free tier (`render.yaml`), region `oregon` |

> **History:** the chatbot originally embedded with Voyage `voyage-3-lite` and
> generated with Groq's `llama-3.3-70b-versatile`. Voyage's free tier drops to
> 3 requests/min once the trial credit is spent — that was the "only a few free
> answers" outage — and Groq later removed every Llama chat model from the
> account. Both were swapped out; the old paths still build for rollback only.

---

## Endpoints

| Method | Path | Notes |
|---|---|---|
| `POST` | `/chat` | RAG answer, streamed as SSE. IP-rate-limited to 10 req/min (HTTP 429 `{"error":"Too many requests…"}` when exceeded). |
| `POST` | `/rating` | Store 👍/👎 for a past answer. |
| `GET` | `/metrics` | Aggregated interaction stats for the website dashboard. |
| `GET` | `/health` | `{"status":"ok"}` — Render health check + website status pill. |
| `GET` | `/` | Serves `frontend/index.html`, a minimal test chat UI. |

CORS: `CORSMiddleware` emits `Access-Control-Allow-Origin` when the request
`Origin` is an **exact** match in the comma-separated `ALLOWED_ORIGINS` list, or
is any `https://<sub>.vercel.app` origin (so the production site and every Vercel
preview deploy work without re-listing per-deploy URLs). `OPTIONS` preflights
short-circuit to `204`.

---

## Pipeline (`internal/rag/pipeline.go`)

`POST /chat` runs, in order:

1. **Guardrails** (`internal/guardrails`, in the handler before the pipeline) —
   reject messages over 1000 chars or containing an injection substring
   (`ignore previous`, `system prompt`, `act as`, …). → HTTP 400.
2. **History truncation** (handler) — keep the last 5 turns, cap total content at
   2000 chars.
3. **Query contextualization** — prepend the previous user message to the current
   one before embedding, so "did he use Go there?" resolves against "what did
   Avyakt do at Uber?".
4. **Embed** the contextualized query (Gemini, `RETRIEVAL_QUERY`, 768-dim).
5. **Hybrid retrieve** (`retriever.HybridTopK`, k=5): pgvector cosine (`<=>`) and
   Postgres full-text (`to_tsvector`/`plainto_tsquery`, ranked by `ts_rank_cd`),
   each fetching `k*2` candidates, merged by **Reciprocal Rank Fusion** (rrfK=60).
   The FTS leg is best-effort — on FTS error it falls back to pure vector results.
6. **Retrieval log** (`logRetrieval`) — one line per request: chunk source labels
   and vector distances (`d=0.00` = FTS-only hit), so retrieval quality is
   auditable from the Render logs.
7. **Topic filter** — if the *minimum* vector distance across the retrieved
   chunks exceeds `SIMILARITY_THRESHOLD` (default **0.75**), return the redirect
   line without calling the LLM. Uses the min distance, not `chunks[0]`, because
   RRF reorders and FTS-only chunks carry `Distance=0`. Skipped when the message
   is ≤ 2 words (greetings).
8. **Build the system prompt** — retrieved chunks joined with
   `[Source: company=…, role=…, date=…]` headers pulled from each chunk's
   `metadata` JSONB. Prompt is XML-tagged (`<rules>` / `<context>`) and asks for
   plain prose, 2–4 sentences, no markdown.
9. **Stream** tokens from the LLM over SSE.
10. **Log the interaction** with `context.WithoutCancel(ctx)` + a 5s timeout, so a
    client disconnecting mid-stream doesn't drop the row (or the rating id it
    returns).

---

## SSE contract (`POST /chat`)

`Content-Type: text/event-stream`; each frame is `data: <json>\n\n`.

| Frame | Meaning |
|---|---|
| `{"token": "…"}` | One streamed token. |
| `{"done": true, "id": "<uuid>"}` | Stream finished. `id` is the interaction id for `POST /rating` (empty string if the log write failed). |
| `{"error": "…"}` | Infrastructure failure (embed / retrieve / LLM / stream). |
| `{"error": "rate_limited", "rate_limited": true}` | The **embedding** API returned HTTP 429. Retry in ~20s. |

There is no `[DONE]` sentinel and no `content` field — the token field is
`token`. A separate transport-level HTTP `429` (not an SSE frame) comes from the
per-IP rate limiter.

### Request body

```json
{
  "message": "what did he do at Uber",
  "history": [
    {"role": "user", "content": "tell me about his projects"},
    {"role": "assistant", "content": "Avyakt built …"}
  ]
}
```

`message` is a **flat string**, not a `messages` array. `history` is optional
(send `[]` or omit for a fresh conversation).

### Other endpoints

```
POST /rating   {"interaction_id": "<uuid>", "rating": 5}   → 204   (rating must be 1 or 5)
GET  /metrics  → {"total_conversations": N, "avg_rating": 4.2|null, "recent_questions": [ … ]}
GET  /health   → {"status": "ok"}
```

---

## Configuration

Copy `.env.example` to `.env` for local runs; set the same keys in the Render
dashboard for deploy.

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `NEON_DATABASE_URL` | **yes** | — | Neon pooled connection string (from `portfolio-store`). |
| `EMBED_PROVIDER` | no | `gemini` | `gemini` or `voyage` (rollback). Must match the model that built the stored vectors. |
| `GEMINI_API_KEY` | for gemini | — | Google AI Studio key. Used for Gemini **embeddings** and the Gemini **LLM** fallback. Free, no card. |
| `LLM_PROVIDER` | no | `groq` | `groq` or `gemini`. |
| `GROQ_API_KEY` | for groq | — | Groq API key. |
| `GROQ_MODEL` | no | `openai/gpt-oss-120b` | Override the Groq model (Groq rotates its catalogue — check `GET https://api.groq.com/openai/v1/models`). |
| `VOYAGE_API_KEY` | for voyage | — | Only when `EMBED_PROVIDER=voyage`. |
| `ALLOWED_ORIGINS` | for non-vercel.app origins | `http://localhost:3000` | Comma-separated exact-match CORS allowlist. Any `*.vercel.app` origin is allowed automatically; add a custom domain here. |
| `PORT` | no | `8080` | — |
| `SIMILARITY_THRESHOLD` | no | `0.75` | Topic-filter cosine-distance cutoff. |

---

## Local development

```bash
cp .env.example .env      # fill in NEON_DATABASE_URL, GEMINI_API_KEY, GROQ_API_KEY, ALLOWED_ORIGINS
make deps                 # go mod tidy + download
make run                  # serves on :8080, test UI at http://localhost:8080/
make test                 # go test ./...
make lint                 # golangci-lint (brew install golangci-lint)
make build                # -> bin/server
```

`curl` examples:

```bash
curl http://localhost:8080/health

curl -N -X POST http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{"message": "what did he do at Uber", "history": []}'

curl -X POST http://localhost:8080/rating \
  -H 'Content-Type: application/json' \
  -d '{"interaction_id": "<id from the done frame>", "rating": 5}'
```

---

## Rate limits (free tier)

| Service | Free-tier limit | Role |
|---|---|---|
| Gemini `gemini-embedding-001` | ~100 RPM, no card | **Current embedder** — comfortable headroom for a portfolio. |
| Groq `openai/gpt-oss-120b` | ~1,000 req/day | **Current LLM.** |
| Voyage `voyage-3-lite` | 3 RPM once trial credit is spent | Old embedder — the cause of the original outage. Rollback only. |
| Gemini `gemini-1.5-flash-latest` | low RPD | LLM fallback only. |

Not yet built: an in-memory embedding cache (5-min TTL) to absorb repeat queries.

---

## Deployment (Render)

Render reads `render.yaml` on push to `main` and auto-deploys. The non-secret
values (`EMBED_PROVIDER`, `LLM_PROVIDER`, `PORT`, `SIMILARITY_THRESHOLD`) are in
`render.yaml`; everything marked `sync: false` there must be set by hand in the
dashboard: `NEON_DATABASE_URL`, `GEMINI_API_KEY`, `GROQ_API_KEY`,
`ALLOWED_ORIGINS` (and `VOYAGE_API_KEY` only if rolling back).

Free-tier dyno sleeps after 15 min idle; the first request then takes 30–60s
(`portfolio-website` shows a "waking" state for this). Health check: `GET /health`.

Live: <https://rag-chatbot-qge9.onrender.com>

---

## Project layout

```
cmd/server/main.go            entry point, config, router
internal/
  api/handlers.go             /chat /rating /metrics /health, SSE helpers, history truncation
  api/middleware.go           per-IP rate limiter (10/min) + CORS allowlist
  api/models.go               request/response types + ssePayload
  db/db.go                    pgxpool (pgvector types registered), interactions + metrics queries
  guardrails/guardrails.go    length + injection-substring checks
  llm/llm.go                  LLM interface (StreamEvent channel), provider switch
  llm/groq.go                 Groq provider — default openai/gpt-oss-120b
  llm/gemini.go               Gemini provider — gemini-1.5-flash-latest (fallback)
  rag/embedder.go             Embedder interface + gemini (768d) and voyage (512d) impls, ErrRateLimit
  rag/retriever.go            vector search, FTS, RRF merge (HybridTopK)
  rag/pipeline.go             the pipeline above + buildContext + logRetrieval
frontend/index.html           minimal test chat UI, served at /
render.yaml  Makefile  .env.example
```

---

## Related repos

- `portfolio-website` — Next.js frontend, embeds this chat.
- `portfolio-store` — Neon schema, seeds, and the `embed.py` that builds `knowledge_base` vectors.
