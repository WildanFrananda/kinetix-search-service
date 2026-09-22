package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

func engine(t *testing.T) (*typesenseClient, context.Context) {
	t.Helper()

	url := os.Getenv("KINETIX_TYPESENSE_URL")
	key := os.Getenv("KINETIX_TYPESENSE_KEY")
	if url == "" || key == "" {
		t.Skip("set KINETIX_TYPESENSE_URL and KINETIX_TYPESENSE_KEY to run the engine tests")
	}

	client := typesense.Connect(typesense.Settings{URL: url, APIKey: key, Timeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	return &typesenseClient{
		index: typesense.NewIndex(client, search.Products, typesense.EncodeProduct),
		searcher: typesense.NewSearcher(
			client,
			search.Products,
			typesense.DecodeProduct,
		),
	}, ctx
}

type typesenseClient struct {
	index    *typesense.Index[search.ProductDoc]
	searcher *typesense.Searcher[search.ProductDoc]
}

func productDoc(t *testing.T, id, title, category string, price int64, at time.Time) search.ProductDoc {
	t.Helper()
	pid, err := search.NewProductID(id)
	require.NoError(t, err)
	merchant, err := search.NewMerchantID("shop-1")
	require.NoError(t, err)
	return search.ProductDoc{
		ID:         pid,
		MerchantID: merchant,
		Title:      title,
		Categories: []string{category},
		PriceMinor: price,
		Currency:   "IDR",
		UpdatedAt:  at,
	}
}

func TestTheWholeAdapterAgainstARealEngine(t *testing.T) {
	ts, ctx := engine(t)
	now := time.Now().UTC()
	gen := typesense.NewGeneration(search.Products, now.UnixNano())

	require.NoError(
		t,
		ts.index.CreateGeneration(ctx, gen),
		"the schema in schema.go has to be one the engine actually accepts",
	)

	docs := []search.ProductDoc{
		productDoc(t, "p-1", "Sepatu kulit hitam", "sepatu", 24_999_00, now),
		productDoc(t, "p-2", "Sandal jepit", "sandal", 3_500_00, now.Add(-time.Hour)),
		productDoc(t, "p-3", "Leather boots", "sepatu", 45_000_00, now.Add(-2*time.Hour)),
	}
	require.NoError(t, ts.index.Bulk(ctx, gen, docs))

	count, err := ts.index.CountIn(ctx, gen)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	require.NoError(t, ts.index.Promote(ctx, gen))
	live, err := ts.index.Live(ctx)
	require.NoError(t, err)
	require.Equal(t, gen, live, "the alias must point at the generation just promoted")

	t.Run("finds by text", func(t *testing.T) {
		q, err := search.NewQuery("sepatu")
		require.NoError(t, err)
		res, err := ts.searcher.Search(ctx, q)
		require.NoError(t, err)
		require.NotEmpty(t, res.Hits)
		require.Equal(t, "p-1", res.Hits[0].Doc.ID.String())
	})

	t.Run("tolerates a typo", func(t *testing.T) {
		q, err := search.NewQuery("sepato")
		require.NoError(t, err)
		res, err := ts.searcher.Search(ctx, q)
		require.NoError(t, err)
		require.NotEmpty(t, res.Hits, "a one-character typo must still find the product")
	})

	t.Run("filters by category", func(t *testing.T) {
		q, err := search.NewQuery("", search.WithCategories("sandal"))
		require.NoError(t, err)
		res, err := ts.searcher.Search(ctx, q)
		require.NoError(t, err)
		require.Len(t, res.Hits, 1)
		require.Equal(t, "p-2", res.Hits[0].Doc.ID.String())
	})

	t.Run("sorts by price", func(t *testing.T) {
		q, err := search.NewQuery("", search.WithRanking(search.RankByPriceAsc))
		require.NoError(t, err)
		res, err := ts.searcher.Search(ctx, q)
		require.NoError(t, err)
		require.Len(t, res.Hits, 3)
		require.Equal(t, "p-2", res.Hits[0].Doc.ID.String(), "cheapest first")
	})

	t.Run("returns facet counts", func(t *testing.T) {
		q, err := search.NewQuery("")
		require.NoError(t, err)
		res, err := ts.searcher.Search(ctx, q)
		require.NoError(t, err)
		require.Contains(t, res.Facets, "categories")
	})

	t.Run("a document survives the round trip intact", func(t *testing.T) {
		q, err := search.NewQuery("Sandal jepit")
		require.NoError(t, err)
		res, err := ts.searcher.Search(ctx, q)
		require.NoError(t, err)
		require.NotEmpty(t, res.Hits)

		got := res.Hits[0].Doc
		require.Equal(t, int64(3_500_00), got.PriceMinor)
		require.Equal(t, "IDR", got.Currency)
		require.Equal(t, []string{"sandal"}, got.Categories)
		require.WithinDuration(t, docs[1].UpdatedAt, got.UpdatedAt, time.Millisecond)
	})

	t.Run("a tombstone removes the document", func(t *testing.T) {
		require.NoError(t, ts.index.Remove(ctx, gen, []string{"p-2"}))

		q, err := search.NewQuery("", search.WithCategories("sandal"))
		require.NoError(t, err)
		res, err := ts.searcher.Search(ctx, q)
		require.NoError(t, err)
		require.Empty(t, res.Hits, "a deleted product that stays searchable cannot be bought")
	})

	t.Run("a tombstone for something absent is not an error", func(t *testing.T) {
		require.NoError(t, ts.index.Remove(ctx, gen, []string{"p-never-existed"}))
	})

	t.Cleanup(func() {
		fmt.Printf("integration: leaving generation %s in place for inspection\n", gen)
	})
}

func TestLiveIsEmptyRatherThanFailingWhenNothingIsIndexed(t *testing.T) {
	ts, ctx := engine(t)
	_ = ts

	client := typesense.Connect(typesense.Settings{
		URL:    os.Getenv("KINETIX_TYPESENSE_URL"),
		APIKey: os.Getenv("KINETIX_TYPESENSE_KEY"),
	})
	index := typesense.NewIndex(
		client,
		search.Collection("merchants-never-created"),
		typesense.EncodeMerchant,
	)

	gen, err := index.Live(ctx)

	require.NoError(t, err, "a fresh deployment has no alias, and that is not a failure")
	require.Equal(t, search.Generation(""), gen)
}
