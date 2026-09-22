FROM golang:1.26-bookworm@sha256:a688600ca24f8a4d3ca77f95b0dd40704a9fc787c826660eb7ba0b641b8b175d AS build

# The wire contracts live in kinetix-contracts, not here: bin/sync-contracts fetches them at a
# pinned commit and generates the Go stubs. libprotobuf-dev is not decoration: Debian's protoc is
# 3.21.12 and reads google/protobuf/*.proto from /usr/include rather than from inside itself, so
# without it every contract that imports Timestamp fails to compile here while building fine on a
# laptop whose Homebrew protoc bundles them. `contracts/` and `internal/contractgen/` are both
# gitignored, so a fresh clone has neither and the build must produce them — which is why protoc
# and both plugins are installed here rather than assumed to be on somebody's laptop.
RUN apt-get update \
    && apt-get install -y --no-install-recommends protobuf-compiler libprotobuf-dev git ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && protoc --version

# Pinned to the versions go.mod already resolves, so the generated code and the runtime library
# are the same generation. A floating @latest here is how a stub starts calling an API the vendored
# library does not have.
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN sh bin/sync-contracts \
    && test -f internal/contractgen/search/v1/search.pb.go

# The hexagon is an import rule, and a rule nothing runs is a comment. This fails the build if
# anything outside the standard library has crept into internal/search.
RUN sh bin/check-core-purity

# CGO off so the binaries are static and the runtime image needs no toolchain. Symbols and DWARF
# stripped: they are of no use in a container that is debugged from the outside.
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/searchd ./cmd/searchd \
    && go build -trimpath -ldflags="-s -w" -o /out/syncd ./cmd/syncd \
    && go build -trimpath -ldflags="-s -w" -o /out/reindex ./cmd/reindex \
    && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251 AS final

# curl is here for the compose healthcheck, which probes /health/ready over HTTP. ca-certificates
# is here because every outbound call this service makes is TLS.
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --no-create-home search

COPY --from=build /out/searchd /usr/local/bin/searchd
COPY --from=build /out/syncd /usr/local/bin/syncd
COPY --from=build /out/reindex /usr/local/bin/reindex
COPY --from=build /out/migrate /usr/local/bin/migrate

ENV SEARCH_HTTP_ADDR=:8088 \
    SEARCH_GRPC_ADDR=:50058

EXPOSE 8088 50058

USER search

HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8088/health/ready > /dev/null || exit 1

# searchd is the service. syncd, reindex and migrate are in the same image on purpose: they are
# the same code against the same contracts, and a separate image for each is one more thing that
# can be a version behind the one serving queries.
ENTRYPOINT ["searchd"]
