package typesense_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

func fieldNames(t *testing.T, c search.Collection) map[string]bool {
	t.Helper()
	schema := typesense.SchemaFor(c, typesense.NewGeneration(c, 1))
	require.NotNil(t, schema)
	names := map[string]bool{}
	for _, f := range schema.Fields {
		names[f.Name] = true
	}
	return names
}

func TestEveryQueryFieldExistsInItsSchema(t *testing.T) {
	cases := map[search.Collection][]string{
		search.Products:  {"title", "categories", "description", "merchant_id", "currency", "price_minor", "updated_at"},
		search.Merchants: {"display_name", "categories", "updated_at"},
		search.Orders:    {"number", "line_titles", "status", "buyer", "merchant_id", "placed_at", "updated_at"},
	}

	for collection, expected := range cases {
		t.Run(string(collection), func(t *testing.T) {
			names := fieldNames(t, collection)
			for _, f := range expected {
				require.Truef(t, names[f], "%s is queried or filtered on but not in the schema", f)
			}
			require.True(t, names["id"], "every collection needs an id")
		})
	}
}

func TestSortableFieldsAreDeclaredSortable(t *testing.T) {
	schema := typesense.SchemaFor(search.Products, typesense.NewGeneration(search.Products, 1))
	sortable := map[string]bool{}
	for _, f := range schema.Fields {
		if f.Sort != nil && *f.Sort {
			sortable[f.Name] = true
		}
	}
	require.True(t, sortable["price_minor"], "RankByPriceAsc sorts on it")
	require.True(t, sortable["updated_at"], "RankByNewest sorts on it")
}

func TestFacetFieldsAreDeclaredFacetable(t *testing.T) {
	for _, c := range []search.Collection{search.Products, search.Merchants, search.Orders} {
		schema := typesense.SchemaFor(c, typesense.NewGeneration(c, 1))
		facetable := map[string]bool{}
		for _, f := range schema.Fields {
			if f.Facet != nil && *f.Facet {
				facetable[f.Name] = true
			}
		}
		for _, wanted := range splitList(typesense.FacetFieldsFor(c)) {
			require.Truef(t, facetable[wanted], "%s/%s is faceted on but not declared a facet", c, wanted)
		}
	}
}

func TestOrdersCarryNothingSensitive(t *testing.T) {
	forbidden := []string{
		"address",
		"shipping_address",
		"total",
		"total_minor",
		"payment",
		"card",
		"phone",
		"email",
		"buyer_name",
	}
	names := fieldNames(t, search.Orders)
	for _, f := range forbidden {
		require.Falsef(t, names[f], "orders must not index %q", f)
	}
}

func TestGenerationNamesAreDistinctAndPrefixed(t *testing.T) {
	first := typesense.NewGeneration(search.Products, time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC).Unix())
	second := typesense.NewGeneration(search.Products, time.Date(2026, 9, 21, 12, 0, 1, 0, time.UTC).Unix())

	require.NotEqual(t, first, second)
	require.Contains(t, string(first), string(search.Products))
}

func TestUnknownCollectionHasNoSchema(t *testing.T) {
	require.Nil(t, typesense.SchemaFor(search.Collection("invoices"), "invoices_v1"))
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return out
}
