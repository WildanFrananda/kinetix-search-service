package searchtest

import (
	"context"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type FakeSource[D any] struct {
	Pages     []search.Changes[D]
	Total     int64
	Err       error
	CountErr  error
	AskedFrom []search.Cursor

	served int
}

var _ search.Source[search.ProductDoc] = (*FakeSource[search.ProductDoc])(nil)

func (f *FakeSource[D]) ChangedSince(_ context.Context, cur search.Cursor, _ int) (search.Changes[D], error) {
	f.AskedFrom = append(f.AskedFrom, cur)
	if f.Err != nil {
		return search.Changes[D]{}, f.Err
	}
	if f.served >= len(f.Pages) {
		return search.Changes[D]{}, nil
	}
	page := f.Pages[f.served]
	f.served++
	return page, nil
}

func (f *FakeSource[D]) Count(context.Context) (int64, error) {
	if f.CountErr != nil {
		return 0, f.CountErr
	}
	return f.Total, nil
}
