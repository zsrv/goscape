# NAI-128 Stage 3 — Production-residual binding probe (plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship six NodeDebug-gated gateway probes spanning the OPNPC2-attack → death → loot pipeline. User runs Java-client smoke against a Lumbridge Man; controller binds production residual to layer L0–L7 from `grep nai128` log shape; close commit ships routing decision.

**Architecture:** One log line per gateway, gated behind `s.cfg.NodeDebug` (defaults true; tests opt in explicitly). Five gateways live in `modules/world` (G1–G4, G6) and one in `pkg/script` (G5). G5 requires adding a `Log *slog.Logger` field to `ScriptState`, wired from goscape state-builders. Each gateway carries a focused TDD unit test pinning its emit shape; the existing `TestNAI128_RatLootCascade/CascadeDispatchTrace` continues as the cross-gateway integration regression gate.

**Tech Stack:** Go 1.26+, `log/slog`, `world.Config.NodeDebug`. No new deps.

**Spec:** `docs/superpowers/specs/2026-05-08-nai-128-cascade-fix-stage3-design.md` (`32b120b`).

---

## Pre-flight summary (verified by controller at HEAD `681ba9c`)

- `Npc.server *Server` back-ref exists at `modules/world/npc.go:81` (set by `Server.addNpc`). Available at G1/G2/G3 sites.
- `worldVarsView` holds `s *Server` at `modules/world/server_varp.go:164`. Available at G6.
- `ScriptState.NodeDebug bool` exists at `pkg/script/state.go:218` but no logger field. G5 requires plumbing.
- `world.Config.NodeDebug` defaults `true` via `world.node-debug` flag (`modules/world/config.go:76`).
- `newTestServer` does NOT set `NodeDebug` (`modules/world/server_test.go:311-323`); test fixture must enable it.
- `buildPlayerScriptState` at `modules/world/script.go:38`; `buildNpcScriptState` at `modules/world/npc_script.go:303`. Both can wire `state.Log = s.log`.
- `capturingHandler` test helper at `modules/world/interaction_debug_test.go:64-89` (with `snapshot()` accessor).

---

## Task 1: Add `ScriptState.Log` field + wire from state-builders

**Why:** G5 (handleNpcFindHero gateway) lives in pkg/script which has no logger field on ScriptState today. Adding a single `Log *slog.Logger` field is the minimum API surface to enable G5; all other gateways live in modules/world and have direct `s.log` access.

**Files:**
- Modify: `pkg/script/state.go` (add field next to `NodeDebug`)
- Modify: `modules/world/script.go` (`buildPlayerScriptState`)
- Modify: `modules/world/npc_script.go` (`buildNpcScriptState`)

- [ ] **Step 1: Read current ScriptState.NodeDebug declaration**

Run: `grep -n "NodeDebug bool" pkg/script/state.go`
Expected: line ~218 with field declaration; surrounding doc-comment from line 213.

- [ ] **Step 2: Add `Log *slog.Logger` field**

In `pkg/script/state.go` immediately after the `NodeDebug bool` field (line 218), add:

```go
	// Log is an optional logger for diagnostic instrumentation (e.g.
	// gateway probes). Wired by goscape state-builders from Server.log;
	// may be nil for pkg/script-internal tests. Always nil-check before
	// use. NAI-128 Stage 3.
	Log *slog.Logger
```

Add `"log/slog"` to the import block if not already present.

- [ ] **Step 3: Wire `state.Log` in `buildPlayerScriptState`**

In `modules/world/script.go`, locate the field-assignment block in `buildPlayerScriptState` (around line 38). Find the line `state.NodeDebug = s.cfg.NodeDebug` (or similar — `state.NodeDebug` assignment near top of function). Add immediately after it:

```go
	state.Log = s.log
```

If `state.NodeDebug` is not assigned in `buildPlayerScriptState`, locate the first `state.<Field> = ...` line and add `state.Log = s.log` adjacent.

- [ ] **Step 4: Wire `state.Log` in `buildNpcScriptState`**

In `modules/world/npc_script.go:303`, locate the assignment block. Find the line `state.NodeDebug = s.cfg.NodeDebug` (line 311). Add immediately after it:

```go
	state.Log = s.log
```

- [ ] **Step 5: Run `go build ./...` to confirm clean**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean exit.

