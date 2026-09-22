package searchtest

import (
	"context"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type UncountableSource[D any] struct {
	Pages     []search.Changes[D]
	Err       error
	AskedFrom []search.Cursor

	served int
}

var _ search.Source[search.MerchantDoc] = (*UncountableSource[search.MerchantDoc])(nil)

func (u *UncountableSource[D]) ChangedSince(
	_ context.Context,
	cur search.Cursor,
	_ int,
) (search.Changes[D], error) {
	u.AskedFrom = append(u.AskedFrom, cur)

	if u.Err != nil {
		return search.Changes[D]{}, u.Err
	}

	if u.served >= len(u.Pages) {
		return search.Changes[D]{}, nil
	}

	page := u.Pages[u.served]
	u.served++

	return page, nil
}
