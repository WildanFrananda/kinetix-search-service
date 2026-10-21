package typesense_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
	"github.com/WildanFrananda/kinetix-search-service/tests/searchtest"
)

const emptyResult = `{"found":0,"hits":[],"out_of":0,"search_time_ms":0}`

func searcherAgainst(t *testing.T, engine *searchtest.FakeEngine) *typesense.Searcher[search.ProductDoc] {
	t.Helper()

	client, err := typesense.Connect(typesense.Settings{URL: engine.URL, APIKey: "fake", Timeout: time.Second})
	require.NoError(t, err)

	return typesense.NewSearcher(client, search.Products, typesense.DecodeProduct)
}

func TestAClientWithNoTimeoutIsRefusedRatherThanGivenOne(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		settings := typesense.Settings{URL: "http://127.0.0.1:1", APIKey: "fake", Timeout: timeout}

		client, err := typesense.Connect(settings)

		require.Error(t, err)
		require.Nil(t, client)
	}
}

func TestTheEngineIsAskedForTheExactOffset(t *testing.T) {
	engine := searchtest.NewFakeEngine(t, http.StatusOK, emptyResult)
	q, err := search.NewQuery("sepatu", search.WithPage(50, 24))
	require.NoError(t, err)

	_, err = searcherAgainst(t, engine).Search(context.Background(), q)

	require.NoError(t, err)
	require.Len(t, engine.Asked(), 1)

	asked := engine.Asked()[0]
	require.Equal(t, "50", asked.Get("offset"), "offset 50 is the 51st hit, not the start of page 3")
	require.Equal(t, "24", asked.Get("limit"))
	require.False(t, asked.Has("page"), "page and offset together would let the engine pick one")
	require.False(t, asked.Has("per_page"))
}

func TestARequestTheEngineRefusesIsTheCallersFault(t *testing.T) {
	refusals := map[int]string{
		http.StatusBadRequest:          `{"message":"Filter value cannot be empty."}`,
		http.StatusUnprocessableEntity: `{"message":"Only upto 250 hits can be fetched per page."}`,
	}

	for status, body := range refusals {
		engine := searchtest.NewFakeEngine(t, status, body)
		q, err := search.NewQuery("sepatu")
		require.NoError(t, err)

		_, err = searcherAgainst(t, engine).Search(context.Background(), q)

		require.Error(t, err)
		require.Equal(t, search.KindMalformedQuery, search.KindOf(err), "engine status %d", status)
	}
}

func TestAFilterTheEngineCannotMatchExactlyNeverReachesIt(t *testing.T) {
	engine := searchtest.NewFakeEngine(t, http.StatusOK, emptyResult)
	q, err := search.NewQuery("", search.WithCategories("a`b"))
	require.NoError(t, err)

	_, err = searcherAgainst(t, engine).Search(context.Background(), q)

	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
	require.Empty(t, engine.Asked())
}

func TestAnEngineThatCannotAnswerIsUnavailable(t *testing.T) {
	failures := map[int]string{
		http.StatusNotFound:           `{"message":"Collection not found"}`,
		http.StatusUnauthorized:       `{"message":"Forbidden - a valid x-typesense-api-key header must be sent."}`,
		http.StatusServiceUnavailable: `{"message":"Not Ready or Lagging"}`,
	}

	for status, body := range failures {
		engine := searchtest.NewFakeEngine(t, status, body)
		q, err := search.NewQuery("sepatu")
		require.NoError(t, err)

		_, err = searcherAgainst(t, engine).Search(context.Background(), q)

		require.Error(t, err)
		require.Equal(t, search.KindIndexUnavailable, search.KindOf(err), "engine status %d", status)
	}
}

func TestSuggestAsksForTheLimitItWasGiven(t *testing.T) {
	engine := searchtest.NewFakeEngine(t, http.StatusOK, emptyResult)
	q, err := search.NewSuggestQuery("sep", 7)
	require.NoError(t, err)

	suggestions, err := searcherAgainst(t, engine).Suggest(context.Background(), q)

	require.NoError(t, err)
	require.Equal(t, []string{}, suggestions)
	require.Len(t, engine.Asked(), 1)
	require.Equal(t, "7", engine.Asked()[0].Get("limit"))
	require.False(t, engine.Asked()[0].Has("per_page"))
}

func TestASuggestionTheEngineRefusesIsTheCallersFault(t *testing.T) {
	engine := searchtest.NewFakeEngine(t, http.StatusBadRequest, `{"message":"Query string exceeds max length."}`)
	q, err := search.NewSuggestQuery("sep", 5)
	require.NoError(t, err)

	_, err = searcherAgainst(t, engine).Suggest(context.Background(), q)

	require.Error(t, err)
	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
}
