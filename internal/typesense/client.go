package typesense

import (
	"time"

	"github.com/typesense/typesense-go/v3/typesense"
)

type Settings struct {
	URL     string
	APIKey  string
	Timeout time.Duration
}

func Connect(s Settings) *typesense.Client {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return typesense.NewClient(
		typesense.WithServer(s.URL),
		typesense.WithAPIKey(s.APIKey),
		typesense.WithConnectionTimeout(timeout),
	)
}
