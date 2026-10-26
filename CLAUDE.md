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
internal/wiring/        the dig providers; the only package besides cmd/ that imports dig

tests/search/           tests for internal/search
tests/querying/         tests for internal/querying
tests/indexing/         tests for internal/indexing
tests/typesense/        tests for the adapter's pure parts, and what it sends a fake engine
tests/integration/      the only tests that talk to a real engine
tests/wiring/           resolves every dig graph, so wrong wiring fails here and not at start-up
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
- **The house layout is enforced, not remembered** (the user's rule for every repository, brought
  here from the notary on 2026-10-21). A call, composite literal or signature that does not fit on
  one line puts every element on its own line, the closing bracket alone; one that fits stays on
  one line. A blank line between contexts, and an assignment cuddled with the check that reads it.
  Three tools hold it: `golines` at 120 columns and `wsl_v5`, both in `.golangci.yml`
  (`golangci-lint fmt` and `golangci-lint run --fix` repair them), and `bin/check-layout`, which
  runs `tools/arglayout` (`go run ./tools/arglayout -fix ./cmd ./internal ./tests ./tools`
  rewrites). It is the notary's, plus one change: it skips a file carrying Go's `Code generated …
  DO NOT EDIT.` header, because sqlc writes `internal/postgres/queries.sql.go` beside hand-written
  code and its next `sqlc generate` would undo any reflow. Both CIs run it in the lint job. No tool joins a wrapped
  call that would fit on one line; that one is still kept by hand.
- **`bin/check-boundaries`** fails when anything outside `cmd/` and `internal/wiring` imports dig.
  Controlled both ways: a dig import in `internal/system` fails it.
- **`bin/test`** runs the suite with the coverage flags the `tests/` layout requires.
- **The generated stubs are committed, and CI proves them current.** `internal/contractgen` is in git since
  2026-10-26, at the user's choice, because Dependabot runs `go mod tidy` on a bare clone and cannot run
  `bin/sync-contracts`. The purity job regenerates it with protoc 36.2 pinned by release and checksum (apt's
  version is written into every file and changes with the runner) and fails when the tree differs. The
  `.proto` files stay out: `contracts/` is ignored.

## Conventions

- **Errors:** every error out of the core is built with `search.Errf` and carries a `Kind`. Read it
  back with `search.KindOf`, which survives wrapping. Only an adapter turns a Kind into an HTTP
  status or a gRPC code. An error from outside this service has no Kind and must not be given one.
- **DI:** `go.uber.org/dig`, as in the notary. Every provider lives in `internal/wiring`, takes one
  `dig.In` struct and returns a value or a `dig.Out` struct; a `dig.Out` field typed as a port
  (`search.Checkpoint`, `search.Source[D]`) stands where `dig.As` would. Constructors everywhere else
  stay plain functions, so adapters are tested without a container. `wiring.Searchd` and
  `wiring.Indexing` build the graphs; `cmd/searchd` and `cmd/syncd` invoke one `dig.In` struct each,
  and `cmd/reindex` invokes one collection at a time, so reindexing products dials catalog and
  nothing else. dig resolves at start-up, not at compile time, which is why `tests/wiring` resolves
  every graph, and a failure is reported through `dig.RootCause` so its Kind survives. `cmd/migrate`
  has nothing to inject and stays plain.
- **Testing:** hand-written fakes in `tests/searchtest`, not a mocking framework. A fake that
  returns what the test set and records what it was asked is shorter to read than a stack of
  expectations.
- **One type per file** in `internal/search`, where a type is the unit of meaning. Relaxed in
  adapters, where Go's convention of naming a file for a concept reads better.
- **Naming:** no stutter. `search.Document`, not `search.SearchDocument`.

## Things decided here, and why

- **Three collections, three document types** — `ProductDoc`, `MerchantDoc`, `OrderDoc`. They are
  ranked by different rules, filtered on different fields, and one of them is private.

- **Orders are private.** Today no edge serves them at all: searchd builds a `Finder` for products
  only, and neither the HTTP router nor the gRPC server names orders or merchants. When an edge is
  added, two layers are already in place: the use case refuses an `Orders` query with no
  `OnBehalfOf`, and `ForPrincipal` is a method rather than a `QueryOption`, so a handler cannot let
  a query parameter set it. A scoped Typesense key with the principal baked in was planned as a
  third layer and does not exist; this file claimed it did until 2026-10-14. `OrderDoc` carries no
  address, no total and no payment detail.

