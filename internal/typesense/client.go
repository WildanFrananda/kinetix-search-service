package typesense

import (
	"time"

	"github.com/typesense/typesense-go/v3/typesense"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Settings struct {
	URL     string
	APIKey  string
	Timeout time.Duration
}

func Connect(s Settings) (*typesense.Client, error) {
	if s.Timeout <= 0 {
		return nil, search.Errf(
			search.KindIndexUnavailable,
			"typesense.Connect",
			nil,
			"a request timeout is required, got %s",
			s.Timeout,
		)
	}

	client := typesense.NewClient(
		typesense.WithServer(s.URL),
		typesense.WithAPIKey(s.APIKey),
		typesense.WithConnectionTimeout(s.Timeout),
	)

	return client, nil
}
