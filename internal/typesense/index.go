package typesense

import (
	"context"
	"fmt"

	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Index[D any] struct {
	client     *typesense.Client
	collection search.Collection
	encode     func(D) map[string]any
}

var _ search.Index[search.ProductDoc] = (*Index[search.ProductDoc])(nil)

func NewIndex[D any](
	client *typesense.Client,
	collection search.Collection,
	encode func(D) map[string]any,
) *Index[D] {
	return &Index[D]{client: client, collection: collection, encode: encode}
}

func (i *Index[D]) CreateGeneration(ctx context.Context, gen search.Generation) error {
	schema := SchemaFor(i.collection, gen)
	if schema == nil {
		return search.Errf(
			search.KindIndexUnavailable,
			"typesense.CreateGeneration",
			nil,
			"no schema is defined for collection %q",
			i.collection,
		)
	}
	if _, err := i.client.Collections().Create(ctx, schema); err != nil {
		return fail("typesense.CreateGeneration", err, "creating %s", gen)
	}
	return nil
}

func (i *Index[D]) Bulk(ctx context.Context, gen search.Generation, docs []D) error {
	if len(docs) == 0 {
		return nil
	}

	payload := make([]any, 0, len(docs))
	for _, d := range docs {
		payload = append(payload, i.encode(d))
	}

	action := api.IndexAction("upsert")
	results, err := i.client.Collection(string(gen)).Documents().Import(
		ctx,
		payload,
		&api.ImportDocumentsParams{Action: &action},
	)
	if err != nil {
		return fail("typesense.Bulk", err, "importing %d documents into %s", len(docs), gen)
	}

	for n, r := range results {
		if r == nil || r.Success {
			continue
		}
		return search.Errf(
			search.KindIndexUnavailable,
			"typesense.Bulk",
			nil,
			"document %d of %d was refused by %s: %s",
			n+1,
			len(docs),
			gen,
			r.Error,
		)
	}
	return nil
}

func (i *Index[D]) Remove(ctx context.Context, gen search.Generation, ids []string) error {
	documents := i.client.Collection(string(gen))
	for _, id := range ids {
		if _, err := documents.Document(id).Delete(ctx); err != nil {
			if notFound(err) {
				continue
			}
			return fail("typesense.Remove", err, "deleting %q from %s", id, gen)
		}
	}
	return nil
}

func (i *Index[D]) CountIn(ctx context.Context, gen search.Generation) (int64, error) {
	got, err := i.client.Collection(string(gen)).Retrieve(ctx)
	if err != nil {
		return 0, fail("typesense.CountIn", err, "reading %s", gen)
	}
	if got.NumDocuments == nil {
		return 0, search.Errf(
			search.KindIndexUnavailable,
			"typesense.CountIn",
			nil,
			"%s reported no document count",
			gen,
		)
	}
	return *got.NumDocuments, nil
}

func (i *Index[D]) Promote(ctx context.Context, gen search.Generation) error {
	_, err := i.client.Aliases().Upsert(
		ctx, string(i.collection),
		&api.CollectionAliasSchema{
			CollectionName: string(gen),
		},
	)
	if err != nil {
		return fail("typesense.Promote", err, "pointing alias %s at %s", i.collection, gen)
	}
	return nil
}

func (i *Index[D]) Live(ctx context.Context) (search.Generation, error) {
	alias, err := i.client.Alias(string(i.collection)).Retrieve(ctx)
	if err != nil {
		if notFound(err) {
			return "", nil
		}
		return "", fail("typesense.Live", err, "reading alias %s", i.collection)
	}
	return search.Generation(alias.CollectionName), nil
}

func NewGeneration(collection search.Collection, unix int64) search.Generation {
	return search.Generation(fmt.Sprintf("%s_v%d", collection, unix))
}
