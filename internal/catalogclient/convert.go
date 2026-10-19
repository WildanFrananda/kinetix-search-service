package catalogclient

import (
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	catalogv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/catalog/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func ToProtoCursor(cur search.Cursor) *catalogv1.Cursor {
	out := &catalogv1.Cursor{
		LastSku:       cur.LastID,
		TransactionId: proto.Uint64(cur.TransactionID),
	}
	if !cur.UpdatedThrough.IsZero() {
		out.UpdatedThrough = timestamppb.New(cur.UpdatedThrough)
	}
	return out
}

func FromProtoCursor(c *catalogv1.Cursor) search.Cursor {
	if c == nil {
		return search.Cursor{}
	}
	out := search.Cursor{TransactionID: c.GetTransactionId(), LastID: c.GetLastSku()}
	if ts := c.GetUpdatedThrough(); ts != nil {
		out.UpdatedThrough = ts.AsTime().UTC()
	}
	return out
}

func CarriesTransaction(c *catalogv1.Cursor) bool {
	return c != nil && c.TransactionId != nil
}

func ToChanges(response *catalogv1.ChangedSinceResponse) search.Changes[search.ProductDoc] {
	changes := search.Changes[search.ProductDoc]{
		Upserted: make([]search.ProductDoc, 0, len(response.GetUpserted())),
		Removed:  response.GetRemovedSkus(),
		Next:     FromProtoCursor(response.GetNext()),
		HasMore:  response.GetHasMore(),
	}

	for _, p := range response.GetUpserted() {
		doc, err := ToDoc(p)
		if err != nil {
			changes.Refused = append(
				changes.Refused,
				search.Refusal{ID: p.GetSku(), Reason: err},
			)
			continue
		}
		changes.Upserted = append(changes.Upserted, doc)
	}

	return changes
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
