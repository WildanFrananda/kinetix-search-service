package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/httpapi"
	"github.com/WildanFrananda/kinetix-search-service/internal/postgres"
	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/system"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("searchd stopped", "err", err, "kind", search.KindOf(err).String())
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
		return search.Errf(search.KindDependencyUnavailable, "searchd.database", err, "connecting")
	}
	defer pool.Close()

	engine := typesense.Connect(typesense.Settings{
		URL:     cfg.TypesenseURL,
		APIKey:  cfg.TypesenseAPIKey,
		Timeout: cfg.TypesenseTimeout,
	})

	checkpoint := postgres.NewCheckpoint(pool)
	products := querying.NewFinder(
		search.Products,
		typesense.NewSearcher(engine, search.Products, typesense.DecodeProduct),
		checkpoint,
		system.Clock{},
		cfg.StaleAfter,
	)

	index := typesense.NewIndex(engine, search.Products, typesense.EncodeProduct)
	ready := func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return search.Errf(search.KindDependencyUnavailable, "searchd.ready", err, "database")
		}
		live, err := index.Live(ctx)
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

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(products, ready),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serving := make(chan error, 1)
	go func() {
		log.Info("searchd listening", "addr", cfg.HTTPAddr)
		serving <- server.ListenAndServe()
	}()

	select {
	case err := <-serving:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return search.Errf(search.KindDependencyUnavailable, "searchd.http", err, "serving")
	case <-ctx.Done():
	}

	log.Info("searchd draining")
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}
