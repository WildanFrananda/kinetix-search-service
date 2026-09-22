package searchtest

import (
	"context"
	"time"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type FakeSearcher[D any] struct {
	Results search.Results[D]
	Err     error
	Asked   []search.Query
}

func (f *FakeSearcher[D]) Search(_ context.Context, q search.Query) (search.Results[D], error) {
	f.Asked = append(f.Asked, q)
	if f.Err != nil {
		return search.Results[D]{}, f.Err
	}
	return f.Results, nil
}

func (f *FakeSearcher[D]) Suggest(context.Context, string, int) ([]string, error) {
	return nil, nil
}

var _ search.Searcher[search.ProductDoc] = (*FakeSearcher[search.ProductDoc])(nil)

type FixedClock struct {
	At time.Time
}

func (c FixedClock) Now() time.Time {
	return c.At
}

type FakeCheckpoint struct {
	Cursors map[search.Collection]search.Cursor
	Err     error
	Saved   []search.Cursor
}

var _ search.Checkpoint = (*FakeCheckpoint)(nil)

func (f *FakeCheckpoint) Load(_ context.Context, c search.Collection) (search.Cursor, error) {
	return f.Cursors[c], f.Err
}

func (f *FakeCheckpoint) Save(_ context.Context, c search.Collection, cur search.Cursor) error {
	if f.Cursors == nil {
		f.Cursors = map[search.Collection]search.Cursor{}
	}
	f.Cursors[c] = cur
	f.Saved = append(f.Saved, cur)
	return nil
}
