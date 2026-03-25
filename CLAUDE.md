# RAG Chatbot — Claude Code Context

## What This Is
Go RAG service — part 2 of a 3-repo portfolio system (website → **chatbot** → store).
Exposes `/chat` (SSE streaming), `/rating`, `/metrics`, `/health`.

## Key Architecture Decisions
- **LLM interface** (`internal/llm/llm.go`): swap providers via `LLM_PROVIDER` env var — no code changes
- **StreamEvent channel** (`<-chan StreamEvent`): carries `{Text, Err}` — mid-stream errors don't get swallowed silently
- **context.WithoutCancel** pattern: if you add async logging, use this — `r.Context()` is canceled when SSE closes
- **Query contextualization** (`pipeline.go`): last user message is prepended before embedding to resolve pronouns
- **Topic filter** (`pipeline.go`): cosine distance > `SIMILARITY_THRESHOLD` (default 0.75) returns redirect msg without LLM call; bypassed for short greetings (≤2 words)
- **XML tags in system prompt**: `<rules>` / `<context>` for stronger injection resistance vs markdown `---`

## Module
`github.com/AG-AGENT47/rag-chatbot`

## Running Locally
```bash
cp .env.example .env   # fill in secrets
make deps              # go mod tidy + download
make run               # starts on :8080, serves frontend/index.html at /
```

## External Dependencies
- **Neon PostgreSQL** (portfolio-store) — pgvector cosine search on `knowledge_base`
- **Voyage AI** `voyage-3-lite` — MUST match the model that built the stored embeddings
- **Gemini 2.5 Flash** — default LLM (1500 free req/day at aistudio.google.com)
- **Groq Llama 3.3 70B** — fallback LLM (set `LLM_PROVIDER=groq`)

## Deployment
Render free tier — `render.yaml` in repo root. Push to GitHub → auto-deploy.
Health check: `GET /health` → `{"status":"ok"}`
