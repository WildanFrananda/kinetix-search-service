package search_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func TestKindSurvivesWrapping(t *testing.T) {
	base := search.Errf(search.KindIndexUnavailable, "typesense.Search", nil, "cluster unreachable")
	wrapped := fmt.Errorf("finding products: %w", base)
	twice := fmt.Errorf("handling request: %w", wrapped)

	require.Equal(t, search.KindIndexUnavailable, search.KindOf(twice))
}

func TestKindOfSomethingElseIsUnknown(t *testing.T) {
	require.Equal(t, search.KindUnknown, search.KindOf(errors.New("some library")))
	require.Equal(t, search.KindUnknown, search.KindOf(nil))
}

func TestCauseIsReachable(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	err := search.Errf(search.KindDependencyUnavailable, "catalog.ChangedSince", cause, "paging from %q", "p-1")

	require.ErrorIs(t, err, cause, "the driver's own message is what an operator needs")
	require.Contains(t, err.Error(), "catalog.ChangedSince")
	require.Contains(t, err.Error(), "dependency_unavailable")
	require.Contains(t, err.Error(), `paging from "p-1"`)
}

func TestEveryKindHasAName(t *testing.T) {
	named := map[search.Kind]string{
		search.KindMalformedQuery:        "malformed_query",
		search.KindNotFound:              "not_found",
		search.KindStale:                 "stale",
		search.KindIndexUnavailable:      "index_unavailable",
		search.KindDependencyUnavailable: "dependency_unavailable",
		search.KindCorruptRecord:         "corrupt_record",
		search.KindUnknown:               "unknown",
	}
	for kind, name := range named {
		require.Equal(t, name, kind.String())
	}

	require.Equal(t, "unknown", search.Kind(200).String())
}