- [ ] **Step 6: Run TestNAI128_RatLootCascade to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -15`
Expected: all 6 subtests PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/state.go modules/world/script.go modules/world/npc_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-128): Stage 3 T1 — add ScriptState.Log field + wire from state-builders

Adds an optional *slog.Logger field on ScriptState plumbed from
Server.log via buildPlayerScriptState and buildNpcScriptState. G5
(handleNpcFindHero gateway probe in T6 of this stage) needs server-side
log access from inside pkg/script; this is the minimum API surface.

Field is nil-friendly: pkg/script-internal tests that build ScriptState
without going through goscape state-builders leave it zero-valued; G5
guards on s.Log != nil.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: G1 — `Npc.Damage` gateway

**Why:** Logs every damage application. Most-upstream gateway in the loot pipeline; if it doesn't fire on a Man kill, combat path doesn't go through `Npc.Damage` (binding L0).

**Files:**
- Modify: `modules/world/npc_masks.go` (`Damage` method, line 165)
- Modify: `modules/world/nai128_rat_loot_test.go` (`CascadeDispatchTrace` — add G1 assertion + enable NodeDebug)

- [ ] **Step 1: Add `s.cfg.NodeDebug = true` to `nai128CacheFixture`**

In `modules/world/nai128_rat_loot_test.go`, locate the start of `nai128CacheFixture` body (after `s := newTestServer(t)`, currently line 31). Add:

```go
	// NAI-128 Stage 3: enable NodeDebug so gateway probes fire during
	// the cascade. capturingHandler in CascadeDispatchTrace reads them
	// back as binding regression gates.
	s.cfg.NodeDebug = true
```

Place this immediately after `s := newTestServer(t)`.

- [ ] **Step 2: Write failing test assertion (TDD red)**

In `modules/world/nai128_rat_loot_test.go`, locate the `CascadeDispatchTrace` subtest body (currently around line 240). After the existing `execErrors` check block (after the closing `}` of the `if len(execErrors) > 0` block, before the diagnostic-dump `if t.Failed() ...`), insert:

```go
		// G1 — Npc.Damage gateway. ai_queue2 → ~npc_default_damage runs
		// NPC_DAMAGE during the cascade; assert at least one nai128.npc.damage
		// record fires for the rat.
		var damageRecs []slog.Record
		for _, r := range records {
			if r.Message == "nai128.npc.damage" {
				damageRecs = append(damageRecs, r)
			}
		}
		if len(damageRecs) == 0 {
			t.Errorf("G1: expected at least one %q record during cascade; got 0", "nai128.npc.damage")
		}
```

- [ ] **Step 3: Run test to verify G1 assertion fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade/CascadeDispatchTrace$' ./modules/world/ -v -count=1 2>&1 | tail -20`
Expected: FAIL with `G1: expected at least one "nai128.npc.damage" record during cascade; got 0`.

- [ ] **Step 4: Add G1 gateway to `Npc.Damage`**

In `modules/world/npc_masks.go:165`, modify the `Damage` method body. Locate the existing function:

```go
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	cur := n.levels[objtype.NpcStatHitpoints]
	n.damageAmt = min(amount, cur)
	n.damageType = dmgType
	cur -= amount
	if cur < 0 {
		cur = 0
	}
	n.levels[objtype.NpcStatHitpoints] = cur
	n.masks |= rsbuf.NpcMaskDamage
}
```

Add a NodeDebug-gated log emit immediately before the final `n.masks |= rsbuf.NpcMaskDamage` line:

```go
	if n.server != nil && n.server.cfg.NodeDebug && n.server.log != nil {
		n.server.log.Info("nai128.npc.damage",
			"npc", n.uid,
			"typeId", n.typeId,
			"amount", amount,
			"dmgType", dmgType,
			"cur", cur+amount, // pre-hit HP
			"new", cur,
		)
	}
```

(Using `cur+amount` reconstructs the pre-hit HP because `cur` was already decremented.)

- [ ] **Step 5: Run test to verify G1 passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -25`
Expected: all 6 subtests PASS.

- [ ] **Step 6: Run modules/world full test sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -10`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_masks.go modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-128): Stage 3 T2 — G1 Npc.Damage gateway probe

NodeDebug-gated s.log.Info("nai128.npc.damage") on every NPC damage
application. CascadeDispatchTrace asserts at least one record fires
during the rat's death cascade.

L0 binding signal: if smoke shows zero nai128.npc.damage records
for the dying Man, combat path doesn't go through Npc.Damage.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: G2 — `Npc.AddHeroPoints` gateway

**Why:** Logs every heroPoints credit. If G1 fires but G2 doesn't on a Man kill, the combat damage path bypasses heroPoints (binding L1).

**Files:**
- Modify: `modules/world/npc_script.go` (`AddHeroPoints` method, line 74)
- Modify: `modules/world/nai128_rat_loot_test.go` (no fixture-test assertion — see Step 1 note)

**Note:** The existing test pre-seeds heroPoints via `rat.heroPoints.AddHero(p.UID(), damage)` (direct slice access, NOT via `(*Npc).AddHeroPoints`). So the cascade-fixture test does NOT exercise `(*Npc).AddHeroPoints` — and G2 won't fire there. Add a focused unit test instead.

- [ ] **Step 1: Write a focused failing test for G2**

In `modules/world/nai128_rat_loot_test.go`, after the closing `}` of `TestNAI128_RatLootCascade` (around line 405), append:

