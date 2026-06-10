package revision

import "testing"

// TestExpectedRevisionIs245 pins the rev-245.2 branch's wire revision.
// TS Environment.ts:27 (3c16994c) defaults ENGINE_REVISION to 245;
// World.ts:2158 rejects any other client revision with login reply 6.
// The predecessor test pinned 244 — the per-branch value, not a durable
// contract — and masked the stale constant until the B6 live client
// smoke surfaced it (PORTING-LESSONS §3: a test can pin a bug).
func TestExpectedRevisionIs245(t *testing.T) {
	if Expected != 245 {
		t.Fatalf("revision.Expected = %d, want 245", Expected)
	}
}
