package typesense

import (
	"context"

	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Searcher[D any] struct {
	client     *typesense.Client
	collection search.Collection
	decode     func(map[string]any) (D, error)
}

var _ search.Searcher[search.ProductDoc] = (*Searcher[search.ProductDoc])(nil)

func NewSearcher[D any](
	client *typesense.Client,
	collection search.Collection,
	decode func(map[string]any) (D, error),
) *Searcher[D] {
	return &Searcher[D]{client: client, collection: collection, decode: decode}
}

func (s *Searcher[D]) Search(ctx context.Context, q search.Query) (search.Results[D], error) {
	fields, weights := QueryFieldsFor(s.collection)

	text := q.Text
	if text == "" {
		text = "*"
	}

	params := &api.SearchCollectionParams{
		Q:                    &text,
		QueryBy:              &fields,
		QueryByWeights:       &weights,
		SortBy:               ptr(SortFor(s.collection, q.Ranking)),
		Offset:               ptr(q.Page.Offset),
		Limit:                ptr(q.Page.Limit),
		PrioritizeExactMatch: ptr(true),
	}

	filter, err := FilterFor(s.collection, q)
	if err != nil {
		return search.Results[D]{}, err
	}

	if filter != "" {
		params.FilterBy = &filter
	}

	if facets := FacetFieldsFor(s.collection); facets != "" {
		params.FacetBy = &facets
	}

	res, err := s.client.Collection(string(s.collection)).Documents().Search(ctx, params)
	if err != nil {
		return search.Results[D]{}, failQuery("typesense.Search", err, "searching %s", s.collection)
	}

	return s.toResults(res)
}

func (s *Searcher[D]) toResults(res *api.SearchResult) (search.Results[D], error) {
	out := search.Results[D]{}

	if res.Found != nil {
		out.Total = int64(*res.Found)
	}

	if res.Hits != nil {
		out.Hits = make([]search.Hit[D], 0, len(*res.Hits))

		for _, hit := range *res.Hits {
			if hit.Document == nil {
				continue
			}

			doc, err := s.decode(*hit.Document)
			if err != nil {
				return search.Results[D]{}, err
			}

			var score float64
			if hit.TextMatch != nil {
				score = float64(*hit.TextMatch)
			}

			out.Hits = append(out.Hits, search.Hit[D]{Score: score, Doc: doc})
		}
	}

	if res.FacetCounts != nil {
		out.Facets = map[string][]search.FacetValue{}

		for _, facet := range *res.FacetCounts {
			if facet.FieldName == nil || facet.Counts == nil {
				continue
			}

			values := make([]search.FacetValue, 0, len(*facet.Counts))

			for _, c := range *facet.Counts {
				if c.Value == nil || c.Count == nil {
					continue
				}

				values = append(values, search.FacetValue{Value: *c.Value, Count: int64(*c.Count)})
			}

			out.Facets[*facet.FieldName] = values
		}
	}

	return out, nil
}

func (s *Searcher[D]) Suggest(ctx context.Context, q search.SuggestQuery) ([]string, error) {
	fields, _ := QueryFieldsFor(s.collection)

	params := &api.SearchCollectionParams{
		Q:             &q.Prefix,
		QueryBy:       &fields,
		Offset:        ptr(0),
		Limit:         &q.Limit,
		IncludeFields: ptr(suggestFieldFor(s.collection)),
	}

	res, err := s.client.Collection(string(s.collection)).Documents().Search(ctx, params)
	if err != nil {
		return nil, failQuery("typesense.Suggest", err, "suggesting in %s", s.collection)
	}

	if res.Hits == nil {
		return []string{}, nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(*res.Hits))

	for _, hit := range *res.Hits {
		if hit.Document == nil {
			continue
		}

		value := text(*hit.Document, suggestFieldFor(s.collection))
		if value == "" {
			continue
		}

		if _, already := seen[value]; already {
			continue
		}

		seen[value] = struct{}{}
		out = append(out, value)
	}

	return out, nil
}

func suggestFieldFor(collection search.Collection) string {
	switch collection {
	case search.Merchants:
		return "display_name"
	case search.Orders:
		return "number"
	case search.Products:
		return "title"
	default:
		return "title"
	}
}
