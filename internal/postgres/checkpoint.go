package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Checkpoint struct {
	q *Queries
}

var _ search.Checkpoint = (*Checkpoint)(nil)

func NewCheckpoint(pool *pgxpool.Pool) *Checkpoint {
	return &Checkpoint{q: New(pool)}
}

func (c *Checkpoint) Load(ctx context.Context, col search.Collection) (search.Cursor, error) {
	row, err := c.q.LoadCheckpoint(ctx, string(col))
	if errors.Is(err, pgx.ErrNoRows) {
		return search.Cursor{}, nil
	}
	if err != nil {
		return search.Cursor{}, search.Errf(
			search.KindDependencyUnavailable,
			"postgres.LoadCheckpoint",
			err,
			"reading the %s cursor",
			col,
		)
	}

	cursor := search.Cursor{LastID: row.LastID}
	if row.UpdatedThrough.Valid {
		cursor.UpdatedThrough = row.UpdatedThrough.Time.UTC()
	}
	return cursor, nil
}

func (c *Checkpoint) Save(ctx context.Context, col search.Collection, cur search.Cursor) error {
	err := c.q.SaveCheckpoint(ctx, SaveCheckpointParams{
		Collection:     string(col),
		UpdatedThrough: pgtype.Timestamptz{Time: cur.UpdatedThrough, Valid: true},
		LastID:         cur.LastID,
	})
	if err != nil {
		return search.Errf(
			search.KindDependencyUnavailable,
			"postgres.SaveCheckpoint",
			err,
			"saving the %s cursor",
			col,
		)
	}
	return nil
}
