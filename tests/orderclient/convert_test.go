package orderclient_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/common/v1"
	orderv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/order/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/orderclient"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

var noon = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

func anOrder() *orderv1.OrderRecord {
	return &orderv1.OrderRecord{
		OrderNumber:         "ORD-20260922-A1B2C3D4",
		BuyerPrincipalId:    "9f1d4a3e-1c62-4d0a-9a7b-2f5c8e0b41d7",
		MerchantPrincipalId: "3aa957c8-b802-4d58-b9fc-f7b76ce60fa3",
		Status:              commonv1.OrderStatus_ORDER_STATUS_IN_TRANSIT,
		LineTitles:          []string{"Sepatu Kulit", "Tas"},
		PlacedAt:            timestamppb.New(noon.Add(-time.Hour)),
		UpdatedAt:           timestamppb.New(noon),
	}
}

func TestAnOrderBecomesADocument(t *testing.T) {
	doc, err := orderclient.ToDoc(anOrder())

	require.NoError(t, err)
	require.Equal(t, "ORD-20260922-A1B2C3D4", doc.ID.String())
	require.Equal(t, "ORD-20260922-A1B2C3D4", doc.Number)
	require.Equal(t, "9f1d4a3e-1c62-4d0a-9a7b-2f5c8e0b41d7", doc.Buyer.String())
	require.Equal(t, "3aa957c8-b802-4d58-b9fc-f7b76ce60fa3", doc.MerchantID.String())
	require.Equal(t, "in_transit", doc.Status)
	require.Equal(t, []string{"Sepatu Kulit", "Tas"}, doc.LineTitles)
	require.Equal(t, noon, doc.UpdatedAt)
	require.Equal(t, noon.Add(-time.Hour), doc.PlacedAt)
}

func TestAnOrderWithNoBuyerIsRefused(t *testing.T) {
	record := anOrder()
	record.BuyerPrincipalId = ""

	_, err := orderclient.ToDoc(record)

	require.Error(t, err, "an order with no buyer cannot be filtered to one, so it must not be indexed")
	require.Equal(t, search.KindCorruptRecord, search.KindOf(err))
}

func TestAnOrderPlacedBeforeMerchantsWereRecordedIsStillIndexed(t *testing.T) {
	record := anOrder()
	record.MerchantPrincipalId = ""

	doc, err := orderclient.ToDoc(record)

	require.NoError(t, err, "the buyer can still find their own order")
	require.True(t, doc.MerchantID.IsZero(), "an unknown seller stays unknown, not an empty id")
}

func TestAnOrderWithNoNumberIsCorruptNotSkipped(t *testing.T) {
	record := anOrder()
	record.OrderNumber = ""

	_, err := orderclient.ToDoc(record)

	require.Error(t, err)
	require.Equal(t, search.KindCorruptRecord, search.KindOf(err))
}

func TestAStatusNobodySetReadsAsUnknown(t *testing.T) {
	record := anOrder()
	record.Status = commonv1.OrderStatus_ORDER_STATUS_UNSPECIFIED

	doc, err := orderclient.ToDoc(record)

	require.NoError(t, err)
	require.Equal(t, "unknown", doc.Status)
}

func TestATimestampComesBackInUTC(t *testing.T) {
	jakarta := time.FixedZone("WIB", 7*60*60)
	record := anOrder()
	record.UpdatedAt = timestamppb.New(noon.In(jakarta))

	doc, err := orderclient.ToDoc(record)

	require.NoError(t, err)
	require.Equal(t, time.UTC, doc.UpdatedAt.Location())
	require.Equal(t, noon, doc.UpdatedAt)
}

func TestAnEmptyCursorStaysEmptyOnTheWire(t *testing.T) {
	out := orderclient.ToProtoCursor(search.Cursor{})

	require.Empty(t, out.GetLastOrderNumber())
	require.Nil(t, out.GetUpdatedThrough(), "the beginning is unset, not the epoch")
}

func TestACursorSurvivesTheRoundTrip(t *testing.T) {
	original := search.Cursor{UpdatedThrough: noon, LastID: "ORD-20260922-A1B2C3D4"}

	back := orderclient.FromProtoCursor(orderclient.ToProtoCursor(original))

	require.Equal(t, original, back)
}

func TestAMissingCursorReadsAsTheBeginning(t *testing.T) {
	back := orderclient.FromProtoCursor(nil)

	require.True(t, back.UpdatedThrough.IsZero())
	require.Empty(t, back.LastID)
}
