package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fumoid/x-course-avito/internal/tx"
)

const driverBusyIndex = "trips_one_active_per_driver_idx"

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (r *Repository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.queryTimeout)
}

func (r *Repository) Create(ctx context.Context, t Trip) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Insert("trips").
		Columns(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status", "started_at",
		).
		Values(
			t.ID, t.UserID, t.DriverID,
			t.StartLat, t.StartLon,
			t.EndLat, t.EndLon,
			t.Price, t.Status, t.StartedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip: %w", err)
	}

	if _, err := tx.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		if constraintName(err) == driverBusyIndex {
			return ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (r *Repository) InsertStatusHistory(ctx context.Context, tripID uuid.UUID, from *string, to, reason string) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, from, to, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status history: %w", err)
	}
	if _, err := tx.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status history: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Trip, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Select(
		"id", "user_id", "driver_id",
		"start_latitude", "start_longitude",
		"end_latitude", "end_longitude",
		"price", "status", "started_at", "finished_at",
	).From("trips").Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	var t Trip
	err = tx.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.StartLat, &t.StartLon,
		&t.EndLat, &t.EndLon,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, ErrTripNotFound
	}
	if err != nil {
		return Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return t, nil
}

func (r *Repository) FinishActive(ctx context.Context, id uuid.UUID, finishedAt time.Time) (Trip, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Update("trips").
		Set("status", StatusCompleted).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(sq.Eq{"id": id, "status": StatusActive}).
		Suffix("RETURNING id, user_id, driver_id, start_latitude, start_longitude, end_latitude, end_longitude, price, status, started_at, finished_at").
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build finish trip: %w", err)
	}

	var t Trip
	err = tx.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.StartLat, &t.StartLon,
		&t.EndLat, &t.EndLon,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := r.Get(ctx, id)
		if getErr != nil {
			return Trip{}, getErr
		}
		if existing.Status == StatusCompleted {
			return Trip{}, ErrTripCompleted
		}
		return Trip{}, ErrTripNotFound
	}
	if err != nil {
		return Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	return t, nil
}

func (r *Repository) LockIdempotencyKey(ctx context.Context, key uuid.UUID) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Select().
		Column(sq.Expr("pg_advisory_xact_lock(hashtext(?))", key.String())).
		ToSql()
	if err != nil {
		return fmt.Errorf("build advisory lock: %w", err)
	}
	if _, err := tx.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	return nil
}

func (r *Repository) FindIdempotency(ctx context.Context, key uuid.UUID, now time.Time) (*IdempotencyRecord, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Select("key", "request_hash", "trip_id", "expires_at").
		From("idempotency_keys").
		Where(sq.Eq{"key": key}).
		Where(sq.Gt{"expires_at": now}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select idempotency key: %w", err)
	}

	var rec IdempotencyRecord
	err = tx.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(
		&rec.Key, &rec.RequestHash, &rec.TripID, &rec.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select idempotency key: %w", err)
	}
	return &rec, nil
}

func (r *Repository) DeleteExpiredIdempotency(ctx context.Context, key uuid.UUID, now time.Time) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Delete("idempotency_keys").
		Where(sq.Eq{"key": key}).
		Where(sq.LtOrEq{"expires_at": now}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build delete expired idempotency key: %w", err)
	}
	if _, err := tx.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("delete expired idempotency key: %w", err)
	}
	return nil
}

func (r *Repository) SaveIdempotency(ctx context.Context, rec IdempotencyRecord) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Insert("idempotency_keys").
		Columns("key", "request_hash", "trip_id", "expires_at").
		Values(rec.Key, rec.RequestHash, rec.TripID, rec.ExpiresAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert idempotency key: %w", err)
	}
	if _, err := tx.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert idempotency key: %w", err)
	}
	return nil
}

func constraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}
