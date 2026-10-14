package identityclient

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	identityv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/identity/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func ToProtoCursor(cur search.Cursor) *identityv1.MerchantCursor {
	out := &identityv1.MerchantCursor{LastPrincipalId: cur.LastID}

	if !cur.UpdatedThrough.IsZero() {
		out.UpdatedThrough = timestamppb.New(cur.UpdatedThrough)
	}

	return out
}

func FromProtoCursor(c *identityv1.MerchantCursor) search.Cursor {
	if c == nil {
		return search.Cursor{}
	}

	out := search.Cursor{LastID: c.GetLastPrincipalId()}

	if ts := c.GetUpdatedThrough(); ts != nil {
		out.UpdatedThrough = ts.AsTime().UTC()
	}

	return out
}

func ToChanges(response *identityv1.MerchantsChangedSinceResponse) search.Changes[search.MerchantDoc] {
	changes := search.Changes[search.MerchantDoc]{
		Upserted: make([]search.MerchantDoc, 0, len(response.GetUpserted())),
		Removed:  response.GetRemovedPrincipalIds(),
		Next:     FromProtoCursor(response.GetNext()),
		HasMore:  response.GetHasMore(),
	}

	for _, m := range response.GetUpserted() {
		doc, err := ToDoc(m)

		if err != nil {
			changes.Refused = append(
				changes.Refused,
				search.Refusal{ID: m.GetPrincipalId(), Reason: err},
			)
			continue
		}

		changes.Upserted = append(changes.Upserted, doc)
	}

	return changes
}

func ToDoc(m *identityv1.MerchantRecord) (search.MerchantDoc, error) {
	id, err := search.NewMerchantID(m.GetPrincipalId())

	if err != nil {
		return search.MerchantDoc{}, search.Errf(
			search.KindCorruptRecord,
			"identityclient.ToDoc",
			err,
			"identity sent a merchant with an unusable principal id",
		)
	}

	return search.MerchantDoc{
		ID:          id,
		DisplayName: m.GetStoreName(),
		Status:      statusOf(m.GetStatus()),
		MaySell:     m.GetMaySell(),
		UpdatedAt:   updatedAt(m),
	}, nil
}

func statusOf(status identityv1.MerchantStatus) string {
	switch status {
	case identityv1.MerchantStatus_MERCHANT_STATUS_PENDING:
		return "pending"
	case identityv1.MerchantStatus_MERCHANT_STATUS_VERIFIED:
		return "verified"
	case identityv1.MerchantStatus_MERCHANT_STATUS_SUSPENDED:
		return "suspended"
	case identityv1.MerchantStatus_MERCHANT_STATUS_CLOSED:
		return "closed"
	case identityv1.MerchantStatus_MERCHANT_STATUS_UNSPECIFIED:
		return "unknown"
	default:
		return "unknown"
	}
}

func updatedAt(m *identityv1.MerchantRecord) time.Time {
	if ts := m.GetUpdatedAt(); ts != nil {
		return ts.AsTime().UTC()
	}

	return time.Time{}
}
