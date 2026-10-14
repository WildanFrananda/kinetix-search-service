package wiring

import (
	tsapi "github.com/typesense/typesense-go/v3/typesense"
	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

type engineParams struct {
	dig.In
	Config config.Config
}

func provideEngine(p engineParams) *tsapi.Client {
	return typesense.Connect(typesense.Settings{
		URL:     p.Config.TypesenseURL,
		APIKey:  p.Config.TypesenseAPIKey,
		Timeout: p.Config.TypesenseTimeout,
	})
}

type indexesParams struct {
	dig.In
	Engine *tsapi.Client
}

type indexesResult struct {
	dig.Out
	Products  search.Index[search.ProductDoc]
	Merchants search.Index[search.MerchantDoc]
	Orders    search.Index[search.OrderDoc]
}

func provideIndexes(p indexesParams) indexesResult {
	return indexesResult{
		Products:  typesense.NewIndex(p.Engine, search.Products, typesense.EncodeProduct),
		Merchants: typesense.NewIndex(p.Engine, search.Merchants, typesense.EncodeMerchant),
		Orders:    typesense.NewIndex(p.Engine, search.Orders, typesense.EncodeOrder),
	}
}
