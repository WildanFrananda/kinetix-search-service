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
	q.OnBehalfOf = p

	return q
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
		q.Page = Page{Offset: offset, Limit: limit}
	}
}

func NewQuery(text string, opts ...QueryOption) (Query, error) {
	const op = "search.NewQuery"

	trimmed, err := searchable(op, "query", text)
	if err != nil {
		return Query{}, err
	}

	q := Query{Text: trimmed, Ranking: RankByRelevance}
	for _, opt := range opts {
		opt(&q)
	}

	if q.Page.Offset < 0 {
		return Query{}, Errf(KindMalformedQuery, op, nil, "offset %d is negative", q.Page.Offset)
	}

	q.Page.Limit, err = pageSize(op, q.Page.Limit, DefaultPageSize)
	if err != nil {
		return Query{}, err
	}

	switch q.Ranking {
	case RankByRelevance, RankByNewest, RankByPriceAsc:
	default:
		return Query{}, Errf(KindMalformedQuery, op, nil, "unknown ranking profile %q", q.Ranking)
	}

	q.Categories, err = categories(op, q.Categories)
	if err != nil {
		return Query{}, err
	}

	return q, nil
}

func searchable(op, what, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > MaxQueryLength {
		return "", Errf(KindMalformedQuery, op, nil, "%s too long", what)
	}

	if strings.ContainsFunc(trimmed, isControl) {
		return "", Errf(KindMalformedQuery, op, nil, "control byte in %s", what)
	}

	return trimmed, nil
}

func pageSize(op string, limit, fallback int) (int, error) {
	switch {
	case limit < 0:
		return 0, Errf(KindMalformedQuery, op, nil, "limit %d is negative", limit)
	case limit == 0:
		return fallback, nil
	default:
		return min(limit, MaxPageSize), nil
	}
}

func categories(op string, raw []string) ([]string, error) {
	kept := make([]string, 0, len(raw))

	for _, c := range raw {
		trimmed, err := searchable(op, "category", c)
		if err != nil {
			return nil, err
		}

		if trimmed != "" {
			kept = append(kept, trimmed)
		}
	}

	if len(kept) == 0 {
		return nil, nil
	}

	return kept, nil
}
