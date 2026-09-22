package indexing

import (
	"context"
	"log/slog"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Syncer[D any] struct {
	collection search.Collection
	source     search.Source[D]
	index      search.Index[D]
	checkpoint search.Checkpoint
	log        *slog.Logger
	pageSize   int
}

func NewSyncer[D any](
	collection search.Collection,
	source search.Source[D],
	index search.Index[D],
	checkpoint search.Checkpoint,
	log *slog.Logger,
	pageSize int,
) *Syncer[D] {
	return &Syncer[D]{
		collection: collection,
		source:     source,
		index:      index,
		checkpoint: checkpoint,
		log:        log,
		pageSize:   pageSize,
	}
}

func (s *Syncer[D]) Once(ctx context.Context) error {
	cur, err := s.checkpoint.Load(ctx, s.collection)
	if err != nil {
		return err
	}

	gen, err := s.index.Live(ctx)
	if err != nil {
		return err
	}

	for {
		changes, err := s.source.ChangedSince(ctx, cur, s.pageSize)
		if err != nil {
			return err
		}

		if len(changes.Upserted) > 0 {
			if err := s.index.Bulk(ctx, gen, changes.Upserted); err != nil {
				return err
			}
		}
		if len(changes.Removed) > 0 {
			if err := s.index.Remove(ctx, gen, changes.Removed); err != nil {
				return err
			}
		}

		if err := s.checkpoint.Save(ctx, s.collection, changes.Next); err != nil {
			return err
		}
		cur = changes.Next

		s.log.InfoContext(
			ctx,
			"sync page applied",
			"collection",
			string(s.collection),
			"upserted",
			len(changes.Upserted),
			"removed",
			len(changes.Removed),
			"through",
			changes.Next.UpdatedThrough,
		)

		if !changes.HasMore {
			return nil
		}
	}
}
