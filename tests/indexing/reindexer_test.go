package indexing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/tests/searchtest"
)

func twoProducts(t *testing.T) *searchtest.FakeSource[search.ProductDoc] {
	t.Helper()
	return &searchtest.FakeSource[search.ProductDoc]{
		Total: 2,
		Pages: []search.Changes[search.ProductDoc]{{
			Upserted: []search.ProductDoc{
				product(t, "p-1", "sepatu", noon),
				product(t, "p-2", "sandal", noon.Add(time.Minute)),
			},
			Next: search.Cursor{UpdatedThrough: noon.Add(time.Minute), LastID: "p-2"},
		}},
	}
}

func TestReindexBuildsANewGenerationAndPromotesIt(t *testing.T) {
	index := &searchtest.FakeIndex[search.ProductDoc]{LiveGeneration: "products_v1"}
	checkpoint := &searchtest.FakeCheckpoint{}

	r := indexing.NewReindexer(
		search.Products,
		twoProducts(t),
		index,
		checkpoint,
		searchtest.FixedClock{At: noon},
		discard, 100,
	)
	require.NoError(t, r.Run(context.Background()))

	require.Len(t, index.Created, 1)
	require.NotEqual(
		t,
		search.Generation("products_v1"),
		index.Created[0],
		"a reindex builds beside the live generation, never into it",
	)
	require.Equal(t, index.Created, index.Promoted)
}

func TestReindexRefusesToPromoteAGenerationThatCameUpShort(t *testing.T) {
	short := int64(1)
	index := &searchtest.FakeIndex[search.ProductDoc]{LiveGeneration: "products_v1", Counted: &short}

	r := indexing.NewReindexer(
		search.Products,
		twoProducts(t),
		index,
		&searchtest.FakeCheckpoint{},
		searchtest.FixedClock{At: noon},
		discard,
		100,
	)
	err := r.Run(context.Background())

	require.Error(t, err)
	require.Equal(t, search.KindIndexUnavailable, search.KindOf(err))
	require.Empty(t, index.Promoted, "the live index must be untouched")
	require.Equal(t, search.Generation("products_v1"), index.LiveGeneration)
}

func TestReindexAcceptsAGenerationAheadOfTheCount(t *testing.T) {
	ahead := int64(5)
	index := &searchtest.FakeIndex[search.ProductDoc]{Counted: &ahead}

	r := indexing.NewReindexer(
		search.Products,
		twoProducts(t),
		index,
		&searchtest.FakeCheckpoint{},
		searchtest.FixedClock{At: noon},
		discard,
		100,
	)

	require.NoError(t, r.Run(context.Background()))
	require.Len(t, index.Promoted, 1)
}

func TestReindexSavesTheCursorOnlyAfterPromoting(t *testing.T) {
	short := int64(0)
	index := &searchtest.FakeIndex[search.ProductDoc]{Counted: &short}
	checkpoint := &searchtest.FakeCheckpoint{}

	r := indexing.NewReindexer(
		search.Products,
		twoProducts(t),
		index,
		checkpoint,
		searchtest.FixedClock{At: noon},
		discard,
		100,
	)

	require.Error(t, r.Run(context.Background()))
	require.Empty(t, checkpoint.Saved)
}

func TestReindexLeavesTheCursorWhereSyncCanContinue(t *testing.T) {
	index := &searchtest.FakeIndex[search.ProductDoc]{}
	checkpoint := &searchtest.FakeCheckpoint{}

	r := indexing.NewReindexer(
		search.Products,
		twoProducts(t),
		index,
		checkpoint,
		searchtest.FixedClock{At: noon},
		discard,
		100,
	)
	require.NoError(t, r.Run(context.Background()))

	require.Equal(
		t,
		search.Cursor{
			UpdatedThrough: noon.Add(time.Minute),
			LastID:         "p-2",
		},
		checkpoint.Cursors[search.Products],
	)
}
