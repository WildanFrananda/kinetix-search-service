package querying

import (
	"context"
	"time"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Finder[D any] struct {
	collection search.Collection
	searcher   search.Searcher[D]
	checkpoint search.Checkpoint
	clock      search.Clock
	staleAfter time.Duration
}

func NewFinder[D any](
	collection search.Collection,
	searcher search.Searcher[D],
	checkpoint search.Checkpoint,
	clock search.Clock,
	staleAfter time.Duration,
) *Finder[D] {
	return &Finder[D]{
		collection: collection,
		searcher:   searcher,
		checkpoint: checkpoint,
		clock:      clock,
		staleAfter: staleAfter,
	}
}

func (f *Finder[D]) Find(ctx context.Context, q search.Query) (search.Results[D], error) {
	if f.collection == search.Orders && q.OnBehalfOf.IsZero() {
		return search.Results[D]{}, search.Errf(
			search.KindMalformedQuery,
			"querying.Find",
			nil,
			"orders may only be searched on behalf of a principal",
		)
	}

	results, err := f.searcher.Search(ctx, q)
	if err != nil {
		return search.Results[D]{}, err
	}

	progress, err := f.checkpoint.Load(ctx, f.collection)
	if err != nil {
		return search.Results[D]{}, err
	}

	age := f.clock.Now().Sub(progress.SavedAt)
	results.Freshness = search.Freshness{
		IndexedThrough: progress.Cursor.UpdatedThrough,
		Age:            age,
		Stale:          age > f.staleAfter,
	}
	return results, nil
}

func (f *Finder[D]) Suggest(ctx context.Context, prefix string, limit int) ([]string, error) {
	if f.collection == search.Orders {
		return nil, search.Errf(
			search.KindMalformedQuery,
			"querying.Suggest",
			nil,
			"orders are private and are not suggested",
		)
	}

	if limit <= 0 || limit > search.MaxPageSize {
		limit = 10
	}

	return f.searcher.Suggest(ctx, prefix, limit)
}
