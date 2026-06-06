package revision

import "testing"

// TestExpectedRevisionIs244 pins the rev-244 branch's wire revision.
// TS Environment.ts:27 (9aadcec4) defaults ENGINE_REVISION to 244;
// World.ts:2158 rejects any other client revision with login reply 6.
// The predecessor test pinned 225 — the per-branch value, not a durable
// contract — and masked the stale constant until the B6 live client
// smoke surfaced it (PORTING-LESSONS §3: a test can pin a bug).
func TestExpectedRevisionIs244(t *testing.T) {
	if Expected != 244 {
		t.Fatalf("revision.Expected = %d, want 244", Expected)
	}
}
