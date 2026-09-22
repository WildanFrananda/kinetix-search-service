package search

import (
	"context"
	"time"
)

type Searcher[D any] interface {
	Search(ctx context.Context, q Query) (Results[D], error)
	Suggest(ctx context.Context, prefix string, limit int) ([]string, error)
}

type Index[D any] interface {
	CreateGeneration(ctx context.Context, gen Generation) error
	Bulk(ctx context.Context, gen Generation, docs []D) error
	Remove(ctx context.Context, gen Generation, ids []string) error
	CountIn(ctx context.Context, gen Generation) (int64, error)
	Promote(ctx context.Context, gen Generation) error
	Live(ctx context.Context) (Generation, error)
}

type Source[D any] interface {
	ChangedSince(ctx context.Context, cur Cursor, limit int) (Changes[D], error)
}

type Counter interface {
	Count(ctx context.Context) (int64, error)
}

type Checkpoint interface {
	Load(ctx context.Context, c Collection) (Cursor, error)
	Save(ctx context.Context, c Collection, cur Cursor) error
}

type Clock interface {
	Now() time.Time
}
