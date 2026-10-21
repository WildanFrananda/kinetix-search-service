package typesense_test

import (
	"slices"
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

func filter(t *testing.T, collection search.Collection, q search.Query) string {
	t.Helper()

	f, err := typesense.FilterFor(collection, q)
	require.NoError(t, err)

	return f
}

func TestOrdersAreScopedToBothSidesOfThePrincipal(t *testing.T) {
	q, err := search.NewQuery("KNX-2026")
	require.NoError(t, err)

	q = q.ForPrincipal(principal(t, "principal-9"))

	require.Equal(
		t,
		"(buyer:=`principal-9` || merchant_id:=`principal-9`)",
		filter(t, search.Orders, q),
		"a principal is the buyer on their purchases and the merchant on their sales",
	)
}

func TestPublicCollectionsIgnoreThePrincipalScope(t *testing.T) {
	q, err := search.NewQuery("sepatu")
	require.NoError(t, err)

	q = q.ForPrincipal(principal(t, "principal-9"))

	require.Empty(t, filter(t, search.Products, q))
	require.Empty(t, filter(t, search.Merchants, q))
}

func TestAFilterValueTheEngineCannotMatchExactlyIsRefused(t *testing.T) {
	for _, category := range []string{"shoes` || id:=*", "sepatu\\", "sep*"} {
		q, err := search.NewQuery("", search.WithCategories(category))
		require.NoError(t, err)

		_, err = typesense.FilterFor(search.Products, q)

		require.Error(t, err, category)
		require.Equal(t, search.KindMalformedQuery, search.KindOf(err), category)
	}

	q, err := search.NewQuery("", search.WithMerchant(principal(t, "shop`1")))
	require.NoError(t, err)

	_, err = typesense.FilterFor(search.Products, q)

	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
}

func TestPunctuationInsideAQuoteIsKept(t *testing.T) {
	q, err := search.NewQuery("", search.WithCategories("Pakaian (Pria)", "c1]", "a && b"))
	require.NoError(t, err)

	require.Equal(t, "categories:=[`Pakaian (Pria)`,`c1]`,`a && b`]", filter(t, search.Products, q))
}

func TestCategoriesAreOredTogether(t *testing.T) {
	q, err := search.NewQuery("", search.WithCategories("sepatu", "sandal"))
	require.NoError(t, err)

	require.Equal(t, "categories:=[`sepatu`,`sandal`]", filter(t, search.Products, q))
}

func TestFiltersCombineWithAnd(t *testing.T) {
	q, err := search.NewQuery("", search.WithMerchant(principal(t, "shop-1")), search.WithCategories("sepatu"))
	require.NoError(t, err)

	require.Equal(t, "merchant_id:=`shop-1` && categories:=[`sepatu`]", filter(t, search.Products, q))
}

func TestEveryFilterFieldIndexesItsPunctuation(t *testing.T) {
	filtered := map[search.Collection][]string{
		search.Products:  {"merchant_id", "categories"},
		search.Merchants: {"categories"},
		search.Orders:    {"buyer", "merchant_id"},
	}

	for collection, names := range filtered {
		schema := typesense.SchemaFor(collection, typesense.NewGeneration(collection, 1))

		for _, field := range schema.Fields {
			if !slices.Contains(names, field.Name) {
				continue
			}

			require.NotNil(t, field.SymbolsToIndex, "%s.%s", collection, field.Name)
			require.Contains(t, *field.SymbolsToIndex, "]", "%s.%s", collection, field.Name)
			require.NotContains(t, *field.SymbolsToIndex, "`", "a backtick indexed breaks the scalar quote")
		}
	}
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

func TestQueryFieldsAndWeightsAgree(t *testing.T) {
	for _, c := range []search.Collection{search.Products, search.Merchants, search.Orders} {
		fields, weights := typesense.QueryFieldsFor(c)
		require.NotEmpty(t, fields, string(c))
		require.Len(t, strings.Split(weights, ","), len(strings.Split(fields, ",")), string(c))
	}
}
