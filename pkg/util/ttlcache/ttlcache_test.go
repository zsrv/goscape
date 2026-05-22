package ttlcache

import (
	"sync"
	"testing"
	"time"
)

// fakeClock returns a controllable time source for tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{t: start} }

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

func newTestCache(t *testing.T, ttl time.Duration) (*Cache[string, int], *fakeClock) {
	t.Helper()
	c := New[string, int](ttl)
	clk := newFakeClock(time.Unix(1_700_000_000, 0))
	c.setNowFunc(clk.Now)
	return c, clk
}

func TestGet_Miss(t *testing.T) {
	c, _ := newTestCache(t, time.Second)
	if v, ok := c.Get("absent"); ok || v != 0 {
		t.Fatalf("expected miss, got value=%d ok=%v", v, ok)
	}
}

func TestSet_Then_Get_HitsBeforeTTL(t *testing.T) {
	c, clk := newTestCache(t, time.Second)
	c.Set("k", 7)
	clk.Advance(500 * time.Millisecond)
	v, ok := c.Get("k")
	if !ok || v != 7 {
		t.Fatalf("expected (7, true), got (%d, %v)", v, ok)
	}
}

func TestGet_ExpiresAtExactlyTTL(t *testing.T) {
	// TS / @isaacs/ttlcache treats expiry as inclusive — entry expires at
	// exactly t0+ttl. We match: now>=expiresAt means evicted.
	c, clk := newTestCache(t, time.Second)
	c.Set("k", 1)
	clk.Advance(time.Second)
	if _, ok := c.Get("k"); ok {
		t.Fatalf("expected expiry at t0+ttl")
	}
	if got := c.Len(); got != 0 {
		t.Fatalf("expected lazy eviction to leave Len()=0, got %d", got)
	}
}

func TestGet_ExpiresPastTTL(t *testing.T) {
	c, clk := newTestCache(t, time.Second)
	c.Set("k", 1)
	clk.Advance(2 * time.Second)
	if v, ok := c.Get("k"); ok || v != 0 {
		t.Fatalf("expected expiry past TTL, got (%d, %v)", v, ok)
	}
}

func TestSet_ResetsTTLWindow(t *testing.T) {
	c, clk := newTestCache(t, time.Second)
	c.Set("k", 1)
	clk.Advance(900 * time.Millisecond)
	c.Set("k", 2) // fresh window starts here
	clk.Advance(900 * time.Millisecond)
	v, ok := c.Get("k")
	if !ok || v != 2 {
		t.Fatalf("expected (2, true) after window reset, got (%d, %v)", v, ok)
	}
}

func TestDelete(t *testing.T) {
	c, _ := newTestCache(t, time.Second)
	c.Set("k", 1)
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatalf("expected miss after Delete")
	}
}

func TestRateLimitPattern(t *testing.T) {
	// Mirrors the World.ts attempt-counting pattern:
	// last := Get; attempts := last+1; Set(attempts); reject if attempts>=limit.
	c, clk := newTestCache(t, 60*time.Second)
	limit := 3
	hit := func() (attempts int, exceeded bool) {
		last, _ := c.Get("ip")
		attempts = last + 1
		c.Set("ip", attempts)
		return attempts, attempts >= limit
	}

	if a, ex := hit(); a != 1 || ex {
		t.Fatalf("attempt 1: got (%d,%v) want (1,false)", a, ex)
	}
	if a, ex := hit(); a != 2 || ex {
		t.Fatalf("attempt 2: got (%d,%v) want (2,false)", a, ex)
	}
	if a, ex := hit(); a != 3 || !ex {
		t.Fatalf("attempt 3: got (%d,%v) want (3,true)", a, ex)
	}
	// Window passes — counter resets.
	clk.Advance(61 * time.Second)
	if a, ex := hit(); a != 1 || ex {
		t.Fatalf("post-TTL reset: got (%d,%v) want (1,false)", a, ex)
	}
}

func TestConcurrent_NoRace(t *testing.T) {
	c := New[int, int](time.Second)
	var wg sync.WaitGroup
	const workers = 16
	const iters = 500
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				k := (seed + i) % 32
				c.Set(k, i)
				_, _ = c.Get(k)
				if i%7 == 0 {
					c.Delete(k)
				}
			}
		}(w)
	}
	wg.Wait()
}
