package indexing_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/tests/searchtest"
)

var (
	noon    = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	discard = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func product(t *testing.T, id, title string, at time.Time) search.ProductDoc {
	t.Helper()
	pid, err := search.NewProductID(id)
	require.NoError(t, err)
	return search.ProductDoc{ID: pid, Title: title, UpdatedAt: at}
}

func TestSyncWalksEveryPage(t *testing.T) {
	source := &searchtest.FakeSource[search.ProductDoc]{Pages: []search.Changes[search.ProductDoc]{
		{
			Upserted: []search.ProductDoc{product(t, "p-1", "sepatu", noon)},
			Next:     search.Cursor{UpdatedThrough: noon, LastID: "p-1"},
			HasMore:  true,
		},
		{
			Upserted: []search.ProductDoc{product(t, "p-2", "sandal", noon.Add(time.Minute))},
			Next:     search.Cursor{UpdatedThrough: noon.Add(time.Minute), LastID: "p-2"},
			HasMore:  false,
		},
	}}
	index := &searchtest.FakeIndex[search.ProductDoc]{LiveGeneration: "products_v1"}
	checkpoint := &searchtest.FakeCheckpoint{}

	syncer := indexing.NewSyncer(search.Products, source, index, checkpoint, discard, 100)
	require.NoError(t, syncer.Once(context.Background()))

	require.Len(t, index.Written["products_v1"], 2)
	require.Equal(
		t,
		search.Cursor{
			UpdatedThrough: noon.Add(time.Minute),
			LastID:         "p-2",
		},
		checkpoint.Cursors[search.Products],
	)
}

func TestSyncResumesFromTheStoredCursor(t *testing.T) {
	stored := search.Cursor{UpdatedThrough: noon, LastID: "p-7"}
	source := &searchtest.FakeSource[search.ProductDoc]{Pages: []search.Changes[search.ProductDoc]{{}}}
	checkpoint := &searchtest.FakeCheckpoint{
		Cursors: map[search.Collection]search.Cursor{search.Products: stored},
	}

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		&searchtest.FakeIndex[search.ProductDoc]{},
		checkpoint,
		discard,
		100,
	)
	require.NoError(t, syncer.Once(context.Background()))

	require.Equal(t, stored, source.AskedFrom[0], "a restart must not re-walk from the beginning")
}

func TestSyncAppliesTombstones(t *testing.T) {
	source := &searchtest.FakeSource[search.ProductDoc]{Pages: []search.Changes[search.ProductDoc]{{
		Removed: []string{"p-9"},
		Next:    search.Cursor{UpdatedThrough: noon, LastID: "p-9"},
	}}}
	index := &searchtest.FakeIndex[search.ProductDoc]{LiveGeneration: "products_v1"}

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		index,
		&searchtest.FakeCheckpoint{},
		discard,
		100,
	)
	require.NoError(t, syncer.Once(context.Background()))

	require.Equal(
		t,
		[]string{"p-9"},
		index.RemovedIDs["products_v1"],
		"a deleted product that stays searchable is a product customers cannot buy",
	)
}

func TestSyncDoesNotAdvanceTheCursorWhenTheWriteFails(t *testing.T) {
	source := &searchtest.FakeSource[search.ProductDoc]{Pages: []search.Changes[search.ProductDoc]{{
		Upserted: []search.ProductDoc{product(t, "p-1", "sepatu", noon)},
		Next:     search.Cursor{UpdatedThrough: noon, LastID: "p-1"},
	}}}
	index := &searchtest.FakeIndex[search.ProductDoc]{BulkErr: errors.New("index refused")}
	checkpoint := &searchtest.FakeCheckpoint{}

	syncer := indexing.NewSyncer(search.Products, source, index, checkpoint, discard, 100)
	err := syncer.Once(context.Background())

	require.Error(t, err)
	require.Empty(t, checkpoint.Saved, "the cursor must stay where it was")
}

func TestSyncStopsWhenTheSourceFails(t *testing.T) {
	source := &searchtest.FakeSource[search.ProductDoc]{
		Err: search.Errf(search.KindDependencyUnavailable, "catalog.ChangedSince", nil, "no answer"),
	}
	checkpoint := &searchtest.FakeCheckpoint{}

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		&searchtest.FakeIndex[search.ProductDoc]{},
		checkpoint,
		discard,
		100,
	)
	err := syncer.Once(context.Background())

	require.Error(t, err)
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
	require.Empty(t, checkpoint.Saved)
}
