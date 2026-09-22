package orderclient

import (
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/common/v1"
	orderv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/order/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func ToProtoCursor(cur search.Cursor) *orderv1.OrderCursor {
	out := &orderv1.OrderCursor{LastOrderNumber: cur.LastID}

	if !cur.UpdatedThrough.IsZero() {
		out.UpdatedThrough = timestamppb.New(cur.UpdatedThrough)
	}

	return out
}

func FromProtoCursor(c *orderv1.OrderCursor) search.Cursor {
	if c == nil {
		return search.Cursor{}
	}

	out := search.Cursor{LastID: c.GetLastOrderNumber()}

	if ts := c.GetUpdatedThrough(); ts != nil {
		out.UpdatedThrough = ts.AsTime().UTC()
	}

	return out
}

func ToDoc(o *orderv1.OrderRecord) (search.OrderDoc, error) {
	id, err := search.NewOrderID(o.GetOrderNumber())

	if err != nil {
		return search.OrderDoc{}, search.Errf(
			search.KindCorruptRecord,
			"orderclient.ToDoc",
			err,
			"order sent a record with an unusable order number",
		)
	}

	buyer, err := search.NewBuyerID(o.GetBuyerPrincipalId())

	if err != nil {
		return search.OrderDoc{}, search.Errf(
			search.KindCorruptRecord,
			"orderclient.ToDoc",
			err,
			"order %s names no buyer, and an order with no buyer cannot be filtered to one",
			o.GetOrderNumber(),
		)
	}

	merchant, err := search.NewMerchantID(o.GetMerchantPrincipalId())

	if err != nil {
		merchant = search.MerchantID{}
	}

	return search.OrderDoc{
		ID:         id,
		Buyer:      buyer,
		MerchantID: merchant,
		Number:     o.GetOrderNumber(),
		Status:     statusOf(o.GetStatus()),
		LineTitles: o.GetLineTitles(),
		PlacedAt:   at(o.GetPlacedAt()),
		UpdatedAt:  at(o.GetUpdatedAt()),
	}, nil
}

func statusOf(status commonv1.OrderStatus) string {
	name := status.String()
	trimmed := strings.TrimPrefix(name, "ORDER_STATUS_")

	if trimmed == "" || trimmed == "UNSPECIFIED" {
		return "unknown"
	}

	return strings.ToLower(trimmed)
}

func at(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}

	return ts.AsTime().UTC()
}
