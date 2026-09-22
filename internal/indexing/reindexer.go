package indexing

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Reindexer[D any] struct {
	collection search.Collection
	source     search.Source[D]
	index      search.Index[D]
	checkpoint search.Checkpoint
	clock      search.Clock
	log        *slog.Logger
	pageSize   int
}

func NewReindexer[D any](
	collection search.Collection,
	source search.Source[D],
	index search.Index[D],
	checkpoint search.Checkpoint,
	clock search.Clock,
	log *slog.Logger,
	pageSize int,
) *Reindexer[D] {
	return &Reindexer[D]{
		collection: collection,
		source:     source,
		index:      index,
		checkpoint: checkpoint,
		clock:      clock,
		log:        log,
		pageSize:   pageSize,
	}
}

func (r *Reindexer[D]) Run(ctx context.Context) error {
	gen := search.Generation(fmt.Sprintf("%s_v%d", r.collection, r.clock.Now().Unix()))

	if err := r.index.CreateGeneration(ctx, gen); err != nil {
		return err
	}

	counted, countable := r.source.(search.Counter)
	var expected int64

	if countable {
		total, err := counted.Count(ctx)

		if err != nil {
			return err
		}

		expected = total
	}

	var sent int64
	var cur search.Cursor

	for {
		changes, err := r.source.ChangedSince(ctx, cur, r.pageSize)
		if err != nil {
			return err
		}

		if len(changes.Upserted) > 0 {
			if err := r.index.Bulk(ctx, gen, changes.Upserted); err != nil {
				return err
			}

			sent += int64(len(changes.Upserted))
		}

		cur = changes.Next
		if !changes.HasMore {
			break
		}
	}

	if !countable {
		expected = sent
	}

	indexed, err := r.index.CountIn(ctx, gen)
	if err != nil {
		return err
	}
	if indexed < expected {
		return search.Errf(
			search.KindIndexUnavailable,
			"indexing.Reindex",
			nil,
			"generation %s holds %d documents, %s reported %d",
			gen,
			indexed,
			reporter(countable),
			expected,
		)
	}

	if err := r.index.Promote(ctx, gen); err != nil {
		return err
	}

	if err := r.checkpoint.Save(ctx, r.collection, cur); err != nil {
		return err
	}

	r.log.InfoContext(
		ctx,
		"reindex promoted",
		"collection",
		string(r.collection),
		"generation",
		string(gen),
		"documents",
		indexed,
		"counted_against",
		reporter(countable),
	)

	return nil
}

func reporter(countable bool) string {
	if countable {
		return "the source"
	}

	return "the walk"
}
