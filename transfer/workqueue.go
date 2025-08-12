package transfer

import (
	"container/heap"
	"math"
	"sync"
	"time"
)

// WorkQueue schedules chunk indices ensuring a bounded number of in-flight
// requests based on the bandwidth-delay product (BDP) and chunk size.
// It also requeues lost chunks with exponential backoff, prioritizing gaps
// near the head of the stream for early completion.
type WorkQueue struct {
	mu          sync.Mutex
	total       int
	next        int
	chunkSize   int64
	bdp         int64
	maxInFlight int
	inFlight    map[int]struct{}
	ready       intHeap
	delayed     delayHeap
	attempts    map[int]int
	baseBackoff time.Duration
	now         func() time.Time
}

// NewWorkQueue creates a WorkQueue for a stream of totalChunks where each
// chunk is chunkSize bytes and the estimated bandwidth-delay product is bdp.
// The queue keeps approximately ceil(bdp/chunkSize) chunks in flight. The
// now function may be nil to use time.Now; it exists to aid testing.
func NewWorkQueue(totalChunks int, chunkSize, bdp int64, now func() time.Time) *WorkQueue {
	if chunkSize <= 0 {
		chunkSize = 1
	}
	max := int(math.Ceil(float64(bdp) / float64(chunkSize)))
	if max < 1 {
		max = 1
	}
	if now == nil {
		now = time.Now
	}
	return &WorkQueue{
		total:       totalChunks,
		chunkSize:   chunkSize,
		bdp:         bdp,
		maxInFlight: max,
		inFlight:    make(map[int]struct{}),
		attempts:    make(map[int]int),
		baseBackoff: 10 * time.Millisecond,
		now:         now,
	}
}

// UpdateBDP updates the bandwidth-delay product and recomputes the
// concurrency bound.
func (q *WorkQueue) UpdateBDP(bdp int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.bdp = bdp
	max := int(math.Ceil(float64(bdp) / float64(q.chunkSize)))
	if max < 1 {
		max = 1
	}
	q.maxInFlight = max
}

// Next returns the next chunk index to process. The second return value is
// false when no work is currently available or the in-flight limit is reached.
func (q *WorkQueue) Next() (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.inFlight) >= q.maxInFlight {
		return 0, false
	}

	now := q.now()
	// Move ready delayed items to the ready heap.
	for q.delayed.Len() > 0 && !q.delayed[0].readyAt.After(now) {
		item := heap.Pop(&q.delayed).(delayedItem)
		heap.Push(&q.ready, item.index)
	}

	var idx int
	if q.ready.Len() > 0 {
		idx = heap.Pop(&q.ready).(int)
	} else if q.next < q.total {
		idx = q.next
		q.next++
	} else {
		return 0, false
	}

	q.inFlight[idx] = struct{}{}
	return idx, true
}

// Complete marks the given chunk index as finished.
func (q *WorkQueue) Complete(idx int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.inFlight, idx)
	delete(q.attempts, idx)
}

// Nack reports a failed chunk transfer. The chunk is requeued with
// exponential backoff prioritizing lower indices when it becomes available.
func (q *WorkQueue) Nack(idx int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.inFlight[idx]; !ok {
		return
	}
	delete(q.inFlight, idx)
	q.attempts[idx]++
	backoff := q.baseBackoff * time.Duration(1<<uint(q.attempts[idx]-1))
	item := delayedItem{index: idx, readyAt: q.now().Add(backoff)}
	heap.Push(&q.delayed, item)
}

// InFlight returns the number of chunks currently in flight.
func (q *WorkQueue) InFlight() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.inFlight)
}

// --- Heaps ---

type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *intHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

type delayedItem struct {
	index   int
	readyAt time.Time
}

type delayHeap []delayedItem

func (h delayHeap) Len() int           { return len(h) }
func (h delayHeap) Less(i, j int) bool { return h[i].readyAt.Before(h[j].readyAt) }
func (h delayHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *delayHeap) Push(x any)        { *h = append(*h, x.(delayedItem)) }
func (h *delayHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
