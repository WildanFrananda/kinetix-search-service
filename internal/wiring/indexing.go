package wiring

import (
	"log/slog"

	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/indexing"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type indexingParams[D any] struct {
	dig.In
	Config     config.Config
	Log        *slog.Logger
	Source     search.Source[D]
	Index      search.Index[D]
	Checkpoint search.Checkpoint
	Lock       search.Lock
	Clock      search.Clock
}

type indexingResult[D any] struct {
	dig.Out
	Syncer    *indexing.Syncer[D]
	Reindexer *indexing.Reindexer[D]
}

func provideIndexing[D any](collection search.Collection) func(indexingParams[D]) indexingResult[D] {
	return func(p indexingParams[D]) indexingResult[D] {
		return indexingResult[D]{
			Syncer: indexing.NewSyncer(
				collection,
				p.Source,
				p.Index,
				p.Checkpoint,
				p.Lock,
				p.Log,
				p.Config.SyncPageSize,
			),
			Reindexer: indexing.NewReindexer(
				collection,
				p.Source,
				p.Index,
				p.Checkpoint,
				p.Lock,
				p.Clock,
				p.Log,
				p.Config.SyncPageSize,
			),
		}
	}
}
