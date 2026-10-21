package typesense

import (
	"fmt"
	"strings"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func quote(value string) (string, error) {
	if strings.ContainsAny(value, "`\\") || strings.HasSuffix(value, "*") {
		return "", search.Errf(
			search.KindMalformedQuery,
			"typesense.FilterFor",
			nil,
			"%q cannot be matched exactly: a filter value may not hold a backtick or a backslash, or end in *",
			value,
		)
	}

	return "`" + value + "`", nil
}

func FilterFor(collection search.Collection, q search.Query) (string, error) {
	var clauses []string

	if !q.OnBehalfOf.IsZero() && collection == search.Orders {
		p, err := quote(q.OnBehalfOf.String())
		if err != nil {
			return "", err
		}

		clauses = append(clauses, fmt.Sprintf("(buyer:=%s || merchant_id:=%s)", p, p))
	}

	if !q.MerchantID.IsZero() {
		m, err := quote(q.MerchantID.String())
		if err != nil {
			return "", err
		}

		clauses = append(clauses, "merchant_id:="+m)
	}

	if len(q.Categories) > 0 {
		quoted := make([]string, 0, len(q.Categories))

		for _, c := range q.Categories {
			value, err := quote(c)
			if err != nil {
				return "", err
			}

			quoted = append(quoted, value)
		}

		clauses = append(clauses, "categories:=["+strings.Join(quoted, ",")+"]")
	}

	return strings.Join(clauses, " && "), nil
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
