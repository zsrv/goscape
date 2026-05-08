# NAI-129 — L7 zone-broadcast UID/slot filter fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `p.slot` comparator with `p.uid` at the two private-drop zone-broadcast filter sites in `modules/world/player_zone.go` (lines 41 and 76) so that the producer-side UID (set by `worldVarsView.AddObj`) matches the player-side filter, restoring TS-faithful per-player delivery for OBJ_ADD/OBJ_COUNT/OBJ_DEL/OBJ_REVEAL Follows events. Pin both filter paths (per-tick `writePartialFollows` and on-zone-load `writeFullFollows` replay) with positive owner + absence non-owner tests.

**Architecture:** Mechanical 2-line filter swap. Producer pipeline (`pkg/script/handlers_obj.go:109` → `modules/world/server_varp.go:170` → `pkg/zone/zone.go:267`) is already UID-space; only the two consumer-side filter comparisons in `modules/world/player_zone.go` are slot-space. After the fix, both sides match TS `Engine-TS/src/engine/zone/Zone.ts:138, 190` (`obj.receiver64 !== player.hash64`). `pkg/entity/obj.go` and `pkg/zone/event.go` doc comments are tightened to reflect UID-space; no symbol rename.

**Tech Stack:** Go 1.26+; existing `composeUID(username37, slot) int` (`modules/world/player_uid.go`); existing test helpers `newZoneTestServer` (`modules/world/world_zone_test.go:13`), `newZoneTestPlayer` (`modules/world/player_zone_test.go:11`), `drainConn` (`modules/world/stat_update_test.go:59`).

**Spec:** `docs/superpowers/specs/2026-05-08-nai-129-l7-zone-broadcast-uid-filter-design.md`

**Convention for test UIDs:** Each test seeds players via `newZoneTestPlayer(t, s, slot, x, z, level)` and then directly assigns `p.uid = composeUID(username37, slot)`. Username37 values are picked so that for slot ≥ 1, the UID is ≥ 2048 (so it never collides with any slot). The minimum non-zero `username37` is `1`, giving `composeUID(1, slot) = (1 << 11) | slot = 2048 + slot`, which is guaranteed > 2047 for all `slot ≥ 1`. This is the canonical pattern used throughout the new tests.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `modules/world/player_zone.go` | Modify (lines 41, 76 + 2 doc-comment blocks) | Per-player private-drop delivery filter — change `p.slot` → `p.uid`. |
| `modules/world/player_zone_test.go` | Modify (rewrite 1 test + add 4 tests) | Pin both filter paths with UID-space owner + non-owner tests. |
| `pkg/zone/event.go` | Modify (line 30 comment only) | Tighten `ReceiverID` field comment to declare UID-space (mirrors TS `receiver64`). |
| `pkg/entity/obj.go` | Modify (line 16 comment only) | Tighten `ReceiverID` field comment to declare UID-space (was misleadingly "slot"). |
| `modules/world/world_zone.go` | Modify (lines 123-124 doc-comment only) | Align `Server.AddObj` doc with UID-space convention. |

No new files. No type changes. No symbol renames.

---

## Task 1: Rewrite `TestPartialFollowsFiltersByReceiverID` to UID-space + apply the `writePartialFollows` fix

**Files:**
- Modify: `modules/world/player_zone_test.go:159-185` (test rewrite)
- Modify: `modules/world/player_zone.go:76` (production fix)
- Test: `modules/world/player_zone_test.go::TestPartialFollowsFiltersByReceiverID`

This task pairs the test rewrite with the matching production fix because rewriting the test alone would leave a red commit. The test already exists at HEAD as a slot/slot fixture; we convert it to UID/UID, which makes it fail under buggy production, then immediately fix `writePartialFollows`.

- [ ] **Step 1: Rewrite the existing test to UID-space**

Replace the body of `TestPartialFollowsFiltersByReceiverID` in `modules/world/player_zone_test.go` (currently lines 159-185) with:

