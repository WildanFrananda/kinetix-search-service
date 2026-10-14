package wiring

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/httpapi"
	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type httpParams struct {
	dig.In
	Config   config.Config
	Pool     *pgxpool.Pool
	Products *querying.Finder[search.ProductDoc]
	Index    search.Index[search.ProductDoc]
}

func provideHTTP(p httpParams) *http.Server {
	ready := func(ctx context.Context) error {
		if err := p.Pool.Ping(ctx); err != nil {
			return search.Errf(search.KindDependencyUnavailable, "searchd.ready", err, "database")
		}

		live, err := p.Index.Live(ctx)
		if err != nil {
			return err
		}

		if live == "" {
			return search.Errf(
				search.KindIndexUnavailable,
				"searchd.ready",
				nil,
				"no generation has been promoted yet; run cmd/reindex",
			)
		}

		return nil
	}

	return &http.Server{
		Addr:              p.Config.HTTPAddr,
		Handler:           httpapi.NewRouter(p.Products, ready),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
