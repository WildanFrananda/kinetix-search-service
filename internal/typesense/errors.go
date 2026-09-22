package typesense

import (
	"errors"
	"net/http"

	"github.com/typesense/typesense-go/v3/typesense"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func notFound(err error) bool {
	var httpErr *typesense.HTTPError
	return errors.As(err, &httpErr) && httpErr.Status == http.StatusNotFound
}

func fail(op string, err error, format string, args ...any) error {
	return search.Errf(search.KindIndexUnavailable, op, err, format, args...)
}
