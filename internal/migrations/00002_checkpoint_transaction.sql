-- +goose Up
ALTER TABLE sync_checkpoint
    ADD COLUMN transaction_id BIGINT,
    ADD CONSTRAINT sync_checkpoint_transaction_unsigned CHECK (transaction_id >= 0);

-- +goose Down
ALTER TABLE sync_checkpoint
    DROP CONSTRAINT sync_checkpoint_transaction_unsigned,
    DROP COLUMN transaction_id;
