package grpcapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/search/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/grpcapi"
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

func newServer(
	t *testing.T,
	indexedThrough time.Time,
	syncedAt time.Time,
	staleAfter time.Duration,
) (*grpcapi.Server, *searchtest.FakeSearcher[search.ProductDoc]) {
	t.Helper()
	searcher := &searchtest.FakeSearcher[search.ProductDoc]{}
	checkpoint := &searchtest.FakeCheckpoint{
		Cursors: map[search.Collection]search.Cursor{
			search.Products: {UpdatedThrough: indexedThrough},
		},
		SavedAt: map[search.Collection]time.Time{search.Products: syncedAt},
	}
	finder := querying.NewFinder(
		search.Products,
		searcher,
		checkpoint,
		searchtest.FixedClock{At: noon},
		staleAfter,
	)

	return grpcapi.NewServer(finder), searcher
}

func TestFreshnessTravelsOverGRPCToo(t *testing.T) {
	server, _ := newServer(t, noon.Add(-2*time.Minute), noon.Add(-2*time.Minute), 10*time.Minute)

	response, err := server.SearchProducts(context.Background(),
		&searchv1.SearchProductsRequest{Text: "sepatu"})

	require.NoError(t, err)
	require.NotNil(t, response.GetFreshness())
	require.Equal(t, int64(120), response.GetFreshness().GetAgeSeconds())
	require.False(t, response.GetFreshness().GetStale())
}

func TestAStaleIndexIsReportedNotHidden(t *testing.T) {
	server, _ := newServer(t, noon.Add(-2*time.Hour), noon.Add(-2*time.Hour), 10*time.Minute)

	response, err := server.SearchProducts(
		context.Background(),
		&searchv1.SearchProductsRequest{Text: "sepatu"})

	require.NoError(t, err)
	require.True(t, response.GetFreshness().GetStale())
}

func TestAnOutageIsUnavailableNotAnEmptyResult(t *testing.T) {
	server, searcher := newServer(t, noon, noon, time.Minute)
	searcher.Err = search.Errf(
		search.KindIndexUnavailable,
		"typesense.Search",
		nil,
		"cluster unreachable",
	)

	_, err := server.SearchProducts(context.Background(),
		&searchv1.SearchProductsRequest{Text: "sepatu"})

	require.Error(t, err)
	require.Equal(
		t,
		codes.Unavailable,
		status.Code(err),
		"an empty response would tell a caller the shop has nothing",
	)
	require.Contains(t, status.Convert(err).Message(), "index_unavailable")
}

func TestAMalformedQueryIsInvalidArgument(t *testing.T) {
	server, _ := newServer(t, noon, noon, time.Minute)

	_, err := server.SearchProducts(
		context.Background(),
		&searchv1.SearchProductsRequest{Text: "sepatu\x1b[31m"},
	)

	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestAnUnusableMerchantFilterIsRefused(t *testing.T) {
	server, searcher := newServer(t, noon, noon, time.Minute)

	_, err := server.SearchProducts(
		context.Background(),
		&searchv1.SearchProductsRequest{MerchantPrincipalId: "   "},
	)

	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Empty(t, searcher.Asked, "a refused filter must not reach the engine")
}

func TestRankingProfilesMapAcross(t *testing.T) {
	cases := map[searchv1.RankingProfile]search.RankingProfile{
		searchv1.RankingProfile_RANKING_PROFILE_UNSPECIFIED: search.RankByRelevance,
		searchv1.RankingProfile_RANKING_PROFILE_RELEVANCE:   search.RankByRelevance,
		searchv1.RankingProfile_RANKING_PROFILE_NEWEST:      search.RankByNewest,
		searchv1.RankingProfile_RANKING_PROFILE_PRICE_ASC:   search.RankByPriceAsc,
	}

	for wire, want := range cases {
		server, searcher := newServer(t, noon, noon, time.Minute)
		_, err := server.SearchProducts(
			context.Background(),
			&searchv1.SearchProductsRequest{Text: "x", Ranking: wire},
		)
		require.NoError(t, err)
		require.Equal(t, want, searcher.Asked[0].Ranking, wire.String())
	}
}

func TestThePageIsStillCapped(t *testing.T) {
	server, searcher := newServer(t, noon, noon, time.Minute)

	_, err := server.SearchProducts(context.Background(), &searchv1.SearchProductsRequest{
		Text: "x",
		Page: &searchv1.Page{Offset: 10, Limit: 1_000_000},
	})

	require.NoError(t, err)
	require.Equal(t, search.MaxPageSize, searcher.Asked[0].Page.Limit)
	require.Equal(t, 10, searcher.Asked[0].Page.Offset)
}

func TestAHitCarriesTheSharedMoneyType(t *testing.T) {
	server, searcher := newServer(t, noon, noon, time.Minute)
	searcher.Results = search.Results[search.ProductDoc]{
		Total: 1,
		Hits: []search.Hit[search.ProductDoc]{{
			Score: 7,
			Doc: search.ProductDoc{
				ID: productID(t, "p-1"), Title: "Sepatu",
				PriceMinor: 75_000_000, Currency: "IDR", UpdatedAt: noon,
			},
		}},
	}

	response, err := server.SearchProducts(
		context.Background(),
		&searchv1.SearchProductsRequest{Text: "sepatu"},
	)

	require.NoError(t, err)
	require.Len(t, response.GetHits(), 1)
	hit := response.GetHits()[0]
	require.Equal(t, "p-1", hit.GetSku())
	require.Equal(t, int64(75_000_000), hit.GetPrice().GetAmountMinor())
	require.Equal(t, "IDR", hit.GetPrice().GetCurrency())
	require.Equal(t, noon, hit.GetUpdatedAt().AsTime().UTC())
}

func TestFacetsCrossTheWire(t *testing.T) {
	server, searcher := newServer(t, noon, noon, time.Minute)
	searcher.Results = search.Results[search.ProductDoc]{
		Facets: map[string][]search.FacetValue{
			"categories": {{Value: "sepatu", Count: 4}},
		},
	}

	response, err := server.SearchProducts(
		context.Background(),
		&searchv1.SearchProductsRequest{Text: "x"},
	)

	require.NoError(t, err)
	require.Len(t, response.GetFacets(), 1)
	require.Equal(t, "categories", response.GetFacets()[0].GetField())
	require.Equal(t, int64(4), response.GetFacets()[0].GetValues()[0].GetCount())
}

func TestSuggestAnswersAnEmptyListRatherThanNil(t *testing.T) {
	server, _ := newServer(t, noon, noon, time.Minute)

	response, err := server.SuggestProducts(
		context.Background(),
		&searchv1.SuggestProductsRequest{Prefix: "sep", Limit: 3},
	)

	require.NoError(t, err)
	require.NotNil(t, response.GetSuggestions())
}
