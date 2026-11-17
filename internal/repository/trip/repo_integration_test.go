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

func TestRepository_ListStale(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	staleTrip := newActiveTrip(uuid.New())
	staleTrip.StartedAt = now.Add(-2 * time.Minute)
	freshTrip := newActiveTrip(uuid.New())
	freshTrip.StartedAt = now.Add(-2 * time.Minute)
	completed := newActiveTrip(uuid.New())
	completed.StartedAt = now.Add(-2 * time.Minute)
	noCoords := newActiveTrip(uuid.New())
	noCoords.StartedAt = now.Add(-2 * time.Minute)

	ids := []uuid.UUID{staleTrip.ID, freshTrip.ID, completed.ID, noCoords.ID}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", id)
		}
	})

	if err := repo.Save(ctx, staleTrip); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, freshTrip); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, completed); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, noCoords); err != nil {
		t.Fatal(err)
	}

	at := now.Add(-time.Second)
	if err := completed.Finish(at); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateFinished(ctx, completed); err != nil {
		t.Fatal(err)
	}

	freshPos := domain.Position{TripID: freshTrip.ID, Latitude: 59.9, Longitude: 30.3, RecordedAt: now.Add(-time.Second)}
	if _, err := repo.SavePosition(ctx, freshPos); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateLastPositionAt(ctx, freshTrip.ID, freshPos.RecordedAt); err != nil {
		t.Fatal(err)
	}

	stalePos := domain.Position{TripID: staleTrip.ID, Latitude: 59.9, Longitude: 30.3, RecordedAt: now.Add(-2 * time.Minute)}
	if _, err := repo.SavePosition(ctx, stalePos); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateLastPositionAt(ctx, staleTrip.ID, stalePos.RecordedAt); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListStale(ctx, time.Minute, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[uuid.UUID]bool{}
	for _, trip := range got {
		found[trip.ID] = true
	}
	if !found[staleTrip.ID] {
		t.Fatal("stale trip with old coordinates must be selected")
	}
	if !found[noCoords.ID] {
		t.Fatal("active trip without coordinates must be selected")
	}
	if found[freshTrip.ID] {
		t.Fatal("fresh trip must not be selected")
	}
	if found[completed.ID] {
		t.Fatal("completed trip must not be selected")
	}
}

func TestRepository_SavePositionAtomic(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	txm := transaction.NewManager(pool, 3*time.Second)
	ctx := context.Background()
	entity := newActiveTrip(uuid.New())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", entity.ID)
	})
	if err := repo.Save(ctx, entity); err != nil {
		t.Fatal(err)
	}

	recorded := time.Now().UTC().Truncate(time.Microsecond)
	err := txm.Do(ctx, func(ctx context.Context) error {
		if _, err := repo.SavePosition(ctx, domain.Position{
			TripID: entity.ID, Latitude: 59.9, Longitude: 30.3, RecordedAt: recorded,
		}); err != nil {
			return err
		}
		if err := repo.UpdateLastPositionAt(ctx, entity.ID, recorded); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}

	got, err := repo.Get(ctx, entity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastPositionAt != nil {
		t.Fatalf("last_position_at = %v", got.LastPositionAt)
	}
	positions, err := repo.ListPositions(ctx, entity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 0 {
		t.Fatalf("positions after rollback: %d", len(positions))
	}

	newer := recorded
	older := recorded.Add(-time.Minute)
	if err := txm.Do(ctx, func(ctx context.Context) error {
		saved, err := repo.SavePosition(ctx, domain.Position{
			TripID: entity.ID, Latitude: 59.9, Longitude: 30.3, RecordedAt: newer,
		})
		if err != nil {
			return err
		}
		if saved.ID == 0 {
			t.Fatal("expected generated position id")
		}
		return repo.UpdateLastPositionAt(ctx, entity.ID, newer)
	}); err != nil {
		t.Fatal(err)
	}
	if err := txm.Do(ctx, func(ctx context.Context) error {
		if _, err := repo.SavePosition(ctx, domain.Position{
			TripID: entity.ID, Latitude: 59.8, Longitude: 30.2, RecordedAt: older,
		}); err != nil {
			return err
		}
		return repo.UpdateLastPositionAt(ctx, entity.ID, older)
	}); err != nil {
		t.Fatal(err)
	}

	got, err = repo.Get(ctx, entity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastPositionAt == nil || !got.LastPositionAt.Equal(newer) && !got.LastPositionAt.Truncate(time.Microsecond).Equal(newer) {
		if got.LastPositionAt == nil || got.LastPositionAt.UTC().Unix() != newer.Unix() {
			t.Fatalf("last_position_at = %v want %v", got.LastPositionAt, newer)
		}
	}
	positions, err = repo.ListPositions(ctx, entity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 2 {
		t.Fatalf("positions = %d", len(positions))
	}
	if !positions[0].RecordedAt.Before(positions[1].RecordedAt) && positions[0].ID > positions[1].ID {
		t.Fatalf("positions are not sorted: %+v", positions)
	}
}

func TestRepository_ListStaleKeyset(t *testing.T) {
	pool := openPool(t)
	repo := triprepo.New(pool, 3*time.Second)
	ctx := context.Background()
	started := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	const n = 5
	ids := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		entity := newActiveTrip(uuid.New())
		entity.StartedAt = started
		ids[i] = entity.ID
		if err := repo.Save(ctx, entity); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", id)
		}
	})

	var (
		all    []domain.StaleTrip
		cursor *domain.StaleCursor
		seen   = map[uuid.UUID]int{}
	)
	for {
		batch, err := repo.ListStale(ctx, time.Minute, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		for _, trip := range batch {
			seen[trip.ID]++
		}
		last := batch[len(batch)-1]
		cursor = &domain.StaleCursor{SilentSince: last.SilentSince, ID: last.ID}
	}

	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("trip %s seen %d times", id, seen[id])
		}
	}
	if len(all) < n {
		t.Fatalf("got %d stale trips, want at least %d", len(all), n)
	}
}
