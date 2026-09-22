package main

import (
	"context"
	"errors"
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
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("syncd stopped", "err", err, "kind", search.KindOf(err).String())
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
		return search.Errf(search.KindDependencyUnavailable, "syncd.database", err, "connecting")
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

	syncer := indexing.NewSyncer(
		search.Products,
		catalog,
		typesense.NewIndex(engine, search.Products, typesense.EncodeProduct),
		postgres.NewCheckpoint(pool),
		log,
		cfg.SyncPageSize,
	)

	log.InfoContext(ctx, "syncd started",
		"collection", string(search.Products), "interval", cfg.SyncInterval.String())

	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	for {
		runOnce(ctx, syncer, log)

		select {
		case <-ctx.Done():
			log.InfoContext(ctx, "syncd draining")
			return nil
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, syncer *indexing.Syncer[search.ProductDoc], log *slog.Logger) {
	err := syncer.Once(ctx)
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}

	log.ErrorContext(ctx, "sync run failed; the cursor has not moved and the index is now older",
		"err", err, "kind", search.KindOf(err).String())
}
