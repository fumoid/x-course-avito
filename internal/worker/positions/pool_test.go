package positions_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	"github.com/fumoid/x-course-avito/internal/worker/positions"
)

type manualTicker struct {
	ch chan time.Time
}

func newManualTicker() *manualTicker {
	return &manualTicker{ch: make(chan time.Time)}
}

func (t *manualTicker) C() <-chan time.Time { return t.ch }
func (t *manualTicker) Stop()               {}

func (t *manualTicker) tick() {
	t.ch <- time.Unix(1, 0).UTC()
}

type metricSink struct {
	stale    sync.Map
	requests sync.Map
	rows     chan int64
	reqDone  chan string
}

func newMetricSink() *metricSink {
	return &metricSink{
		rows:    make(chan int64, 8),
		reqDone: make(chan string, 1024),
	}
}

func (s *metricSink) add(m *sync.Map, key string) {
	actual, _ := m.LoadOrStore(key, new(atomic.Int64))
	actual.(*atomic.Int64).Add(1)
}

func (s *metricSink) get(m *sync.Map, key string) int64 {
	actual, ok := m.Load(key)
	if !ok {
		return 0
	}
	return actual.(*atomic.Int64).Load()
}

func attachMetrics(ctrl *gomock.Controller) (*metricSink, *Mockmetrics) {
	sink := newMetricSink()
	metrics := NewMockmetrics(ctrl)
	metrics.EXPECT().StaleCheck(gomock.Any(), gomock.Any()).AnyTimes().Do(func(_ context.Context, result string) {
		sink.add(&sink.stale, result)
	})
	metrics.EXPECT().StaleChecksRowsRead(gomock.Any(), gomock.Any()).AnyTimes().Do(func(_ context.Context, rows int64) {
		sink.rows <- rows
	})
	metrics.EXPECT().PositionRequest(gomock.Any(), gomock.Any()).AnyTimes().Do(func(_ context.Context, result string) {
		sink.add(&sink.requests, result)
		select {
		case sink.reqDone <- result:
		default:
		}
	})
	metrics.EXPECT().PositionRequestDuration(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	return sink, metrics
}

func waitRequest(t *testing.T, sink *metricSink, want string) {
	t.Helper()
	for {
		select {
		case got := <-sink.reqDone:
			if got == want {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for position request result %s", want)
			return
		}
	}
}

func waitRows(t *testing.T, sink *metricSink) int64 {
	t.Helper()
	select {
	case n := <-sink.rows:
		return n
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scanner tick")
		return 0
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func releaseGate() (chan struct{}, func()) {
	ch := make(chan struct{})
	var once sync.Once
	return ch, func() { once.Do(func() { close(ch) }) }
}

func stale(id uuid.UUID, silent time.Time) domain.StaleTrip {
	return domain.StaleTrip{ID: id, DriverID: uuid.New(), SilentSince: silent}
}

func startPool(
	t *testing.T,
	cfg positions.Config,
	source *MockstaleSource,
	pusher *Mockpusher,
	tick *manualTicker,
	metrics *Mockmetrics,
) *positions.Pool {
	t.Helper()
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = time.Second
	}
	if cfg.StaleAfter == 0 {
		cfg.StaleAfter = time.Minute
	}
	if cfg.Workers == 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 10
	}
	if cfg.ScanBatchSize == 0 {
		cfg.ScanBatchSize = 100
	}
	if cfg.PushTimeout == 0 {
		cfg.PushTimeout = time.Second
	}
	pool := positions.New(cfg, source, pusher, tick, metrics, testLogger(), noop.NewTracerProvider().Tracer("test"))
	pool.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := pool.Stop(ctx); err != nil {
			t.Errorf("stop: %v", err)
		}
	})
	return pool
}

func TestDuplicateNotEnqueuedTwice(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	tripID := uuid.New()
	item := stale(tripID, time.Unix(1, 0).UTC())
	entered := make(chan struct{})
	release, closeRelease := releaseGate()
	defer closeRelease()
	source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.StaleTrip{item}, nil).Times(2)
	pusher.EXPECT().RequestPosition(gomock.Any(), tripID, item.DriverID).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			close(entered)
			<-release
			return nil
		},
	)

	startPool(t, positions.Config{Workers: 1, QueueSize: 10, ScanBatchSize: 100}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	<-entered
	tick.tick()
	waitRows(t, sink)
	if sink.get(&sink.stale, "duplicate") != 1 {
		t.Fatalf("duplicate = %d", sink.get(&sink.stale, "duplicate"))
	}
	closeRelease()
}

