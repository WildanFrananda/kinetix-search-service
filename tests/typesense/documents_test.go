package typesense_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

var noon = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

func throughJSON(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	var back map[string]any
	require.NoError(t, json.Unmarshal(raw, &back))
	return back
}

func TestProductSurvivesARoundTrip(t *testing.T) {
	id, err := search.NewProductID("p-1")
	require.NoError(t, err)
	merchant, err := search.NewMerchantID("shop-1")
	require.NoError(t, err)

	original := search.ProductDoc{
		ID:          id,
		MerchantID:  merchant,
		Title:       "Sepatu kulit",
		Description: "Leather shoes, handmade",
		Categories:  []string{"sepatu", "kulit"},
		PriceMinor:  24_999_00,
		Currency:    "IDR",
		UpdatedAt:   noon,
	}

	back, err := typesense.DecodeProduct(throughJSON(t, typesense.EncodeProduct(original)))
	require.NoError(t, err)
	require.Equal(t, original, back)
}

func TestMerchantSurvivesARoundTrip(t *testing.T) {
	id, err := search.NewMerchantID("shop-1")
	require.NoError(t, err)

	original := search.MerchantDoc{
		ID:          id,
		DisplayName: "Toko Sepatu Merdeka",
		Categories:  []string{"sepatu"},
		UpdatedAt:   noon,
	}

	back, err := typesense.DecodeMerchant(throughJSON(t, typesense.EncodeMerchant(original)))
	require.NoError(t, err)
	require.Equal(t, original, back)
}

func TestOrderSurvivesARoundTrip(t *testing.T) {
	id, err := search.NewOrderID("KNX-20260921-7QF3")
	require.NoError(t, err)
	buyer, err := search.NewBuyerID("principal-9")
	require.NoError(t, err)
	merchant, err := search.NewMerchantID("shop-1")
	require.NoError(t, err)

	original := search.OrderDoc{
		ID:         id,
		Buyer:      buyer,
		MerchantID: merchant,
		Number:     "KNX-20260921-7QF3",
		Status:     "packed",
		LineTitles: []string{"Sepatu kulit"},
		PlacedAt:   noon.Add(-time.Hour),
		UpdatedAt:  noon,
	}

	back, err := typesense.DecodeOrder(throughJSON(t, typesense.EncodeOrder(original)))
	require.NoError(t, err)
	require.Equal(t, original, back)
}

func TestAPriceIsNotLostToFloatingPoint(t *testing.T) {
	id, err := search.NewProductID("p-1")
	require.NoError(t, err)

	for _, price := range []int64{0, 1, 99, 1_000_000_00, 9_007_199_254_740_99} {
		original := search.ProductDoc{ID: id, PriceMinor: price, UpdatedAt: noon}
		back, err := typesense.DecodeProduct(throughJSON(t, typesense.EncodeProduct(original)))
		require.NoError(t, err)
		require.Equalf(t, price, back.PriceMinor, "price %d did not survive", price)
	}
}

func TestADocumentWithAnUnusableIdIsCorruptNotEmpty(t *testing.T) {
	_, err := typesense.DecodeProduct(map[string]any{"id": "   ", "title": "x"})
	require.Error(t, err)
	require.Equal(t, search.KindCorruptRecord, search.KindOf(err))

	_, err = typesense.DecodeOrder(map[string]any{"id": ""})
	require.Error(t, err)
	require.Equal(t, search.KindCorruptRecord, search.KindOf(err))
}

func TestAMissingMerchantDoesNotFailTheWholePage(t *testing.T) {
	doc, err := typesense.DecodeProduct(map[string]any{"id": "p-1", "title": "Sepatu"})
	require.NoError(t, err)
	require.True(t, doc.MerchantID.IsZero())
}
