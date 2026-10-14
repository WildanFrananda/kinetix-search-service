package wiring

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/postgres"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type poolParams struct {
	dig.In
	Config config.Config
}

func providePool(p poolParams) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), p.Config.DatabaseURL)
	if err != nil {
		return nil, search.Errf(search.KindDependencyUnavailable, "wiring.database", err, "connecting")
	}

	return pool, nil
}

type storeParams struct {
	dig.In
	Pool *pgxpool.Pool
}

type storeResult struct {
	dig.Out
	Checkpoint search.Checkpoint
	Lock       search.Lock
}

func provideStore(p storeParams) storeResult {
	return storeResult{
		Checkpoint: postgres.NewCheckpoint(p.Pool),
		Lock:       postgres.NewLock(p.Pool),
	}
}
