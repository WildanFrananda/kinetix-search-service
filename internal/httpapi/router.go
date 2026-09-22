package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Server struct {
	products *querying.Finder[search.ProductDoc]
	ready    func(context.Context) error
}

func NewRouter(
	products *querying.Finder[search.ProductDoc],
	ready func(context.Context) error,
) http.Handler {
	s := &Server{products: products, ready: ready}

	r := chi.NewRouter()
	r.Get("/api/v1/products/search", s.searchProducts)
	r.Get("/api/v1/products/suggest", s.suggestProducts)
	r.Get("/health/live", s.live)
	r.Get("/health/ready", s.readiness)
	return r
}

func (s *Server) searchProducts(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	q, err := search.NewQuery(
		params.Get("q"),
		search.WithPage(intParam(params.Get("offset")), intParam(params.Get("limit"))),
		search.WithRanking(search.RankingProfile(params.Get("sort"))),
		search.WithCategories(params["category"]...),
	)
	if err != nil {
		writeFailure(w, err)
		return
	}

	results, err := s.products.Find(r.Context(), q)
	if err != nil {
		writeFailure(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toSearchResponse(results))
}

func (s *Server) suggestProducts(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("q")
	if prefix == "" {
		writeJSON(w, http.StatusOK, suggestResponse{Suggestions: []string{}})
		return
	}

	suggestions, err := s.products.Suggest(r.Context(), prefix, intParam(r.URL.Query().Get("limit")))
	if err != nil {
		writeFailure(w, err)
		return
	}
	if suggestions == nil {
		suggestions = []string{}
	}

	writeJSON(w, http.StatusOK, suggestResponse{Suggestions: suggestions})
}

func (s *Server) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "live"})
}

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.ready(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{
			Error: search.KindOf(err).String(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func intParam(raw string) int {
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}