- **A failed search is a failure, never an empty result set.** A shop that answers "no products"
  during an outage teaches its customers the shop is empty. This is the estate's house failure
  mode, and `TestFindFailsRatherThanReturningNothing` is the gate.

- **Freshness travels with every result.** `indexed_through`, `age_seconds`, `stale`. The caller is
  told how current the answer is rather than left to assume — and the two facts are different
  questions. `indexed_through` is the source's own watermark, the newest record indexed; `stale` is
  measured from `sync_checkpoint.saved_at`, which syncd writes on every completed run including an
  empty one. Measured from the record instead, a catalogue nobody edits for a week reports itself
  stale for ever while the sync is confirming every thirty seconds that it is caught up — which is
  what production did on 2026-09-23, a healthy index telling every caller it could not be trusted.
  Before the first completed sync there is no `saved_at` and nothing to measure: `Freshness.Synced`
  is false, `stale` is true, and HTTP answers `indexed_through_ms` and `age_seconds` as `null`
  rather than ages measured from year one (−62135596800000 ms, about 9.2e9 s, until 2026-10-21).
  A sync over an empty source has an age but no watermark, so `indexed_through_ms` alone is `null`.
  `search/v1` cannot say null for `age_seconds`; gRPC leaves `indexed_through` unset and sends 0
  with `stale: true`.

