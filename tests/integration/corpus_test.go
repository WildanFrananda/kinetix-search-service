package integration_test

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/typesense"
)

func TestLoadCorpusForMeasurement(t *testing.T) {
	size := os.Getenv("KINETIX_TYPESENSE_CORPUS")
	if size == "" {
		t.Skip("set KINETIX_TYPESENSE_CORPUS to a document count to load a measurement corpus")
	}
	total, err := strconv.Atoi(size)
	require.NoError(t, err)

	ts, ctx := engine(t)
	gen := typesense.NewGeneration(search.Products, time.Now().UnixNano())
	require.NoError(t, ts.index.CreateGeneration(ctx, gen))

	nouns := []string{
		"Sepatu",
		"Sandal",
		"Tas",
		"Kemeja",
		"Celana",
		"Jaket",
		"Topi",
		"Kaos",
		"Shoes",
		"Sandals",
		"Bag",
		"Shirt",
		"Trousers",
		"Jacket",
		"Hat",
		"T-shirt",
	}
	adjectives := []string{
		"kulit",
		"kanvas",
		"hitam",
		"putih",
		"anak",
		"pria",
		"wanita",
		"leather",
		"canvas",
		"black",
		"white",
		"kids",
		"men",
		"women",
	}
	categories := []string{"sepatu", "sandal", "tas", "pakaian", "aksesoris"}

	source := rand.New(rand.NewSource(20260921))
	const batchSize = 2000
	started := time.Now()

	for offset := 0; offset < total; offset += batchSize {
		n := min(batchSize, total-offset)
		batch := make([]search.ProductDoc, 0, n)

		for i := range n {
			id, err := search.NewProductID(fmt.Sprintf("p-%d", offset+i))
			require.NoError(t, err)
			merchant, err := search.NewMerchantID(fmt.Sprintf("shop-%d", (offset+i)%2000))
			require.NoError(t, err)

			batch = append(batch, search.ProductDoc{
				ID:         id,
				MerchantID: merchant,
				Title: fmt.Sprintf(
					"%s %s %s %d",
					nouns[source.Intn(len(nouns))],
					adjectives[source.Intn(len(adjectives))],
					adjectives[source.Intn(len(adjectives))],
					offset+i,
				),
				Description: "Deskripsi produk nomor " + strconv.Itoa(offset+i) + ". Bahan berkualitas, pengiriman cepat dari seluruh Indonesia.",
				Categories: []string{categories[source.Intn(len(categories))]},
				PriceMinor: int64(source.Intn(5_000_000) + 10_000),
				Currency:   "IDR",
				UpdatedAt:  time.Now().UTC().Add(-time.Duration(source.Intn(90*24)) * time.Hour),
			})
		}

		require.NoError(t, ts.index.Bulk(ctx, gen, batch))
	}

	count, err := ts.index.CountIn(ctx, gen)
	require.NoError(t, err)
	require.Equal(t, int64(total), count)
	require.NoError(t, ts.index.Promote(ctx, gen))

	t.Logf("loaded %d documents into %s in %s", total, gen, time.Since(started).Round(time.Millisecond))

	q, err := search.NewQuery("sepatu kulit")
	require.NoError(t, err)
	queried := time.Now()
	res, err := ts.searcher.Search(ctx, q)
	require.NoError(t, err)
	t.Logf("query matched %d of %d in %s", res.Total, total, time.Since(queried).Round(time.Microsecond))
	require.NotEmpty(t, res.Hits)
}