```go
func TestPartialFollowsFiltersByReceiverID(t *testing.T) {
	s := newZoneTestServer(t)
	// Player at slot 7 with a derived UID (username37=1 → uid = (1<<11)|7 = 2055).
	p, cc := newZoneTestPlayer(t, s, 7, 3094, 3106, 0)
	p.uid = composeUID(1, 7)
	otherUID := composeUID(2, 3) // username37=2, slot 3 → uid = (2<<11)|3 = 4099 (distinct, > 2047)

	z := s.zoneMap.Get(0, 3094, 3106)
	// Two Follows events: one for otherUID, one for p.uid.
	objOther := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	objMine := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 995, 1)
	s.AddObj(objOther, otherUID)
	s.AddObj(objMine, p.uid)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, cc)
	p.writePartialFollows(z)
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected follows packets for p.uid")
	}
	// Should include exactly one Follows wrapper + one ObjAdd for p.uid
	// (otherUID filtered). 2 + 5 = 7 bytes payload + 2 opcode bytes = 9.
	if len(got) != 9 {
		t.Errorf("want 9 bytes (1 header + 1 ObjAdd for p.uid); got %d", len(got))
	}
}
```

- [ ] **Step 2: Run the test against unmodified production to confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPartialFollowsFiltersByReceiverID -v`

Expected: FAIL. Both `objOther` (UID 4099) and `objMine` (UID 2055) are filtered out by `e.ReceiverID != p.slot` (slot=7), so no Follows wrapper is written and `len(got) == 0` triggers `t.Fatal("expected follows packets for p.uid")`.

If the test does NOT fail in this expected way, stop and re-verify the diagnosis before proceeding.

- [ ] **Step 3: Apply the `writePartialFollows` production fix**

In `modules/world/player_zone.go`, replace line 76:

```go
		if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.slot {
```

with:

```go
		if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.uid {
```

- [ ] **Step 4: Run the test to confirm it now passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPartialFollowsFiltersByReceiverID -v`

Expected: PASS.

- [ ] **Step 5: Run the full `modules/world` test package to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS (all tests in the package).

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_zone.go modules/world/player_zone_test.go
git commit --no-gpg-sign -m "fix(nai-129): writePartialFollows filter uses UID-space (Task 1)

Rewrites TestPartialFollowsFiltersByReceiverID from slot/slot to UID/UID
(producer side passes uid via composeUID, filter side compares against
p.uid). Mirrors TS Engine-TS Zone.ts:190 — event.receiver64 vs
player.hash64."
```

---

## Task 2: Add positive owner-pin test for `writePartialFollows`

**Files:**
- Modify: `modules/world/player_zone_test.go` (append new test)
- Test: `modules/world/player_zone_test.go::TestPartialFollowsDeliversPrivateDropToOwnerByUID`

This pin asserts the positive case: a private drop with `ReceiverID == p.uid` produces a Follows wrapper + OBJ_ADD packet for the owner. Distinct from the rewrite in Task 1 because that test mixes positive + filter; this one isolates the positive path with a single drop.

- [ ] **Step 1: Add the new test at the end of `modules/world/player_zone_test.go`**

Append:

```go
func TestPartialFollowsDeliversPrivateDropToOwnerByUID(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	p.uid = composeUID(1, 5) // uid = (1<<11)|5 = 2053

	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	s.AddObj(obj, p.uid)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, cc)
	p.writePartialFollows(z)
	p.client.flushWrite()
	got := <-received
	// 1 Follows wrapper (opcode + 2-byte payload = 3 bytes) +
	// 1 ObjAdd nested (opcode + 4-byte payload = 6 bytes; rsbuf.EncodeObjAdd
	// writes 1-byte coord-pack stub + 2-byte type + 1-byte count … but the
	// existing slot=7 test pins this combined wire shape at 9 bytes).
	if len(got) != 9 {
		t.Errorf("want 9 bytes (1 Follows wrapper + 1 ObjAdd for p.uid); got %d", len(got))
	}
}
```

- [ ] **Step 2: Run the new test to confirm it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPartialFollowsDeliversPrivateDropToOwnerByUID -v`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_zone_test.go
git commit --no-gpg-sign -m "test(nai-129): partial-follows owner-pin (Task 2)

Positive UID-space pin: private drop with ReceiverID == p.uid produces
the Follows wrapper + OBJ_ADD packet."
```

---

## Task 3: Add absence-pin test for `writePartialFollows`

**Files:**
- Modify: `modules/world/player_zone_test.go` (append new test)
- Test: `modules/world/player_zone_test.go::TestPartialFollowsHidesPrivateDropFromNonOwnerByUID`

A second player at a different UID standing in the same zone must NOT receive the private-drop OBJ_ADD bytes.

- [ ] **Step 1: Add the new test**

Append:

```go
func TestPartialFollowsHidesPrivateDropFromNonOwnerByUID(t *testing.T) {
	s := newZoneTestServer(t)
	// Owner at slot 5 with uid = (1<<11)|5 = 2053.
	owner, _ := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	owner.uid = composeUID(1, 5)
	// Other player at slot 9 with uid = (2<<11)|9 = 4105 (distinct).
	other, otherCC := newZoneTestPlayer(t, s, 9, 3094, 3106, 0)
	other.uid = composeUID(2, 9)

	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	s.AddObj(obj, owner.uid)
	for zi := range s.zonesTracking {
		zi.ComputeShared()
	}

	received := drainConn(t, otherCC)
	other.writePartialFollows(z)
	other.client.flushWrite()
	got := <-received
	if len(got) != 0 {
		t.Errorf("non-owner must receive no bytes; got %d (%v)", len(got), got)
	}
}
```

- [ ] **Step 2: Run the test to confirm it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestPartialFollowsHidesPrivateDropFromNonOwnerByUID -v`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_zone_test.go
git commit --no-gpg-sign -m "test(nai-129): partial-follows non-owner absence-pin (Task 3)

Absence-pin: a second player with a different UID standing in the same
zone receives no bytes when a private drop is queued for the owner's UID."
```

---

## Task 4: Add positive owner-pin test for `writeFullFollows` replay + apply the second production fix

**Files:**
- Modify: `modules/world/player_zone_test.go` (append new test)
- Modify: `modules/world/player_zone.go:41` (production fix)
- Test: `modules/world/player_zone_test.go::TestFullFollowsReplaysPrivateDropToOwnerByUID`

Pairs the new test with the second filter-site fix. The full-follows replay path (`writeFullFollows` lines 21-65) iterates `z.Objs` and filters by `obj.ReceiverID`; tests must seed `obj.ReceiverID` directly because `Server.AddObj` does not set it on the obj struct (only `worldVarsView.AddObj` does, at `modules/world/server_varp.go:169`).

- [ ] **Step 1: Add the failing test**

Append to `modules/world/player_zone_test.go`:

```go
func TestFullFollowsReplaysPrivateDropToOwnerByUID(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	p.uid = composeUID(1, 5) // uid = 2053

	// Preload a dynamic Obj into the zone with ReceiverID == p.uid. Bypass
	// Server.AddObj so nothing lives in zonesTracking — we're testing the
	// replay path only. Set obj.ReceiverID directly to mirror what
	// worldVarsView.AddObj does at server_varp.go:169.
	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	obj.ReceiverID = p.uid
	z.Objs = append(z.Objs, obj)

	received := drainConn(t, cc)
	p.writeFullFollows(z, 1) // currentTick=1, obj.LastLifecycleTick=0 → replay.
	p.client.flushWrite()
	got := <-received
	if len(got) == 0 {
		t.Fatal("expected FullFollows + PartialFollows + ObjAdd packets")
	}
	// FullFollows header (3 bytes: opcode + 2 payload) + PartialFollows
	// wrapper (3 bytes) + ObjAdd (5 bytes) = 11 bytes. The existing
	// TestWriteFullFollowsSkipsThisTickTransitions pins the
	// header-only baseline at 3 bytes.
	if len(got) != 11 {
		t.Errorf("want 11 bytes (FullFollows header + PartialFollows wrapper + 1 ObjAdd); got %d", len(got))
	}
}
```

- [ ] **Step 2: Run the test against unmodified production to confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFullFollowsReplaysPrivateDropToOwnerByUID -v`

Expected: FAIL with `len(got) == 3` (only FullFollows header) — the obj is filtered by `obj.ReceiverID != p.slot` (slot=5, ReceiverID=2053).

- [ ] **Step 3: Apply the `writeFullFollows` production fix**

In `modules/world/player_zone.go`, replace line 41:

```go
		if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.slot {
```

with:

```go
		if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.uid {
```

- [ ] **Step 4: Run the test to confirm it now passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFullFollowsReplaysPrivateDropToOwnerByUID -v`

Expected: PASS.

- [ ] **Step 5: Run the full `modules/world` test package to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_zone.go modules/world/player_zone_test.go
git commit --no-gpg-sign -m "fix(nai-129): writeFullFollows replay filter uses UID-space (Task 4)

Mirrors TS Engine-TS Zone.ts:138 — obj.receiver64 vs player.hash64.
Pairs the production fix with the failing-then-passing replay
owner-pin test."
```

---

## Task 5: Add absence-pin test for `writeFullFollows` replay

**Files:**
- Modify: `modules/world/player_zone_test.go` (append new test)
- Test: `modules/world/player_zone_test.go::TestFullFollowsHidesPrivateDropFromNonOwnerInReplay`

A second player loading the same zone with a different UID must NOT see the private drop in the replay.

- [ ] **Step 1: Add the test**

Append:

```go
func TestFullFollowsHidesPrivateDropFromNonOwnerInReplay(t *testing.T) {
	s := newZoneTestServer(t)
	owner, _ := newZoneTestPlayer(t, s, 5, 3094, 3106, 0)
	owner.uid = composeUID(1, 5) // uid = 2053
	other, otherCC := newZoneTestPlayer(t, s, 9, 3094, 3106, 0)
	other.uid = composeUID(2, 9) // uid = 4105

	z := s.zoneMap.Get(0, 3094, 3106)
	obj := entitypkg.NewObj(0, 3094, 3106, entitypkg.LifecycleDespawn, 526, 1)
	obj.ReceiverID = owner.uid
	z.Objs = append(z.Objs, obj)

	received := drainConn(t, otherCC)
	other.writeFullFollows(z, 1)
	other.client.flushWrite()
	got := <-received
	// Expect only the FullFollows header (3 bytes); no PartialFollows
	// wrapper, no ObjAdd. Mirrors TestWriteFullFollowsSkipsThisTickTransitions
	// header-only baseline.
	if len(got) != 3 {
		t.Errorf("non-owner replay must produce header-only (3 bytes); got %d (%v)", len(got), got)
	}
}
```

- [ ] **Step 2: Run the test to confirm it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFullFollowsHidesPrivateDropFromNonOwnerInReplay -v`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add modules/world/player_zone_test.go
git commit --no-gpg-sign -m "test(nai-129): full-follows replay non-owner absence-pin (Task 5)

Absence-pin: a second player loading a zone where a private drop
already exists for a different UID gets only the FullFollows header
in the replay, no PartialFollows wrapper, no ObjAdd."
```

---

## Task 6: Tighten doc comments to UID-space

**Files:**
- Modify: `pkg/zone/event.go:30` (field comment)
- Modify: `pkg/entity/obj.go:16` (field comment — current text incorrectly says "slot")
- Modify: `modules/world/world_zone.go:123-124` (Server.AddObj doc)
- Modify: `modules/world/player_zone.go:14-20, 67-69` (writeFullFollows + writePartialFollows doc preambles)

No behavior change. Documents UID-space semantics so future audits don't re-introduce the slot/UID confusion.

- [ ] **Step 1: Update `pkg/zone/event.go:30`**

Replace the line:

```go
	ReceiverID int // PublicReceiver = -1 for Enclosed events and public Follows
```

with:

```go
	// ReceiverID is UID-space — mirrors TS Engine-TS Zone.ts ZoneEvent.receiver64.
	// PublicReceiver (-1) for Enclosed events and public Follows; otherwise the
	// owning player's UID per modules/world.composeUID(username37, slot).
	ReceiverID int
```

- [ ] **Step 2: Update `pkg/entity/obj.go:16`**

Replace the line:

```go
	ReceiverID int // -1 = public; else the owning player's slot
```

with:

```go
	// ReceiverID is UID-space — mirrors TS Engine-TS entity/Obj.ts receiver64.
	// PublicReceiver (-1) for public drops; else the owning player's UID per
	// modules/world.composeUID(username37, slot). Set by worldVarsView.AddObj
	// at modules/world/server_varp.go:169 for private drops.
	ReceiverID int
```

- [ ] **Step 3: Update `modules/world/world_zone.go:123-124`**

Replace the doc comment:

```go
// AddObj routes a ground-item spawn. receiverID == zone.PublicReceiver for
// public drops; otherwise the receiver's player slot.
```

with:

```go
// AddObj routes a ground-item spawn. receiverID == zone.PublicReceiver for
// public drops; otherwise the receiving player's UID per composeUID. The
// per-player delivery filter at player_zone.go (writeFullFollows /
// writePartialFollows) compares this against p.uid, mirroring TS
// Engine-TS Zone.ts:138, 190 (obj.receiver64 vs player.hash64).
```

- [ ] **Step 4: Update `modules/world/player_zone.go:14-20`**

Replace the doc preamble for `writeFullFollows`:

```go
// writeFullFollows sends UpdateZoneFullFollows (client zone reset) followed
// by a PartialFollows wrapper + synthesized per-entity messages replaying
// every currently-active dynamic loc/obj in the zone. Entities transitioned
// THIS tick are skipped — the Enclosed buffer already carries their change.
//
// TODO(beyond-4b): handle Respawn-lifecycle (static) loc branches once
// static loading from cache maps is wired up.
```

with:

```go
// writeFullFollows sends UpdateZoneFullFollows (client zone reset) followed
// by a PartialFollows wrapper + synthesized per-entity messages replaying
// every currently-active dynamic loc/obj in the zone. Entities transitioned
// THIS tick are skipped — the Enclosed buffer already carries their change.
//
// Private drops are filtered by obj.ReceiverID against p.uid, matching TS
// Engine-TS Zone.ts:138 (obj.receiver64 vs player.hash64). PublicReceiver
// drops are visible to all observers.
//
// TODO(beyond-4b): handle Respawn-lifecycle (static) loc branches once
// static loading from cache maps is wired up.
```

- [ ] **Step 5: Update `modules/world/player_zone.go:67-69`**

Replace the doc preamble for `writePartialFollows`:

```go
// writePartialFollows iterates the zone's per-tick Follows events, filtered
// by recipient, emitting a PartialFollows header once (if any match) then
// each event as its own top-level zone-nested packet.
```

with:

```go
// writePartialFollows iterates the zone's per-tick Follows events, filtered
// by recipient, emitting a PartialFollows header once (if any match) then
// each event as its own top-level zone-nested packet.
//
// Recipient filter compares e.ReceiverID against p.uid, matching TS
// Engine-TS Zone.ts:190 (event.receiver64 vs player.hash64). PublicReceiver
// events are delivered to all observers.
```

- [ ] **Step 6: Run the full test sweep to confirm no behavior regression from the doc edits**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS across the entire module.

- [ ] **Step 7: Commit**

```bash
git add pkg/zone/event.go pkg/entity/obj.go modules/world/world_zone.go modules/world/player_zone.go
git commit --no-gpg-sign -m "docs(nai-129): clarify UID-space semantics for ReceiverID (Task 6)

No behavior change. Tightens field/method doc comments at pkg/zone/event.go,
pkg/entity/obj.go (current text said 'slot' — incorrect), modules/world/world_zone.go,
and modules/world/player_zone.go to declare UID-space throughout, citing
TS Engine-TS Zone.ts:138, 190 as the canonical comparator."
```

---

## Task 7: Final test sweep + smoke handoff resume prompt

**Files:** none modified.

This is the close task. Confirms both filter sites green, both doc-comment paths consistent, and emits the smoke-handoff resume prompt for the user.

- [ ] **Step 1: Run the full test sweep with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: PASS.

- [ ] **Step 2: Verify the production diff**

Run: `git diff main -- modules/world/player_zone.go`

Expected diff (production code only — doc-comment changes are separate):

- Line 41: `p.slot` → `p.uid`
- Line 76: `p.slot` → `p.uid`

Plus the doc-comment additions from Task 6.

- [ ] **Step 3: Verify all 5 NAI-129 tests are present and green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestPartialFollowsFiltersByReceiverID|TestPartialFollowsDeliversPrivateDropToOwnerByUID|TestPartialFollowsHidesPrivateDropFromNonOwnerByUID|TestFullFollowsReplaysPrivateDropToOwnerByUID|TestFullFollowsHidesPrivateDropFromNonOwnerInReplay" -v`

Expected: 5 PASS lines.

- [ ] **Step 4: Verify the existing NAI-128 cascade integration test still passes (regression check)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestNAI128 -v` (if no exact name match, run the test file directly: `go test ./modules/world/... -run CascadeDispatchTrace -v`).

Expected: PASS — NAI-128's G6 gateway probe assertions are independent of the L7 fix.

- [ ] **Step 5: Smoke handoff (user-launched)**

The smoke is run by the user, not the implementer subagent. Per `smoke_test_server_handoff` memory: Claude's sandboxed process is unreachable from the host Java client. The implementer should NOT attempt to launch the server.

After all tasks pass, emit the resume prompt for the user, asking them to:

1. Build & run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml`
2. Login Lumbridge newbieman; attack a Citizen until kill.
3. Verify in client UI: bones (id=526, ×1) + coins (id=995, ×3) appear at the kill tile.
4. Capture the NodeDebug `nai128.obj.add` log lines and report them back.

- [ ] **Step 6: Final close commit (after smoke confirms loot visible)**

If smoke passes, write a close commit citing:
- The two pre-fix smoke G6 lines from `6599a15` (verbatim).
- The two post-fix smoke G6 lines (same shape, plus client-visible loot).
- A `Closes memory:` trailer per `close_commit_memory_trailer` listing any non-derivable lessons (e.g. UID-vs-slot identifier-space audit pattern, if novel).

If smoke fails, do NOT close — open NAI-130 with the new symptom per the spec §4 failure routing.

---

## Self-Review

**1. Spec coverage:**

| Spec section | Plan task |
|---|---|
| §2 production fix `player_zone.go:76` (writePartialFollows) | Task 1 Step 3 |
| §2 production fix `player_zone.go:41` (writeFullFollows replay) | Task 4 Step 3 |
| §2 doc-comment `pkg/zone/event.go:30` | Task 6 Step 1 |
| §2 doc-comment `modules/world/world_zone.go:123` | Task 6 Step 3 |
| §2 doc-comment `modules/world/player_zone.go:14-20, 67-69` | Task 6 Steps 4, 5 |
| §2 (additional) doc-comment `pkg/entity/obj.go:16` (current text says "slot") | Task 6 Step 2 |
| §3 rewrite TestPartialFollowsFiltersByReceiverID | Task 1 Step 1 |
| §3 new TestPartialFollowsDeliversPrivateDropToOwnerByUID | Task 2 |
| §3 new TestPartialFollowsHidesPrivateDropFromNonOwnerByUID | Task 3 |
| §3 new TestFullFollowsReplaysPrivateDropToOwnerByUID | Task 4 Step 1 |
| §3 new TestFullFollowsHidesPrivateDropFromNonOwnerInReplay | Task 5 |
| §3 fixture utility (extend or use direct `p.uid` assignment) | Convention block above task list — direct assignment chosen, no helper change |
| §4 smoke handoff | Task 7 Step 5 |
| §8 close criteria | Task 7 Step 6 |

All §2, §3, §4, §8 items mapped. §5 risk register and §6 deviations are advisory; no per-task work.

**2. Placeholder scan:** Reviewed all "Step N" entries. Every code step shows complete code blocks. No "TBD", no "fill in similar", no "add appropriate validation". ✓

**3. Type consistency:**

- `composeUID(username37 uint64, slot int) int` — used identically across Tasks 1, 2, 3, 4, 5.
- `entitypkg.NewObj(level, x, z, lc, typ, count)` — 6 args, used identically across Tasks 1-5.
- `obj.ReceiverID = ...` — used in Tasks 4, 5 only (where the obj does not flow through `worldVarsView.AddObj`).
- `s.AddObj(obj, receiverID)` — used in Tasks 1, 2, 3 (per-tick Follows path).
- `p.writePartialFollows(z)` and `p.writeFullFollows(z, currentTick)` — signatures unchanged.
- All test names follow the convention from §3 of the spec.

No name drift between tasks. ✓

**4. Spec-test-coverage cross-check** (per `plan_test_coverage_crosscheck` memory): every spec test in §3 maps to a Task body that contains its full Go source. ✓

**5. Plan-codified test fixture runnability** (per `plan_runnable_test_fixtures` memory): walked each test fixture mentally — `newZoneTestPlayer` populates `p.client`, `p.x/z/level`, `p.originX/Z`, and `s.players[slot]`/`s.playerLoop`; tests then directly assign `p.uid` and add an Obj. The `received := drainConn(t, cc); ...write...; <-received` pattern matches the existing test that this rewrite is based on (`TestPartialFollowsFiltersByReceiverID` at HEAD lines 173-176). ✓
