package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tsapi "github.com/typesense/typesense-go/v3/typesense"

	"github.com/WildanFrananda/kinetix-search-service/internal/catalogclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/identityclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/mesh"
	"github.com/WildanFrananda/kinetix-search-service/internal/orderclient"
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

func wanted(arg string) ([]search.Collection, error) {
	switch arg {
	case string(search.Products):
		return []search.Collection{search.Products}, nil
	case string(search.Merchants):
		return []search.Collection{search.Merchants}, nil
	case string(search.Orders):
		return []search.Collection{search.Orders}, nil
	case "all":
		return []search.Collection{search.Products, search.Merchants, search.Orders}, nil
	default:
		return nil, search.Errf(
			search.KindMalformedQuery,
			"reindex.arguments",
			nil,
			"usage: reindex <products|merchants|orders|all>, not %q",
			arg,
		)
	}
}

func run(log *slog.Logger) error {
	if len(os.Args) != 2 {
		return search.Errf(
			search.KindMalformedQuery,
			"reindex.arguments",
			nil,
			"usage: reindex <products|merchants|orders|all>",
		)
	}

	collections, err := wanted(os.Args[1])

	if err != nil {
		return err
	}

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

	engine := typesense.Connect(typesense.Settings{
		URL:     cfg.TypesenseURL,
		APIKey:  cfg.TypesenseAPIKey,
		Timeout: cfg.TypesenseTimeout,
	})

	checkpoint := postgres.NewCheckpoint(pool)

	for _, collection := range collections {
		started := time.Now()
		log.InfoContext(ctx, "reindex starting", "collection", string(collection))

		if err := rebuild(ctx, collection, cfg, engine, checkpoint, log); err != nil {
			return err
		}

		log.InfoContext(
			ctx,
			"reindex finished",
			"collection",
			string(collection),
			"took",
			time.Since(started).Round(time.Millisecond),
		)
	}

	return nil
}

func rebuild(
	ctx context.Context,
	collection search.Collection,
	cfg config.Config,
	engine *tsapi.Client,
	checkpoint search.Checkpoint,
	log *slog.Logger,
) error {
	switch collection {
	case search.Products:
		source, err := catalogclient.Dial(mesh.Settings{
			Endpoint:   cfg.CatalogEndpoint,
			PKIDir:     cfg.PKIDir,
			ServerName: cfg.CatalogServerName,
			Deadline:   cfg.CatalogDeadline,
		})
		if err != nil {
			return err
		}
		defer func() { _ = source.Close() }()

		return indexing.NewReindexer(
			collection,
			source,
			typesense.NewIndex(engine, collection, typesense.EncodeProduct),
			checkpoint,
			system.Clock{},
			log,
			cfg.SyncPageSize,
		).Run(ctx)

	case search.Merchants:
		source, err := identityclient.Dial(mesh.Settings{
			Endpoint:   cfg.IdentityEndpoint,
			PKIDir:     cfg.PKIDir,
			ServerName: cfg.IdentityServerName,
			Deadline:   cfg.IdentityDeadline,
		})
		if err != nil {
			return err
		}
		defer func() {
			_ = source.Close()
		}()

		return indexing.NewReindexer(
			collection,
			source,
			typesense.NewIndex(engine, collection, typesense.EncodeMerchant),
			checkpoint,
			system.Clock{},
			log,
			cfg.SyncPageSize,
		).Run(ctx)

	case search.Orders:
		source, err := orderclient.Dial(mesh.Settings{
			Endpoint:   cfg.OrderEndpoint,
			PKIDir:     cfg.PKIDir,
			ServerName: cfg.OrderServerName,
			Deadline:   cfg.OrderDeadline,
		})
		if err != nil {
			return err
		}
		defer func() {
			_ = source.Close()
		}()

		return indexing.NewReindexer(
			collection,
			source,
			typesense.NewIndex(engine, collection, typesense.EncodeOrder),
			checkpoint,
			system.Clock{},
			log,
			cfg.SyncPageSize,
		).Run(ctx)

	default:
		return search.Errf(
			search.KindMalformedQuery,
			"reindex.rebuild",
			nil,
			"no source for collection %q",
			string(collection),
		)
	}
}
