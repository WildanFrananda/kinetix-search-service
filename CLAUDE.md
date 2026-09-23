# CLAUDE.md — kinetix-search-service

Guidance for Claude Code when working in this repository. This service is part of the **Kinetix**
polyglot e-commerce estate (one language per service, gRPC-only between services). It is written
in **Go**, in a strict **hexagonal architecture**.

**`PLAN.md` is the agreed design.** It records ten decisions the user took, the facts each rests
on, and the sequence of work. Read it before changing anything structural. This file is the short
version: the rules, and how they are enforced.

## What this service is

Search answers *"which of these things matches what this person typed"*, across three
collections: **products**, **merchants**, **orders**.

**It is a read model, never a source of truth.** Catalog owns products, identity owns merchants
and who anybody is, order owns orders, pricing owns prices, warehouse owns stock. If search and a
source disagree, the source is right and search is stale — and search must be able to say so
rather than look complete.

No client ever writes to search.

## Layout, and the rule it encodes

```
cmd/searchd/            the service. composition root.
cmd/syncd/              the incremental sync loop over all three collections.
cmd/reindex/            one-shot full reindex, named: reindex <products|merchants|orders|all>.
cmd/migrate/            one-shot migrator. the schema is embedded, not mounted.
internal/migrations/    the .sql goose files, embedded by cmd/migrate.
internal/mesh/          mTLS credentials shared by the three gRPC clients.
internal/search/        THE CORE. imports the standard library and nothing else.
internal/querying/      use case: Finder[D]
internal/indexing/      use cases: Syncer[D], Reindexer[D]
internal/typesense/     adapter → Typesense
internal/postgres/      adapter → Postgres; sqlc output
internal/catalogclient/ internal/identityclient/ internal/orderclient/   adapters → gRPC
internal/grpcapi/ internal/httpapi/                                      adapters → the edge
internal/typesense/      adapter → Typesense: schema, encode/decode, filters, alias swap
internal/config/        environment reading

tests/search/           tests for internal/search
tests/querying/         tests for internal/querying
tests/indexing/         tests for internal/indexing
tests/typesense/        tests for the adapter's pure parts
tests/integration/      the only tests that talk to a real engine
tests/searchtest/       hand-written fakes
```

The engine tests **skip** without `KINETIX_TYPESENSE_URL` and `KINETIX_TYPESENSE_KEY`, so a
developer without an engine still gets a green run — and CI must set them, or the skip becomes a
permanent hiding place. A skipped test proves nothing.

```sh
docker volume create kinetix-ts
docker run -d --name kinetix-typesense -p 8108:8108 -v kinetix-ts:/data \
  typesense/typesense:30.2 --data-dir /data --api-key=dev --enable-cors
KINETIX_TYPESENSE_URL=http://localhost:8108 KINETIX_TYPESENSE_KEY=dev ./bin/test
```

The volume is not optional: Typesense refuses to start when its `--data-dir` does not exist and
does not create one.

**Tests live in `tests/`, never beside the code they exercise.** That is the estate's rule, and it
is a deliberate departure from Go's convention of putting `foo_test.go` next to `foo.go`. It costs
nothing in access, because every test here is already an external `package x_test` and could not
reach unexported identifiers either way.

It costs one thing, and `bin/test` exists so that nobody pays it: **`go test -cover ./...` reports
`0.0%` for every package under `internal/`**, because Go measures a package against its own test
files and there are none. The same code measured with `-coverpkg=./internal/...` reports 90.9% for
`internal/search`. Use `bin/test`; the plain flag tells a lie that looks like a damning fact.

Package names say **what a package provides**, not its role in the architecture. That is why there
is no `ports/` and no `adapters/` — in Go the call site reads `typesense.NewIndex`,
`postgres.NewCheckpoint`, `indexing.NewSyncer`. The hexagon lives in the import rule instead:

| Package | May import |
|---|---|
| `internal/search` | the standard library. **Nothing else.** |
| `internal/querying`, `internal/indexing` | `internal/search` only |
| every adapter | `internal/search` (+ its own library) |
| `cmd/…` | all of them |

## How this repository enforces the rules