```go
// TestNAI128_G2_AddHeroPointsGateway pins the G2 gateway probe at
// (*Npc).AddHeroPoints. NAI-128 Stage 3.
func TestNAI128_G2_AddHeroPointsGateway(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeDebug = true
	rec := &capturingHandler{}
	s.log = slog.New(rec)

	npcType := &objtype.NpcType{}
	n := NewNpc(1, 100, 0, 0, 0, npcType)
	n.server = s

	n.AddHeroPoints(123, 5)

	records := rec.snapshot()
	var found *slog.Record
	for i := range records {
		if records[i].Message == "nai128.heropoints.add" {
			found = &records[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("G2: expected one %q record; got %d total records", "nai128.heropoints.add", len(records))
	}
	var npcUID, playerUID, amount int64
	found.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "npc":
			npcUID = a.Value.Int64()
		case "playerUID":
			playerUID = a.Value.Int64()
		case "amount":
			amount = a.Value.Int64()
		}
		return true
	})
	if npcUID != int64(n.uid) {
		t.Errorf("G2 npc attr = %d; want %d", npcUID, n.uid)
	}
	if playerUID != 123 {
		t.Errorf("G2 playerUID attr = %d; want 123", playerUID)
	}
	if amount != 5 {
		t.Errorf("G2 amount attr = %d; want 5", amount)
	}
}
```

- [ ] **Step 2: Run test to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_G2_AddHeroPointsGateway$' ./modules/world/ -v -count=1 2>&1 | tail -15`
Expected: FAIL with `G2: expected one "nai128.heropoints.add" record; got 0 total records`.

- [ ] **Step 3: Add G2 gateway to `Npc.AddHeroPoints`**

In `modules/world/npc_script.go:74`, modify:

```go
func (n *Npc) AddHeroPoints(playerUID, amount int) {
	n.heroPoints.AddHero(playerUID, amount)
}
```

to:

```go
func (n *Npc) AddHeroPoints(playerUID, amount int) {
	if n.server != nil && n.server.cfg.NodeDebug && n.server.log != nil {
		n.server.log.Info("nai128.heropoints.add",
			"npc", n.uid,
			"typeId", n.typeId,
			"playerUID", playerUID,
			"amount", amount,
		)
	}
	n.heroPoints.AddHero(playerUID, amount)
}
```

- [ ] **Step 4: Run test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_G2_AddHeroPointsGateway$' ./modules/world/ -v -count=1 2>&1 | tail -10`
Expected: PASS.

- [ ] **Step 5: Run TestNAI128_RatLootCascade to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_' ./modules/world/ -v -count=1 2>&1 | tail -20`
Expected: all NAI128 tests PASS (incl. T5 + T6 + the new G2 test).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_script.go modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-128): Stage 3 T3 — G2 Npc.AddHeroPoints gateway probe

NodeDebug-gated s.log.Info("nai128.heropoints.add") on every
heroPoints credit via the (*Npc).AddHeroPoints adapter (the only
production path; NPC_HEROPOINTS opcode handler routes through it).

L1 binding signal: if smoke shows G1 records but no G2 for the dying
Man, combat damage path bypasses the heroPoints ledger.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: G3 — `Npc.EnqueueScriptForTrigger` gateway

**Why:** Logs every queued ai_queueN dispatch. If G2 fires but G3(TriggerAiQueue2) doesn't on a Man kill, the death-cascade trigger is never enqueued (binding L2).

**Files:**
- Modify: `modules/world/npc.go` (`EnqueueScriptForTrigger` method, line 329-335)
- Modify: `modules/world/nai128_rat_loot_test.go` (G3 assertion + focused test)

- [ ] **Step 1: Write CascadeDispatchTrace G3 assertion (TDD red)**

In `CascadeDispatchTrace`, after the G1 assertion block added in Task 2, insert:

```go
		// G3 — Npc.EnqueueScriptForTrigger gateway. The test pre-enqueues
		// TriggerAiQueue2 manually; the cascade re-enters via NPC_QUEUE
		// inside ~npc_default_damage which enqueues TriggerAiQueue3.
		// Assert both fire (one per enqueue).
		var enqueueRecs []slog.Record
		var sawAiQueue2, sawAiQueue3 bool
		for _, r := range records {
			if r.Message == "nai128.npc.enqueue" {
				enqueueRecs = append(enqueueRecs, r)
				r.Attrs(func(a slog.Attr) bool {
					if a.Key == "trigger" {
						switch int(a.Value.Int64()) {
						case int(script.TriggerAiQueue2):
							sawAiQueue2 = true
						case int(script.TriggerAiQueue3):
							sawAiQueue3 = true
						}
					}
					return true
				})
			}
		}
		if !sawAiQueue2 {
			t.Errorf("G3: expected at least one %q record with trigger=TriggerAiQueue2 (%d); got %d enqueue records",
				"nai128.npc.enqueue", script.TriggerAiQueue2, len(enqueueRecs))
		}
		if !sawAiQueue3 {
			t.Errorf("G3: expected at least one %q record with trigger=TriggerAiQueue3 (%d); got %d enqueue records",
				"nai128.npc.enqueue", script.TriggerAiQueue3, len(enqueueRecs))
		}
```

