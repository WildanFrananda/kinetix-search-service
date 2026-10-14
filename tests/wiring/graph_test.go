package wiring_test

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/dig"
	"google.golang.org/grpc"

	"github.com/WildanFrananda/kinetix-search-service/internal/catalogclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/identityclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/orderclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/wiring"
	"github.com/WildanFrananda/kinetix-search-service/tests/searchtest"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func environment(t *testing.T) {
	t.Helper()

	t.Setenv("SEARCH_DATABASE_URL", "postgres://search:pw@127.0.0.1:1/search")
	t.Setenv("SEARCH_TYPESENSE_URL", "http://127.0.0.1:1")
	t.Setenv("SEARCH_TYPESENSE_API_KEY", "graph-test")
	t.Setenv("SEARCH_CATALOG_ENDPOINT", "kinetix-catalog-grpc:50058")
	t.Setenv("SEARCH_IDENTITY_ENDPOINT", "kinetix-identity-service:50052")
	t.Setenv("SEARCH_ORDER_ENDPOINT", "kinetix-order-service:50055")
	t.Setenv("KINETIX_PKI_DIR", searchtest.WritePKI(t, "kinetix-search-service"))
}

func TestTheSearchdGraphResolvesBothEdges(t *testing.T) {
	environment(t)

	container, err := wiring.Searchd(discard)
	require.NoError(t, err)

	err = container.Invoke(func(pool *pgxpool.Pool, h *http.Server, g *grpc.Server) {
		defer pool.Close()

		require.Equal(t, ":8088", h.Addr)
		require.NotNil(t, h.Handler)
		require.Contains(t, g.GetServiceInfo(), "search.v1.SearchService")
		require.Contains(t, g.GetServiceInfo(), "grpc.health.v1.Health")
	})

	require.NoError(t, err)
}

func TestTheIndexingGraphResolvesEveryCollection(t *testing.T) {
	environment(t)

	container, err := wiring.Indexing(discard)
	require.NoError(t, err)

	err = container.Invoke(
		func(
			pool *pgxpool.Pool,
			catalog *catalogclient.Client,
			identity *identityclient.Client,
			orders *orderclient.Client,
			products *indexing.Syncer[search.ProductDoc],
			merchants *indexing.Syncer[search.MerchantDoc],
			placed *indexing.Syncer[search.OrderDoc],
			productReindexer *indexing.Reindexer[search.ProductDoc],
			merchantReindexer *indexing.Reindexer[search.MerchantDoc],
			orderReindexer *indexing.Reindexer[search.OrderDoc],
		) {
			defer pool.Close()
			defer func() {
				_ = catalog.Close()
				_ = identity.Close()
				_ = orders.Close()
			}()

			require.NotNil(t, products)
			require.NotNil(t, merchants)
			require.NotNil(t, placed)
			require.NotNil(t, productReindexer)
			require.NotNil(t, merchantReindexer)
			require.NotNil(t, orderReindexer)
		},
	)

	require.NoError(t, err)
}

func TestAMissingSettingSurfacesAsTheRootCause(t *testing.T) {
	t.Setenv("SEARCH_DATABASE_URL", "")

	container, err := wiring.Searchd(discard)
	require.NoError(t, err)

	err = container.Invoke(func(*http.Server) {})

	require.Error(t, err)
	require.ErrorContains(t, dig.RootCause(err), "SEARCH_DATABASE_URL")
}
