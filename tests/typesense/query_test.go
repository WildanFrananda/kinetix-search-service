package typesense_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

func principal(t *testing.T, raw string) search.MerchantID {
	t.Helper()
	id, err := search.NewMerchantID(raw)
	require.NoError(t, err)
	return id
}

func TestOrdersAreScopedToBothSidesOfThePrincipal(t *testing.T) {
	q, err := search.NewQuery("KNX-2026")
	require.NoError(t, err)
	q = q.ForPrincipal(principal(t, "principal-9"))

	filter := typesense.FilterFor(search.Orders, q)

	require.Equal(
		t,
		"(buyer:=`principal-9` || merchant_id:=`principal-9`)",
		filter,
		"a principal is the buyer on their purchases and the merchant on their sales",
	)
}

func TestPublicCollectionsIgnoreThePrincipalScope(t *testing.T) {
	q, err := search.NewQuery("sepatu")
	require.NoError(t, err)
	q = q.ForPrincipal(principal(t, "principal-9"))

	require.Empty(t, typesense.FilterFor(search.Products, q))
	require.Empty(t, typesense.FilterFor(search.Merchants, q))
}

func TestFilterValuesCannotCloseTheirOwnQuote(t *testing.T) {
	q, err := search.NewQuery("sepatu", search.WithCategories("shoes` || id:=*"))
	require.NoError(t, err)

	filter := typesense.FilterFor(search.Products, q)

	require.Equal(t, "categories:=[`shoes || id:=*`]", filter)
	require.Equal(t, 2, strings.Count(filter, "`"), "exactly one opening and one closing quote")
}

func TestCategoriesAreOredTogether(t *testing.T) {
	q, err := search.NewQuery("", search.WithCategories("sepatu", "sandal"))
	require.NoError(t, err)

	require.Equal(t, "categories:=[`sepatu`,`sandal`]", typesense.FilterFor(search.Products, q))
}

func TestFiltersCombineWithAnd(t *testing.T) {
	q, err := search.NewQuery(
		"",
		search.WithMerchant(principal(t, "shop-1")),
		search.WithCategories("sepatu"),
	)
	require.NoError(t, err)

	require.Equal(
		t,
		"merchant_id:=`shop-1` && categories:=[`sepatu`]",
		typesense.FilterFor(search.Products, q),
	)
}

func TestSortByProfile(t *testing.T) {
	require.Equal(t, "_text_match:desc", typesense.SortFor(search.Products, search.RankByRelevance))
	require.Equal(t, "updated_at:desc", typesense.SortFor(search.Products, search.RankByNewest))
	require.Equal(t, "price_minor:asc", typesense.SortFor(search.Products, search.RankByPriceAsc))
}

func TestSortByPriceFallsBackWhereThereIsNoPrice(t *testing.T) {
	require.Equal(t, "_text_match:desc", typesense.SortFor(search.Merchants, search.RankByPriceAsc))
	require.Equal(t, "_text_match:desc", typesense.SortFor(search.Orders, search.RankByPriceAsc))
}

func TestPageConversionIsOneBased(t *testing.T) {
	page, perPage := typesense.PageFor(search.Page{Offset: 0, Limit: 24})
	require.Equal(t, 1, page)
	require.Equal(t, 24, perPage)

	page, _ = typesense.PageFor(search.Page{Offset: 48, Limit: 24})
	require.Equal(t, 3, page)

	page, _ = typesense.PageFor(search.Page{Offset: 50, Limit: 24})
	require.Equal(t, 3, page)
}

func TestQueryFieldsAndWeightsAgree(t *testing.T) {
	for _, c := range []search.Collection{search.Products, search.Merchants, search.Orders} {
		fields, weights := typesense.QueryFieldsFor(c)
		require.NotEmpty(t, fields, string(c))
		require.Len(t, strings.Split(weights, ","), len(strings.Split(fields, ",")), string(c))
	}
}