- [ ] **Step 2: Run test to verify G3 assertion fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade/CascadeDispatchTrace$' ./modules/world/ -v -count=1 2>&1 | tail -25`
Expected: FAIL with both G3 assertion errors.

- [ ] **Step 3: Add G3 gateway to `Npc.EnqueueScriptForTrigger`**

In `modules/world/npc.go:329-335`, modify the function:

```go
func (n *Npc) EnqueueScriptForTrigger(trigger script.ServerTriggerType, delay, lastIntArg int) {
	n.queue = append(n.queue, NpcQueueRequest{
		Trigger: trigger,
		Delay:   delay,
		LastInt: lastIntArg,
	})
}
```

to:

```go
func (n *Npc) EnqueueScriptForTrigger(trigger script.ServerTriggerType, delay, lastIntArg int) {
	n.queue = append(n.queue, NpcQueueRequest{
		Trigger: trigger,
		Delay:   delay,
		LastInt: lastIntArg,
	})
	if n.server != nil && n.server.cfg.NodeDebug && n.server.log != nil {
		n.server.log.Info("nai128.npc.enqueue",
			"npc", n.uid,
			"typeId", n.typeId,
			"trigger", int(trigger),
			"delay", delay,
			"lastInt", lastIntArg,
			"queueLen", len(n.queue),
		)
	}
}
```

(Confirm the actual existing function body before applying — fields may differ slightly. The plan-shown body is from `grep` of `npc.go:329`. If the actual `n.queue` element type or field names differ, preserve them and only add the log block at the end.)

- [ ] **Step 4: Run test to verify both G3 assertions pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -20`
Expected: all 6 subtests PASS.

- [ ] **Step 5: Run modules/world sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -10`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc.go modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-128): Stage 3 T4 — G3 Npc.EnqueueScriptForTrigger gateway probe

