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

func refused(err error) bool {
	var httpErr *typesense.HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}

	return httpErr.Status == http.StatusBadRequest || httpErr.Status == http.StatusUnprocessableEntity
}

func fail(op string, err error, format string, args ...any) error {
	return search.Errf(search.KindIndexUnavailable, op, err, format, args...)
}

func failQuery(op string, err error, format string, args ...any) error {
	if refused(err) {
		return search.Errf(search.KindMalformedQuery, op, err, format, args...)
	}

	return fail(op, err, format, args...)
}
