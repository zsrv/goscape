package world

import (
	"fmt"
	"sync"
	"time"
)

// Login-attempt rate-limit windows. Mirror the TS TTLCache constructions
// at World.ts:176-177 @2e3bcf43:
//
//	loginAddressAttempts: TTLCache<string, number> = new TTLCache({ ttl: 60000 });
//	loginDeviceAttempts:  TTLCache<string, number> = new TTLCache({ ttl: 15000 });
//
// The TTLs are TS-hardcoded (not Environment-configurable); only the
// thresholds (NODE_RATELIMIT_ADDRESS_LOGIN / NODE_RATELIMIT_DEVICE_LOGIN)
// are config-driven — goscape mirrors that split (config.go).
const (
	loginAddressAttemptTTL = 60 * time.Second
	loginDeviceAttemptTTL  = 15 * time.Second
)

// ttlAttemptPurgeThreshold bounds ttlAttemptCache memory: once the map
// holds this many entries, bump() sweeps expired entries before
// inserting. Go-side hygiene only — TS TTLCache evicts via its own
// internal timer wheel; the observable counting semantics are identical.
const ttlAttemptPurgeThreshold = 1024

type ttlAttemptEntry struct {
	count   int
	expires time.Time
}

// ttlAttemptCache is a minimal wall-clock TTL counter map backing the
// login rate limits. It models exactly the subset of isaacs/ttlcache
// semantics the TS code path exercises (World.ts:2107-2110 / 2173-2176
// @2e3bcf43):
//
//   - get(key) returns nothing once the entry's TTL has elapsed
//     (wall-clock, per-entry — NOT tick-time);
//   - set(key, v) (re-)arms the FULL default TTL on every write, so the
//     window slides from the LAST attempt, not the first.
//
// The zero value is ready to use (map allocated lazily under mu).
// Safe for concurrent use — handleLogin runs on per-connection
// goroutines. nowFn is a test seam; nil means time.Now.
type ttlAttemptCache struct {
	mu      sync.Mutex
	entries map[string]ttlAttemptEntry
	nowFn   func() time.Time
}

// bump records one attempt for key and returns the post-increment count,
// mirroring the TS attempt pattern verbatim:
//
//	const last = cache.get(key);
//	const attempts = last ? last + 1 : 1;
//	cache.set(key, attempts);
//
// The count resets to 1 once ttl elapses since the previous attempt
// (sliding window — every set re-arms the full TTL). Note the increment
// happens unconditionally, BEFORE any threshold comparison: rejected
// attempts keep the window armed, exactly as in TS.
func (c *ttlAttemptCache) bump(key string, ttl time.Duration) int {
	now := time.Now()
	if c.nowFn != nil {
		now = c.nowFn()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]ttlAttemptEntry)
	} else if len(c.entries) >= ttlAttemptPurgeThreshold {
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
	}
	attempts := 1
	if e, ok := c.entries[key]; ok && now.Before(e.expires) {
		attempts = e.count + 1
	}
	c.entries[key] = ttlAttemptEntry{count: attempts, expires: now.Add(ttl)}
	return attempts
}

// deviceLoginLimited records one op-16/18 login attempt for this
// connection's (uid, remote IP) pair and reports whether the attempt
// must be rejected. Port of the TS per-device limit (World.ts:2172-2184
// @2e3bcf43):
//
//	if (Environment.NODE_PRODUCTION && Environment.NODE_RATELIMIT_DEVICE_LOGIN > 0) {
//	    const last = this.loginDeviceAttempts.get(`${uid}@${client.remoteAddress}`);
//	    const attempts = last ? last + 1 : 1;
//	    this.loginDeviceAttempts.set(`${uid}@${client.remoteAddress}`, attempts);
//	    if (attempts >= Environment.NODE_RATELIMIT_DEVICE_LOGIN) {
//	        client.send(Uint8Array.from([16]));
//	        client.close();
//	        return;
//	    }
//	}
//
// Gated on production mode + a positive threshold; increments on EVERY
// attempt (before the >= comparison, so rejected attempts keep the 15s
// window armed). Key is `${uid}@${remoteAddress}` (World.ts:2173); TS
// reads the uid signed (g4s, World.ts:2168), so the Go uint32 is cast
// through int32 for an identical key rendering. The caller sends reply
// byte 16 + closes when this returns true.
func (c *client) deviceLoginLimited(uid uint32) bool {
	s := c.server
	if s == nil || !s.cfg.NodeProduction || s.cfg.NodeRatelimitDeviceLogin <= 0 {
		return false
	}
	host, _ := splitHostPort(c.conn.RemoteAddr().String())
	key := fmt.Sprintf("%d@%s", int32(uid), host)
	return s.loginDeviceAttempts.bump(key, loginDeviceAttemptTTL) >= s.cfg.NodeRatelimitDeviceLogin
}
