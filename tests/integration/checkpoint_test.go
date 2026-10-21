package integration_test

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/migrations"
	"github.com/WildanFrananda/kinetix-search-service/internal/postgres"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func migrated(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	pool, ctx := database(t)

	db, err := sql.Open("pgx", pool.Config().ConnString())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpContext(ctx, db, "."))

	_, err = pool.Exec(ctx, "DELETE FROM sync_checkpoint")
	require.NoError(t, err)

	return pool, ctx
}

func TestACheckpointKeepsItsTransaction(t *testing.T) {
	pool, ctx := migrated(t)
	checkpoint := postgres.NewCheckpoint(pool)
	noon := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	cursor := search.Cursor{TransactionID: 812, LastID: "SHOE-3", UpdatedThrough: noon}

	require.NoError(t, checkpoint.Save(ctx, search.Products, cursor))

	progress, err := checkpoint.Load(ctx, search.Products)

	require.NoError(t, err)
	require.Equal(t, cursor, progress.Cursor)
}

func TestACheckpointFromBeforeTheTransactionCursorStartsTheWalkAgain(t *testing.T) {
	pool, ctx := migrated(t)
	noon := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	_, err := pool.Exec(
		ctx,
		"INSERT INTO sync_checkpoint (collection, updated_through, last_id) VALUES ('orders', $1, 'ORD-9')",
		noon,
	)
	require.NoError(t, err)

	progress, err := postgres.NewCheckpoint(pool).Load(ctx, search.Orders)

	require.NoError(t, err)
	require.Zero(t, progress.Cursor.TransactionID)
	require.Empty(t, progress.Cursor.LastID, "a key from the time-ordered walk means nothing in the transaction order")
	require.Equal(t, noon, progress.Cursor.UpdatedThrough, "what was indexed stays indexed while the walk catches up")
}

func TestTheIndexedThroughWatermarkNeverMovesBack(t *testing.T) {
	pool, ctx := migrated(t)
	checkpoint := postgres.NewCheckpoint(pool)
	noon := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	earlier := search.Cursor{TransactionID: 701, UpdatedThrough: noon.Add(-time.Hour)}

	require.NoError(t, checkpoint.Save(ctx, search.Merchants, search.Cursor{TransactionID: 700, UpdatedThrough: noon}))
	require.NoError(t, checkpoint.Save(ctx, search.Merchants, earlier))

	progress, err := checkpoint.Load(ctx, search.Merchants)

	require.NoError(t, err)
	require.Equal(t, uint64(701), progress.Cursor.TransactionID)
	require.Equal(t, noon, progress.Cursor.UpdatedThrough)
}

func TestACheckpointRefusesATransactionItCannotStore(t *testing.T) {
	err := postgres.NewCheckpoint(nil).Save(
		context.Background(),
		search.Products,
		search.Cursor{TransactionID: math.MaxInt64 + 1},
	)

	require.Error(t, err)
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
}

func TestACheckpointOverAnEmptySourceKeepsNoWatermark(t *testing.T) {
	pool, ctx := migrated(t)
	checkpoint := postgres.NewCheckpoint(pool)

	require.NoError(t, checkpoint.Save(ctx, search.Products, search.Cursor{TransactionID: 9}))

	progress, err := checkpoint.Load(ctx, search.Products)

	require.NoError(t, err)
	require.True(t, progress.Cursor.UpdatedThrough.IsZero(), "nothing indexed reads back as nothing, not as a date")
	require.False(t, progress.SavedAt.IsZero())
}

func TestACollectionNeverSyncedHasNoProgress(t *testing.T) {
	pool, ctx := migrated(t)

	progress, err := postgres.NewCheckpoint(pool).Load(ctx, search.Orders)

	require.NoError(t, err)
	require.True(t, progress.SavedAt.IsZero())
}
