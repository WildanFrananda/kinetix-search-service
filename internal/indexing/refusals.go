package indexing

import (
	"context"
	"log/slog"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

func logRefusals(
	ctx context.Context,
	log *slog.Logger,
	collection search.Collection,
	refused []search.Refusal,
) {
	for _, r := range refused {
		log.ErrorContext(
			ctx,
			"refused a record the source sent",
			"collection",
			string(collection),
			"id",
			r.ID,
			"err",
			r.Reason,
			"kind",
			search.KindOf(r.Reason).String(),
		)
	}
}
