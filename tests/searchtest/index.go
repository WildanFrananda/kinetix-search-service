package searchtest

import (
	"context"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type FakeIndex[D any] struct {
	LiveGeneration search.Generation
	Created        []search.Generation
	Promoted       []search.Generation
	Written        map[search.Generation][]D
	RemovedIDs     map[search.Generation][]string
	Counted        *int64
	BulkErr        error
	LiveErr        error
	CreateErr      error
	Lock           *FakeLock
	WrittenHeld    []bool
	PromotedHeld   []bool
}

var _ search.Index[search.ProductDoc] = (*FakeIndex[search.ProductDoc])(nil)

func (f *FakeIndex[D]) CreateGeneration(_ context.Context, gen search.Generation) error {
	if f.CreateErr != nil {
		return f.CreateErr
	}

	f.Created = append(f.Created, gen)

	return nil
}

func (f *FakeIndex[D]) Bulk(_ context.Context, gen search.Generation, docs []D) error {
	if f.BulkErr != nil {
		return f.BulkErr
	}

	if f.Written == nil {
		f.Written = map[search.Generation][]D{}
	}

	f.Written[gen] = append(f.Written[gen], docs...)
	f.WrittenHeld = append(f.WrittenHeld, f.Lock.holding())

	return nil
}

func (f *FakeIndex[D]) Remove(_ context.Context, gen search.Generation, ids []string) error {
	if f.RemovedIDs == nil {
		f.RemovedIDs = map[search.Generation][]string{}
	}

	f.RemovedIDs[gen] = append(f.RemovedIDs[gen], ids...)

	return nil
}

func (f *FakeIndex[D]) CountIn(_ context.Context, gen search.Generation) (int64, error) {
	if f.Counted != nil {
		return *f.Counted, nil
	}

	return int64(len(f.Written[gen])), nil
}

func (f *FakeIndex[D]) Promote(_ context.Context, gen search.Generation) error {
	f.Promoted = append(f.Promoted, gen)
	f.PromotedHeld = append(f.PromotedHeld, f.Lock.holding())
	f.LiveGeneration = gen

	return nil
}

func (f *FakeIndex[D]) Live(context.Context) (search.Generation, error) {
	if f.LiveErr != nil {
		return "", f.LiveErr
	}

	return f.LiveGeneration, nil
}
