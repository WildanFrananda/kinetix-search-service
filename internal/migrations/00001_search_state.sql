-- +goose Up
CREATE TABLE sync_checkpoint (
    collection      TEXT        PRIMARY KEY,
    updated_through TIMESTAMPTZ NOT NULL,
    last_id         TEXT        NOT NULL,
    saved_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT sync_checkpoint_known_collection
        CHECK (collection IN ('products', 'merchants', 'orders'))
);

CREATE TABLE index_generation (
    collection  TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    built_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    promoted_at TIMESTAMPTZ,
    documents   BIGINT      NOT NULL DEFAULT 0,

    PRIMARY KEY (collection, name),

    CONSTRAINT index_generation_known_collection
        CHECK (collection IN ('products', 'merchants', 'orders'))
);

CREATE INDEX index_generation_live
    ON index_generation (collection, promoted_at DESC)
    WHERE promoted_at IS NOT NULL;

-- +goose Down
DROP TABLE index_generation;
DROP TABLE sync_checkpoint;
