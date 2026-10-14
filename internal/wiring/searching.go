package wiring

import (
	tsapi "github.com/typesense/typesense-go/v3/typesense"
	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/querying"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

type finderParams struct {
	dig.In
	Config     config.Config
	Engine     *tsapi.Client
	Checkpoint search.Checkpoint
	Clock      search.Clock
}

func provideProductFinder(p finderParams) *querying.Finder[search.ProductDoc] {
	return querying.NewFinder(
		search.Products,
		typesense.NewSearcher(p.Engine, search.Products, typesense.DecodeProduct),
		p.Checkpoint,
		p.Clock,
		p.Config.StaleAfter,
	)
}
