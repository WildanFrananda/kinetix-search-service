package indexing_test

import (
	"bytes"
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

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		index,
		checkpoint,
		&searchtest.FakeLock{},
		discard,
		100,
	)
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
		&searchtest.FakeLock{},
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
		&searchtest.FakeLock{},
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

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		index,
		checkpoint,
		&searchtest.FakeLock{},
		discard,
		100,
	)
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
		&searchtest.FakeLock{},
		discard,
		100,
	)
	err := syncer.Once(context.Background())

	require.Error(t, err)
	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
	require.Empty(t, checkpoint.Saved)
}

func TestSyncHoldsTheCollectionForTheWholePass(t *testing.T) {
	lock := &searchtest.FakeLock{}
	source := &searchtest.FakeSource[search.ProductDoc]{Pages: []search.Changes[search.ProductDoc]{
		{
			Upserted: []search.ProductDoc{product(t, "p-1", "sepatu", noon)},
			Next:     search.Cursor{UpdatedThrough: noon, LastID: "p-1"},
			HasMore:  true,
		},
		{
			Upserted: []search.ProductDoc{product(t, "p-2", "sandal", noon.Add(time.Minute))},
			Next:     search.Cursor{UpdatedThrough: noon.Add(time.Minute), LastID: "p-2"},
		},
	}}
	index := &searchtest.FakeIndex[search.ProductDoc]{LiveGeneration: "products_v1", Lock: lock}
	checkpoint := &searchtest.FakeCheckpoint{Lock: lock}

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		index,
		checkpoint,
		lock,
		discard,
		100,
	)
	require.NoError(t, syncer.Once(context.Background()))

	require.Equal(t, []search.Collection{search.Products}, lock.Held)
	require.Equal(t, []bool{true, true}, index.WrittenHeld)
	require.Equal(
		t,
		[]bool{true, true},
		checkpoint.SavedHeld,
		"a cursor saved outside the lock can land after a reindex promoted and skip what it wrote",
	)
	require.Equal(t, 1, lock.Released)
	require.False(t, lock.Holding)
}

func TestSyncWritesNothingWhenItCannotHoldTheCollection(t *testing.T) {
	lock := &searchtest.FakeLock{
		Err: search.Errf(search.KindDependencyUnavailable, "postgres.Lock", nil, "no answer"),
	}
	source := &searchtest.FakeSource[search.ProductDoc]{Pages: []search.Changes[search.ProductDoc]{{
		Upserted: []search.ProductDoc{product(t, "p-1", "sepatu", noon)},
		Next:     search.Cursor{UpdatedThrough: noon, LastID: "p-1"},
	}}}
	index := &searchtest.FakeIndex[search.ProductDoc]{LiveGeneration: "products_v1"}
	checkpoint := &searchtest.FakeCheckpoint{}

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		index,
		checkpoint,
		lock,
		discard,
		100,
	)
	err := syncer.Once(context.Background())

	require.Equal(t, search.KindDependencyUnavailable, search.KindOf(err))
	require.Empty(t, source.AskedFrom)
	require.Empty(t, index.Written)
	require.Empty(t, checkpoint.Saved)
}

func TestSyncLogsARefusedRecordAndKeepsGoing(t *testing.T) {
	var logged bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logged, nil))

	refusal := search.Refusal{
		ID:     "ORD-7",
		Reason: search.Errf(search.KindCorruptRecord, "orderclient.ToDoc", nil, "names no buyer"),
	}
	source := &searchtest.FakeSource[search.ProductDoc]{Pages: []search.Changes[search.ProductDoc]{{
		Upserted: []search.ProductDoc{
			product(t, "p-1", "sepatu", noon),
			product(t, "p-3", "topi", noon.Add(2*time.Minute)),
		},
		Refused: []search.Refusal{refusal},
		Next:    search.Cursor{UpdatedThrough: noon.Add(2 * time.Minute), LastID: "p-3"},
	}}}
	index := &searchtest.FakeIndex[search.ProductDoc]{LiveGeneration: "products_v1"}
	checkpoint := &searchtest.FakeCheckpoint{}

	syncer := indexing.NewSyncer(
		search.Products,
		source,
		index,
		checkpoint,
		&searchtest.FakeLock{},
		log,
		100,
	)
	require.NoError(t, syncer.Once(context.Background()))

	require.Len(t, index.Written["products_v1"], 2, "one bad record must not hold back the rest")
	require.Equal(
		t,
		search.Cursor{
			UpdatedThrough: noon.Add(2 * time.Minute),
			LastID:         "p-3",
		},
		checkpoint.Cursors[search.Products],
		"the cursor moves past a refused record, or every pass stops on it for ever",
	)
	require.Contains(t, logged.String(), `"level":"ERROR","msg":"refused a record the source sent"`)
	require.Contains(t, logged.String(), `"id":"ORD-7"`)
	require.Contains(t, logged.String(), `"kind":"corrupt_record"`)
	require.Contains(t, logged.String(), `"refused":1`)
}