NodeDebug-gated s.log.Info("nai128.npc.enqueue") on every queued
ai_queueN dispatch. CascadeDispatchTrace asserts both TriggerAiQueue2
(test-side enqueue) and TriggerAiQueue3 (cascade-side re-enqueue from
~npc_default_damage's NPC_QUEUE) fire during the rat's death cascade.

L2 binding signal: if smoke shows G1+G2 records but no G3 with
trigger=TriggerAiQueue2 for the dying Man, the death-cascade trigger
is never queued.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: G4 — `processNpcQueue` per-fire gateway

**Why:** Logs every script that fires from the NPC queue. If G3 fires but G4 doesn't (matching trigger), the queue holds the entry but `processNpcQueue` isn't dispatching it (binding L3).

**Files:**
- Modify: `modules/world/npc_script.go` (`processNpcQueue` method, lines 497-526)
- Modify: `modules/world/nai128_rat_loot_test.go` (G4 assertion in CascadeDispatchTrace)

- [ ] **Step 1: Add G4 assertion to CascadeDispatchTrace (TDD red)**

After the G3 block, insert:

```go
		// G4 — processNpcQueue per-fire gateway. Both ai_queue2 and
		// ai_queue3 should fire during the cascade per spec §4.4
		// phase-collapse. Assert one queuefire record each by sf.Name
		// shape (rat-specific scripts).
		var queueFireRecs []slog.Record
		var sawAi2Fire, sawAi3Fire bool
		for _, r := range records {
			if r.Message == "nai128.npc.queuefire" {
				queueFireRecs = append(queueFireRecs, r)
				r.Attrs(func(a slog.Attr) bool {
					if a.Key == "sf" {
						name := a.Value.String()
						if name == "[ai_queue2,_]" || name == "[ai_queue2,newbiegiantrat]" {
							sawAi2Fire = true
						}
						if name == "[ai_queue3,_]" || name == "[ai_queue3,newbiegiantrat]" {
							sawAi3Fire = true
						}
					}
					return true
				})
			}
		}
		if !sawAi2Fire {
			t.Errorf("G4: expected one %q record for ai_queue2 (specific or generic); got %d queuefire records",
				"nai128.npc.queuefire", len(queueFireRecs))
		}
		if !sawAi3Fire {
			t.Errorf("G4: expected one %q record for ai_queue3 (specific or generic); got %d queuefire records",
				"nai128.npc.queuefire", len(queueFireRecs))
		}
```

- [ ] **Step 2: Run test to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade/CascadeDispatchTrace$' ./modules/world/ -v -count=1 2>&1 | tail -25`
Expected: FAIL with both G4 assertions.

- [ ] **Step 3: Add G4 gateway to `processNpcQueue`**

In `modules/world/npc_script.go:497-526`, locate the body of `processNpcQueue`. The relevant section:

```go
		sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
		if sf == nil {
			continue
		}
		state := s.buildNpcScriptState(sf, n, nil, nil, nil)
		state.LastInt = lastIntArg
		s.resumeOrFinishNpc(state, n)
```

Insert immediately after `if sf == nil { continue }`:

```go
		if s.cfg.NodeDebug && s.log != nil {
			s.log.Info("nai128.npc.queuefire",
				"npc", n.uid,
				"typeId", n.typeId,
				"trigger", int(trigger),
				"sf", sf.Name,
				"lastInt", lastIntArg,
			)
		}
```

- [ ] **Step 4: Run test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -20`
Expected: all 6 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_script.go modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-128): Stage 3 T5 — G4 processNpcQueue per-fire gateway probe

NodeDebug-gated s.log.Info("nai128.npc.queuefire") on every script
that fires from the NPC queue post-GetByTrigger. Records the script
name (e.g. "[ai_queue3,newbiegiantrat]") + lastInt arg.

CascadeDispatchTrace asserts both ai_queue2 and ai_queue3 fire during
the rat cascade (specific or generic forms accepted).

L3/L4 binding signal: if smoke shows G3 records but no matching G4,
the queue holds entries that processNpcQueue isn't dispatching
(L3: dispatch gap) or the cascade re-enqueue never runs (L4:
~npc_default_damage doesn't enqueue ai_queue3).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: G5 — `handleNpcFindHero` exit gateway

**Why:** Logs the result of every NPC_FINDHERO call. If G4(ai_queue3) fires but G5 returns `pushed=0`, the heroPoints ledger is empty or the player lookup misses at cascade time (binding L5).

**Files:**
- Modify: `pkg/script/handlers_npc.go` (`handleNpcFindHero`, lines 1105-1132)
- Modify: `modules/world/nai128_rat_loot_test.go` (G5 assertion in CascadeDispatchTrace)

- [ ] **Step 1: Add G5 assertion to CascadeDispatchTrace (TDD red)**

After the G4 block, insert:

```go
		// G5 — handleNpcFindHero exit gateway. ai_queue3's npc_findhero
		// call should fire one record with pushed=1 (heroPoints credited
		// via test setup; player lookup resolves post-Phase-A).
		var findHeroRecs []slog.Record
		for _, r := range records {
			if r.Message == "nai128.npc.findhero" {
				findHeroRecs = append(findHeroRecs, r)
			}
		}
		if len(findHeroRecs) == 0 {
			t.Errorf("G5: expected at least one %q record during cascade; got 0", "nai128.npc.findhero")
		} else {
			var pushed int64 = -1
			findHeroRecs[0].Attrs(func(a slog.Attr) bool {
				if a.Key == "pushed" {
					pushed = a.Value.Int64()
				}
				return true
			})
			if pushed != 1 {
				t.Errorf("G5: first record pushed=%d; want 1 (test setup credits heroPoints + Phase A wires LookupPlayerByUID)", pushed)
			}
		}
```

- [ ] **Step 2: Run test to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade/CascadeDispatchTrace$' ./modules/world/ -v -count=1 2>&1 | tail -20`
Expected: FAIL with G5 assertion.

- [ ] **Step 3: Add G5 gateway to `handleNpcFindHero`**

In `pkg/script/handlers_npc.go:1105-1132`, locate `handleNpcFindHero`. The current body ends with `s.PushInt(1)` on success, `s.PushInt(0)` on each early return. Add a single log emit at the very end (just before `return nil` on the success path) AND on each early-return path. Cleanest: refactor to compute the result then log+push.

Replace the body of `handleNpcFindHero` with:

```go
func handleNpcFindHero(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_FINDHERO"); err != nil {
		return err
	}
	pushed := 0
	var topUID int
	lookupNonNil := false
	defer func() {
		if s.NodeDebug && s.Log != nil {
			s.Log.Info("nai128.npc.findhero",
				"topUID", topUID,
				"lookupNonNil", lookupNonNil,
				"pushed", pushed,
			)
		}
	}()
	if s.World == nil {
		s.PushInt(0)
		return nil
	}
	topUID = s.ActiveNpc.TopContributor()
	if topUID == 0 {
		s.PushInt(0)
		return nil
	}
	player := s.World.LookupPlayerByUID(topUID)
	if player == nil {
		s.PushInt(0)
		return nil
	}
	lookupNonNil = true
	if s.Script.IntOperands[s.PC] == 0 {
		s.Self = player
		s.Pointers |= PtrActivePlayer
	} else {
		s.Self2 = player
		s.Pointers |= PtrActivePlayer2
	}
	s.PushInt(1)
	pushed = 1
	return nil
}
```

(Note: the deferred log captures the final values of `pushed`, `topUID`, `lookupNonNil` regardless of which return path is taken. The `requireActiveNpc` failure path returns before the defer registers — which is correct because that's a separate error category, not a FINDHERO-result we want to log.)

- [ ] **Step 4: Run pkg/script tests to verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... 2>&1 | tail -10`
Expected: all PASS (existing handleNpcFindHero tests use ScriptState without Log; nil-guard suppresses the log emit).

- [ ] **Step 5: Run TestNAI128_RatLootCascade to verify G5 PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -20`
Expected: all 6 subtests PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_npc.go modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-128): Stage 3 T6 — G5 handleNpcFindHero exit gateway probe

NodeDebug-gated deferred s.Log.Info("nai128.npc.findhero") at every
NPC_FINDHERO exit (success + each early-return). Records topUID,
lookupNonNil, and pushed value. Refactors handleNpcFindHero to a
single-defer pattern so all three nil paths share one emit site.

CascadeDispatchTrace asserts at least one record with pushed=1 fires
during the rat cascade.

L5 binding signal: if smoke shows G4(ai_queue3) but G5 pushed=0,
NPC_FINDHERO gates obj_add out — heroPoints empty or LookupPlayerByUID
miss in production.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: G6 — `worldVarsView.AddObj` gateway

**Why:** Logs every obj added to the world. If G5 returns `pushed=1` but G6 doesn't fire, an opcode between FINDHERO and OBJ_ADD blocked execution (binding L6). If G6 fires but client shows no loot, the bug is downstream zone-broadcast (binding L7).

**Files:**
- Modify: `modules/world/server_varp.go` (`worldVarsView.AddObj`, lines 164-171)
- Modify: `modules/world/nai128_rat_loot_test.go` (G6 assertion in CascadeDispatchTrace)

- [ ] **Step 1: Add G6 assertion to CascadeDispatchTrace (TDD red)**

After the G5 block, insert:

```go
		// G6 — worldVarsView.AddObj gateway. ai_queue3's two obj_add
		// calls fire one record each (death_drop + raw_rat_meat).
		var addObjRecs []slog.Record
		for _, r := range records {
			if r.Message == "nai128.obj.add" {
				addObjRecs = append(addObjRecs, r)
			}
		}
		if len(addObjRecs) < 2 {
			t.Errorf("G6: expected at least 2 %q records during cascade (death_drop + raw_rat_meat); got %d",
				"nai128.obj.add", len(addObjRecs))
		}
```

- [ ] **Step 2: Run test to verify FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade/CascadeDispatchTrace$' ./modules/world/ -v -count=1 2>&1 | tail -15`
Expected: FAIL with G6 assertion.

- [ ] **Step 3: Add G6 gateway to `worldVarsView.AddObj`**

In `modules/world/server_varp.go:164-171`, locate the existing implementation. It currently looks like:

```go
func (w worldVarsView) AddObj(level, x, z, typeID, count, duration, receiverID int) script.ActiveObj {
	if w.s == nil {
		return nil
	}
	obj := entitypkg.NewObj(typeID, x, z, /* ... */)
	w.s.AddObj(obj, receiverID)
	return obj
}
```

(Confirm the actual implementation before applying — `entitypkg.NewObj` arg shape may include lifecycle/duration. Read the file before editing.)

Add the log emit after `w.s.AddObj(obj, receiverID)`:

```go
	if w.s.cfg.NodeDebug && w.s.log != nil {
		w.s.log.Info("nai128.obj.add",
			"level", level,
			"x", x,
			"z", z,
			"typeID", typeID,
			"count", count,
			"duration", duration,
			"receiverID", receiverID,
		)
	}
```

- [ ] **Step 4: Run test to verify PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI128_RatLootCascade$' ./modules/world/ -v -count=1 2>&1 | tail -20`
Expected: all 6 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/server_varp.go modules/world/nai128_rat_loot_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-128): Stage 3 T7 — G6 worldVarsView.AddObj gateway probe

