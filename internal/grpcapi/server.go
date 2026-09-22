package grpcapi

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	searchv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/search/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Server struct {
	searchv1.UnimplementedSearchServiceServer

	products *querying.Finder[search.ProductDoc]
}

func NewServer(products *querying.Finder[search.ProductDoc]) *Server {
	return &Server{products: products}
}

func (s *Server) SearchProducts(
	ctx context.Context,
	request *searchv1.SearchProductsRequest,
) (*searchv1.SearchProductsResponse, error) {
	q, err := toQuery(request)

	if err != nil {
		return nil, toStatus(err)
	}

	results, err := s.products.Find(ctx, q)

	if err != nil {
		return nil, toStatus(err)
	}

	return toSearchResponse(results), nil
}

func (s *Server) SuggestProducts(
	ctx context.Context,
	request *searchv1.SuggestProductsRequest,
) (*searchv1.SuggestProductsResponse, error) {
	suggestions, err := s.products.Suggest(ctx, request.GetPrefix(), int(request.GetLimit()))

	if err != nil {
		return nil, toStatus(err)
	}

	if suggestions == nil {
		suggestions = []string{}
	}

	return &searchv1.SuggestProductsResponse{Suggestions: suggestions}, nil
}

func toQuery(request *searchv1.SearchProductsRequest) (search.Query, error) {
	options := []search.QueryOption{
		search.WithCategories(request.GetCategories()...),
		search.WithRanking(rankingFrom(request.GetRanking())),
	}

	if page := request.GetPage(); page != nil {
		options = append(options, search.WithPage(int(page.GetOffset()), int(page.GetLimit())))
	}

	if raw := request.GetMerchantPrincipalId(); raw != "" {
		merchant, err := search.NewMerchantID(raw)

		if err != nil {
			return search.Query{}, err
		}

		options = append(options, search.WithMerchant(merchant))
	}

	return search.NewQuery(request.GetText(), options...)
}

func rankingFrom(profile searchv1.RankingProfile) search.RankingProfile {
	switch profile {
	case searchv1.RankingProfile_RANKING_PROFILE_NEWEST:
		return search.RankByNewest
	case searchv1.RankingProfile_RANKING_PROFILE_PRICE_ASC:
		return search.RankByPriceAsc
	case searchv1.RankingProfile_RANKING_PROFILE_RELEVANCE,
		searchv1.RankingProfile_RANKING_PROFILE_UNSPECIFIED:
		return search.RankByRelevance
	default:
		return search.RankByRelevance
	}
}

func toSearchResponse(results search.Results[search.ProductDoc]) *searchv1.SearchProductsResponse {
	hits := make([]*searchv1.ProductHit, 0, len(results.Hits))

	for _, h := range results.Hits {
		hit := &searchv1.ProductHit{
			Sku:                 h.Doc.ID.String(),
			MerchantPrincipalId: h.Doc.MerchantID.String(),
			Title:               h.Doc.Title,
			Description:         h.Doc.Description,
			Price:               toMoney(h.Doc),
			Categories:          h.Doc.Categories,
			Score:               h.Score,
		}

		if !h.Doc.UpdatedAt.IsZero() {
			hit.UpdatedAt = timestamppb.New(h.Doc.UpdatedAt)
		}

		hits = append(hits, hit)
	}

	response := &searchv1.SearchProductsResponse{
		Hits:      hits,
		Total:     results.Total,
		Facets:    toFacets(results.Facets),
		Freshness: toFreshness(results.Freshness),
	}

	return response
}

func toFreshness(f search.Freshness) *searchv1.IndexFreshness {
	out := &searchv1.IndexFreshness{
		AgeSeconds: int64(f.Age.Seconds()),
		Stale:      f.Stale,
	}

	if !f.IndexedThrough.IsZero() {
		out.IndexedThrough = timestamppb.New(f.IndexedThrough)
	}

	return out
}

func toFacets(in map[string][]search.FacetValue) []*searchv1.Facet {
	if len(in) == 0 {
		return nil
	}

	out := make([]*searchv1.Facet, 0, len(in))

	for field, values := range in {
		facet := &searchv1.Facet{Field: field, Values: make([]*searchv1.FacetValue, 0, len(values))}

		for _, v := range values {
			facet.Values = append(facet.Values,
				&searchv1.FacetValue{Value: v.Value, Count: v.Count})
		}

		out = append(out, facet)
	}

	return out
}

func toStatus(err error) error {
	kind := search.KindOf(err)

	return status.Error(codeFor(kind), kind.String())
}

func codeFor(kind search.Kind) codes.Code {
	switch kind {
	case search.KindMalformedQuery:
		return codes.InvalidArgument
	case search.KindNotFound:
		return codes.NotFound
	case search.KindIndexUnavailable, search.KindDependencyUnavailable:
		return codes.Unavailable
	case search.KindStale:
		return codes.OK
	case search.KindCorruptRecord, search.KindUnknown:
		return codes.Internal
	default:
		return codes.Internal
	}
}
