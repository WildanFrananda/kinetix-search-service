package grpcapi

import (
	commonv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/common/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func toMoney(doc search.ProductDoc) *commonv1.Money {
	if doc.Currency == "" && doc.PriceMinor == 0 {
		return nil
	}

	return &commonv1.Money{AmountMinor: doc.PriceMinor, Currency: doc.Currency}
}