NodeDebug-gated s.log.Info("nai128.obj.add") on every obj_add that
reaches the zone-write layer. Records coord, typeID, count, duration,
receiverID.

CascadeDispatchTrace asserts at least 2 records during the rat cascade
(death_drop + raw_rat_meat).

L6/L7 binding signal: if smoke shows G5 pushed=1 but no G6, an opcode
between FINDHERO and OBJ_ADD blocked execution. If G6 fires but client
shows no loot, the bug is downstream zone-broadcast (separate sub-spec).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Cross-package regression sweep

**Why:** Six gateway adds + one ScriptState API change have non-zero blast radius. Sweep before smoke handoff.

- [ ] **Step 1: Run all script tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... 2>&1 | tail -10`
Expected: all PASS.

- [ ] **Step 2: Run all world-module tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -10`
Expected: all PASS.

- [ ] **Step 3: Run full `./...` sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -25`
Expected: all PASS. Address any unrelated failures.

- [ ] **Step 4: Run race detector on the integration test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run 'TestNAI128_RatLootCascade$' ./modules/world/ -count=1 2>&1 | tail -10`
Expected: PASS, no race warnings.

(No commit — verification only.)

---

## Task 9: Phase D — Smoke handoff to user

**Why:** Per `smoke_test_server_handoff` memory: user launches the server. The probe binds production residual via Java-client-driven smoke output. Stage 3 close depends on the bound layer.

