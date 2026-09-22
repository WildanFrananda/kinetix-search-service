package identityclient_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/identity/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/identityclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

var noon = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

func TestAMerchantBecomesADocument(t *testing.T) {
	doc, err := identityclient.ToDoc(&identityv1.MerchantRecord{
		PrincipalId: "3aa957c8-b802-4d58-b9fc-f7b76ce60fa3",
		StoreName:   "Toko Sepatu",
		Status:      identityv1.MerchantStatus_MERCHANT_STATUS_VERIFIED,
		MaySell:     true,
		UpdatedAt:   timestamppb.New(noon),
	})

	require.NoError(t, err)
	require.Equal(t, "3aa957c8-b802-4d58-b9fc-f7b76ce60fa3", doc.ID.String())
	require.Equal(t, "Toko Sepatu", doc.DisplayName)
	require.Equal(t, "verified", doc.Status)
	require.True(t, doc.MaySell)
	require.Equal(t, noon, doc.UpdatedAt)
}

func TestAMerchantThatMayNotSellIsStillIndexed(t *testing.T) {
	doc, err := identityclient.ToDoc(&identityv1.MerchantRecord{
		PrincipalId: "m-2",
		StoreName:   "Toko Tutup",
		Status:      identityv1.MerchantStatus_MERCHANT_STATUS_SUSPENDED,
		MaySell:     false,
		UpdatedAt:   timestamppb.New(noon),
	})

	require.NoError(t, err, "a suspended shop is a fact to record, not a record to drop")
	require.Equal(t, "suspended", doc.Status)
	require.False(t, doc.MaySell)
}

func TestAStandingNobodyNamedReadsAsUnknown(t *testing.T) {
	doc, err := identityclient.ToDoc(&identityv1.MerchantRecord{
		PrincipalId: "m-3",
		StoreName:   "Toko Baru",
		Status:      identityv1.MerchantStatus_MERCHANT_STATUS_UNSPECIFIED,
	})

	require.NoError(t, err)
	require.Equal(t, "unknown", doc.Status, "an unset standing must not read as a real one")
}

func TestAMerchantWithNoPrincipalIsCorruptNotSkipped(t *testing.T) {
	_, err := identityclient.ToDoc(&identityv1.MerchantRecord{StoreName: "Toko Hantu"})

	require.Error(t, err)
	require.Equal(t, search.KindCorruptRecord, search.KindOf(err))
}

func TestATimestampComesBackInUTC(t *testing.T) {
	jakarta := time.FixedZone("WIB", 7*60*60)
	doc, err := identityclient.ToDoc(&identityv1.MerchantRecord{
		PrincipalId: "m-1",
		UpdatedAt:   timestamppb.New(noon.In(jakarta)),
	})

	require.NoError(t, err)
	require.Equal(t, time.UTC, doc.UpdatedAt.Location())
	require.Equal(t, noon, doc.UpdatedAt)
}

func TestAMerchantWithNoTimestampReadsAsUnset(t *testing.T) {
	doc, err := identityclient.ToDoc(&identityv1.MerchantRecord{PrincipalId: "m-1"})

	require.NoError(t, err)
	require.True(t, doc.UpdatedAt.IsZero(), "an unset time is not the epoch")
}

func TestAnEmptyCursorStaysEmptyOnTheWire(t *testing.T) {
	out := identityclient.ToProtoCursor(search.Cursor{})

	require.Empty(t, out.GetLastPrincipalId())
	require.Nil(t, out.GetUpdatedThrough(), "the beginning is unset, not the epoch")
}

func TestACursorSurvivesTheRoundTrip(t *testing.T) {
	original := search.Cursor{UpdatedThrough: noon, LastID: "m-9"}

	back := identityclient.FromProtoCursor(identityclient.ToProtoCursor(original))

	require.Equal(t, original, back)
}

func TestAMissingCursorReadsAsTheBeginning(t *testing.T) {
	back := identityclient.FromProtoCursor(nil)

	require.True(t, back.UpdatedThrough.IsZero())
	require.Empty(t, back.LastID)
}
