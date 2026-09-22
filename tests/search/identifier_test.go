package search_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func TestIdentifiersRefuseWhatIsNotAnIdentifier(t *testing.T) {
	makers := map[string]func(string) (interface{ String() string }, error){
		"ProductID": func(s string) (interface{ String() string }, error) {
			return search.NewProductID(s)
		},
		"MerchantID": func(s string) (interface{ String() string }, error) {
			return search.NewMerchantID(s)
		},
		"OrderID": func(s string) (interface{ String() string }, error) {
			return search.NewOrderID(s)
		},
	}

	refused := map[string]string{
		"empty":            "",
		"whitespace only":  "   ",
		"tab and newline":  "\t\n",
		"escape sequence":  "id\x1b[31m",
		"embedded newline": "two\nlines",
		"too long":         strings.Repeat("x", 65),
	}

	for name, make := range makers {
		t.Run(name, func(t *testing.T) {
			for why, raw := range refused {
				_, err := make(raw)
				require.Errorf(t, err, "should have refused %s", why)
				require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
			}

			at := strings.Repeat("x", 64)
			got, err := make(at)
			require.NoError(t, err, "64 characters is the limit, not one past it")
			require.Equal(t, at, got.String())

			trimmed, err := make("  p-1  ")
			require.NoError(t, err)
			require.Equal(t, "p-1", trimmed.String(), "surrounding space is trimmed, not refused")
		})
	}
}

func TestZeroIdentifierIsRecognisable(t *testing.T) {
	var zero search.ProductID
	require.True(t, zero.IsZero())
	require.Equal(t, "", zero.String())

	made, err := search.NewProductID("p-1")
	require.NoError(t, err)
	require.False(t, made.IsZero())
}

func TestIdentifiersOfDifferentKindsDoNotMix(t *testing.T) {
	p, err := search.NewProductID("same-text")
	require.NoError(t, err)
	m, err := search.NewMerchantID("same-text")
	require.NoError(t, err)

	require.Equal(t, p.String(), m.String())
}
