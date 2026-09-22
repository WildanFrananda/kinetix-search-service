package querying_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/tests/searchtest"
)

var noon = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

func productFinder(s *searchtest.FakeSearcher[search.ProductDoc], c *searchtest.FakeCheckpoint, stale time.Duration) *querying.Finder[search.ProductDoc] {
	return querying.NewFinder(search.Products, s, c, searchtest.FixedClock{At: noon}, stale)
}

func TestFindReportsFreshness(t *testing.T) {
	cp := &searchtest.FakeCheckpoint{Cursors: map[search.Collection]search.Cursor{
		search.Products: {UpdatedThrough: noon.Add(-2 * time.Minute)},
	}}
	f := productFinder(&searchtest.FakeSearcher[search.ProductDoc]{Results: search.Results[search.ProductDoc]{Total: 3}}, cp, 10*time.Minute)

	q, err := search.NewQuery("sepatu")
	require.NoError(t, err)

	res, err := f.Find(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, 2*time.Minute, res.Freshness.Age)
	require.False(t, res.Freshness.Stale)
}

func TestFindMarksAnOldIndexStale(t *testing.T) {
	cp := &searchtest.FakeCheckpoint{Cursors: map[search.Collection]search.Cursor{
		search.Products: {UpdatedThrough: noon.Add(-2 * time.Hour)},
	}}
	f := productFinder(&searchtest.FakeSearcher[search.ProductDoc]{}, cp, 10*time.Minute)

	q, _ := search.NewQuery("sepatu")
	res, err := f.Find(context.Background(), q)
	require.NoError(t, err)
	require.True(t, res.Freshness.Stale, "the caller has to be told, not left to assume")
}

func TestFindFailsRatherThanReturningNothing(t *testing.T) {
	s := &searchtest.FakeSearcher[search.ProductDoc]{
		Err: search.Errf(search.KindIndexUnavailable, "typesense.Search", nil, "cluster unreachable"),
	}
	f := productFinder(s, &searchtest.FakeCheckpoint{}, time.Minute)

	q, _ := search.NewQuery("sepatu")
	res, err := f.Find(context.Background(), q)

	require.Error(t, err)
	require.Equal(t, search.KindIndexUnavailable, search.KindOf(err))
	require.Empty(t, res.Hits)
}

func TestOrdersCannotBeSearchedWithoutAPrincipal(t *testing.T) {
	s := &searchtest.FakeSearcher[search.OrderDoc]{}
	f := querying.NewFinder(search.Orders, s, &searchtest.FakeCheckpoint{}, searchtest.FixedClock{At: noon}, time.Minute)

	q, _ := search.NewQuery("KNX-2026")
	_, err := f.Find(context.Background(), q)

	require.Error(t, err)
	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
	require.Empty(t, s.Asked, "the adapter must never see an unscoped orders query")
}

func TestOrdersSearchScopedToThePrincipal(t *testing.T) {
	s := &searchtest.FakeSearcher[search.OrderDoc]{}
	f := querying.NewFinder(search.Orders, s, &searchtest.FakeCheckpoint{}, searchtest.FixedClock{At: noon}, time.Minute)

	buyer, err := search.NewMerchantID("principal-9")
	require.NoError(t, err)

	q, _ := search.NewQuery("KNX-2026")
	_, err = f.Find(context.Background(), q.ForPrincipal(buyer))
	require.NoError(t, err)
	require.Equal(t, buyer, s.Asked[0].OnBehalfOf)
}

func TestQueryCapsThePage(t *testing.T) {
	q, err := search.NewQuery("sepatu", search.WithPage(0, 1_000_000))
	require.NoError(t, err)
	require.Equal(t, search.MaxPageSize, q.Page.Limit)
}

func TestSealedIdentifierRefusesRubbish(t *testing.T) {
	_, err := search.NewProductID("   ")
	require.Error(t, err)
	_, err = search.NewProductID("id\x1b[31m")
	require.Error(t, err)
}
