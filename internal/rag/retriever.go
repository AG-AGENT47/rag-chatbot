package rag

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// Chunk is a retrieved knowledge base entry with its similarity distance.
type Chunk struct {
	Content    string
	Source     string
	SourceType string
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
	vec := pgvector.NewVector(embedding)

	rows, err := r.pool.Query(ctx,
		`SELECT content, source, source_type,
		        (embedding <=> $1) AS distance
		 FROM knowledge_base
		 ORDER BY distance ASC
		 LIMIT $2`,
		vec, k,
	)
	if err != nil {
		return nil, fmt.Errorf("retriever: query: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.Content, &c.Source, &c.SourceType, &c.Distance); err != nil {
			return nil, fmt.Errorf("retriever: scan: %w", err)
		}
		chunks = append(chunks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("retriever: iterate: %w", err)
	}

	return chunks, nil
}
