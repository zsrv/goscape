package world

import (
	"strconv"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/util/ttlcache"
)

// newRateLimitTestServer builds a bare *Server with only the fields the
// rate-limit helpers touch (cfg, log, addressLoginCache, deviceLoginCache),
// so tests don't need the full NewServer pipeline.
func newRateLimitTestServer(addressLimit, deviceLimit int, production bool, addressTTL, deviceTTL time.Duration) *Server {
	return &Server{
		cfg: Config{
			NodeProduction:            production,
			NodeRateLimitAddressLogin: addressLimit,
			NodeRateLimitDeviceLogin:  deviceLimit,
		},
		log:               discardLogger(),
		addressLoginCache: ttlcache.New[string, int](addressTTL),
		deviceLoginCache:  ttlcache.New[string, int](deviceTTL),
	}
}

func TestAddressLoginRateLimit_UnderLimitAllowed(t *testing.T) {
	s := newRateLimitTestServer(3 /*addressLimit*/, 0, true, time.Minute, time.Minute)
	addr := "203.0.113.7:54321"

	// 2 attempts under limit=3 (rejection fires on attempts>=limit, i.e. 3rd attempt).
	for i := 1; i <= 2; i++ {
		if s.addressLoginRateLimitExceeded(addr) {
			t.Fatalf("attempt %d under limit=3 should be allowed", i)
		}
	}
}

func TestAddressLoginRateLimit_OverLimitRejected(t *testing.T) {
	s := newRateLimitTestServer(3, 0, true, time.Minute, time.Minute)
	addr := "203.0.113.7:54321"

	// Attempts 1,2 allowed; 3rd attempt triggers rejection (attempts>=limit).
	_ = s.addressLoginRateLimitExceeded(addr)
	_ = s.addressLoginRateLimitExceeded(addr)
	if !s.addressLoginRateLimitExceeded(addr) {
		t.Fatalf("3rd attempt at limit=3 should be rejected")
	}
}

func TestAddressLoginRateLimit_TTLExpiryReleases(t *testing.T) {
	// Short TTL so we don't have to inject a clock through helpers.
	s := newRateLimitTestServer(2, 0, true, 50*time.Millisecond, time.Minute)
	addr := "203.0.113.7:54321"

	_ = s.addressLoginRateLimitExceeded(addr) // attempts=1
	if !s.addressLoginRateLimitExceeded(addr) {
		t.Fatalf("2nd attempt at limit=2 should be rejected")
	}

	time.Sleep(60 * time.Millisecond)

	// TTL window has elapsed; counter resets. 1st attempt after reset is allowed again.
	if s.addressLoginRateLimitExceeded(addr) {
		t.Fatalf("attempt after TTL window should be allowed again")
	}
}

func TestAddressLoginRateLimit_GateDisabledWhenNotProduction(t *testing.T) {
	s := newRateLimitTestServer(1, 0, false /*production off*/, time.Minute, time.Minute)
	addr := "203.0.113.7:54321"
	// Even with limit=1 and many attempts, gate stays open when production=false.
	for i := range 10 {
		if s.addressLoginRateLimitExceeded(addr) {
			t.Fatalf("attempt %d should be allowed when production=false", i)
		}
	}
}

func TestAddressLoginRateLimit_GateDisabledWhenLimitZero(t *testing.T) {
	s := newRateLimitTestServer(0 /*limit=0 disables*/, 0, true, time.Minute, time.Minute)
	addr := "203.0.113.7:54321"
	for i := range 10 {
		if s.addressLoginRateLimitExceeded(addr) {
			t.Fatalf("attempt %d should be allowed when limit=0", i)
		}
	}
}

func TestAddressLoginRateLimit_KeysOnBareIPNotPort(t *testing.T) {
	// Two clients from the same IP but different ports must share the bucket.
	s := newRateLimitTestServer(2, 0, true, time.Minute, time.Minute)

	if s.addressLoginRateLimitExceeded("198.51.100.1:11111") {
		t.Fatalf("attempt 1 from :11111 should be allowed")
	}
	// Same IP, different port: counted in the same bucket → 2nd attempt rejects.
	if !s.addressLoginRateLimitExceeded("198.51.100.1:22222") {
		t.Fatalf("attempt 2 from same IP different port should hit the same bucket and reject")
	}
}

func TestAddressLoginRateLimit_DifferentIPsIndependent(t *testing.T) {
	s := newRateLimitTestServer(2, 0, true, time.Minute, time.Minute)

	// Saturate IP A.
	_ = s.addressLoginRateLimitExceeded("198.51.100.1:11111")
	if !s.addressLoginRateLimitExceeded("198.51.100.1:11112") {
		t.Fatalf("IP A should be at limit")
	}

	// IP B is unaffected.
	if s.addressLoginRateLimitExceeded("203.0.113.50:11111") {
		t.Fatalf("IP B should be allowed while IP A is throttled")
	}
}

