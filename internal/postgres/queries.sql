-- name: LoadCheckpoint :one
SELECT updated_through, last_id, saved_at FROM sync_checkpoint WHERE collection = $1;

-- name: SaveCheckpoint :exec
INSERT INTO sync_checkpoint (collection, updated_through, last_id, saved_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (collection) DO UPDATE
   SET updated_through = EXCLUDED.updated_through,
       last_id         = EXCLUDED.last_id,
       saved_at        = EXCLUDED.saved_at;

-- name: RecordGeneration :exec
INSERT INTO index_generation (collection, name, documents)
VALUES ($1, $2, $3)
ON CONFLICT (collection, name) DO UPDATE SET documents = EXCLUDED.documents;

-- name: MarkGenerationPromoted :exec
UPDATE index_generation SET promoted_at = now()
WHERE collection = $1 AND name = $2;
