package wiring

import (
	"fmt"
	"log/slog"

	"go.uber.org/dig"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func Searchd(log *slog.Logger) (*dig.Container, error) {
	return build(
		log,
		provideConfig,
		provideClock,
		providePool,
		provideStore,
		provideEngine,
		provideIndexes,
		provideProductFinder,
		provideHTTP,
		provideGRPC,
	)
}

func Indexing(log *slog.Logger) (*dig.Container, error) {
	return build(
		log,
		provideConfig,
		provideClock,
		providePool,
		provideStore,
		provideEngine,
		provideIndexes,
		provideCatalog,
		provideIdentity,
		provideOrders,
		provideIndexing[search.ProductDoc](search.Products),
		provideIndexing[search.MerchantDoc](search.Merchants),
		provideIndexing[search.OrderDoc](search.Orders),
	)
}

func build(log *slog.Logger, providers ...any) (*dig.Container, error) {
	container := dig.New()

	logger := func() *slog.Logger {
		return log
	}

	for _, provider := range append([]any{logger}, providers...) {
		if err := container.Provide(provider); err != nil {
			return nil, fmt.Errorf("provide %T: %w", provider, err)
		}
	}

	return container, nil
}
