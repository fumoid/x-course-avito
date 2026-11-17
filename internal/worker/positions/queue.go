package positions

import (
	"bytes"
	"container/heap"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type enqueueResult int

const (
	enqueueQueued enqueueResult = iota
	enqueueDuplicate
	enqueueDropped
	enqueueReplaced
)

type queueItem struct {
	tripID      uuid.UUID
	driverID    uuid.UUID
	silentSince time.Time
	spanCtx     trace.SpanContext
	index       int
}

type priorityHeap []*queueItem

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	if h[i].silentSince.Equal(h[j].silentSince) {
		return bytes.Compare(h[i].tripID[:], h[j].tripID[:]) < 0
	}
	return h[i].silentSince.Before(h[j].silentSince)
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x any) {
	item := x.(*queueItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *priorityHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[:n-1]
	return item
}

func newerThan(a, b *queueItem) bool {
	if a.silentSince.Equal(b.silentSince) {
		return bytes.Compare(a.tripID[:], b.tripID[:]) > 0
	}
	return a.silentSince.After(b.silentSince)
}

func silentLonger(item, fresh *queueItem) bool {
	if item.silentSince.Equal(fresh.silentSince) {
		return bytes.Compare(item.tripID[:], fresh.tripID[:]) < 0
	}
	return item.silentSince.Before(fresh.silentSince)
}

type queue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  map[uuid.UUID]*queueItem
	heap   priorityHeap
	cap    int
	closed bool
}

func newQueue(capacity int) *queue {
	q := &queue{
		items: make(map[uuid.UUID]*queueItem, capacity),
		heap:  make(priorityHeap, 0, capacity),
		cap:   capacity,
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *queue) tryEnqueue(item *queueItem) (enqueueResult, *queueItem) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return enqueueDropped, item
	}
	if _, exists := q.items[item.tripID]; exists {
		return enqueueDuplicate, nil
	}

	if len(q.heap) < q.cap {
		heap.Push(&q.heap, item)
		q.items[item.tripID] = item
		q.cond.Signal()
		return enqueueQueued, nil
	}

	fresh := q.freshestLocked()
	if silentLonger(item, fresh) {
		heap.Remove(&q.heap, fresh.index)
		delete(q.items, fresh.tripID)
		heap.Push(&q.heap, item)
		q.items[item.tripID] = item
		q.cond.Signal()
		return enqueueReplaced, fresh
	}
	return enqueueDropped, item
}

func (q *queue) freshestLocked() *queueItem {
	var fresh *queueItem
	for _, item := range q.heap {
		if fresh == nil || newerThan(item, fresh) {
			fresh = item
		}
	}
	return fresh
}

func (q *queue) pop() (*queueItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.heap) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.heap) == 0 {
		return nil, false
	}
	return heap.Pop(&q.heap).(*queueItem), true
}

func (q *queue) cancel(tripID uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.items[tripID]
	if !ok {
		return false
	}
	delete(q.items, tripID)
	if item.index >= 0 {
		heap.Remove(&q.heap, item.index)
		return true
	}
	return false
}

func (q *queue) contains(tripID uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.items[tripID]
	return ok
}

func (q *queue) done(item *queueItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.items[item.tripID] == item {
		delete(q.items, item.tripID)
	}
}

func (q *queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Broadcast()
}

func (q *queue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.heap)
}
