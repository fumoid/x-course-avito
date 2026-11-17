-- +goose Up
-- +goose NO TRANSACTION
CREATE INDEX CONCURRENTLY trips_active_stale_idx
ON trips (COALESCE(last_position_at, started_at), id)
WHERE status = 'active';

-- +goose Down
-- +goose NO TRANSACTION
DROP INDEX CONCURRENTLY IF EXISTS trips_active_stale_idx;
