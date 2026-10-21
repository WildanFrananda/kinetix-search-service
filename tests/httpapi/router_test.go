package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/httpapi"
	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/tests/searchtest"
)

var noon = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

func productID(t *testing.T, raw string) search.ProductID {
	t.Helper()

	id, err := search.NewProductID(raw)
	require.NoError(t, err)

	return id
}

type harness struct {
	server   http.Handler
	searcher *searchtest.FakeSearcher[search.ProductDoc]
	ready    error
}

func newHarness(t *testing.T, indexedThrough, syncedAt time.Time, staleAfter time.Duration) *harness {
	t.Helper()

	h := &harness{searcher: &searchtest.FakeSearcher[search.ProductDoc]{}}
	checkpoint := &searchtest.FakeCheckpoint{
		Cursors: map[search.Collection]search.Cursor{
			search.Products: {UpdatedThrough: indexedThrough},
		},
		SavedAt: map[search.Collection]time.Time{search.Products: syncedAt},
	}
	finder := querying.NewFinder(search.Products, h.searcher, checkpoint, searchtest.FixedClock{At: noon}, staleAfter)
	h.server = httpapi.NewRouter(finder, func(context.Context) error { return h.ready })

	return h
}

func (h *harness) get(t *testing.T, target string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

	var body map[string]any

	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	}

	return rec, body
}

func TestASearchCarriesItsOwnFreshness(t *testing.T) {
	h := newHarness(t, noon.Add(-2*time.Minute), noon.Add(-2*time.Minute), 10*time.Minute)
	h.searcher.Results = search.Results[search.ProductDoc]{
		Total: 1,
		Hits: []search.Hit[search.ProductDoc]{{
			Score: 12,
			Doc: search.ProductDoc{
				ID:         productID(t, "p-1"),
				Title:      "Sepatu kulit",
				PriceMinor: 75_000_000,
				Currency:   "IDR",
				UpdatedAt:  noon,
			},
		}},
	}

	rec, body := h.get(t, "/api/v1/products/search?q=sepatu")

	require.Equal(t, http.StatusOK, rec.Code)

	index, ok := body["index"].(map[string]any)
	require.True(t, ok, "every response says how current it is")
	require.Equal(t, float64(120), index["age_seconds"])
	require.Equal(t, false, index["stale"])
}

func TestAStaleIndexSaysSoInsteadOfLookingComplete(t *testing.T) {
	h := newHarness(t, noon.Add(-2*time.Hour), noon.Add(-2*time.Hour), 10*time.Minute)

	_, body := h.get(t, "/api/v1/products/search?q=sepatu")

	index, ok := body["index"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, index["stale"])
}

func TestAnOutageIsNotAnEmptyShop(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)
	h.searcher.Err = search.Errf(search.KindIndexUnavailable, "typesense.Search", nil, "cluster unreachable")

	rec, body := h.get(t, "/api/v1/products/search?q=sepatu")

	require.Equal(
		t,
		http.StatusServiceUnavailable,
		rec.Code,
		"200 with no hits would teach customers the shop is empty",
	)
	require.Equal(t, "index_unavailable", body["error"])
}

func TestAMalformedQueryIsTheCallersFault(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	rec, body := h.get(t, "/api/v1/products/search?q=sepatu%1b%5b31m")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "malformed_query", body["error"])
}

func TestQueryParametersReachTheUseCase(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	_, _ = h.get(t, "/api/v1/products/search?q=sepatu&limit=5&offset=10&sort=price_asc&category=sepatu&category=sandal")

	require.Len(t, h.searcher.Asked, 1)
	asked := h.searcher.Asked[0]
	require.Equal(t, "sepatu", asked.Text)
	require.Equal(t, 5, asked.Page.Limit)
	require.Equal(t, 10, asked.Page.Offset)
	require.Equal(t, search.RankByPriceAsc, asked.Ranking)
	require.Equal(t, []string{"sepatu", "sandal"}, asked.Categories)
}

func TestAPageThatCannotBeReadIsRefusedRatherThanReplaced(t *testing.T) {
	for _, target := range []string{
		"/api/v1/products/search?q=sepatu&limit=banyak",
		"/api/v1/products/search?q=sepatu&limit=-1",
		"/api/v1/products/search?q=sepatu&offset=-3",
		"/api/v1/products/search?q=sepatu&offset=1.5",
		"/api/v1/products/search?q=sepatu&offset=99999999999999999999",
	} {
		h := newHarness(t, noon, noon, time.Minute)

		rec, body := h.get(t, target)

		require.Equal(t, http.StatusBadRequest, rec.Code, target)
		require.Equal(t, "malformed_query", body["error"], target)
		require.Empty(t, h.searcher.Asked, target)
	}
}

func TestAnUnknownSortIsRefusedRatherThanReplaced(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	rec, body := h.get(t, "/api/v1/products/search?q=sepatu&sort=cheapest")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "malformed_query", body["error"])
	require.Empty(t, h.searcher.Asked)
}

func TestAnAbsentPageAndSortTakeTheDefaults(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	rec, _ := h.get(t, "/api/v1/products/search?q=sepatu&limit=&offset=&sort=")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, search.DefaultPageSize, h.searcher.Asked[0].Page.Limit)
	require.Equal(t, 0, h.searcher.Asked[0].Page.Offset)
	require.Equal(t, search.RankByRelevance, h.searcher.Asked[0].Ranking)
}