- **Bad input is refused, never replaced, and the rules live once, in the core.** `search.NewQuery`
  and `search.NewSuggestQuery` serve both edges and both operations. A limit of 0 (absent, or
  proto3's unset) takes the default (24 for search, 10 for suggest), above 100 it is capped at 100,
  and a negative one is `malformed_query`; a negative offset is too, and HTTP refuses any `limit`
  or `offset` that is not a whole number. An unknown `sort` (or gRPC ranking) is refused. Text,
  prefix and each category are trimmed and held to the same length and control-byte rules; an
  empty category is dropped, because Typesense answers a filter on an empty category with **200
  and zero hits** — the empty shop again, not an error. An empty prefix suggests nothing without asking
  the engine. Until 2026-10-21 suggest skipped the text rules and reset a limit over 100 to 10.

- **The offset is sent as an offset.** The searcher asks Typesense for `offset` and `limit`,
  never `page` and `per_page`: `page = offset/limit + 1` rounded every offset down to a multiple
  of the limit, so `offset=50&limit=24` answered from the 49th hit.

- **An engine refusal is the caller's fault; an engine failure is not.** Typesense 400 and 422
  from a search or a suggestion are `malformed_query` (400 / `InvalidArgument`): a request over the
  engine's 4000-character query string (a few hundred categories) is the caller's. 404 (no collection behind the alias), 401, 5xx and transport
  errors stay `index_unavailable`. A 400 can also mean the adapter named a field the schema lacks,
  which would then read as the caller's fault, so the engine tests exercise every filter and sort
  field the products searcher names; an edge for merchants or orders needs the same.
  Indexing keeps every engine error as `index_unavailable`. `typesense.Connect` refuses a zero or
  negative timeout rather than choosing one; config supplies it (`SEARCH_TYPESENSE_TIMEOUT_MS`).

- **A filter matches its value exactly, apart from case.** Typesense's `:=` on a string field
  compares tokens, and by default it drops punctuation from both sides: `c1` matched `c1]`, `(c1`,
  `!c1` and `c1*`, and `Pakaian (Pria)` matched `pakaian pria` (measured on 30.2, 2026-10-21). It
  was never the backtick quoting. Every filter field (`categories`, `merchant_id`, `buyer`) is
  therefore created with field-level `symbols_to_index` holding all ASCII punctuation **except the
  backtick**: with the backtick indexed too, the scalar form (`merchant_id:=` and a quoted value)
  matches nothing.
  Three values cannot be matched exactly however they are written, so the adapter refuses them as
  `malformed_query` before asking: one holding a backtick or a backslash (a backslash escapes the
  closing quote), and one ending in `*` (a prefix wildcard inside `[...]`). Case still folds, which
  the engine does not let a filter change; principals are UUIDs, so the orders scope is unaffected.
  The cost: text search on `categories` now sees punctuation, so `pria` no longer finds
  `Pakaian (Pria)` through that field (titles still carry it). A schema change reaches only a
  generation built after it: the live index keeps the old behaviour until `reindex products` and
  `reindex merchants` run.

- **The cursor is saved only after the write lands.** A cursor saved first skips exactly the page
  that failed, and the gap is never noticed.

- **The cursor is a transaction, never a time.** Every source used to page by `(updated_at, key)`,
  and `updated_at` is written before the transaction commits: a record whose transaction committed
  after a later one's was behind the cursor by the time it was visible, and was never indexed. Since
  2026-10-19 `search.Cursor` carries `TransactionID`, every request names one (zero at the start),
  and catalog, identity and order answer in `(transaction, key)` order below the oldest transaction
  still running. A source that answers without `transaction_id` predates contracts 1.5.0 and is
  refused as unavailable: paging it by time would skip again, quietly. `sync_checkpoint` keeps the
  transaction (`00002`); a checkpoint saved before it has none and starts its collection's walk again
  from zero over the live generation, which re-applies upserts and tombstones and loses nothing.
  `updated_through` stays as the freshness watermark only, and is never moved back.

- **A reindex builds a new generation and promotes it only if it is not short of the source's
  count.** `<` rather than `!=`, because records created during the walk are legitimately ahead.
  The checkpoint is saved only after promotion, or incremental sync would write into a generation
  nobody queries.

- **Tombstones are explicit.** A record missing from a page could be a deletion or a paging bug,
  and a service that guesses will quietly empty its own index.

- **A sync pass and a reindex's promotion never overlap.** `search.Lock` is a Postgres advisory lock
  per collection, taken with `pg_advisory_xact_lock` inside a transaction, so it is released when the
  transaction ends or the session dies — a crashed holder cannot keep a collection for ever. A sync
  pass holds it for the whole pass; a reindex holds it only to promote and save its cursor, so sync
  keeps running during the walk. Until 2026-10-14 there was no lock: a pass that read the old
  generation could write into it after the promotion and save a cursor past the reindex's, and
  everything between the two — a deactivated product's tombstone included — was never written to the
  live generation.

- **One bad record is refused, not allowed to stop the feed.** A record a client cannot convert (an
  order with no buyer, a product with no usable SKU, a merchant with no principal) goes into
  `Changes.Refused`; the rest of the page is written, the cursor moves past it, and syncd logs it at
  ERROR with its id. Until 2026-10-14 the whole page failed instead, and because the cursor never
  moved, every pass failed on the same record and the collection stopped syncing for good. A reindex
  subtracts refused records from the source's count, or catalog's count of a record nobody can index
  would block every promotion.

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

**The image declares no HEALTHCHECK, and that is deliberate.** It carries four binaries and only
searchd serves HTTP, so an image-level probe against `/health/ready` is answered by one of them and
inherited by the rest: syncd was reported unhealthy forty seconds after every start and failed the
deploy while working perfectly. compose declares the probe on the container that can answer it, and
states `healthcheck: disable: true` on the sync worker. Giving syncd a real liveness signal is worth
doing and has not been done.

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

Three traps the image found, all invisible on a laptop:

- **Debian's protoc is 3.21.12 and does not bundle the well-known types.** Homebrew's does, so
  `bin/sync-contracts` worked here and failed in the image with
  `google/protobuf/timestamp.proto: File not found`, which reads like a broken contract. The script
  now adds `/usr/include` to the proto path when the files are there, and the image installs
  `libprotobuf-dev` to put them there.
- **Typesense will not create its `--data-dir`.** The compose entry mounts `typesense_data:/data`,
  and without it the container exits rather than starting empty.
- **A base pinned by digest never receives Debian's security updates.** The merge build of 3dec6f5
  (2026-10-14) failed its Trivy scan on three fixed CRITICALs, all `perl-base` at
  `5.36.0-7+deb12u3`. The final stage now runs `apt-get upgrade` before `install`, as pricing,
  notary, payment, review, matching and warehouse already do, and the image carries `deb12u4`. The
  digest still decides the release; the upgrade only lets a fix in. Because the scan runs before the
  push, no image exists for 3dec6f5, so a release tag on that commit fails at "no image" — tag the
  merge that carries this fix instead.

Kong routes `/api/v1/products/search` and `/api/v1/products/suggest` to searchd, so the product
search is public. Nothing may call the gRPC surface yet: `KINETIX_GRPC_ALLOWED_PEERS` is empty in
compose, and an empty list permits nobody.

All four contract changes are published: `catalog/v1` with `ChangedSince` (and catalog's first gRPC
server), `search/v1`, `identity.v1.MerchantsChangedSince` and `order.v1.OrdersChangedSince`, pinned
here at **v1.5.0**, the first version with the transaction cursor. Search ships only after catalog, identity
and order serve it.

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
