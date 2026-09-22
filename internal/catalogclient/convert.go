package catalogclient

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	catalogv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/catalog/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func ToProtoCursor(cur search.Cursor) *catalogv1.Cursor {
	out := &catalogv1.Cursor{LastSku: cur.LastID}
	if !cur.UpdatedThrough.IsZero() {
		out.UpdatedThrough = timestamppb.New(cur.UpdatedThrough)
	}
	return out
}

func FromProtoCursor(c *catalogv1.Cursor) search.Cursor {
	if c == nil {
		return search.Cursor{}
	}
	out := search.Cursor{LastID: c.GetLastSku()}
	if ts := c.GetUpdatedThrough(); ts != nil {
		out.UpdatedThrough = ts.AsTime().UTC()
	}
	return out
}

func ToDoc(p *catalogv1.Product) (search.ProductDoc, error) {
	id, err := search.NewProductID(p.GetSku())
	if err != nil {
		return search.ProductDoc{}, search.Errf(
			search.KindCorruptRecord,
			"catalogclient.ToDoc",
			err,
			"catalog sent a product with an unusable sku",
		)
	}

	merchant, err := search.NewMerchantID(p.GetMerchantPrincipalId())
	if err != nil {
		merchant = search.MerchantID{}
	}

	doc := search.ProductDoc{
		ID:          id,
		MerchantID:  merchant,
		Title:       p.GetTitle(),
		Description: p.GetDescription(),
		Currency:    p.GetPrice().GetCurrency(),
		PriceMinor:  p.GetPrice().GetAmountMinor(),
		UpdatedAt:   updatedAt(p),
	}

	if slug := p.GetCategorySlug(); slug != "" {
		doc.Categories = []string{slug}
	}

	return doc, nil
}

func updatedAt(p *catalogv1.Product) time.Time {
	if ts := p.GetUpdatedAt(); ts != nil {
		return ts.AsTime().UTC()
	}
	return time.Time{}
}
