package search_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func TestNewQueryDefaults(t *testing.T) {
	q, err := search.NewQuery("  sepatu kulit  ")
	require.NoError(t, err)

	require.Equal(t, "sepatu kulit", q.Text, "surrounding space is trimmed, inner space is not")
	require.Equal(t, search.DefaultPageSize, q.Page.Limit)
	require.Equal(t, search.RankByRelevance, q.Ranking)
}

func TestAnEmptyQueryIsBrowsing(t *testing.T) {
	q, err := search.NewQuery("", search.WithCategories("sepatu"))
	require.NoError(t, err)
	require.Empty(t, q.Text)
	require.Equal(t, []string{"sepatu"}, q.Categories)
}

func TestQueryCapsThePageWhateverIsAsked(t *testing.T) {
	q, err := search.NewQuery("sepatu", search.WithPage(0, 1_000_000))
	require.NoError(t, err)
	require.Equal(t, search.MaxPageSize, q.Page.Limit)
}

func TestQueryIgnoresANonsensePage(t *testing.T) {
	q, err := search.NewQuery("sepatu", search.WithPage(-5, -1))
	require.NoError(t, err)
	require.Equal(t, 0, q.Page.Offset)
	require.Equal(t, search.DefaultPageSize, q.Page.Limit)
}

func TestQueryRefusesWhatCannotBeSearchedFor(t *testing.T) {
	_, err := search.NewQuery(strings.Repeat("x", search.MaxQueryLength+1))
	require.Error(t, err)
	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))

	_, err = search.NewQuery("sepatu\x1b[31m")
	require.Error(t, err)
	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
}

func TestForPrincipalIsNotAnOption(t *testing.T) {
	q, err := search.NewQuery("KNX-2026")
	require.NoError(t, err)
	require.True(t, q.OnBehalfOf.IsZero())

	buyer, err := search.NewMerchantID("principal-9")
	require.NoError(t, err)

	scoped := q.ForPrincipal(buyer)
	require.Equal(t, buyer, scoped.OnBehalfOf)
	require.True(t, q.OnBehalfOf.IsZero(), "ForPrincipal returns a copy; the original is untouched")
}

func TestRankingProfileFallsBackRatherThanBeingEmpty(t *testing.T) {
	q, err := search.NewQuery("sepatu", search.WithRanking(""))
	require.NoError(t, err)
	require.Equal(t, search.RankByRelevance, q.Ranking)

	q, err = search.NewQuery("sepatu", search.WithRanking(search.RankByPriceAsc))
	require.NoError(t, err)
	require.Equal(t, search.RankByPriceAsc, q.Ranking)
}
