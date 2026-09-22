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
	"github.com/WildanFrananda/kinetix-search-service/internal/identityclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
	"github.com/WildanFrananda/kinetix-search-service/internal/orderclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/postgres"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

type collectionSync struct {
	collection search.Collection
	once       func(context.Context) error
}

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

	catalog, err := catalogclient.Dial(mesh.Settings{
		Endpoint:   cfg.CatalogEndpoint,
		PKIDir:     cfg.PKIDir,
		ServerName: cfg.CatalogServerName,
		Deadline:   cfg.CatalogDeadline,
	})
	if err != nil {
		return err
	}
	defer func() { _ = catalog.Close() }()

	identity, err := identityclient.Dial(mesh.Settings{
		Endpoint:   cfg.IdentityEndpoint,
		PKIDir:     cfg.PKIDir,
		ServerName: cfg.IdentityServerName,
		Deadline:   cfg.IdentityDeadline,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = identity.Close()
	}()

	orders, err := orderclient.Dial(mesh.Settings{
		Endpoint:   cfg.OrderEndpoint,
		PKIDir:     cfg.PKIDir,
		ServerName: cfg.OrderServerName,
		Deadline:   cfg.OrderDeadline,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = orders.Close()
	}()

	engine := typesense.Connect(typesense.Settings{
		URL:     cfg.TypesenseURL,
		APIKey:  cfg.TypesenseAPIKey,
		Timeout: cfg.TypesenseTimeout,
	})

	checkpoint := postgres.NewCheckpoint(pool)

	products := indexing.NewSyncer(
		search.Products,
		catalog,
		typesense.NewIndex(engine, search.Products, typesense.EncodeProduct),
		checkpoint,
		log,
		cfg.SyncPageSize,
	)

	merchants := indexing.NewSyncer(
		search.Merchants,
		identity,
		typesense.NewIndex(engine, search.Merchants, typesense.EncodeMerchant),
		checkpoint,
		log,
		cfg.SyncPageSize,
	)

	placed := indexing.NewSyncer(
		search.Orders,
		orders,
		typesense.NewIndex(engine, search.Orders, typesense.EncodeOrder),
		checkpoint,
		log,
		cfg.SyncPageSize,
	)

	runs := []collectionSync{
		{search.Products, products.Once},
		{search.Merchants, merchants.Once},
		{search.Orders, placed.Once},
	}

	log.InfoContext(ctx, "syncd started",
		"collections", len(runs), "interval", cfg.SyncInterval.String())

	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	for {
		for _, r := range runs {
			runOnce(ctx, r, log)
		}

		select {
		case <-ctx.Done():
			log.InfoContext(ctx, "syncd draining")
			return nil
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, r collectionSync, log *slog.Logger) {
	err := r.once(ctx)

	if err == nil {
		return
	}

	if errors.Is(err, context.Canceled) {
		return
	}

	log.ErrorContext(ctx, "sync run failed; the cursor has not moved and the index is now older",
		"collection", string(r.collection),
		"err", err, "kind", search.KindOf(err).String())
}
