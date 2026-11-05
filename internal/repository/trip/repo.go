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

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/infra/postgres/transaction"
)

const driverBusyIndex = "trips_one_active_per_driver_idx"

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type Repository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func New(pool *pgxpool.Pool, queryTimeout time.Duration) *Repository {
	return &Repository{pool: pool, queryTimeout: queryTimeout}
}

func (r *Repository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.queryTimeout)
}

func (r *Repository) Save(ctx context.Context, t domain.Trip) error {
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
			t.Price, string(t.Status), t.StartedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip: %w", err)
	}

	if _, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		if constraintName(err) == driverBusyIndex {
			return domain.ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

func (r *Repository) SaveStatusHistory(
	ctx context.Context,
	tripID uuid.UUID,
	from *domain.Status,
	to domain.Status,
	reason string,
	changedAt time.Time,
) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	var fromValue any
	if from != nil {
		fromValue = string(*from)
	}

	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(tripID, fromValue, string(to), reason, changedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status history: %w", err)
	}
	if _, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status history: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Select(
		"id", "user_id", "driver_id",
		"start_latitude", "start_longitude",
		"end_latitude", "end_longitude",
		"price", "status", "started_at", "finished_at",
	).From("trips").Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	var t domain.Trip
	var status string
	err = transaction.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.StartLat, &t.StartLon,
		&t.EndLat, &t.EndLon,
		&t.Price, &status, &t.StartedAt, &t.FinishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, domain.ErrTripNotFound
	}
	if err != nil {
		return domain.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	t.Status = domain.Status(status)
	return t, nil
}

func (r *Repository) UpdateFinished(ctx context.Context, t domain.Trip) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Update("trips").
		Set("status", string(t.Status)).
		Set("finished_at", t.FinishedAt).
		Set("updated_at", t.FinishedAt).
		Where(sq.Eq{"id": t.ID, "status": string(domain.StatusActive)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build finish trip: %w", err)
	}

	tag, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("finish trip: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	existing, getErr := r.Get(ctx, t.ID)
	if getErr != nil {
		return getErr
	}
	if existing.Status == domain.StatusCompleted {
		return domain.ErrTripCompleted
	}
	return domain.ErrTripNotFound
}

func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Select("COUNT(*)").
		From("trips").
		Where(sq.Eq{"status": string(domain.StatusActive)}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips: %w", err)
	}

	var count int64
	if err := transaction.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
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
	if _, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	return nil
}

func (r *Repository) FindIdempotency(ctx context.Context, key uuid.UUID, now time.Time) (*domain.IdempotencyRecord, error) {
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

	var rec domain.IdempotencyRecord
	err = transaction.Executor(ctx, r.pool).QueryRow(ctx, query, args...).Scan(
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
	if _, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("delete expired idempotency key: %w", err)
	}
	return nil
}

func (r *Repository) SaveIdempotency(ctx context.Context, rec domain.IdempotencyRecord) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	query, args, err := psql.Insert("idempotency_keys").
		Columns("key", "request_hash", "trip_id", "expires_at").
		Values(rec.Key, rec.RequestHash, rec.TripID, rec.ExpiresAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert idempotency key: %w", err)
	}
	if _, err := transaction.Executor(ctx, r.pool).Exec(ctx, query, args...); err != nil {
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
