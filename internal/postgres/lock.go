package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

const releaseTimeout = 5 * time.Second

type Lock struct {
	pool *pgxpool.Pool
}

var _ search.Lock = (*Lock)(nil)

func NewLock(pool *pgxpool.Pool) *Lock {
	return &Lock{pool: pool}
}

func (l *Lock) Hold(ctx context.Context, col search.Collection) (func(), error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"postgres.Lock",
			err,
			"opening a transaction for the %s lock",
			col,
		)
	}

	release := func() {
		done, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
		defer cancel()

		_ = tx.Rollback(done)
	}

	if err := New(tx).HoldCollection(ctx, string(col)); err != nil {
		release()

		return nil, search.Errf(
			search.KindDependencyUnavailable,
			"postgres.Lock",
			err,
			"waiting for the %s lock",
			col,
		)
	}

	return release, nil
}