- **`bin/check-core-purity`** runs `go list -deps` over `internal/search` and fails if anything
  outside the standard library appears, transitively included. It is controlled both ways: a
  stdlib import passes, a `testify` import fails. Run it in CI.
- **`internal/` is a Go language feature.** Nothing outside this module can import any of it. The
  compiler refuses; it is not a convention.
- **Every adapter carries a compile-time assertion** — `var _ search.Searcher[search.ProductDoc] =
  (*Searcher)(nil)` — because Go interfaces are structural and will not tell you when a type has
  drifted out of one.
- **Identifiers are sealed.** `ProductID`, `MerchantID` and `OrderID` are structs with an
  unexported field, so the validating factory is the only way in from outside the package.
  `search.ProductID{v: "   "}` is a compile error. The zero value is still constructible — Go
  cannot prevent that — so every one has `IsZero`.
- **`golangci-lint`** with errcheck, errorlint, contextcheck and gosec among others. Verified to
  fire rather than merely report zero: an ignored `checkpoint.Save` is caught by errcheck.
- **`gofmt` and `goimports`.** Not negotiable in Go, and that is a feature.
- **`bin/test`** runs the suite with the coverage flags the `tests/` layout requires.

## Conventions

- **Errors:** every error out of the core is built with `search.Errf` and carries a `Kind`. Read it
  back with `search.KindOf`, which survives wrapping. Only an adapter turns a Kind into an HTTP
  status or a gRPC code. An error from outside this service has no Kind and must not be given one.
- **DI:** manual constructor injection, wired in `cmd/*/main.go`. No service locator, no global
  singleton, no reflection container. `google/wire` is the escalation if wiring grows — it is code
  generation, so wrong wiring stays a compile error.
- **Testing:** hand-written fakes in `tests/searchtest`, not a mocking framework. A fake that
  returns what the test set and records what it was asked is shorter to read than a stack of
  expectations.
- **One type per file** in `internal/search`, where a type is the unit of meaning. Relaxed in
  adapters, where Go's convention of naming a file for a concept reads better.
- **Naming:** no stutter. `search.Document`, not `search.SearchDocument`.

## Things decided here, and why

- **Three collections, three document types** — `ProductDoc`, `MerchantDoc`, `OrderDoc`. They are
  ranked by different rules, filtered on different fields, and one of them is private.

- **Orders are private, in three layers.** The use case refuses an `Orders` query with no
  `OnBehalfOf`; `ForPrincipal` is a method rather than a `QueryOption`, so a handler cannot let a
  query parameter set it; and the Typesense adapter uses a scoped search key with the principal
  filter baked in. `OrderDoc` carries no address, no total and no payment detail.

- **A failed search is a failure, never an empty result set.** A shop that answers "no products"
  during an outage teaches its customers the shop is empty. This is the estate's house failure
  mode, and `TestFindFailsRatherThanReturningNothing` is the gate.

- **Freshness travels with every result.** `indexed_through`, `age_seconds`, `stale`. The caller is
  told how current the answer is rather than left to assume.

- **The cursor is saved only after the write lands.** A cursor saved first skips exactly the page
  that failed, and the gap is never noticed.

- **A reindex builds a new generation and promotes it only if it is not short of the source's
  count.** `<` rather than `!=`, because records created during the walk are legitimately ahead.
  The checkpoint is saved only after promotion, or incremental sync would write into a generation
  nobody queries.

- **Tombstones are explicit.** A record missing from a page could be a deletion or a paging bug,
  and a service that guesses will quietly empty its own index.

- **`PriceMinor` is carried for sorting and range filters only.** It is catalog's list price at
  indexing time and is never presented as the price a customer pays — that is pricing's answer, at
  checkout, every time.

- **Typesense, not Elasticsearch.** Chosen on licence: Meilisearch is now `MIT AND BUSL-1.1` and
  its Enterprise parts require a commercial licence for production use, while Typesense is GPL-3.0
  across the whole engine and runs as a separate process, which places no obligation on this
  estate. Pin **`typesense-go/v3`** — `v4` is an alpha.

## Running, and what still is not built

