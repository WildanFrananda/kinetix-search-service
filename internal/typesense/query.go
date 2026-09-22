package typesense

import (
	"fmt"
	"strings"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func quote(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "") + "`"
}

func FilterFor(collection search.Collection, q search.Query) string {
	var clauses []string

	if !q.OnBehalfOf.IsZero() {
		switch collection {
		case search.Orders:
			p := quote(q.OnBehalfOf.String())
			clauses = append(clauses, fmt.Sprintf("(buyer:=%s || merchant_id:=%s)", p, p))
		case search.Products, search.Merchants:
		}
	}

	if !q.MerchantID.IsZero() {
		clauses = append(clauses, "merchant_id:="+quote(q.MerchantID.String()))
	}

	if len(q.Categories) > 0 {
		quoted := make([]string, 0, len(q.Categories))
		for _, c := range q.Categories {
			quoted = append(quoted, quote(c))
		}
		clauses = append(clauses, "categories:=["+strings.Join(quoted, ",")+"]")
	}

	return strings.Join(clauses, " && ")
}

func SortFor(collection search.Collection, profile search.RankingProfile) string {
	switch profile {
	case search.RankByNewest:
		return "updated_at:desc"
	case search.RankByPriceAsc:
		if collection == search.Products {
			return "price_minor:asc"
		}
		return "_text_match:desc"
	case search.RankByRelevance:
		return "_text_match:desc"
	default:
		return "_text_match:desc"
	}
}

func PageFor(p search.Page) (page, perPage int) {
	perPage = p.Limit
	if perPage <= 0 {
		perPage = search.DefaultPageSize
	}
	return p.Offset/perPage + 1, perPage
}
