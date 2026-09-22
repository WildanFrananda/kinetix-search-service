package typesense

import (
	"time"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func millis(t time.Time) int64 {
	return t.UnixMilli()
}

func timeFromMillis(v any) time.Time {
	switch n := v.(type) {
	case float64:
		return time.UnixMilli(int64(n)).UTC()
	case int64:
		return time.UnixMilli(n).UTC()
	default:
		return time.Time{}
	}
}

func text(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func number(m map[string]any, key string) int64 {
	switch n := m[key].(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}

func stringList(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func EncodeProduct(d search.ProductDoc) map[string]any {
	return map[string]any{
		"id":          d.ID.String(),
		"merchant_id": d.MerchantID.String(),
		"title":       d.Title,
		"description": d.Description,
		"categories":  d.Categories,
		"price_minor": d.PriceMinor,
		"currency":    d.Currency,
		"updated_at":  millis(d.UpdatedAt),
	}
}

func DecodeProduct(m map[string]any) (search.ProductDoc, error) {
	id, err := search.NewProductID(text(m, "id"))
	if err != nil {
		return search.ProductDoc{}, search.Errf(
			search.KindCorruptRecord,
			"typesense.DecodeProduct",
			err,
			"indexed product has an unusable id",
		)
	}

	merchant, _ := search.NewMerchantID(text(m, "merchant_id"))

	return search.ProductDoc{
		ID:          id,
		MerchantID:  merchant,
		Title:       text(m, "title"),
		Description: text(m, "description"),
		Categories:  stringList(m, "categories"),
		PriceMinor:  number(m, "price_minor"),
		Currency:    text(m, "currency"),
		UpdatedAt:   timeFromMillis(m["updated_at"]),
	}, nil
}

func EncodeMerchant(d search.MerchantDoc) map[string]any {
	return map[string]any{
		"id":           d.ID.String(),
		"display_name": d.DisplayName,
		"categories":   d.Categories,
		"updated_at":   millis(d.UpdatedAt),
	}
}

func DecodeMerchant(m map[string]any) (search.MerchantDoc, error) {
	id, err := search.NewMerchantID(text(m, "id"))
	if err != nil {
		return search.MerchantDoc{}, search.Errf(
			search.KindCorruptRecord,
			"typesense.DecodeMerchant",
			err,
			"indexed merchant has an unusable id",
		)
	}
	return search.MerchantDoc{
		ID:          id,
		DisplayName: text(m, "display_name"),
		Categories:  stringList(m, "categories"),
		UpdatedAt:   timeFromMillis(m["updated_at"]),
	}, nil
}

func EncodeOrder(d search.OrderDoc) map[string]any {
	return map[string]any{
		"id":          d.ID.String(),
		"buyer":       d.Buyer.String(),
		"merchant_id": d.MerchantID.String(),
		"number":      d.Number,
		"status":      d.Status,
		"line_titles": d.LineTitles,
		"placed_at":   millis(d.PlacedAt),
		"updated_at":  millis(d.UpdatedAt),
	}
}

func DecodeOrder(m map[string]any) (search.OrderDoc, error) {
	id, err := search.NewOrderID(text(m, "id"))
	if err != nil {
		return search.OrderDoc{}, search.Errf(
			search.KindCorruptRecord,
			"typesense.DecodeOrder",
			err,
			"indexed order has an unusable id",
		)
	}
	buyer, _ := search.NewMerchantID(text(m, "buyer"))
	merchant, _ := search.NewMerchantID(text(m, "merchant_id"))

	return search.OrderDoc{
		ID:         id,
		Buyer:      buyer,
		MerchantID: merchant,
		Number:     text(m, "number"),
		Status:     text(m, "status"),
		LineTitles: stringList(m, "line_titles"),
		PlacedAt:   timeFromMillis(m["placed_at"]),
		UpdatedAt:  timeFromMillis(m["updated_at"]),
	}, nil
}