The Dockerfile, CI (GitHub and the paired GitLab pipeline) and the compose entries all landed on
2026-09-22. `kinetix-search-migrate`, `kinetix-search-service` and `kinetix-search-sync` are in
`compose.yaml`; `kinetix-search-reindex` is in the `tools` profile so `docker compose up` never
starts it. Elasticsearch is gone from the estate in the same change.

One image carries all four binaries on purpose: they are the same code against the same contracts,
and a separate image for each is one more thing that can be a version behind the one serving
queries.

A fresh volume needs one bootstrap: `/health/ready` answers 503 with `index_unavailable` until a
generation has been promoted, so `docker compose run --rm kinetix-search-reindex all` is what makes
the container healthy the first time. That is the readiness check doing its job — a search service
that calls itself ready with no index answers every query "no products".

A third trap, found only in production: **never set `ServerName` on the credentials' tls.Config.**
grpc-go overwrites it from the `:authority` at handshake time, so it verifies nothing — and a
non-empty ServerName *becomes* the authority, which carries no port. order maps its gRPC route with
`RequireHost("*:50055")`, so a portless authority matched no endpoint and ASP.NET answered a plain
404 (`Unimplemented … unexpected HTTP status code received from server: 404`). catalog and identity
impose no host filter, so products and merchants synced while orders did not — a half-filled index
that looked like it worked. `mesh.MutualTLS` now leaves it unset, and there is no
"expected server name" setting at all: the handshake verifies the host being dialled, and a leaf
here carries several DNS names on purpose — catalog's covers both `kinetix-catalog-service` and
`kinetix-catalog-grpc`, and the endpoint decides which one is checked. An assertion that the host
must equal a configured name was added and removed the same day: it refused
`kinetix-catalog-grpc:50058` against a default of `kinetix-catalog-service` and crash-looped syncd
in production.

Two traps the image found, both invisible on a laptop:

- **Debian's protoc is 3.21.12 and does not bundle the well-known types.** Homebrew's does, so
  `bin/sync-contracts` worked here and failed in the image with
  `google/protobuf/timestamp.proto: File not found`, which reads like a broken contract. The script
  now adds `/usr/include` to the proto path when the files are there, and the image installs
  `libprotobuf-dev` to put them there.
- **Typesense will not create its `--data-dir`.** The compose entry mounts `typesense_data:/data`,
  and without it the container exits rather than starting empty.

Still not built: the storefront has no gateway route to `/api/v1/products/search` — the HTTP edge
is reachable from inside the mesh and from nowhere else — and production carries no image digest
for this service yet, so `docker compose config` refuses to render prod until the first
`build-and-push` fills `deploy/prod/versions.env`.

All four contract changes are published: `catalog/v1` with `ChangedSince` (and catalog's first gRPC
server), `search/v1`, `identity.v1.MerchantsChangedSince` and `order.v1.OrdersChangedSince`, pinned
here at **v1.0.19**.

## Three sources, two of which cannot count

`search.Source[D]` is the walk. Counting is a **separate** interface, `search.Counter`, because only
catalog serves a total:

- **catalog** implements both. A reindex of products is checked against catalog's own count — a walk
  that ends early is caught.
- **identity** and **order** implement `Source` only. A reindex of those collections is checked
  against what the walk sent, which catches an engine that dropped documents but not a source that
  stopped short. The log line says which check ran (`counted_against`), so a weaker guarantee is
  never mistaken for the strong one.

The alternative was to have those clients return a count they had invented, which is the estate's
house failure mode wearing a number.

Two more things are deliberate:

- **An order with no buyer is refused, not indexed.** Privacy here is a filter on the buyer, so a
  record with no buyer would sit in the index reachable by text alone. An order with no *merchant*
  is kept: orders placed before order recorded its seller carry none, and the buyer can still find
  their own order. Unknown stays unknown rather than becoming an id nobody owns.
- **A merchant that may not sell is still indexed**, carrying `status` and `may_sell`. Deciding
  which shops a customer may find is this service's job, not identity's; identity reports standing
  and search filters on it. `removed_principal_ids` is for a merchant that ceases to exist, and
  identity has no such path today, so that list is honestly always empty.
