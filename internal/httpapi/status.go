package httpapi

import (
	"net/http"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func statusFor(kind search.Kind) int {
	switch kind {
	case search.KindMalformedQuery:
		return http.StatusBadRequest
	case search.KindNotFound:
		return http.StatusNotFound
	case search.KindStale:
		return http.StatusOK
	case search.KindIndexUnavailable, search.KindDependencyUnavailable:
		return http.StatusServiceUnavailable
	case search.KindCorruptRecord, search.KindUnknown:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
