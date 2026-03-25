package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

// NewPool creates a pgxpool with pgvector types registered on every connection.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	// Register pgvector types so pgx can encode/decode vector columns.
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return pool, nil
}

// DB wraps pgxpool for interaction-specific operations.
type DB struct {
	pool *pgxpool.Pool
}

// New creates a DB from an existing pool.
func New(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// Metrics is the aggregated data for the portfolio dashboard.
type Metrics struct {
	TotalConversations int
	AvgRating          *float64
	RecentQuestions    []string
}

// InsertInteraction logs a new chatbot interaction and returns its UUID string.
func (d *DB) InsertInteraction(ctx context.Context, question, answer string, latencyMs int64) (string, error) {
	var id string
	err := d.pool.QueryRow(ctx,
		`INSERT INTO interactions (question, answer, latency_ms)
		 VALUES ($1, $2, $3)
		 RETURNING id::text`,
		question, answer, latencyMs,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("db: insert interaction: %w", err)
	}
	return id, nil
}

// UpdateRating sets the rating for an existing interaction.
func (d *DB) UpdateRating(ctx context.Context, interactionID string, rating int) error {
	tag, err := d.pool.Exec(ctx,
		`UPDATE interactions SET rating = $1 WHERE id = $2::uuid`,
		rating, interactionID,
	)
	if err != nil {
		return fmt.Errorf("db: update rating: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("db: interaction %q not found", interactionID)
	}
	return nil
}

// GetMetrics returns aggregated interaction metrics for the dashboard.
func (d *DB) GetMetrics(ctx context.Context) (Metrics, error) {
	var m Metrics

	if err := d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM interactions`,
	).Scan(&m.TotalConversations); err != nil {
		return m, fmt.Errorf("db: get total: %w", err)
	}

	if err := d.pool.QueryRow(ctx,
		`SELECT AVG(rating::float) FROM interactions WHERE rating IS NOT NULL`,
	).Scan(&m.AvgRating); err != nil {
		return m, fmt.Errorf("db: get avg rating: %w", err)
	}

	rows, err := d.pool.Query(ctx,
		`SELECT question FROM interactions ORDER BY created_at DESC LIMIT 5`,
	)
	if err != nil {
		return m, fmt.Errorf("db: get recent questions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err != nil {
			return m, fmt.Errorf("db: scan question: %w", err)
		}
		m.RecentQuestions = append(m.RecentQuestions, q)
	}
	if err := rows.Err(); err != nil {
		return m, fmt.Errorf("db: iterate questions: %w", err)
	}

	return m, nil
}
