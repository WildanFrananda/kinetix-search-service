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
