package typesense

import (
	"github.com/typesense/typesense-go/v3/typesense/api"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func ptr[T any](v T) *T {
	return &v
}

func SchemaFor(collection search.Collection, gen search.Generation) *api.CollectionSchema {
	switch collection {
	case search.Products:
		return productSchema(gen)
	case search.Merchants:
		return merchantSchema(gen)
	case search.Orders:
		return orderSchema(gen)
	default:
		return nil
	}
}

func QueryFieldsFor(collection search.Collection) (fields, weights string) {
	switch collection {
	case search.Products:
		return "title,categories,description", "4,2,1"
	case search.Merchants:
		return "display_name,categories", "3,1"
	case search.Orders:
		return "number,line_titles,status", "5,2,1"
	default:
		return "", ""
	}
}

func FacetFieldsFor(collection search.Collection) string {
	switch collection {
	case search.Products:
		return "categories,merchant_id,currency"
	case search.Merchants:
		return "categories"
	case search.Orders:
		return "status"
	default:
		return ""
	}
}

func productSchema(gen search.Generation) *api.CollectionSchema {
	return &api.CollectionSchema{
		Name: string(gen),
		Fields: []api.Field{
			{Name: "id", Type: "string"},
			{Name: "merchant_id", Type: "string", Facet: ptr(true)},
			{Name: "title", Type: "string"},
			{Name: "description", Type: "string", Optional: ptr(true)},
			{Name: "categories", Type: "string[]", Facet: ptr(true), Optional: ptr(true)},
			{Name: "price_minor", Type: "int64", Sort: ptr(true), RangeIndex: ptr(true)},
			{Name: "currency", Type: "string", Facet: ptr(true)},
			{Name: "updated_at", Type: "int64", Sort: ptr(true)},
		},
	}
}

func merchantSchema(gen search.Generation) *api.CollectionSchema {
	return &api.CollectionSchema{
		Name: string(gen),
		Fields: []api.Field{
			{Name: "id", Type: "string"},
			{Name: "display_name", Type: "string"},
			{Name: "categories", Type: "string[]", Facet: ptr(true), Optional: ptr(true)},
			{Name: "updated_at", Type: "int64", Sort: ptr(true)},
		},
	}
}

func orderSchema(gen search.Generation) *api.CollectionSchema {
	return &api.CollectionSchema{
		Name: string(gen),
		Fields: []api.Field{
			{Name: "id", Type: "string"},
			{Name: "buyer", Type: "string", Facet: ptr(true)},
			{Name: "merchant_id", Type: "string", Facet: ptr(true)},
			{Name: "number", Type: "string"},
			{Name: "status", Type: "string", Facet: ptr(true)},
			{Name: "line_titles", Type: "string[]", Optional: ptr(true)},
			{Name: "placed_at", Type: "int64", Sort: ptr(true)},
			{Name: "updated_at", Type: "int64", Sort: ptr(true)},
		},
	}
}
