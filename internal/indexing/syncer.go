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
	lock       search.Lock
	log        *slog.Logger
	pageSize   int
}

func NewSyncer[D any](
	collection search.Collection,
	source search.Source[D],
	index search.Index[D],
	checkpoint search.Checkpoint,
	lock search.Lock,
	log *slog.Logger,
	pageSize int,
) *Syncer[D] {
	return &Syncer[D]{
		collection: collection,
		source:     source,
		index:      index,
		checkpoint: checkpoint,
		lock:       lock,
		log:        log,
		pageSize:   pageSize,
	}
}

func (s *Syncer[D]) Once(ctx context.Context) error {
	release, err := s.lock.Hold(ctx, s.collection)
	if err != nil {
		return err
	}
	defer release()

	progress, err := s.checkpoint.Load(ctx, s.collection)
	if err != nil {
		return err
	}

	cur := progress.Cursor

	gen, err := s.index.Live(ctx)
	if err != nil {
		return err
	}

	for {
		changes, err := s.source.ChangedSince(ctx, cur, s.pageSize)
		if err != nil {
			return err
		}

		logRefusals(ctx, s.log, s.collection, changes.Refused)

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
			"refused",
			len(changes.Refused),
			"through",
			changes.Next.UpdatedThrough,
		)

		if !changes.HasMore {
			return nil
		}
	}
}