func TestDeviceLoginRateLimit_UnderLimitAllowed(t *testing.T) {
	s := newRateLimitTestServer(0, 3, true, time.Minute, time.Minute)
	addr := "203.0.113.7:54321"
	const uid uint32 = 0xdeadbeef
	for i := 1; i <= 2; i++ {
		if s.deviceLoginRateLimitExceeded(uid, addr) {
			t.Fatalf("device attempt %d under limit=3 should be allowed", i)
		}
	}
}

func TestDeviceLoginRateLimit_OverLimitRejected(t *testing.T) {
	s := newRateLimitTestServer(0, 3, true, time.Minute, time.Minute)
	addr := "203.0.113.7:54321"
	const uid uint32 = 0xdeadbeef
	_ = s.deviceLoginRateLimitExceeded(uid, addr)
	_ = s.deviceLoginRateLimitExceeded(uid, addr)
	if !s.deviceLoginRateLimitExceeded(uid, addr) {
		t.Fatalf("3rd device attempt at limit=3 should be rejected")
	}
}

func TestDeviceLoginRateLimit_TTLExpiryReleases(t *testing.T) {
	s := newRateLimitTestServer(0, 2, true, time.Minute, 50*time.Millisecond)
	addr := "203.0.113.7:54321"
	const uid uint32 = 0x1234

	_ = s.deviceLoginRateLimitExceeded(uid, addr)
	if !s.deviceLoginRateLimitExceeded(uid, addr) {
		t.Fatalf("2nd device attempt at limit=2 should be rejected")
	}

	time.Sleep(60 * time.Millisecond)

	if s.deviceLoginRateLimitExceeded(uid, addr) {
		t.Fatalf("device attempt after TTL window should be allowed again")
	}
}

func TestDeviceLoginRateLimit_KeysOnUidPlusIP(t *testing.T) {
	// Same IP, different uid → independent buckets.
	s := newRateLimitTestServer(0, 2, true, time.Minute, time.Minute)
	addr := "203.0.113.7:54321"

	_ = s.deviceLoginRateLimitExceeded(1, addr)
	if !s.deviceLoginRateLimitExceeded(1, addr) {
		t.Fatalf("uid=1 should be at limit")
	}

	// Different uid from same IP still allowed.
	if s.deviceLoginRateLimitExceeded(2, addr) {
		t.Fatalf("uid=2 from same IP should be in its own bucket")
	}

	// Different IP, same uid → also independent.
	if s.deviceLoginRateLimitExceeded(1, "192.0.2.99:5555") {
		t.Fatalf("uid=1 from a different IP should be in its own bucket")
	}
}

func TestDeviceLoginRateLimit_GateDisabledWhenNotProduction(t *testing.T) {
	s := newRateLimitTestServer(0, 1, false, time.Minute, time.Minute)
	for i := range 10 {
		if s.deviceLoginRateLimitExceeded(7, "203.0.113.7:5") {
			t.Fatalf("device attempt %d should be allowed when production=false", i)
		}
	}
}

func TestDeviceLoginRateLimit_GateDisabledWhenLimitZero(t *testing.T) {
	s := newRateLimitTestServer(0, 0, true, time.Minute, time.Minute)
	for i := range 10 {
		if s.deviceLoginRateLimitExceeded(7, "203.0.113.7:5") {
			t.Fatalf("device attempt %d should be allowed when device-limit=0", i)
		}
	}
}

func TestBareHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"127.0.0.1:34567", "127.0.0.1"},
		{"[::1]:80", "::1"},
		{"192.0.2.5:5", "192.0.2.5"},
		// Malformed (no port) — returns input as-is.
		{"not-an-addr", "not-an-addr"},
	}
	for _, tc := range cases {
		if got := bareHost(tc.in); got != tc.want {
			t.Errorf("bareHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Sanity check: the cache key format matches what TS uses
// (`${uid}@${remoteAddress}`) so logs and any future shared-store
// migrations stay aligned.
func TestDeviceLoginRateLimit_KeyFormatMatchesTS(t *testing.T) {
	s := newRateLimitTestServer(0, 999, true, time.Minute, time.Minute)
	_ = s.deviceLoginRateLimitExceeded(42, "203.0.113.7:54321")
	wantKey := strconv.FormatUint(42, 10) + "@" + "203.0.113.7"
	if _, ok := s.deviceLoginCache.Get(wantKey); !ok {
		t.Fatalf("expected device cache to be keyed as %q", wantKey)
	}
}
