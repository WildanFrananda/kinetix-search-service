package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/WildanFrananda/kinetix-search-service/internal/migrations"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("migrate failed", "err", err, "kind", search.KindOf(err).String())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	url := os.Getenv("SEARCH_DATABASE_URL")

	if url == "" {
		return search.Errf(
			search.KindDependencyUnavailable,
			"migrate.config",
			nil,
			"SEARCH_DATABASE_URL must be set",
		)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := sql.Open("pgx", url)

	if err != nil {
		return search.Errf(search.KindDependencyUnavailable, "migrate.open", err, "opening")
	}

	defer func() { _ = db.Close() }()

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return search.Errf(search.KindDependencyUnavailable, "migrate.dialect", err, "postgres")
	}

	before, err := goose.GetDBVersionContext(ctx, db)

	if err != nil {
		return search.Errf(search.KindDependencyUnavailable, "migrate.version", err, "reading")
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return search.Errf(search.KindDependencyUnavailable, "migrate.up", err, "applying")
	}

	after, err := goose.GetDBVersionContext(ctx, db)

	if err != nil {
		return search.Errf(search.KindDependencyUnavailable, "migrate.version", err, "reading")
	}

	log.InfoContext(ctx, "migrations applied", "from", before, "to", after)

	return nil
}
