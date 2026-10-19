-- name: LoadCheckpoint :one
SELECT transaction_id, updated_through, last_id, saved_at FROM sync_checkpoint WHERE collection = $1;

-- name: SaveCheckpoint :exec
INSERT INTO sync_checkpoint (collection, transaction_id, updated_through, last_id, saved_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (collection) DO UPDATE
   SET transaction_id  = EXCLUDED.transaction_id,
       updated_through = GREATEST(sync_checkpoint.updated_through, EXCLUDED.updated_through),
       last_id         = EXCLUDED.last_id,
       saved_at        = EXCLUDED.saved_at;

-- name: RecordGeneration :exec
INSERT INTO index_generation (collection, name, documents)
VALUES ($1, $2, $3)
ON CONFLICT (collection, name) DO UPDATE SET documents = EXCLUDED.documents;

-- name: MarkGenerationPromoted :exec
UPDATE index_generation SET promoted_at = now()
WHERE collection = $1 AND name = $2;

-- name: HoldCollection :exec
SELECT pg_advisory_xact_lock(hashtext('kinetix-search.collection'), hashtext(sqlc.arg(collection)::text));
