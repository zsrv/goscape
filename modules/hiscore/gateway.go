package hiscore

import (
	"net/http"
	"sync"
	"time"
)

// caller is who we believe sent a request. It is used for log lines and
// for keying the backstop limiter — never for authorization. Nothing in
// this module grants anything on the strength of these fields, which is
// what makes trusting the gateway optional.
type caller struct {
	Consumer  string // gateway consumer name; "" when unknown
	Anonymous bool
	IP        string
}

// Kong sets these on the upstream request after key-auth runs.
const (
	hdrConsumerUsername  = "X-Consumer-Username"
	hdrAnonymousConsumer = "X-Anonymous-Consumer"
)

// consumerFromHeaders reads the gateway's identity headers. When trust
// is false the headers are ignored entirely and every caller is
// anonymous — the safe default, since an unverified header should not
// reach a log line and be mistaken for an identity.
func consumerFromHeaders(r *http.Request, trust bool) caller {
	if !trust {
		return caller{Anonymous: true}
	}
	if r.Header.Get(hdrAnonymousConsumer) == "true" {
		return caller{Anonymous: true}
	}
	name := r.Header.Get(hdrConsumerUsername)
	return caller{Consumer: name, Anonymous: name == ""}
}

// limiterKey buckets by gateway consumer when we have one, else by
// client IP. Consumer-keyed buckets are only reachable when gateway
// headers are trusted; otherwise every caller keys by IP.
func (c caller) limiterKey() string {
	if c.Consumer != "" {
		return "consumer:" + c.Consumer
	}
	return "ip:" + c.IP
}

// backstop is a coarse fixed-window limiter for the case where the
// module is reached without a gateway in front of it. It is not the
// quota system — Kong's per-consumer rate-limiting is — so a fixed
// window (rather than a token bucket) is deliberate: it is cheap,
// allocation-free per request, and precise enough for a safety net.
type backstop struct {
	perMinute int

	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	start time.Time
	count int
}

func newBackstop(perMinute int) *backstop {
	return &backstop{perMinute: perMinute, windows: make(map[string]*window)}
}

// allow reports whether this request fits in the caller's current
// window. A perMinute of 0 disables the limiter entirely.
func (b *backstop) allow(key string, now time.Time) bool {
	if b.perMinute <= 0 {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Evict windows that rolled over, so a long-lived process does not
	// accumulate an entry per distinct caller forever.
	for k, w := range b.windows {
		if now.Sub(w.start) >= time.Minute {
			delete(b.windows, k)
		}
	}

	w, ok := b.windows[key]
	if !ok {
		b.windows[key] = &window{start: now, count: 1}
		return true
	}
	if w.count >= b.perMinute {
		return false
	}
	w.count++
	return true
}
