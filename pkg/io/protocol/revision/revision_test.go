package revision

import "testing"

// TestExpectedRevisionIs254 pins the rev-254 branch's wire revision.
// TS Environment.ts:27 (43e02957) defaults ENGINE_REVISION to 254;
// World.ts rejects any other client revision with login reply 6.
// The predecessor test pinned 244 — the per-branch value, not a durable
// contract — and masked the stale constant until the B6 live client
// smoke surfaced it (PORTING-LESSONS §3: a test can pin a bug).
func TestExpectedRevisionIs254(t *testing.T) {
	if Expected != 254 {
		t.Fatalf("revision.Expected = %d, want 254", Expected)
	}
}
