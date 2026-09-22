package catalogclient_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/WildanFrananda/kinetix-search-service/internal/catalogclient"
	catalogv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/catalog/v1"
	commonv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/common/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

var noon = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

func TestAProductBecomesADocument(t *testing.T) {
	doc, err := catalogclient.ToDoc(&catalogv1.Product{
		Sku:                 "SHOE-1",
		MerchantPrincipalId: "shop-1",
		Title:               "Sepatu kulit",
		Description:         "Leather shoes",
		Price:               &commonv1.Money{AmountMinor: 75_000_000, Currency: "IDR"},
		CategorySlug:        "sepatu",
		CategoryName:        "Sepatu",
		UpdatedAt:           timestamppb.New(noon),
	})
	require.NoError(t, err)

	require.Equal(t, "SHOE-1", doc.ID.String())
	require.Equal(t, "shop-1", doc.MerchantID.String())
	require.Equal(t, int64(75_000_000), doc.PriceMinor)
	require.Equal(t, "IDR", doc.Currency)
	require.Equal(t, []string{"sepatu"}, doc.Categories)
	require.Equal(t, noon, doc.UpdatedAt)
}

func TestATimestampComesBackInUTC(t *testing.T) {
	jakarta := time.FixedZone("WIB", 7*60*60)
	doc, err := catalogclient.ToDoc(&catalogv1.Product{
		Sku:       "SHOE-1",
		Price:     &commonv1.Money{Currency: "IDR"},
		UpdatedAt: timestamppb.New(noon.In(jakarta)),
	})
	require.NoError(t, err)

	require.Equal(t, time.UTC, doc.UpdatedAt.Location(),
		"a document decoded on a WIB laptop must equal one decoded in a UTC container")
	require.True(t, doc.UpdatedAt.Equal(noon))
}

func TestAProductWithAnUnusableSkuIsCorruptNotSkipped(t *testing.T) {
	_, err := catalogclient.ToDoc(&catalogv1.Product{Sku: "   "})

	require.Error(t, err)
	require.Equal(t, search.KindCorruptRecord, search.KindOf(err))
}

func TestAMissingMerchantDoesNotSinkTheProduct(t *testing.T) {
	doc, err := catalogclient.ToDoc(&catalogv1.Product{
		Sku:   "SHOE-1",
		Price: &commonv1.Money{Currency: "IDR"},
	})

	require.NoError(t, err)
	require.True(t, doc.MerchantID.IsZero())
}

func TestAProductWithNoPriceIsStillIndexable(t *testing.T) {
	doc, err := catalogclient.ToDoc(&catalogv1.Product{Sku: "SHOE-1"})

	require.NoError(t, err)
	require.Equal(t, int64(0), doc.PriceMinor)
	require.Empty(t, doc.Currency)
}

func TestAnEmptyCursorStaysEmptyOnTheWire(t *testing.T) {
	out := catalogclient.ToProtoCursor(search.Cursor{})

	require.Nil(t, out.GetUpdatedThrough(),
		"an unset timestamp is how the contract says from the beginning; sending the epoch would "+
			"be a different instruction")
	require.Empty(t, out.GetLastSku())
}

func TestACursorSurvivesTheRoundTrip(t *testing.T) {
	original := search.Cursor{UpdatedThrough: noon, LastID: "SHOE-9"}

	back := catalogclient.FromProtoCursor(catalogclient.ToProtoCursor(original))

	require.Equal(t, original, back)
}

func TestAMissingCursorReadsAsTheBeginning(t *testing.T) {
	back := catalogclient.FromProtoCursor(nil)

	require.True(t, back.UpdatedThrough.IsZero())
	require.Empty(t, back.LastID)
}
