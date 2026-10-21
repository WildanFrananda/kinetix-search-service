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

func productFinder(
	s *searchtest.FakeSearcher[search.ProductDoc],
	c *searchtest.FakeCheckpoint,
	stale time.Duration,
) *querying.Finder[search.ProductDoc] {
	return querying.NewFinder(search.Products, s, c, searchtest.FixedClock{At: noon}, stale)
}

func orderFinder(s *searchtest.FakeSearcher[search.OrderDoc]) *querying.Finder[search.OrderDoc] {
	return querying.NewFinder(
		search.Orders,
		s,
		&searchtest.FakeCheckpoint{},
		searchtest.FixedClock{At: noon},
		time.Minute,
	)
}

func TestFindReportsFreshness(t *testing.T) {
	cp := &searchtest.FakeCheckpoint{
		Cursors: map[search.Collection]search.Cursor{
			search.Products: {UpdatedThrough: noon.Add(-2 * time.Minute)},
		},
		SavedAt: map[search.Collection]time.Time{search.Products: noon.Add(-2 * time.Minute)},
	}
	searcher := &searchtest.FakeSearcher[search.ProductDoc]{Results: search.Results[search.ProductDoc]{Total: 3}}
	f := productFinder(searcher, cp, 10*time.Minute)

	q, err := search.NewQuery("sepatu")
	require.NoError(t, err)

	res, err := f.Find(context.Background(), q)
	require.NoError(t, err)
	require.True(t, res.Freshness.Synced)
	require.Equal(t, 2*time.Minute, res.Freshness.Age)
	require.False(t, res.Freshness.Stale)
}

func TestFindMarksAnOldIndexStale(t *testing.T) {
	cp := &searchtest.FakeCheckpoint{
		Cursors: map[search.Collection]search.Cursor{
			search.Products: {UpdatedThrough: noon.Add(-2 * time.Hour)},
		},
		SavedAt: map[search.Collection]time.Time{search.Products: noon.Add(-2 * time.Hour)},
	}
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
	f := orderFinder(s)

	q, _ := search.NewQuery("KNX-2026")
	_, err := f.Find(context.Background(), q)

	require.Error(t, err)
	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
	require.Empty(t, s.Asked, "the adapter must never see an unscoped orders query")
}

func TestOrdersSearchScopedToThePrincipal(t *testing.T) {
	s := &searchtest.FakeSearcher[search.OrderDoc]{}
	f := orderFinder(s)

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

func TestAnIndexNobodyHasChangedIsNotStale(t *testing.T) {
	cp := &searchtest.FakeCheckpoint{
		Cursors: map[search.Collection]search.Cursor{
			search.Products: {UpdatedThrough: noon.Add(-7 * 24 * time.Hour)},
		},
		SavedAt: map[search.Collection]time.Time{search.Products: noon.Add(-20 * time.Second)},
	}
	f := productFinder(&searchtest.FakeSearcher[search.ProductDoc]{}, cp, 10*time.Minute)

	q, err := search.NewQuery("sepatu")
	require.NoError(t, err)

	results, err := f.Find(context.Background(), q)

	require.NoError(t, err)
	require.False(t, results.Freshness.Stale, "a catalogue nobody edits is not an index nobody syncs")
	require.Equal(t, 20*time.Second, results.Freshness.Age)
	require.Equal(
		t,
		noon.Add(-7*24*time.Hour),
		results.Freshness.IndexedThrough,
		"the walk's own watermark is still reported, it just does not decide staleness",
	)
}

func TestAnIndexNobodyHasSyncedIsStale(t *testing.T) {
	cp := &searchtest.FakeCheckpoint{
		Cursors: map[search.Collection]search.Cursor{
			search.Products: {UpdatedThrough: noon.Add(-time.Minute)},
		},
		SavedAt: map[search.Collection]time.Time{search.Products: noon.Add(-2 * time.Hour)},
	}
	f := productFinder(&searchtest.FakeSearcher[search.ProductDoc]{}, cp, 10*time.Minute)

	q, err := search.NewQuery("sepatu")
	require.NoError(t, err)

	results, err := f.Find(context.Background(), q)

	require.NoError(t, err)
	require.True(t, results.Freshness.Stale)
}

func TestACollectionNeverSyncedSaysSoRatherThanAgeingFromYearOne(t *testing.T) {
	f := productFinder(&searchtest.FakeSearcher[search.ProductDoc]{}, &searchtest.FakeCheckpoint{}, 10*time.Minute)

	q, err := search.NewQuery("sepatu")
	require.NoError(t, err)

	results, err := f.Find(context.Background(), q)

	require.NoError(t, err)
	require.Equal(t, search.Freshness{Synced: false, Stale: true}, results.Freshness)
}

func TestSuggestWithNothingTypedAsksNothing(t *testing.T) {
	s := &searchtest.FakeSearcher[search.ProductDoc]{}
	f := productFinder(s, &searchtest.FakeCheckpoint{}, time.Minute)

	q, err := search.NewSuggestQuery("   ", 5)
	require.NoError(t, err)

	suggestions, err := f.Suggest(context.Background(), q)

	require.NoError(t, err)
	require.Equal(t, []string{}, suggestions)
	require.Empty(t, s.Suggested)
}

func TestSuggestPassesTheQueryThrough(t *testing.T) {
	s := &searchtest.FakeSearcher[search.ProductDoc]{}
	f := productFinder(s, &searchtest.FakeCheckpoint{}, time.Minute)

	q, err := search.NewSuggestQuery("sep", 500)
	require.NoError(t, err)

	_, err = f.Suggest(context.Background(), q)

	require.NoError(t, err)
	require.Equal(t, []search.SuggestQuery{{Prefix: "sep", Limit: search.MaxPageSize}}, s.Suggested)
}

func TestOrdersAreNeverSuggested(t *testing.T) {
	s := &searchtest.FakeSearcher[search.OrderDoc]{}

	q, err := search.NewSuggestQuery("KNX", 5)
	require.NoError(t, err)

	_, err = orderFinder(s).Suggest(context.Background(), q)

	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
	require.Empty(t, s.Suggested)
}
