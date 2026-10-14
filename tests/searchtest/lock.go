package searchtest

import (
	"context"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type FakeLock struct {
	Err      error
	Holding  bool
	Held     []search.Collection
	Released int
}

var _ search.Lock = (*FakeLock)(nil)

func (f *FakeLock) Hold(_ context.Context, c search.Collection) (func(), error) {
	if f.Err != nil {
		return nil, f.Err
	}

	f.Held = append(f.Held, c)
	f.Holding = true

	return func() {
		f.Holding = false
		f.Released++
	}, nil
}

func (f *FakeLock) holding() bool {
	return f != nil && f.Holding
}