func TestCancelSkipsPush(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	blockerID := uuid.New()
	targetID := uuid.New()
	blocker := stale(blockerID, time.Unix(1, 0).UTC())
	target := stale(targetID, time.Unix(2, 0).UTC())
	entered := make(chan struct{})
	release, closeRelease := releaseGate()
	defer closeRelease()

	gomock.InOrder(
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.StaleTrip{blocker}, nil),
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.StaleTrip{target}, nil),
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), blockerID, blocker.DriverID).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			close(entered)
			<-release
			return nil
		},
	)

	pool := startPool(t, positions.Config{Workers: 1, QueueSize: 10, ScanBatchSize: 100}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	<-entered
	tick.tick()
	waitRows(t, sink)
	pool.Cancel(context.Background(), targetID)
	waitRequest(t, sink, "cancelled")
	closeRelease()
}

func TestOverflowDropsAndDoesNotBlock(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	blocker := stale(uuid.New(), time.Unix(0, 0).UTC())
	trips := make([]domain.StaleTrip, 3)
	base := time.Unix(10, 0).UTC()
	for i := range trips {
		trips[i] = stale(uuid.New(), base.Add(time.Duration(i)*time.Second))
	}
	entered := make(chan struct{})
	release, closeRelease := releaseGate()
	defer closeRelease()
	gomock.InOrder(
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.StaleTrip{blocker}, nil),
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(trips, nil),
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), blocker.ID, blocker.DriverID).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			close(entered)
			<-release
			return nil
		},
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	startPool(t, positions.Config{Workers: 1, QueueSize: 1, ScanBatchSize: 100}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	<-entered
	tick.tick()
	waitRows(t, sink)
	if sink.get(&sink.stale, "dropped") < 1 {
		t.Fatalf("dropped = %d", sink.get(&sink.stale, "dropped"))
	}
	closeRelease()
}

func TestWorkerPoolBound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	const n = 8
	const workers = 4
	trips := make([]domain.StaleTrip, n)
	for i := range trips {
		trips[i] = stale(uuid.New(), time.Unix(int64(i+1), 0).UTC())
	}
	source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(trips, nil)
	source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	var concurrent atomic.Int64
	var max atomic.Int64
	started := make(chan struct{}, n)
	done := make(chan struct{}, n)
	gate := make(chan struct{})
	pusher.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Times(n).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			cur := concurrent.Add(1)
			for {
				old := max.Load()
				if cur <= old || max.CompareAndSwap(old, cur) {
					break
				}
			}
			started <- struct{}{}
			<-gate
			concurrent.Add(-1)
			done <- struct{}{}
			return nil
		},
	)

	startPool(t, positions.Config{Workers: workers, QueueSize: n, ScanBatchSize: n}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	for i := 0; i < workers; i++ {
		<-started
	}
	close(gate)
	for i := 0; i < n; i++ {
		<-done
	}
	if max.Load() > int64(workers) || max.Load() < 1 {
		t.Fatalf("max concurrent = %d, workers = %d", max.Load(), workers)
	}
}

func TestStopLeavesNoGoroutines(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	_, metrics := attachMetrics(ctrl)
	tick := newManualTicker()
	source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	startPool(t, positions.Config{Workers: 2, QueueSize: 4, ScanBatchSize: 10}, source, pusher, tick, metrics)
}

func TestPushErrorDoesNotKillWorker(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	first := stale(uuid.New(), time.Unix(1, 0).UTC())
	second := stale(uuid.New(), time.Unix(2, 0).UTC())
	source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.StaleTrip{first, second}, nil)
	done := make(chan uuid.UUID, 2)
	gomock.InOrder(
		pusher.EXPECT().RequestPosition(gomock.Any(), first.ID, first.DriverID).DoAndReturn(
			func(context.Context, uuid.UUID, uuid.UUID) error {
				done <- first.ID
				return errors.New("push down")
			},
		),
		pusher.EXPECT().RequestPosition(gomock.Any(), second.ID, second.DriverID).DoAndReturn(
			func(context.Context, uuid.UUID, uuid.UUID) error {
				done <- second.ID
				return nil
			},
		),
	)

	startPool(t, positions.Config{Workers: 1, QueueSize: 10, ScanBatchSize: 10}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	if got := <-done; got != first.ID {
		t.Fatalf("first = %s", got)
	}
	if got := <-done; got != second.ID {
		t.Fatalf("second = %s", got)
	}
	waitRequest(t, sink, "error")
	waitRequest(t, sink, "success")
}

