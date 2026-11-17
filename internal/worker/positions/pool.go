package positions

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
)

const (
	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
	resultSuccess   = "success"
	resultError     = "error"
	resultCancelled = "cancelled"
	spanStaleCheck  = "stale.check"
	spanPositionReq = "position.request"
)

type Config struct {
	CheckInterval time.Duration
	StaleAfter    time.Duration
	Workers       int
	QueueSize     int
	ScanBatchSize int
	PushTimeout   time.Duration
}

type realTicker struct {
	*time.Ticker
}

func NewTicker(d time.Duration) *realTicker {
	return &realTicker{Ticker: time.NewTicker(d)}
}

func (t *realTicker) C() <-chan time.Time {
	return t.Ticker.C
}

type Pool struct {
	cfg     Config
	source  staleSource
	pusher  pusher
	ticker  ticker
	metrics metrics
	log     logger
	tracer  trace.Tracer

	queue  *queue
	active atomic.Int64

	scanCancel context.CancelFunc
	scanDone   chan struct{}
	workersWg  sync.WaitGroup
}

func New(
	cfg Config,
	source staleSource,
	pusher pusher,
	tick ticker,
	metrics metrics,
	log logger,
	tracer trace.Tracer,
) *Pool {
	return &Pool{
		cfg:      cfg,
		source:   source,
		pusher:   pusher,
		ticker:   tick,
		metrics:  metrics,
		log:      log,
		tracer:   tracer,
		queue:    newQueue(cfg.QueueSize),
		scanDone: make(chan struct{}),
	}
}

func (p *Pool) Start() {
	scanCtx, cancel := context.WithCancel(context.Background())
	p.scanCancel = cancel
	p.log.InfoContext(scanCtx, "position scanner started")
	p.log.InfoContext(scanCtx, "position workers started", "workers", p.cfg.Workers)
	go p.scanLoop(scanCtx)
	for i := 0; i < p.cfg.Workers; i++ {
		p.workersWg.Add(1)
		go p.workerLoop()
	}
}

func (p *Pool) Stop(ctx context.Context) error {
	p.log.InfoContext(ctx, "position scanner stopping")
	if p.scanCancel != nil {
		p.scanCancel()
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	select {
	case <-p.scanDone:
	case <-ctx.Done():
		p.queue.close()
		return ctx.Err()
	}

	p.log.InfoContext(ctx, "position workers draining")
	p.queue.close()
	done := make(chan struct{})
	go func() {
		p.workersWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		p.log.InfoContext(ctx, "position workers stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) Cancel(ctx context.Context, tripID uuid.UUID) {
	if p.queue.cancel(tripID) {
		p.metrics.PositionRequest(ctx, resultCancelled)
	}
}

func (p *Pool) QueueSize() int64 {
	return int64(p.queue.len())
}

func (p *Pool) ActiveWorkers() int64 {
	return p.active.Load()
}

func (p *Pool) scanLoop(ctx context.Context) {
	defer close(p.scanDone)
	defer p.log.InfoContext(context.Background(), "position scanner stopped")
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-p.ticker.C():
			if !ok || ctx.Err() != nil {
				return
			}
			p.scanTick(ctx)
		}
	}
}

func (p *Pool) scanTick(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, spanStaleCheck)
	defer span.End()

	var (
		cursor   *domain.StaleCursor
		rowsRead int64
		stop     bool
	)
	for !stop {
		if ctx.Err() != nil {
			break
		}
		batch, err := p.source.ListStale(ctx, p.cfg.StaleAfter, p.cfg.ScanBatchSize, cursor)
		if err != nil {
			p.log.ErrorContext(ctx, "stale trip scan failed", "error", err)
			break
		}
		if len(batch) == 0 {
			break
		}
		rowsRead += int64(len(batch))
		spanCtx := span.SpanContext()
		for i := range batch {
			if p.enqueue(ctx, batch[i], spanCtx) {
				stop = true
				break
			}
		}
		if stop || len(batch) < p.cfg.ScanBatchSize {
			break
		}
		last := batch[len(batch)-1]
		cursor = &domain.StaleCursor{SilentSince: last.SilentSince, ID: last.ID}
	}
	p.metrics.StaleChecksRowsRead(ctx, rowsRead)
}

func (p *Pool) enqueue(ctx context.Context, trip domain.StaleTrip, spanCtx trace.SpanContext) (dropped bool) {
	item := &queueItem{
		tripID:      trip.ID,
		driverID:    trip.DriverID,
		silentSince: trip.SilentSince,
		spanCtx:     spanCtx,
		index:       -1,
	}
	result, evicted := p.queue.tryEnqueue(item)
	switch result {
	case enqueueQueued:
		p.metrics.StaleCheck(ctx, resultQueued)
	case enqueueDuplicate:
		p.metrics.StaleCheck(ctx, resultDuplicate)
	case enqueueDropped:
		p.metrics.StaleCheck(ctx, resultDropped)
		p.log.WarnContext(ctx, "stale trip dropped from queue",
			"trip_id", trip.ID.String(),
			"reason", "queue_full",
		)
		return true
	case enqueueReplaced:
		p.metrics.StaleCheck(ctx, resultQueued)
		p.metrics.StaleCheck(ctx, resultDropped)
		droppedID := trip.ID
		if evicted != nil {
			droppedID = evicted.tripID
		}
		p.log.WarnContext(ctx, "stale trip dropped from queue",
			"trip_id", droppedID.String(),
			"reason", "evicted",
		)
		return true
	}
	return false
}

func (p *Pool) workerLoop() {
	defer p.workersWg.Done()
	for {
		item, ok := p.queue.pop()
		if !ok {
			return
		}
		p.active.Add(1)
		p.handle(item)
		p.active.Add(-1)
	}
}

func (p *Pool) handle(item *queueItem) {
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.PushTimeout)
	defer cancel()

	opts := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindInternal)}
	if item.spanCtx.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: item.spanCtx}))
	}
	ctx, span := p.tracer.Start(ctx, spanPositionReq, opts...)
	defer span.End()

	if !p.queue.contains(item.tripID) {
		p.metrics.PositionRequest(ctx, resultCancelled)
		return
	}

	start := time.Now()
	err := p.pusher.RequestPosition(ctx, item.tripID, item.driverID)
	seconds := time.Since(start).Seconds()
	p.queue.done(item)
	if err != nil {
		p.metrics.PositionRequest(ctx, resultError)
		p.metrics.PositionRequestDuration(ctx, seconds, resultError)
		p.log.WarnContext(ctx, "push position request failed",
			"trip_id", item.tripID.String(),
			"error", err,
		)
		return
	}
	p.metrics.PositionRequest(ctx, resultSuccess)
	p.metrics.PositionRequestDuration(ctx, seconds, resultSuccess)
}
