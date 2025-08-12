package transfer

import (
	"testing"
	"time"
)

func TestWorkQueueBackoffPrioritizesHead(t *testing.T) {
	now := time.Unix(0, 0)
	q := NewWorkQueue(5, 1, 3, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		idx, ok := q.Next()
		if !ok || idx != i {
			t.Fatalf("expected index %d, got %d (ok=%v)", i, idx, ok)
		}
	}

	q.Nack(1)

	idx, ok := q.Next()
	if !ok || idx != 3 {
		t.Fatalf("expected index 3 after nack, got %d", idx)
	}

	q.Complete(0)
	idx, ok = q.Next()
	if !ok || idx != 4 {
		t.Fatalf("expected index 4, got %d", idx)
	}

	now = now.Add(20 * time.Millisecond)
	q.Complete(2)
	idx, ok = q.Next()
	if !ok || idx != 1 {
		t.Fatalf("expected requeued index 1, got %d", idx)
	}
}

func TestWorkQueueConcurrencyLimit(t *testing.T) {
	q := NewWorkQueue(10, 1, 4, nil)

	for i := 0; i < 4; i++ {
		idx, ok := q.Next()
		if !ok || idx != i {
			t.Fatalf("expected index %d, got %d (ok=%v)", i, idx, ok)
		}
	}

	if _, ok := q.Next(); ok {
		t.Fatalf("expected limit to block scheduling")
	}

	q.Complete(0)
	idx, ok := q.Next()
	if !ok || idx != 4 {
		t.Fatalf("expected index 4 after completion, got %d", idx)
	}
}
