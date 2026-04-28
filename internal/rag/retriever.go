package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// Chunk is a retrieved knowledge base entry with its similarity distance.
type Chunk struct {
	Content    string
	Source     string
	SourceType string
	Metadata   map[string]interface{}
	Distance   float64
}

// Retriever searches the knowledge base using pgvector cosine similarity.
type Retriever struct {
	pool *pgxpool.Pool
}

// NewRetriever creates a Retriever using the given pgxpool.
// The pool must have pgvector types registered (see db.NewPool).
func NewRetriever(pool *pgxpool.Pool) *Retriever {
	return &Retriever{pool: pool}
}

// TopK returns the k most semantically similar chunks for the given embedding.
// Results are ordered by cosine distance ascending (most similar first).
func (r *Retriever) TopK(ctx context.Context, embedding []float32, k int) ([]Chunk, error) {
	return r.vectorSearch(ctx, embedding, k)
}

// HybridTopK combines vector and full-text search via Reciprocal Rank Fusion.
// It fetches k*2 candidates from each leg, merges by RRF score, and returns top k.
func (r *Retriever) HybridTopK(ctx context.Context, embedding []float32, query string, k int) ([]Chunk, error) {
	vecs, err := r.vectorSearch(ctx, embedding, k*2)
	if err != nil {
		return nil, err
	}

	fts, err := r.fullTextSearch(ctx, query, k*2)
	if err != nil {
		// FTS is best-effort: fall back to pure vector results on failure.
		end := k
		if len(vecs) < end {
			end = len(vecs)
		}
		return vecs[:end], nil
	}

	return rrfMerge(vecs, fts, k), nil
}

// vectorSearch returns the k nearest chunks by cosine distance.
func (r *Retriever) vectorSearch(ctx context.Context, embedding []float32, k int) ([]Chunk, error) {
	vec := pgvector.NewVector(embedding)

	rows, err := r.pool.Query(ctx,
		`SELECT content, source, source_type, metadata, (embedding <=> $1) AS distance
		 FROM knowledge_base
		 ORDER BY distance ASC
		 LIMIT $2`,
		vec, k,
	)
	if err != nil {
		return nil, fmt.Errorf("retriever: vector query: %w", err)
	}
	defer rows.Close()

	return scanChunks(rows)
}

// fullTextSearch returns the k highest-ranked chunks by Postgres full-text search.
// Returned chunks have Distance=0 (no vector distance available).
func (r *Retriever) fullTextSearch(ctx context.Context, query string, k int) ([]Chunk, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT content, source, source_type, metadata,
		        ts_rank_cd(to_tsvector('english', content), plainto_tsquery('english', $1)) AS rank
		 FROM knowledge_base
		 WHERE to_tsvector('english', content) @@ plainto_tsquery('english', $1)
		 ORDER BY rank DESC
		 LIMIT $2`,
		query, k,
	)
	if err != nil {
		return nil, fmt.Errorf("retriever: fts query: %w", err)
	}
	defer rows.Close()

	return scanChunks(rows)
}

type scannable interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// scanChunks reads rows into a Chunk slice. Each row must have columns:
// content, source, source_type, metadata (JSONB), distance_or_rank (float64).
func scanChunks(rows scannable) ([]Chunk, error) {
	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		var rawMeta []byte
		if err := rows.Scan(&c.Content, &c.Source, &c.SourceType, &rawMeta, &c.Distance); err != nil {
			return nil, fmt.Errorf("retriever: scan: %w", err)
		}
		if rawMeta != nil {
			_ = json.Unmarshal(rawMeta, &c.Metadata)
		}
		chunks = append(chunks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("retriever: iterate: %w", err)
	}
	return chunks, nil
}

// rrfMerge combines two ranked chunk lists using Reciprocal Rank Fusion and
// returns the top k unique chunks by merged score. Vector distances are
// preserved for chunks that appeared in the vector list.
func rrfMerge(vecs, fts []Chunk, k int) []Chunk {
	const rrfK = 60

	scores := make(map[string]float64)
	distances := make(map[string]float64)
	byContent := make(map[string]Chunk)

	addList := func(list []Chunk) {
		for i, c := range list {
			scores[c.Content] += 1.0 / float64(rrfK+i+1)
			byContent[c.Content] = c
		}
	}

	addList(vecs)
	for _, c := range vecs {
		distances[c.Content] = c.Distance
	}
	addList(fts)

	keys := make([]string, 0, len(scores))
	for key := range scores {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return scores[keys[i]] > scores[keys[j]]
	})

	if k > len(keys) {
		k = len(keys)
	}
	result := make([]Chunk, 0, k)
	for _, key := range keys[:k] {
		c := byContent[key]
		c.Distance = distances[key] // 0 for FTS-only chunks
		result = append(result, c)
	}
	return result
}
