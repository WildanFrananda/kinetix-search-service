# kinetix-search-service — the agreed plan

Written 2026-09-21. Revised twice the same day: once for the layout, once to record your answers
to all ten decisions. Nothing has been built yet — no `git init`, no source files in this
repository.

Every fact below was checked today, and the check is named next to it. Where something is a
judgement it says so, and where a number has not been measured it says that too.

---

## 0. Decisions taken

| # | Question | Your answer | What it means here |
|---|---|---|---|
| 1 | Ingestion | **A** | Sources serve a cursor-paged gRPC RPC; search polls. No event bus. |
| 2 | Engine | **something lighter** | **Typesense**, not Elasticsearch (§3). Elasticsearch is then removed (§11). |
| 3 | Own database | **yes** | A small Postgres for cursors and index generations. |
| 4 | Scope | **everything** | Three collections: products, merchants, orders. Orders are private, see §7. |
| 5 | One type per file | *my recommendation* | Kept in `internal/search`; relaxed in adapters. |
| 6 | Language | **both** | Indonesian and English in one index — and the lighter engine makes this easier, see §3. |
| 7 | Callers | **gRPC** | Search publishes `search/v1`. It is no longer REST-only. |
| 8 | Catalog work | *my recommendation* | Tracked as its own change in the catalog repo, done **first** — nothing here can be proven without it. |
| 9 | `PriceMinor` | *my recommendation* | Kept, for sorting and range filters only, never presented as the price paid. |
| 10 | Sealed value types | *my recommendation* | Identifiers sealed; document structs stay plain. Verified — see §6. |

**#2 has a consequence, and it is now decided too.** You ruled on 2026-08-31 that Elasticsearch
stays, and `compose.yaml` says it is *"Earmarked for the search service the user intends to
build"*. Choosing Typesense voids that earmark. On your instruction the call is mine:
**Elasticsearch comes out — in the same change that adds Typesense, and only after its volume is
confirmed empty.** The reasoning, and the three conditions, are in §11.

**Nothing in this document is open any more.** What remains is work, sequenced in §10.

---

## 1. What this service is, and what it must never become

Search answers *"which of these things matches what this person typed"*. Nothing in the estate
does it today: catalog's search is a database `LIKE` (`title__icontains`), which cannot rank,
cannot tolerate a typo, and gets slower with every product added.

**Search owns:** the index, the query language, relevance and ranking, facets, autocomplete,
synonyms, and the freshness of its own index.

**Search does not own, and must never appear to own:**

| Thing | Owner |
|---|---|
| Products | catalog |
| Merchants and who anybody is | identity |
| Orders | order |
| Stock levels | warehouse / bin-stock |
| Prices | pricing |

**Search is a read model, never a source of truth.** No client ever writes to it. If search and a
source disagree, the source is right and search is stale — and search must be able to say so
rather than look complete.

---

## 2. Ingestion: decision #1, and what it costs

Three facts, verified today:

1. **There is no `catalog` package in kinetix-contracts.** The repository holds `common`, `fleet`,
   `fulfillment`, `geo`, `identity`, `notification`, `order`, `payment`, `pricing`, `returns`,
   `shipping`.
2. **Catalog serves no gRPC at all.** Its `Dockerfile` exposes `8000`, and a grep across
   `core/**.py` finds gRPC *clients* only — controlled against a grep for `def ` that matched 62
   files, so the empty result is real.
3. **The only list-shaped RPC in the whole contract is `ListOrdersForPrincipal`.** A grep for
   `rpc (List|Search|Changed|Stream)…` across every proto returns that and
   `StreamDriverLocation`. Nothing anywhere pages over a whole collection with a cursor.

So decision #4 ("everything") and decision #1 ("pull with a cursor") together mean **three sources
have to grow one RPC each**:

| Collection | Source | What it needs | Size of the change |
|---|---|---|---|
| products | catalog | a whole new `catalog/v1` package **and** catalog's first gRPC server | large |
| merchants | identity | one new RPC on an existing service | small |
| orders | order | one new RPC on an existing service | small |

Each is `ChangedSince(cursor, limit)` returning upserts, **explicit tombstones**, and the next
cursor. Tombstones are not optional: a record missing from a page could be a deletion or a paging
bug, and a service that guesses will quietly empty its own index.

**Products first, completely, before the other two start.** That is the recommendation you took in
#8, and the reason is that the catalog change is the large one and everything else is a variation
on it. It also forces a fix that is already owed: catalog's `list_products` calls `find_all()`,
materialises the whole table, slices in Python, then makes one gRPC call per product per page
through a ten-thread pool. That is already what stands between you and "produk banyak kaya
e-commerce real", and a cursor-paged endpoint cannot be built on top of it.

