package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/catalogclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/identityclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/orderclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/wiring"
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	container, err := wiring.Indexing(log)
	if err != nil {
		return err
	}

	var pool *pgxpool.Pool

	err = container.Invoke(func(p *pgxpool.Pool) {
		pool = p
	})
	if err != nil {
		return dig.RootCause(err)
	}
	defer pool.Close()

	for _, collection := range collections {
		started := time.Now()

		log.InfoContext(ctx, "reindex starting", "collection", string(collection))

		if err := rebuild(ctx, container, collection); err != nil {
			return dig.RootCause(err)
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

func rebuild(ctx context.Context, container *dig.Container, collection search.Collection) error {
	switch collection {
	case search.Products:
		return reindex[*catalogclient.Client, search.ProductDoc](ctx, container)
	case search.Merchants:
		return reindex[*identityclient.Client, search.MerchantDoc](ctx, container)
	case search.Orders:
		return reindex[*orderclient.Client, search.OrderDoc](ctx, container)
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

func reindex[C io.Closer, D any](ctx context.Context, container *dig.Container) error {
	return container.Invoke(func(r rebuilding[C, D]) error {
		defer func() {
			_ = r.Client.Close()
		}()

		return r.Reindexer.Run(ctx)
	})
}