- [ ] **Step 1: Confirm `config.yaml` exists**

Run: `ls -la config.yaml 2>&1 | head`
Expected: file present.

- [ ] **Step 2: Emit smoke prompt to user**

Output to user (do NOT execute the server command yourself):

```
Stage 3 smoke handoff — please run from a non-sandboxed shell:

  CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml 2>&1 | tee /tmp/nai128-stage3.log

Then connect with the Client-Java #225 build. Walk to a Lumbridge Man
(any newbieman, e.g. ~3221, 3219, 0). Attack until death — ONE Man only;
restart server before re-running if log gets noisy.

After the Man dies, stop the server. Filter the log:

  grep nai128 /tmp/nai128-stage3.log

Paste the output back. Controller binds layer L0–L7 from log shape.

If the Man does NOT die: separate signal — note that and stop the
server; we route to a combat-side investigation rather than NAI-128
loot binding.
```

- [ ] **Step 3: Wait for user smoke result before close**

Block on user response. Continue to Task 10 once log shape is captured.

(No commit — handoff only.)

---

## Task 10: Bind layer + close commit + memory + resume prompt

**Why:** Per `close_commit_memory_trailer` memory: NAI-N close commits add `Closes memory:` trailer for git-log provenance. Bound layer is recorded in commit body for future grep.

- [ ] **Step 1: Match log shape to layer-routing table (spec §3)**

Compare user's `grep nai128` output against the table:

| Observed | Bound | Routes to |
|---|---|---|
| No `nai128.` lines at all | **L−1**: NodeDebug not on or smoke didn't hit Man-attack | re-run smoke |
| No G1 | **L0** | combat-damage path investigation (likely new sub-spec) |
| G1 yes, no G2 | **L1**: heroPoints credit gap | Stage 4 (NAI-128) — wire AddHeroPoints from combat path |
| G1+G2, no G3(118) | **L2**: ai_queue2 enqueue gap | Stage 4 (NAI-128) — wire combat→ai_queue2 |
| G3(118) yes, no G4 ai_queue2 | **L3**: queue-dispatch gap | Stage 4 (NAI-128) — investigate processNpcQueue dispatch |
| G4(ai_queue2), no G3(119) | **L4**: cascade re-enqueue gap | Stage 4 (NAI-128) — investigate ~npc_default_damage NPC_QUEUE handler |
| G4(ai_queue3), G5 `pushed=0` | **L5**: NPC_FINDHERO production gap | Stage 4 (NAI-128) — heroPoints/lookup-uid mismatch |
| G5 `pushed=1`, no G6 | **L6**: obj_add never reached | Stage 4 (NAI-128) — script-side opcode gap |
| G6 fires, no client loot | **L7**: zone-broadcast gap | NAI-129 fresh sub-spec (out of NAI-128 scope) |

- [ ] **Step 2: Add memory entry for the gateway-probe pattern**

Create `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nodedebug_gateway_probe_pattern.md`:

```markdown
---
name: NodeDebug-gated gateway probe pattern for binding production residuals
description: When fixture passes but smoke fails, instrument N suspected pipeline gateways with NodeDebug-gated s.log.Info("nai128.<key>") + tee+grep for binding; permanent diagnostic, no revert
type: feedback
---

When a Stage-2-style cascade fix lands but Phase-D smoke still fails,
the production residual is in a layer the test fixture doesn't model.
Bind it via this pattern (NAI-128 Stage 3 reference impl):

1. Identify ~6 gateways spanning the suspect pipeline end-to-end.
2. Add `if s.cfg.NodeDebug && s.log != nil { s.log.Info("nai128.<key>", ...) }`
   at each gateway. Use a unique prefix (e.g. `nai128.`) for grep.
3. Per-gateway focused TDD test (or assertion in existing integration
   test) pins the emit shape — log key + critical attrs.
4. Hand off smoke to user: server runs with `2>&1 | tee /tmp/<id>.log`;
   user grep'd output binds the layer.
5. Layer-routing table in spec §3 maps log shape → bound layer.
6. Stage closes on layer identification + routing decision; production
   fix lands in follow-up sub-spec.

**Why:** NAI-128 Stage 1 framed binding around manual rat-fixture probes
(T1–T5). When Stage 2 fixture-side fix made T5 GREEN but smoke still
failed, single-cycle probes couldn't bind. The gateway-probe pattern
gave smoke a structured signal (log shape) that binds in one round-trip.

**How to apply:** When Phase D smoke fails after a Stage 2 close, before
brainstorming Stage 3 fixes, instrument the pipeline with gateway
probes. The smoke binds; only THEN brainstorm fixes. Probe instrumentation
ships permanently NodeDebug-gated — future debugging benefits from the
diagnostic without re-instrumentation.

**Cost:** ~10–15 LOC per gateway (log emit + focused unit test), plus a
ScriptState.Log field if any gateway lives in pkg/script. NAI-128 Stage 3
shipped 6 gateways in ~100 LOC across 5 files + 1 plumbing change.
```

