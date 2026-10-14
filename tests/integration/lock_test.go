package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/postgres"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func database(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	url := os.Getenv("SEARCH_DATABASE_URL")
	if url == "" {
		t.Skip("set SEARCH_DATABASE_URL to run the Postgres tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool, ctx
}

func TestASecondHolderWaitsUntilTheFirstLetsGo(t *testing.T) {
	pool, ctx := database(t)
	lock := postgres.NewLock(pool)

	release, err := lock.Hold(ctx, search.Products)
	require.NoError(t, err)

	waiting, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	_, err = lock.Hold(waiting, search.Products)
	require.Error(t, err, "the collection is held, so a second holder must wait rather than proceed")
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))

	release()

	again, err := lock.Hold(ctx, search.Products)
	require.NoError(t, err, "a released collection can be held again")
	again()
}

func TestHoldingOneCollectionDoesNotHoldAnother(t *testing.T) {
	pool, ctx := database(t)
	lock := postgres.NewLock(pool)

	products, err := lock.Hold(ctx, search.Products)
	require.NoError(t, err)
	defer products()

	waiting, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	orders, err := lock.Hold(waiting, search.Orders)
	require.NoError(t, err, "a reindex of products must not stop the order sync")
	orders()
}

func TestAHolderWhoseSessionDiesLetsGo(t *testing.T) {
	pool, ctx := database(t)
	lock := postgres.NewLock(pool)

	held, err := lock.Hold(ctx, search.Merchants)
	require.NoError(t, err)
	defer held()

	_, err = pool.Exec(
		ctx,
		"SELECT pg_terminate_backend(pid) FROM pg_locks "+
			"WHERE locktype = 'advisory' AND granted AND pid <> pg_backend_pid()",
	)
	require.NoError(t, err)

	waiting, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	release, err := lock.Hold(waiting, search.Merchants)
	require.NoError(t, err, "a holder whose session died must not hold the collection for ever")
	release()
}
