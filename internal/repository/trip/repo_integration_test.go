//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/infra/postgres/transaction"
	triprepo "github.com/fumoid/x-course-avito/internal/repository/trip"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

func newActiveTrip(id uuid.UUID) domain.Trip {
	return domain.Trip{
		ID:        id,
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		StartLat:  59.9,
		StartLon:  30.3,
		EndLat:    59.8,
		EndLon:    30.4,
		Price:     100,
		Status:    domain.StatusActive,
		StartedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func TestRepository_SaveAndGet(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	ctx := context.Background()
	entity := newActiveTrip(uuid.New())

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", entity.ID)
	})

	if err := repo.Save(ctx, entity); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, entity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != entity.ID || got.Status != domain.StatusActive {
		t.Fatalf("got %+v", got)
	}
}

func TestRepository_SaveStatusHistory(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	ctx := context.Background()
	entity := newActiveTrip(uuid.New())
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", entity.ID)
	})

	if err := repo.Save(ctx, entity); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveStatusHistory(ctx, entity.ID, nil, domain.StatusActive, domain.ReasonCreated, now); err != nil {
		t.Fatal(err)
	}

	var tripID uuid.UUID
	var from *string
	var to, reason string
	var changedAt time.Time
	err := pool.QueryRow(ctx, `
		SELECT trip_id, from_status, to_status, reason, changed_at
		FROM trip_status_history WHERE trip_id = $1`, entity.ID,
	).Scan(&tripID, &from, &to, &reason, &changedAt)
	if err != nil {
		t.Fatal(err)
	}
	if tripID != entity.ID || to != string(domain.StatusActive) || reason != domain.ReasonCreated {
		t.Fatalf("history trip=%s to=%s reason=%s", tripID, to, reason)
	}
	if from != nil {
		t.Fatalf("from = %v", from)
	}
	if !changedAt.Equal(now) && !changedAt.Truncate(time.Microsecond).Equal(now) {
		if changedAt.UTC().Unix() != now.Unix() {
			t.Fatalf("changed_at = %v want %v", changedAt, now)
		}
	}
}

func TestRepository_HistoryFailureRollsBackTrip(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	txm := transaction.NewManager(pool, 3*time.Second)
	ctx := context.Background()
	entity := newActiveTrip(uuid.New())

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", entity.ID)
	})

	err := txm.Do(ctx, func(ctx context.Context) error {
		if err := repo.Save(ctx, entity); err != nil {
			return err
		}
		return repo.SaveStatusHistory(ctx, entity.ID, nil, domain.Status("nope"), domain.ReasonCreated, time.Now().UTC())
	})
	if err == nil {
		t.Fatal("expected history insert to fail")
	}
	_, getErr := repo.Get(ctx, entity.ID)
	if !errors.Is(getErr, domain.ErrTripNotFound) {
		t.Fatalf("trip should be rolled back, get err = %v", getErr)
	}
}

func TestRepository_ConcurrentDriverBusy(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	ctx := context.Background()
	driverID := uuid.New()
	start := make(chan struct{})
	const n = 2
	ids := make([]uuid.UUID, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		ids[i] = uuid.New()
		entity := newActiveTrip(ids[i])
		entity.DriverID = driverID
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", entity.ID)
		})
		wg.Add(1)
		go func(i int, entity domain.Trip) {
			defer wg.Done()
			<-start
			errs[i] = repo.Save(ctx, entity)
		}(i, entity)
	}
	close(start)
	wg.Wait()

	ok, busy := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrDriverBusy):
			busy++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || busy != 1 {
		t.Fatalf("ok=%d busy=%d", ok, busy)
	}
}

func TestRepository_UpdateFinished(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	ctx := context.Background()
	entity := newActiveTrip(uuid.New())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", entity.ID)
	})
	if err := repo.Save(ctx, entity); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	if err := entity.Finish(at); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateFinished(ctx, entity); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, entity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusCompleted {
		t.Fatalf("status = %s", got.Status)
	}
	if err := repo.UpdateFinished(ctx, entity); !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("second finish: %v", err)
	}
}