---

## 3. Engine: Typesense

You asked for something lighter. Between the two real candidates the licence decides it.

| | Meilisearch | Typesense |
|---|---|---|
| Engine licence | **`MIT AND BUSL-1.1`** | **GPL-3.0** |
| What that means | Anything under `enterprise_editions` is Business Source: *"Production use of the Licensed Work requires a commercial license agreement with Meilisearch."* Change Date is four years. | Copyleft, but the engine is a separate process reached over HTTP. It is not linked into your code and you are not redistributing it, so it places no obligation on this estate. |
| Go client | `meilisearch-go`, MIT | `typesense-go`, Apache-2.0 |

Read from each project's own `LICENSE` today, not from memory. Meilisearch used to be plain MIT;
it is not any more.

**Typesense.** One licence covering the whole engine, with no per-feature question of "is this one
Enterprise?" to re-ask at every upgrade. For a shop that is about to take real money, that
certainty is worth more than a feature comparison.

Two things checked in the client, because the design depends on them:

- **`Alias()` / `Aliases()` exist** — so the blue/green generation swap in §6 works as designed.
- **`GenerateScopedSearchKey(searchKey, params)` exists** — an API key with a filter baked into it,
  which is defence in depth for the private orders collection.

**Client version matters here.** `go get github.com/typesense/typesense-go/v4@latest` resolves to
**v4.0.0-alpha2**, an alpha. The stable line is **`github.com/typesense/typesense-go/v3 v3.2.0`**,
and that is what this service pins. Do not let a tidy "upgrade to v4" land without reading that
sentence again.

**Decision #6 gets easier, not harder.** Elasticsearch would have needed an analyzer chain chosen
per language and a mapping that is painful to change later. Typesense is language-agnostic by
design: one collection holds Indonesian and English text together, and typo tolerance does most of
the work a stemmer would. Synonyms stay an explicit, editable list.

**The number is not measured yet, and I will not invent one.** The 981 MB for Elasticsearch is
real — it is in your own `compose.yaml`, measured 2026-09-10. Typesense is published as
substantially lighter, but the figure *for this estate* is an exit criterion for step 2, measured
with the real corpus loaded rather than against an empty container. See §11.

---

## 4. Stack

