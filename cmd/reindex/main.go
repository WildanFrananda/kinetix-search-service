package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/WildanFrananda/kinetix-search-service/internal/catalogclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/postgres"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/system"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("reindex failed", "err", err, "kind", search.KindOf(err).String())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return search.Errf(search.KindDependencyUnavailable, "reindex.database", err, "connecting")
	}
	defer pool.Close()

	catalog, err := catalogclient.Dial(catalogclient.Settings{
		Endpoint:   cfg.CatalogEndpoint,
		PKIDir:     cfg.PKIDir,
		ServerName: cfg.CatalogServerName,
		Deadline:   cfg.CatalogDeadline,
	})
	if err != nil {
		return err
	}
	defer func() { _ = catalog.Close() }()

	engine := typesense.Connect(typesense.Settings{
		URL:     cfg.TypesenseURL,
		APIKey:  cfg.TypesenseAPIKey,
		Timeout: cfg.TypesenseTimeout,
	})

	index := typesense.NewIndex(engine, search.Products, typesense.EncodeProduct)
	checkpoint := postgres.NewCheckpoint(pool)

	reindexer := indexing.NewReindexer(
		search.Products,
		catalog,
		index,
		checkpoint,
		system.Clock{},
		log,
		cfg.SyncPageSize,
	)

	started := time.Now()
	log.InfoContext(ctx, "reindex starting", "collection", string(search.Products))

	if err := reindexer.Run(ctx); err != nil {
		return err
	}

	log.InfoContext(ctx, "reindex finished", "took", time.Since(started).Round(time.Millisecond))
	return nil
}
