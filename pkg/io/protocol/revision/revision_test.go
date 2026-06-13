package revision

import "testing"

// TestExpectedRevisionIs274 pins the rev-274 branch's wire revision.
// TS WorldConfig.ts:91 (dee467c8) sets the engine revision to 274;
// World.ts:2140-2143 rejects any other client revision with login reply 6.
// 274 no longer fits one byte, so the wire carries it as the 0xff escape
// marker followed by a u2 (World.ts:2136-2138) — see login/req.
// The predecessor test pinned 254 — the per-branch value, not a durable
// contract — and earlier 244 (PORTING-LESSONS §3: a test can pin a bug).
func TestExpectedRevisionIs274(t *testing.T) {
	if Expected != 274 {
		t.Fatalf("revision.Expected = %d, want 274", Expected)
	}
}
