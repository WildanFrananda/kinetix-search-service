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
	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/catalogclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/identityclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/orderclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/wiring"
)

type syncd struct {
	dig.In
	Config    config.Config
	Pool      *pgxpool.Pool
	Catalog   *catalogclient.Client
	Identity  *identityclient.Client
	Orders    *orderclient.Client
	Products  *indexing.Syncer[search.ProductDoc]
	Merchants *indexing.Syncer[search.MerchantDoc]
	Placed    *indexing.Syncer[search.OrderDoc]
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("syncd stopped", "err", err, "kind", search.KindOf(err).String())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	container, err := wiring.Indexing(log)
	if err != nil {
		return err
	}

	err = container.Invoke(func(d syncd) error {
		defer d.Pool.Close()
		defer func() {
			_ = d.Catalog.Close()
		}()
		defer func() {
			_ = d.Identity.Close()
		}()
		defer func() {
			_ = d.Orders.Close()
		}()

		return loop(ctx, log, d)
	})
	if err != nil {
		return dig.RootCause(err)
	}

	return nil
}

func loop(ctx context.Context, log *slog.Logger, d syncd) error {
	runs := []collectionSync{
		{search.Products, d.Products.Once},
		{search.Merchants, d.Merchants.Once},
		{search.Orders, d.Placed.Once},
	}

	log.InfoContext(ctx, "syncd started", "collections", len(runs), "interval", d.Config.SyncInterval.String())

	ticker := time.NewTicker(d.Config.SyncInterval)
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

	log.ErrorContext(
		ctx,
		"sync run failed; the cursor has not moved and the index is now older",
		"collection",
		string(r.collection),
		"err",
		err,
		"kind",
		search.KindOf(err).String(),
	)
}