- [ ] **Step 3: Add MEMORY.md index entry**

Append to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (insert near other binding-pattern entries; put new entries at the top of the file):

```markdown
- [NodeDebug gateway probe pattern](nodedebug_gateway_probe_pattern.md) — binding via 6 NodeDebug-gated s.log.Info gateways + tee+grep on smoke; permanent diagnostic (NAI-128 Stage 3 reference)
```

- [ ] **Step 4: Verify MEMORY.md size**

Run: `wc -c /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`
If now well over 28KB, retire one stale CLOSED entry from the index (move detail to its topic file if needed).

- [ ] **Step 5: Verify clean working tree**

Run: `git status`
Expected: clean.

- [ ] **Step 6: Stage 3 close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
docs(nai-128): Stage 3 close — production residual bound to L<N>

Six gateway probes (G1..G6) shipped NodeDebug-gated; ScriptState.Log
field plumbed for G5. CascadeDispatchTrace integration test asserts
all six fire during the test cascade.

Smoke ran 2026-05-08 against a Lumbridge Man. grep nai128 output:

  [PASTE: user's verbatim grep output, fenced 10-30 lines]

Binding: L<N> per spec §3 layer-routing table.
Routing: [Stage 4 NAI-128 / NAI-129 fresh sub-spec — pick per table].

Closes memory: nodedebug_gateway_probe_pattern

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Replace `L<N>`, the verbatim grep output, and the routing decision with the actual values.)

- [ ] **Step 7: Emit resume prompt for next session**

Output to user (per `post_task_handoff` memory):

```
NAI-128 Stage 3 closed. Production residual bound to L<N>:
[one-sentence summary of which gateway log signal pinned it].

Six gateway probes shipped NodeDebug-gated as permanent diagnostics.
TestNAI128_RatLootCascade/CascadeDispatchTrace now covers all six as
positive-contract regression gates.

New memory: nodedebug_gateway_probe_pattern.

Next: [Stage 4 NAI-128 brainstorm | NAI-129 brainstorm | other follow-up
per the routing decision].
```

---

## Self-review

- **Spec coverage:**
  - §1 Goal → close criterion in Task 10.
  - §2 Pre-flight findings → Task 1 plumbing premise; Tasks 2–7 act on the verified gateway sites.
  - §3 Layer-routing table → Task 10 Step 1 (binding step).
  - §4 Architecture → Tasks 1–7 (each gateway plus plumbing).
  - §5 Test strategy → Tasks 2–7 each ship a focused or integration assertion; Task 8 sweep.
  - §6 Risks → R1 (grep prefix) baked into log keys; R2/R3/R4 in Task 9 prompt; R5/R6 nil guards baked into all gateway emits; R7 acknowledged in Task 10 routing table.
  - §7 Out of scope → not in plan.
  - §8 Close criterion → Task 10 covers items 1–8 of the spec.
  - §9 Memory entries → Task 10 Step 2 writes new entry; existing memories applied throughout (cited in commit bodies).

- **Placeholder scan:** Task 7 Step 3 contains "(Confirm the actual implementation before applying — `entitypkg.NewObj` arg shape may include lifecycle/duration. Read the file before editing.)" — this is a reasonable controller-pre-flight instruction, not an unfilled placeholder. Same for Task 4 Step 3's similar caveat. Both are concrete pointers to read before editing, with the gateway-add code fully specified. No "TBD" / "TODO" / "implement later" / "fill in" patterns elsewhere.

- **Type consistency:**
  - `nai128.npc.damage` / `nai128.heropoints.add` / `nai128.npc.enqueue` / `nai128.npc.queuefire` / `nai128.npc.findhero` / `nai128.obj.add` — keys consistent across spec §3 table, gateway adds, and test assertions.
  - `s.cfg.NodeDebug && s.log != nil` guard — same shape across G1, G2, G3, G4, G6.
  - `s.NodeDebug && s.Log != nil` (capital-L) — G5 only, on ScriptState.
  - `(*Npc).server` access via `n.server.cfg.NodeDebug` / `n.server.log` — consistent across G1/G2/G3.
  - `(worldVarsView).s` access via `w.s.cfg.NodeDebug` / `w.s.log` — used at G6.
  - `processNpcQueue` is `(*Server)` method — uses `s.cfg.NodeDebug` / `s.log` directly at G4.
  - `script.TriggerAiQueue2 = 118`, `script.TriggerAiQueue3 = 119` (verified pre-flight at `pkg/script/trigger.go:118-119`) — used in G3 assertions.
  - `script.ScriptFile.Name` accessor at G4 emits `sf.Name` (the script identifier like `[ai_queue3,newbiegiantrat]`).
  - `slog.Record.Attrs` callback signature `func(slog.Attr) bool` — used identically across G1/G3/G4/G5 readback assertions (matches existing T6 pattern at `nai128_rat_loot_test.go:280-289`).