func TestAnEmptyCategoryIsNoFilterAtAll(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	rec, _ := h.get(t, "/api/v1/products/search?category=&category=%20%20&category=sepatu")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(
		t,
		[]string{"sepatu"},
		h.searcher.Asked[0].Categories,
		"an empty category matches no product, so sending one empties the shop",
	)
}

func TestSuggestRefusesWhatSearchRefuses(t *testing.T) {
	for _, target := range []string{
		"/api/v1/products/suggest?q=sep%1b%5b31m",
		"/api/v1/products/suggest?q=" + strings.Repeat("x", search.MaxQueryLength+1),
		"/api/v1/products/suggest?q=sep&limit=banyak",
		"/api/v1/products/suggest?q=sep&limit=-1",
	} {
		h := newHarness(t, noon, noon, time.Minute)

		rec, body := h.get(t, target)

		require.Equal(t, http.StatusBadRequest, rec.Code, target)
		require.Equal(t, "malformed_query", body["error"], target)
		require.Empty(t, h.searcher.Suggested, target)
	}
}

func TestSuggestAndSearchCapTheLimitAlike(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	_, _ = h.get(t, "/api/v1/products/search?q=sepatu&limit=500")
	_, _ = h.get(t, "/api/v1/products/suggest?q=sep&limit=500")

	require.Equal(t, search.MaxPageSize, h.searcher.Asked[0].Page.Limit)
	require.Equal(t, []search.SuggestQuery{{Prefix: "sep", Limit: search.MaxPageSize}}, h.searcher.Suggested)
}

func TestFreshnessBeforeTheFirstSyncSaysNeverRatherThanYearOne(t *testing.T) {
	h := newHarness(t, time.Time{}, time.Time{}, 10*time.Minute)

	rec, body := h.get(t, "/api/v1/products/search?q=sepatu")

	require.Equal(t, http.StatusOK, rec.Code)

	index, ok := body["index"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, index, "indexed_through_ms")
	require.Nil(t, index["indexed_through_ms"], "no record has been indexed")
	require.Contains(t, index, "age_seconds")
	require.Nil(t, index["age_seconds"], "no sync has completed, so there is no age to report")
	require.Equal(t, true, index["stale"])
}

func TestASyncOverNothingHasAnAgeButNoWatermark(t *testing.T) {
	h := newHarness(t, time.Time{}, noon.Add(-time.Minute), 10*time.Minute)

	_, body := h.get(t, "/api/v1/products/search?q=sepatu")

	index, ok := body["index"].(map[string]any)
	require.True(t, ok)
	require.Nil(t, index["indexed_through_ms"])
	require.Equal(t, float64(60), index["age_seconds"])
	require.Equal(t, false, index["stale"])
}

func TestAnEmptyQueryIsBrowsing(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	rec, _ := h.get(t, "/api/v1/products/search?category=sepatu")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, h.searcher.Asked[0].Text)
}

func TestHitsCarryThePriceExactly(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)
	h.searcher.Results = search.Results[search.ProductDoc]{
		Total: 1,
		Hits: []search.Hit[search.ProductDoc]{{
			Doc: search.ProductDoc{
				ID:         productID(t, "p-1"),
				PriceMinor: 9_007_199_254_740_99,
				Currency:   "IDR",
			},
		}},
	}

	_, body := h.get(t, "/api/v1/products/search?q=x")

	hits, ok := body["hits"].([]any)
	require.True(t, ok)
	require.Len(t, hits, 1)
	first, ok := hits[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "p-1", first["id"])
	require.Equal(t, "IDR", first["currency"])
}

func TestSuggestReturnsAnArrayRatherThanNull(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)

	rec, body := h.get(t, "/api/v1/products/suggest?q=sep")

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, body["suggestions"])
	require.IsType(t, []any{}, body["suggestions"])
}

func TestSuggestWithNoPrefixAsksNothing(t *testing.T) {
	for _, target := range []string{"/api/v1/products/suggest", "/api/v1/products/suggest?q=%20%20"} {
		h := newHarness(t, noon, noon, time.Minute)

		rec, body := h.get(t, target)

		require.Equal(t, http.StatusOK, rec.Code, target)
		require.Equal(t, []any{}, body["suggestions"], target)
		require.Empty(t, h.searcher.Suggested, target)
	}
}

func TestReadinessRefusesBeforeAnythingIsIndexed(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)
	h.ready = search.Errf(search.KindIndexUnavailable, "searchd.ready", nil, "no generation has been promoted yet")

	rec, body := h.get(t, "/health/ready")

	require.Equal(
		t,
		http.StatusServiceUnavailable,
		rec.Code,
		"serving searches against an index that does not exist answers every query with nothing",
	)
	require.Equal(t, "index_unavailable", body["error"])
}

func TestLivenessDoesNotDependOnAnybodyElse(t *testing.T) {
	h := newHarness(t, noon, noon, time.Minute)
	h.ready = search.Errf(search.KindDependencyUnavailable, "searchd.ready", nil, "database down")

	rec, _ := h.get(t, "/health/live")

	require.Equal(t, http.StatusOK, rec.Code)
}
