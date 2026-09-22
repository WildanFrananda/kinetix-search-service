package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type productHit struct {
	ID          string   `json:"id"`
	MerchantID  string   `json:"merchant_id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	PriceMinor  int64    `json:"price_minor"`
	Currency    string   `json:"currency"`
	UpdatedAtMS int64    `json:"updated_at_ms"`
	Score       float64  `json:"score"`
}

type facetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type indexInfo struct {
	IndexedThroughMS int64 `json:"indexed_through_ms"`
	AgeSeconds       int64 `json:"age_seconds"`
	Stale            bool  `json:"stale"`
}

type searchResponse struct {
	Total  int64                   `json:"total"`
	Hits   []productHit            `json:"hits"`
	Facets map[string][]facetValue `json:"facets,omitempty"`
	Index  indexInfo               `json:"index"`
}

type suggestResponse struct {
	Suggestions []string `json:"suggestions"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func toSearchResponse(results search.Results[search.ProductDoc]) searchResponse {
	hits := make([]productHit, 0, len(results.Hits))
	for _, h := range results.Hits {
		hits = append(hits, productHit{
			ID:          h.Doc.ID.String(),
			MerchantID:  h.Doc.MerchantID.String(),
			Title:       h.Doc.Title,
			Description: h.Doc.Description,
			Categories:  h.Doc.Categories,
			PriceMinor:  h.Doc.PriceMinor,
			Currency:    h.Doc.Currency,
			UpdatedAtMS: h.Doc.UpdatedAt.UnixMilli(),
			Score:       h.Score,
		})
	}

	var facets map[string][]facetValue
	if len(results.Facets) > 0 {
		facets = make(map[string][]facetValue, len(results.Facets))
		for name, values := range results.Facets {
			out := make([]facetValue, 0, len(values))
			for _, v := range values {
				out = append(out, facetValue{Value: v.Value, Count: v.Count})
			}
			facets[name] = out
		}
	}

	return searchResponse{
		Total:  results.Total,
		Hits:   hits,
		Facets: facets,
		Index: indexInfo{
			IndexedThroughMS: results.Freshness.IndexedThrough.UnixMilli(),
			AgeSeconds:       int64(results.Freshness.Age.Seconds()),
			Stale:            results.Freshness.Stale,
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeFailure(w http.ResponseWriter, err error) {
	kind := search.KindOf(err)
	writeJSON(w, statusFor(kind), errorResponse{Error: kind.String()})
}
