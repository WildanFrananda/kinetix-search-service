package search

import "strings"

const (
	DefaultPageSize = 24
	MaxPageSize     = 100
	MaxQueryLength  = 256
)

type RankingProfile string

const (
	RankByRelevance RankingProfile = "relevance"
	RankByNewest    RankingProfile = "newest"
	RankByPriceAsc  RankingProfile = "price_asc"
)

type Page struct {
	Offset int
	Limit  int
}

type Query struct {
	Text       string
	Categories []string
	MerchantID MerchantID
	OnBehalfOf MerchantID
	Page       Page
	Ranking    RankingProfile
}

func (q Query) ForPrincipal(p MerchantID) Query {
	q.OnBehalfOf = p; return q
}

type QueryOption func(*Query)

func WithCategories(c ...string) QueryOption {
	return func(q *Query) {
		q.Categories = c
	}
}
func WithMerchant(m MerchantID) QueryOption {
	return func(q *Query) {
		q.MerchantID = m
	}
}

func WithRanking(r RankingProfile) QueryOption {
	return func(q *Query) {
		if r != "" {
			q.Ranking = r
		}
	}
}

func WithPage(offset, limit int) QueryOption {
	return func(q *Query) {
		if offset > 0 {
			q.Page.Offset = offset
		}
		if limit > 0 {
			q.Page.Limit = min(limit, MaxPageSize)
		}
	}
}

func NewQuery(text string, opts ...QueryOption) (Query, error) {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > MaxQueryLength {
		return Query{}, Errf(KindMalformedQuery, "search.NewQuery", nil, "query too long")
	}
	if strings.ContainsFunc(trimmed, isControl) {
		return Query{}, Errf(KindMalformedQuery, "search.NewQuery", nil, "control byte in query")
	}

	q := Query{Text: trimmed, Page: Page{Limit: DefaultPageSize}, Ranking: RankByRelevance}
	for _, opt := range opts {
		opt(&q)
	}
	return q, nil
}
