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

func merchant(t *testing.T, id, name string, at time.Time) search.MerchantDoc {
	t.Helper()
	principal, err := search.NewMerchantID(id)
	require.NoError(t, err)

	return search.MerchantDoc{
		ID:          principal,
		DisplayName: name,
		Status:      "verified",
		MaySell:     true,
		UpdatedAt:   at,
	}
}

func twoMerchants(t *testing.T) *searchtest.UncountableSource[search.MerchantDoc] {
	t.Helper()

	return &searchtest.UncountableSource[search.MerchantDoc]{
		Pages: []search.Changes[search.MerchantDoc]{{
			Upserted: []search.MerchantDoc{
				merchant(t, "m-1", "Toko Sepatu", noon),
				merchant(t, "m-2", "Toko Tas", noon.Add(time.Minute)),
			},
			Next: search.Cursor{UpdatedThrough: noon.Add(time.Minute), LastID: "m-2"},
		}},
	}
}

func TestASourceThatCannotCountIsStillReindexed(t *testing.T) {
	index := &searchtest.FakeIndex[search.MerchantDoc]{LiveGeneration: "merchants_v1"}
	checkpoint := &searchtest.FakeCheckpoint{}

	r := indexing.NewReindexer(
		search.Merchants,
		twoMerchants(t),
		index,
		checkpoint,
		searchtest.FixedClock{At: noon},
		discard, 100,
	)

	require.NoError(t, r.Run(context.Background()))
	require.Len(t, index.Promoted, 1)
	require.Equal(t, "m-2", checkpoint.Cursors[search.Merchants].LastID)
}

func TestAnUncountableSourceIsStillCheckedAgainstWhatTheWalkSent(t *testing.T) {
	short := int64(1)
	index := &searchtest.FakeIndex[search.MerchantDoc]{
		LiveGeneration: "merchants_v1",
		Counted:        &short,
	}
	checkpoint := &searchtest.FakeCheckpoint{}

	r := indexing.NewReindexer(
		search.Merchants,
		twoMerchants(t),
		index,
		checkpoint,
		searchtest.FixedClock{At: noon},
		discard, 100,
	)

	err := r.Run(context.Background())

	require.Error(t, err, "two merchants were sent and only one landed; that is not a promotion")
	require.Empty(t, index.Promoted)
	require.Contains(t, err.Error(), "the walk")
	require.Empty(
		t,
		checkpoint.Cursors,
		"a refused promotion must not leave a checkpoint pointing into a dead generation",
	)
}