| Layer | Choice | Why this, and what was rejected |
|---|---|---|
| Language | **Go 1.26** | Installed (`go1.26.4 darwin/arm64`). First Go service in the estate. |
| HTTP | **chi v5** | A router, not a framework: handlers stay `http.Handler`, so no framework type reaches the core. Rejected Gin and Echo (their own context type spreads); rejected Fiber (fasthttp is not `net/http`). |
| gRPC | **google.golang.org/grpc**, stubs generated at build time from vendored `.proto` | buf *generates* Go (`go_package_prefix: github.com/WildanFrananda/kinetix-contracts/gen/go`) but the release workflow publishes only **npm, pypi, rubygems, packagist** — no Go registry job, and `gen/go` is not committed. Go follows the Rust/Elixir/Scala pattern: `bin/sync-contracts`, pinned to a tag *and* a commit. |
| Search | **Typesense**, `typesense-go/v3 v3.2.0` | §3. |
| DI | **Manual constructor injection**, wired in `cmd/searchd/main.go` | What you asked for in the C++ guidance: compile-time, not reflection. Rejected **uber/fx** and **dig** (runtime reflection). **google/wire** is the escalation if wiring grows — codegen, so wrong wiring stays a compile error. Start without it. |
| Database | **PostgreSQL**, small (decision #3) | Cursors per collection, index generation registry, synonyms. |
| "ORM" | **sqlc + pgx/v5** | sqlc reads real SQL and generates typed Go. No reflection, no struct tags, no magic. Rejected **GORM** (reflection, string-keyed conditions, silent zero-value behaviour) and **ent** (a large codegen graph for three tables). |
| Migrations | **goose**, plain `.sql` | Same shape as pricing's raw-SQL migrations; one-shot migrator container. |
| Config | Environment only, **fail closed** | Name every missing variable at once and refuse to start, as chat does. |
| Logging | **log/slog** (stdlib) | |
| Metrics | **prometheus/client_golang** | |
| Testing | stdlib `testing` + **testify/require** + hand-written fakes | Go's idiom for small interfaces. **mockgen** only if a port grows wide enough that a fake stops being cheaper. |
| Lint | **golangci-lint** | errcheck, govet, staticcheck, revive, gosec. |

---

## 5. Layout

The first draft used `internal/ports/` and `internal/adapters/{http,grpc,…}`. That is a Java and
C# shape — it names folders after their *role in the architecture*. Go names a package after **what
it provides**, read at the call site: `typesense.NewIndex`, `postgres.NewCheckpoint`,
`httpapi.NewRouter`, `indexing.NewSyncer`.

```
kinetix-search-service/
├── go.mod                       module github.com/WildanFrananda/kinetix-search-service
├── cmd/
│   ├── searchd/main.go          the service. composition root.
│   └── reindex/main.go          one-shot full reindex. its own binary and container.
├── internal/
│   ├── search/                  ← THE CORE. imports nothing but the standard library.
│   │   ├── identifier.go          ProductID, MerchantID, OrderID — sealed
│   │   ├── document.go            Collection, ProductDoc, MerchantDoc, OrderDoc
│   │   ├── query.go               Query, Page, RankingProfile, options
│   │   ├── result.go              Hit[D], Results[D], Facet, Freshness
│   │   ├── cursor.go              Generation, Cursor, Changes[D]
│   │   ├── errors.go              Kind, Error, KindOf
│   │   └── service.go             Searcher[D], Index[D], Source[D], Checkpoint, Clock
│   ├── querying/                use case: Finder[D]
│   ├── indexing/                use cases: Syncer[D], Reindexer[D]
│   ├── typesense/               adapter → Typesense
│   ├── postgres/                adapter → Postgres; sqlc output lives here
│   ├── catalogclient/           adapter → catalog over gRPC   (products)
│   ├── identityclient/          adapter → identity over gRPC  (merchants)
│   ├── orderclient/             adapter → order over gRPC     (orders)
│   ├── grpcapi/                 adapter → the search/v1 server  (decision #7)
│   ├── httpapi/                 adapter → chi handlers
│   └── config/                  environment reading
├── tests/                       ← every test lives here, never beside the code it exercises
│   ├── search/  querying/  indexing/
│   └── searchtest/              hand-written fakes
├── bin/test                     the suite, with the -coverpkg the split requires
├── bin/check-core-purity
├── migrations/
├── contracts/                   gitignored; filled by bin/sync-contracts
├── bin/sync-contracts
├── sqlc.yaml
└── .golangci.yml
```

The hexagon stops being spelled in folder names and becomes a rule about **which package may import
which** — which the compiler can check:

| Package | May import |
|---|---|
| `internal/search` | the standard library. **Nothing else.** |
| `internal/querying`, `internal/indexing` | `internal/search` only |
| every adapter | `internal/search` (+ its own library) |
| `cmd/…` | all of them — they are the composition roots |

Three things make that structural rather than aspirational:

- **`internal/` is a Go language feature.** Nothing outside this module can import any of it. The
  compiler refuses; it is not a convention.
- **The core has no dependency to leak** — only `context`, `time`, `strings`, `fmt`, `errors`. A
  `go list -deps` assertion in CI fails the build the day somebody puts a Typesense type in a
  domain struct.
- **Every adapter carries `var _ search.Searcher[search.ProductDoc] = (*Searcher)(nil)`**, because
  Go interfaces are structural and will not tell you on their own.

This is Ben Johnson's "standard package layout", the closest thing Go has to a consensus answer for
hexagonal — worth looking up for a second opinion on the shape.

### Is this object-oriented?

Three of the four pillars. **Abstraction** (`Searcher`, `Index`, `Source`, `Checkpoint`, `Clock`),
**polymorphism** (interface dispatch; `RankingProfile` is Strategy), **encapsulation** (package
level, sealed identifiers). **Inheritance does not exist in Go** — and nothing is lost, because
every pattern here composes rather than subclasses: a `Finder` *holds* its ports, a circuit breaker
*wraps* a client, `main` *assembles* the graph. In the chat service the ports are abstract base
classes and the polymorphism rides on inheritance; in Go a type satisfies an interface without
declaring it and the hierarchy never exists.

### Design patterns, named

Ports & adapters · read model / CQRS-lite projection · repository · **cursor checkpoint** (one code
path for sync and reindex) · **blue/green index with alias swap** · strategy (ranking profiles) ·
decorator (circuit breaker and metrics wrapped around outbound clients) · functional options (Go's
builder) · composition root.

---

## 6. What the code actually looks like

**These are not sketches.** `internal/search`, `internal/querying` and `tests/searchtest` below
were written into a scratch module today: `go build ./...` clean, `go vet ./...` clean, **seven
tests pass**. Two of them were then proven able to fail — with the error swallowed,
`TestFindFailsRatherThanReturningNothing` goes red; with the orders guard removed,
`TestOrdersCannotBeSearchedWithoutAPrincipal` goes red. A test never seen to fail is not a gate.

### Sealed identifiers — decision #10, and why it is not just style

`type ProductID string` reads like Go, and it can be walked straight past:

```go
bad   := search.ProductID("   ")         // compiles. NewProductID refuses this.
worse := search.ProductID("id\x1b[31m")  // compiles. So would this.
```

That was run from outside the package: both succeed. An unexported field closes it.

```go
package search

import "strings"

// ProductID is sealed: the unexported field means the validating factory is the only way in from
// outside this package.
type ProductID struct{ v string }

func NewProductID(raw string) (ProductID, error) { return newID[ProductID](raw, "search.NewProductID") }
func (p ProductID) String() string               { return p.v }
func (p ProductID) IsZero() bool                 { return p.v == "" }
func (p ProductID) set(s string) ProductID       { p.v = s; return p }

type MerchantID struct{ v string }

func NewMerchantID(raw string) (MerchantID, error) { return newID[MerchantID](raw, "search.NewMerchantID") }
func (m MerchantID) String() string                { return m.v }
func (m MerchantID) IsZero() bool                  { return m.v == "" }
func (m MerchantID) set(s string) MerchantID       { m.v = s; return m }

type OrderID struct{ v string }

func NewOrderID(raw string) (OrderID, error) { return newID[OrderID](raw, "search.NewOrderID") }
func (o OrderID) String() string             { return o.v }
func (o OrderID) IsZero() bool               { return o.v == "" }
func (o OrderID) set(s string) OrderID       { o.v = s; return o }

// identifier is satisfied only by the sealed types above, because `set` is unexported. A type in
// another package cannot implement it, so this generic helper cannot be borrowed to mint an
// identifier the factory never saw.
type identifier[T any] interface{ set(string) T }

const maxIDLength = 64

func newID[T identifier[T]](raw, op string) (T, error) {
	var zero T
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return zero, Errf(KindMalformedQuery, op, nil, "empty identifier")
	case len(trimmed) > maxIDLength:
		return zero, Errf(KindMalformedQuery, op, nil, "identifier too long")
	case strings.ContainsFunc(trimmed, isControl):
		return zero, Errf(KindMalformedQuery, op, nil, "control byte in identifier")
	}
	return zero.set(trimmed), nil
}
```

The bypass then stops compiling:

```
cannot refer to unexported field v in struct literal of type search.ProductID
```

The zero value `ProductID{}` is still constructible — Go cannot prevent that — but a zero value is
*detectable* with `IsZero`, whereas arbitrary junk is not.

### Three collections, three document types — decision #4

```go
package search

import "time"

// Collection names one searchable set. One physical generation sits behind each alias.
type Collection string

const (
	Products  Collection = "products"
	Merchants Collection = "merchants"
	Orders    Collection = "orders"
)

// Separate types on purpose: they are ranked by different rules, filtered on different fields,
// and one of them is private to a single person.
type ProductDoc struct {
	ID          ProductID
	MerchantID  MerchantID
	Title       string
	Description string
	Categories  []string

	// PriceMinor is carried for sorting and range filters only — decision #9. It is catalog's
	// list price at the moment of indexing, and it is NOT what a customer pays: that is pricing's
	// answer, at checkout, every time. No API response presents it as the price.
	PriceMinor int64
	Currency   string

	// UpdatedAt is the source's own timestamp, not the time this service indexed it. The cursor
	// advances on it, so it has to come from the source.
	UpdatedAt time.Time
}

type MerchantDoc struct {
	ID          MerchantID
	DisplayName string
	Categories  []string
	UpdatedAt   time.Time
}

// OrderDoc is the one document that is not public. Every query against Orders is filtered by the
// requesting principal, in the use case, before it reaches an adapter.
type OrderDoc struct {
	ID         OrderID
	Buyer      MerchantID
	MerchantID MerchantID
	Number     string
	Status     string
	LineTitles []string
	PlacedAt   time.Time
	UpdatedAt  time.Time
}
```

### The interfaces, generic over the document type

```go
package search

import (
	"context"
	"time"
)

type Searcher[D any] interface {
	Search(ctx context.Context, q Query) (Results[D], error)
	Suggest(ctx context.Context, prefix string, limit int) ([]string, error)
}

// Index is generation-aware on purpose: a reindex builds a new generation and promotes it only
// after it has been checked, so a half-built index is never served and a failed reindex changes
// nothing.
type Index[D any] interface {
	CreateGeneration(ctx context.Context, gen Generation) error
	Bulk(ctx context.Context, gen Generation, docs []D) error
	Remove(ctx context.Context, gen Generation, ids []string) error
	CountIn(ctx context.Context, gen Generation) (int64, error)
	Promote(ctx context.Context, gen Generation) error
	Live(ctx context.Context) (Generation, error)
}

// Source is whoever owns the records: catalog for products, identity for merchants, order for
// orders. Search never owns any of them.
type Source[D any] interface {
	ChangedSince(ctx context.Context, cur Cursor, limit int) (Changes[D], error)
	Count(ctx context.Context) (int64, error)
}

// Checkpoint is per collection, because the three sources advance independently.
type Checkpoint interface {
	Load(ctx context.Context, c Collection) (Cursor, error)
	Save(ctx context.Context, c Collection, cur Cursor) error
}

type Clock interface{ Now() time.Time }
```

### The query, and the one filter that is not a preference

```go
type Query struct {
	Text       string
	Categories []string
	MerchantID MerchantID

	// OnBehalfOf scopes a query to one principal. It is the only filter that carries a rule
	// rather than a preference: an Orders query without it would read somebody else's orders,
	// so the use case sets it and no option can.
	OnBehalfOf MerchantID

	Page    Page
	Ranking RankingProfile
}

// ForPrincipal is deliberately not a QueryOption: options come from the caller, and this one must
// come from the authenticated identity.
func (q Query) ForPrincipal(p MerchantID) Query { q.OnBehalfOf = p; return q }
```

### The use case

```go
package querying

import (
	"context"
	"time"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Finder[D any] struct {
	collection search.Collection
	searcher   search.Searcher[D]
	checkpoint search.Checkpoint
	clock      search.Clock
	staleAfter time.Duration
}

func NewFinder[D any](
	collection search.Collection,
	searcher search.Searcher[D],
	checkpoint search.Checkpoint,
	clock search.Clock,
	staleAfter time.Duration,
) *Finder[D] {
	return &Finder[D]{collection, searcher, checkpoint, clock, staleAfter}
}

func (f *Finder[D]) Find(ctx context.Context, q search.Query) (search.Results[D], error) {
	// Orders are private. A query that reaches this collection without a principal is a bug, and
	// it fails here rather than returning somebody else's orders.
	if f.collection == search.Orders && q.OnBehalfOf.IsZero() {
		return search.Results[D]{}, search.Errf(search.KindMalformedQuery, "querying.Find", nil,
			"orders may only be searched on behalf of a principal")
	}

	// A failed search is a failure. Never an empty result set: a shop that answers "no products"
	// during an outage teaches its customers the shop is empty.
	results, err := f.searcher.Search(ctx, q)
	if err != nil {
		return search.Results[D]{}, err
	}

	cur, err := f.checkpoint.Load(ctx, f.collection)
	if err != nil {
		return search.Results[D]{}, err
	}

	age := f.clock.Now().Sub(cur.UpdatedThrough)
	results.Freshness = search.Freshness{
		IndexedThrough: cur.UpdatedThrough,
		Age:            age,
		Stale:          age > f.staleAfter,
	}
	return results, nil
}
```

### The two tests that are gates, not decoration

```go
func TestFindFailsRatherThanReturningNothing(t *testing.T) {
	s := &searchtest.FakeSearcher[search.ProductDoc]{
		Err: search.Errf(search.KindIndexUnavailable, "typesense.Search", nil, "cluster unreachable"),
	}
	f := productFinder(s, &searchtest.FakeCheckpoint{}, time.Minute)

	q, _ := search.NewQuery("sepatu")
	res, err := f.Find(context.Background(), q)

	require.Error(t, err)
	require.Equal(t, search.KindIndexUnavailable, search.KindOf(err))
	require.Empty(t, res.Hits)
}

func TestOrdersCannotBeSearchedWithoutAPrincipal(t *testing.T) {
	s := &searchtest.FakeSearcher[search.OrderDoc]{}
	f := querying.NewFinder(search.Orders, s, &searchtest.FakeCheckpoint{},
		searchtest.FixedClock{At: noon}, time.Minute)

	q, _ := search.NewQuery("KNX-2026")
	_, err := f.Find(context.Background(), q)

	require.Error(t, err)
	require.Equal(t, search.KindMalformedQuery, search.KindOf(err))
	require.Empty(t, s.Asked, "the adapter must never see an unscoped orders query")
}
```

### The Typesense adapter

```go
package typesense

import (
	"context"
	"fmt"

	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"

	"github.com/WildanFrananda/kinetix-search-service/internal/search"
)

type Searcher[D any] struct {
	client     *typesense.Client
	collection search.Collection
	decode     func(map[string]any) (D, error)
}

// The assertion Go does not make for you.
var _ search.Searcher[search.ProductDoc] = (*Searcher[search.ProductDoc])(nil)

func (s *Searcher[D]) Search(ctx context.Context, q search.Query) (search.Results[D], error) {
	params := &api.SearchCollectionParams{
		Q:       ptr(q.Text),
		QueryBy: ptr(queryFieldsFor(s.collection)),
		FilterBy: ptr(filterFor(q)),   // includes OnBehalfOf for Orders
		SortBy:   ptr(sortFor(q.Ranking)),
		Page:     ptr(q.Page.Offset/max(q.Page.Limit, 1) + 1),
		PerPage:  ptr(q.Page.Limit),
	}

	res, err := s.client.Collection(string(s.collection)).Documents().Search(ctx, params)
	if err != nil {
		// Every failure out of the engine is KindIndexUnavailable, never an empty Results. The
		// engine not answering is not the same as nothing matching, and the two must not arrive
		// at the caller looking identical.
		return search.Results[D]{}, search.Errf(search.KindIndexUnavailable, "typesense.Search",
			err, "searching %s", s.collection)
	}
	return s.decode(res)
}

// Promote is the blue/green swap: queries hit the alias, never a physical collection, so moving
// the alias is the whole of a release.
func (i *Index[D]) Promote(ctx context.Context, gen search.Generation) error {
	_, err := i.client.Aliases().Upsert(ctx, string(i.collection), &api.CollectionAliasSchema{
		CollectionName: string(gen),
	})
	if err != nil {
		return search.Errf(search.KindIndexUnavailable, "typesense.Promote", err,
			"promoting %s to %s", gen, i.collection)
	}
	return nil
}

func ptr[T any](v T) *T { return &v }
func filterFor(search.Query) string       { /* decided with the mapping, step 2 */ return "" }
func sortFor(search.RankingProfile) string { return "" }
func queryFieldsFor(search.Collection) string { return "" }
```

The three elided helpers depend on the index mapping, which is step 2 and a separate short review.
They are left empty rather than guessed.

### The reindexer — blue/green, and why it refuses

```go
func (r *Reindexer[D]) Run(ctx context.Context) error {
	gen := search.Generation(fmt.Sprintf("%s_v%d", r.collection, r.clock.Now().Unix()))

	if err := r.index.CreateGeneration(ctx, gen); err != nil {
		return err
	}
	expected, err := r.source.Count(ctx)
	if err != nil {
		return err
	}

	var cur search.Cursor
	for {
		changes, err := r.source.ChangedSince(ctx, cur, r.pageSize)
		if err != nil {
			return err
		}
		if err := r.index.Bulk(ctx, gen, changes.Upserted); err != nil {
			return err
		}
		cur = changes.Next
		if !changes.HasMore {
			break
		}
	}

	indexed, err := r.index.CountIn(ctx, gen)
	if err != nil {
		return err
	}
	if indexed != expected {
		// Refusing to promote is the whole value of this design. A generation short by four
		// hundred products looks exactly like a working search until a merchant asks why their
		// listing vanished.
		return search.Errf(search.KindIndexUnavailable, "indexing.Reindex", nil,
			"generation %s holds %d documents, the source reports %d", gen, indexed, expected)
	}
	return r.index.Promote(ctx, gen)
}
```

### The sync loop

```go
func (s *Syncer[D]) Once(ctx context.Context) error {
	cur, err := s.checkpoint.Load(ctx, s.collection)
	if err != nil {
		return err
	}
	gen, err := s.index.Live(ctx)
	if err != nil {
		return err
	}

	for {
		changes, err := s.source.ChangedSince(ctx, cur, s.pageSize)
		if err != nil {
			// The cursor does not move. Staleness grows and becomes visible in every search
			// response — which is the intended behaviour. Skipping a page to make progress would
			// lose those records until the next full reindex.
			return err
		}

		if len(changes.Upserted) > 0 {
			if err := s.index.Bulk(ctx, gen, changes.Upserted); err != nil {
				return err
			}
		}
		if len(changes.Removed) > 0 {
			if err := s.index.Remove(ctx, gen, changes.Removed); err != nil {
				return err
			}
		}

		// Saved only after the write landed. A cursor saved first would skip exactly the page
		// that failed, and nothing would ever notice.
		if err := s.checkpoint.Save(ctx, s.collection, changes.Next); err != nil {
			return err
		}
		cur = changes.Next

		if !changes.HasMore {
			return nil
		}
	}
}
```

### Postgres: sqlc, not an ORM

`migrations/0001_search_state.sql`:

```sql
-- +goose Up

-- One row per collection. The high-water mark of what has been indexed, and the only mutable
-- state this service owns.
CREATE TABLE sync_checkpoint (
    collection       TEXT        PRIMARY KEY,
    updated_through  TIMESTAMPTZ NOT NULL,
    last_id          TEXT        NOT NULL,
    saved_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT sync_checkpoint_known_collection
        CHECK (collection IN ('products', 'merchants', 'orders'))
);

CREATE TABLE index_generation (
    collection  TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    built_at    TIMESTAMPTZ NOT NULL,
    promoted_at TIMESTAMPTZ,
    documents   BIGINT      NOT NULL,

    PRIMARY KEY (collection, name)
);

-- +goose Down
DROP TABLE index_generation;
DROP TABLE sync_checkpoint;
```

`internal/postgres/queries.sql`:

```sql
-- name: LoadCheckpoint :one
SELECT updated_through, last_id FROM sync_checkpoint WHERE collection = $1;

-- name: SaveCheckpoint :exec
INSERT INTO sync_checkpoint (collection, updated_through, last_id, saved_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (collection) DO UPDATE
   SET updated_through = EXCLUDED.updated_through,
       last_id         = EXCLUDED.last_id,
       saved_at        = EXCLUDED.saved_at;
```

```go
func (c *Checkpoint) Load(ctx context.Context, col search.Collection) (search.Cursor, error) {
	row, err := c.q.LoadCheckpoint(ctx, string(col))
	if errors.Is(err, pgx.ErrNoRows) {
		// Never synced is not a failure: the zero cursor is exactly "start from the beginning".
		return search.Cursor{}, nil
	}
	if err != nil {
		return search.Cursor{}, search.Errf(
			search.KindDependencyUnavailable, "postgres.LoadCheckpoint", err, "")
	}
	return search.Cursor{UpdatedThrough: row.UpdatedThrough.Time, LastID: row.LastID}, nil
}
```

---

## 7. Orders are private, and that shapes the design

Decision #4 puts every order in a search index. That is a privacy surface the other two
collections do not have, and it gets three layers rather than one:

1. **The use case refuses.** An `Orders` query with no `OnBehalfOf` fails before any adapter sees
   it. Proven by a test that goes red when the guard is removed.
2. **`ForPrincipal` is a method, not an option.** Options come from the caller; this one comes from
   the authenticated identity, so a handler cannot accidentally let a query parameter set it.
3. **A scoped Typesense key.** `GenerateScopedSearchKey` bakes the principal filter into the API
   key itself, so even a bug in filter construction cannot widen the result set.

`OrderDoc` also carries the minimum: identifiers, a status, a number, and the line titles that make
a result readable. No addresses, no totals, no payment detail.

---

## 8. Contract changes

Four, and only the first two are on the path to anything shipping.

| Package | Change | Why |
|---|---|---|
| **`catalog/v1`** | new file, new service | products. Catalog's first gRPC server. |
| **`search/v1`** | new file, new service | decision #7 — the recommendation service will call search. |
| `identity/v1` | one new RPC | merchants. |
| `order/v1` | one new RPC | orders. |

All additive in new files or as new RPCs, so `buf breaking` has nothing to object to — which is the
opposite of the trap the chat service hit with `has_location`.

---

## 9. Infrastructure

- **Typesense** container, its own volume, api key from the secret store — not parked, because it
  will have a user from the day it starts.
- **Elasticsearch** loses its earmark. Removed, or still parked? — the open question in §0.
- **New Postgres** for search plus a one-shot migrator, following the estate's pattern.
- **Ports:** gRPC `50051`–`50057` are taken; search takes **`50058`**. Only `8443` is published on
  the host — search sits behind Kong like everything else.
- **mTLS/SPIFFE** identity for the service, as every gRPC participant has.

---

## 10. Build order

1. ~~`internal/search` + `querying` + `indexing` with hand-written fakes.~~ **Done 2026-09-21.**
   34 tests, `go vet`, `gofmt`, `golangci-lint` and `bin/check-core-purity` all clean; four
   invariants each proven red before green.
2. `internal/typesense`. **Code done 2026-09-21** — schema, encode/decode, filter and sort
   building, bulk import with per-document result checking, tombstones, alias swap, suggestions.
   24 unit tests over the pure parts. **Two things are still owed before this step closes:**
   - the engine tests in `tests/integration/` have never been run against a real Typesense —
     they skip without `KINETIX_TYPESENSE_URL`, and a skipped test proves nothing;
   - the memory figure, measured with a real corpus (§11).

   The index mapping is in `internal/typesense/schema.go` and is the part that is expensive to
   change later. It is the subject of the short review promised here.
3. **`catalog/v1` + catalog's gRPC server + its read-path fix.** Work in the catalog repo, tracked
   separately (decision #8), and nothing here can be proven end to end until it lands.
4. Sync worker and the reindex binary, for products.
5. HTTP edge, `search/v1` gRPC server, freshness, ranking profiles.
6. Merchants and orders: two more sources, two more collections, the same code paths.
7. Metrics, compose, CI, Dockerfile — with the other new services, as you deferred.

---

## 11. The last three, decided

You asked for my recommendation on the remainder. Here it is, with what each one rests on.

### Elasticsearch — remove it, in the same change that adds Typesense, not before

Its one stated purpose is gone. Keeping it now would leave infrastructure with no user *and no
prospect of one*, which is precisely what the compose file already warns against in its own words:
parked datastores "tell every reader of this file that the estate is event-driven and search-backed
when, today, it is neither."

It is not free while it sits there, and the cost is larger than the 981 MB:

- **`KINETIX_INFRA_ELASTIC_PASSWORD` is a required variable** — `${…:?…}` — and is listed in
  `docs/SECRETS.md`. While the block exists, that secret must exist in the store, be rotated with
  everything else, and be present for compose to render **even though the container never starts**.
- The image carries CVE surface that someone has to keep answering for.

**Three conditions on the removal, and none of them is optional:**

1. **Confirm `es_data` is empty first.** Nothing has ever written to it, but "nothing has ever
   written to it" is a claim, and Docker is not running, so it has not been checked. `docker volume
   inspect` and a byte count settle it. A volume is not deleted on a guess.
2. **Do it in the same commit that adds Typesense**, so the compose file never shows two search
   engines and nobody has to work out which one is real.
3. **Drop the secret in the same change** — out of `compose.yaml`, out of `docs/SECRETS.md`, out of
   the store — because a secret that outlives its service is a secret nobody can explain later.

Reversible if Typesense disappoints: the block is in git, the image is public, and there is no data
to restore.

**Related, and not part of this plan:** MongoDB is parked under the same 2026-08-31 ruling, and its
note says plainly that nobody has said what data it is for. Elasticsearch at least *had* an answer
until today. Worth a decision of its own at some point; it is not mine to fold into this one.

### The Typesense memory figure — measured 2026-09-21

Typesense **30.2** in Docker on the laptop, with the schema in `internal/typesense/schema.go` and
a synthetic product corpus. Read with `docker stats --no-stream`, five seconds after the load
settled.

| State | Resident | Load time | Query |
|---|---|---|---|
| Empty, fresh container | **182 MiB** | — | — |
| 100,000 products | **320 MiB** | 3.5 s | 7.9 ms |
| 500,000 products | **551 MiB** | 18.3 s | 20.8 ms |

**Elasticsearch, for comparison: 981 MB with an empty index** — measured 2026-09-10 and written in
the estate's own `compose.yaml`. Half a million products cost Typesense less than Elasticsearch
costs holding nothing. Extrapolating the slope (~0.74 MiB per thousand documents between the two
loaded points), a million products lands near 920 MiB — roughly where Elasticsearch *starts*.

Decision #2 is confirmed by the numbers, not only by the licence.

**Two caveats, stated rather than glossed:**

- **The corpus is synthetic.** Catalog has no gRPC endpoint yet (step 3), so there is no real
  product data. Titles and categories are drawn from a sixteen-word vocabulary, which understates
  the term dictionary a real catalogue builds. These figures are a floor, not a forecast.
- **Memory is not returned when a collection is deleted.** Dropping every collection and
  restarting left the container at 373 MiB where a genuinely fresh one sits at 182 MiB. An
  operator watching RSS after a reindex will see the retired generation linger; that is the
  allocator holding pages, not a leak, and it is what makes the blue/green swap cost roughly
  double the live index for a while. Worth knowing before sizing the production container.

### The index mapping — unchanged: step 2, its own short review

Fields, analyzers, synonyms, ranking weights and facets depend on the real product data. Deciding
them now would produce a schema that looks agreed and is not.