func TestBatchScanAllFit(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	blocker := stale(uuid.New(), time.Unix(0, 0).UTC())
	trips := make([]domain.StaleTrip, 250)
	base := time.Unix(100, 0).UTC()
	for i := range trips {
		trips[i] = stale(uuid.New(), base.Add(time.Duration(i)*time.Millisecond))
	}
	entered := make(chan struct{})
	release, closeRelease := releaseGate()
	defer closeRelease()
	gomock.InOrder(
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), 100, nil).Return([]domain.StaleTrip{blocker}, nil),
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), 100, nil).Return(trips[:100], nil),
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), 100, gomock.Any()).Return(trips[100:200], nil),
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), 100, gomock.Any()).Return(trips[200:], nil),
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), blocker.ID, blocker.DriverID).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			close(entered)
			<-release
			return nil
		},
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	startPool(t, positions.Config{Workers: 1, QueueSize: 300, ScanBatchSize: 100}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	<-entered
	tick.tick()
	if rows := waitRows(t, sink); rows != 250 {
		t.Fatalf("rows = %d", rows)
	}
	if sink.get(&sink.stale, "queued") != 251 {
		t.Fatalf("queued = %d", sink.get(&sink.stale, "queued"))
	}
	closeRelease()
}

func TestBatchScanStopsOnDropped(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	blocker := stale(uuid.New(), time.Unix(0, 0).UTC())
	trips := make([]domain.StaleTrip, 100)
	base := time.Unix(100, 0).UTC()
	for i := range trips {
		trips[i] = stale(uuid.New(), base.Add(time.Duration(i)*time.Millisecond))
	}
	entered := make(chan struct{})
	release, closeRelease := releaseGate()
	defer closeRelease()
	gomock.InOrder(
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), 100, nil).Return([]domain.StaleTrip{blocker}, nil),
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), 100, nil).Return(trips, nil),
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), blocker.ID, blocker.DriverID).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			close(entered)
			<-release
			return nil
		},
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	startPool(t, positions.Config{Workers: 1, QueueSize: 50, ScanBatchSize: 100}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	<-entered
	tick.tick()
	waitRows(t, sink)
	if sink.get(&sink.stale, "dropped") < 1 {
		t.Fatalf("dropped = %d", sink.get(&sink.stale, "dropped"))
	}
	if sink.get(&sink.stale, "queued") != 51 {
		t.Fatalf("queued = %d", sink.get(&sink.stale, "queued"))
	}
	closeRelease()
}

func TestHeapOrderAndEviction(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	sink, metrics := attachMetrics(ctrl)
	tick := newManualTicker()

	blocker := stale(uuid.New(), time.Unix(0, 0).UTC())
	newest := stale(uuid.New(), time.Unix(30, 0).UTC())
	middle := stale(uuid.New(), time.Unix(20, 0).UTC())
	oldest := stale(uuid.New(), time.Unix(10, 0).UTC())
	entered := make(chan struct{})
	release, closeRelease := releaseGate()
	defer closeRelease()
	order := make(chan uuid.UUID, 4)

	gomock.InOrder(
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.StaleTrip{blocker}, nil),
		source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.StaleTrip{newest, middle, oldest}, nil),
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), blocker.ID, blocker.DriverID).DoAndReturn(
		func(context.Context, uuid.UUID, uuid.UUID) error {
			close(entered)
			<-release
			order <- blocker.ID
			return nil
		},
	)
	pusher.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(_ context.Context, tripID, _ uuid.UUID) error {
			order <- tripID
			return nil
		},
	)

	startPool(t, positions.Config{Workers: 1, QueueSize: 2, ScanBatchSize: 10}, source, pusher, tick, metrics)
	tick.tick()
	waitRows(t, sink)
	<-entered
	tick.tick()
	waitRows(t, sink)
	if sink.get(&sink.stale, "dropped") != 1 {
		t.Fatalf("dropped = %d", sink.get(&sink.stale, "dropped"))
	}
	closeRelease()
	if got := <-order; got != blocker.ID {
		t.Fatalf("first = %s", got)
	}
	if got := <-order; got != oldest.ID {
		t.Fatalf("second = %s want oldest", got)
	}
	if got := <-order; got != middle.ID {
		t.Fatalf("third = %s want middle", got)
	}
}

func TestCancelContextStopsPool(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	source := NewMockstaleSource(ctrl)
	pusher := NewMockpusher(ctrl)
	_, metrics := attachMetrics(ctrl)
	tick := newManualTicker()
	source.EXPECT().ListStale(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	pool := positions.New(
		positions.Config{Workers: 2, QueueSize: 4, ScanBatchSize: 10, CheckInterval: time.Second, StaleAfter: time.Minute, PushTimeout: time.Second},
		source, pusher, tick, metrics, testLogger(), noop.NewTracerProvider().Tracer("test"),
	)
	pool.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pool.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
