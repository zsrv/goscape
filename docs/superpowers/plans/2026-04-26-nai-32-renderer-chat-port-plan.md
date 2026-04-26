# NAI-32 Implementation Plan — Renderer dual-cache CHAT port + rs-server-225 citation sweep

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port upstream `info.rs:289-291` self-CHAT-mask suppression into goscape's eager-cache renderer architecture by adding a parallel `highDefWithChat` cache variant consumed by `writePlayers` for tracked-other reads (sole HighDefOf swap site for non-self players); fix a latent header/payload mismatch in `buildPayload` by stripping CHAT from the mask set when chat-suppression is requested; drop the now-redundant `suppressChat` arg from `writeMaskPayloads`. Sweep 3 unauthorized `rs-server-225` provenance citations from production source files. Close NAI-30-D2 (deviation count 14 → 13).

**Architecture:** Renderer gains a parallel high-def cache slot (`highDefWithChat [2048][]byte`) populated alongside `highDef` in `ComputePlayers`. `buildPayload` strips CHAT from the mask set BEFORE both `writeMaskHeader` and `writeMaskPayloads` when `suppressChat=true`, fixing the latent header/payload mismatch as a side effect. `writeLocalPlayer` reads `HighDefOf` (chat-stripped, for self per `info.rs:289-291`); `writePlayers` reads `HighDefWithChatOf` (CHAT preserved, for tracked others); `writeNewPlayers` is unchanged (reads only the low-def caches). 2-client public-chat smoke gate post-Bundle-1 binds feature correctness.

**Tech Stack:** Go 1.26+; `LostCityRS/Engine-TS` (TS canonical for non-`pkg/rsbuf`); `2004scape/rsbuf` branch 225 (Rust canonical for `pkg/rsbuf`); `LostCityRS/Client-Java` (binding wire spec).

**Spec:** `docs/superpowers/specs/2026-04-26-nai-32-renderer-chat-port-design.md` (commit `6aa41e6` after `writeNewPlayers` correction).

---

## File structure

| File | Bundle | Change kind | Approx LOC delta |
|---|---|---|---|
| `pkg/rsbuf/renderer.go` | 1 | `+ highDefWithChat` field; `+ HighDefWithChatOf` accessor; dual-build in `ComputePlayers`; `+ masks &^= MaskChat` strip step in `buildPayload` | +20 / −1 |
| `pkg/rsbuf/mask_payload.go` | 1 | drop `suppressChat bool` param + `&& !suppressChat` guard | +0 / −3 |
| `pkg/rsbuf/mask_payload_test.go` | 1 | drop `suppressChat` arg from existing call sites; add 1 new test for header/payload consistency | +25 / −5 (4 existing call sites need arg drop + 1 new test) |
| `pkg/rsbuf/playerinfo.go` | 1 | strike NAI-30-D2 doc-comment block (lines 113-124); 1 swap site at line 246; brief inline cite | +1 / −12 |
| `pkg/rsbuf/playerinfo_test.go` | 1 | un-skip + implement `TestPlayerInfo_LocalPlayer_ChatMaskStripped` | +60 / −1 |
| `pkg/rsbuf/renderer_test.go` | 1 | + 3 new tests pinning dual-cache contract | +60 |
| `pkg/script/file.go` | 2 | doc-comment edit (line 40) | +1 / −1 |
| `pkg/zone/grid.go` | 2 | doc-comment edit (lines 3-4 → 1 line) | +1 / −2 |
| `pkg/objtype/npctype.go` | 2 | doc-comment edits (lines 25, 36) | +2 / −2 |

**Net:** Bundle 1 ~+170 / −22 LOC; Bundle 2 ~+4 / −5 LOC.

---

## Bundle 0 — Controller pre-flight (no commits, no implementer dispatch)

This is controller-side verification before dispatching the Bundle 1 implementer subagent. No code changes, no commits.

- [ ] **Step 0.1: Re-grep all 3 Bundle 2 citation sites at HEAD**

```bash
rg -n "rs-server-225" pkg/ modules/ cmd/
```

Expected: exactly 4 lines:
```
pkg/script/file.go:40:...
pkg/zone/grid.go:3:...
pkg/objtype/npctype.go:25:...
pkg/objtype/npctype.go:36:...
```

If a 5th site surfaces, flag as scope-expansion; update Bundle 2 task list before dispatch.

- [ ] **Step 0.2: Verify 5 D2 touchpoint lines at HEAD**

```bash
sed -n '113,124p' pkg/rsbuf/playerinfo.go    # NAI-30-D2 block
sed -n '125,140p' pkg/rsbuf/playerinfo.go    # writeLocalPlayer header + line 128 HighDefOf
sed -n '208,250p' pkg/rsbuf/playerinfo.go    # writePlayers header + line 246 HighDefOf
sed -n '301,360p' pkg/rsbuf/playerinfo.go    # writeNewPlayers header + lines 321+352 LowDef reads
sed -n '20,55p' pkg/rsbuf/renderer.go        # ComputePlayers + 3 buildPayload sites (36, 47, 53)
sed -n '122,128p' pkg/rsbuf/renderer.go      # buildPayload helper
```

Expected: line numbers match the spec. If they've drifted, update task code blocks below to match HEAD.

- [ ] **Step 0.3: Enumerate ALL `writeMaskPayloads(` call sites at HEAD per `enumerate_all_sites.md`**

```bash
rg -n 'writeMaskPayloads\(' pkg/ modules/ cmd/
```

Expected: 2 hits — `pkg/rsbuf/renderer.go:125` (production) + `pkg/rsbuf/mask_payload_test.go` lines 80, 95, 108, 125, 141 (test fixtures, all using `suppressChat=false` shape).

If a 3rd production caller surfaces, flag as scope-expansion: Task 1's signature drop must update it too.

- [ ] **Step 0.4: Verify low-def-stays-single-cache assumption per `spec_test_runtime_behavior_verify.md`**

Read the FULL `info.rs::lowdefinition()` body in `/home/owner/Code/github.com/2004scape/rsbuf/src/info.rs` starting at line 296. Search for any `CHAT` reference inside the function body.

Expected: zero CHAT references in `lowdefinition()`. The function only manipulates APPEARANCE / FACE_ENTITY / FACE_COORD / orientation-fallback masks.

If CHAT IS branched in low-def, plan must extend the dual-cache pattern to `lowDefFull` and `lowDefNoApp`. Stop and re-spec.

- [ ] **Step 0.5: Verify R8 — immediate `Player.say` echo path does NOT read from `HighDefOf`**

```bash
rg -n 'HighDefOf|HighDefWithChatOf' modules/world/
```

Expected: zero hits in `modules/world/` outside `player_npc_info.go` (which calls the encoder, not the cache directly). The immediate `Player.say` channel uses `MaskSay` (a separate mask from `MaskChat`) and writes through `writeSay` in `mask_payload.go:56-61` — distinct from `writeChat`. Self's own-chat rendering should come from a separate local-display-layer path or from `MaskSay`, not from the high-def cache.

If `HighDefOf` IS read in `modules/world/`, NAI-32 needs a 4th cache variant. Stop and re-spec.

- [ ] **Step 0.6: Pre-pull canonical TS file paths for Bundle 2 citation replacements**

```bash
ls /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/ScriptFile.ts \
   /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/zone/ZoneGrid.ts \
   /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/MoveRestrict.ts \
   /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/BlockWalk.ts
```

Expected: all 4 files exist. If any path is wrong, update Bundle 2 Task 5 below.

---

## Bundle 1 — D2 dual-cache port

### Task 1: buildPayload consistency fix + writeMaskPayloads signature drop

**Files:**
- Modify: `pkg/rsbuf/renderer.go:122-128` (buildPayload helper)
- Modify: `pkg/rsbuf/mask_payload.go:21-49` (writeMaskPayloads signature drop + body simplification)
- Modify: `pkg/rsbuf/mask_payload_test.go:80,95,108,125,141` (drop `suppressChat` arg from 5 call sites)
- Test: `pkg/rsbuf/mask_payload_test.go` (1 new test)

- [ ] **Step 1.1: Write the failing test**

Append to `pkg/rsbuf/mask_payload_test.go`:

```go
// TestBuildPayload_HeaderPayloadConsistent_ChatStripped pins the
// invariant that buildPayload's chat-strip path produces a header
// AND payload that are mutually consistent: when CHAT is stripped
// from the body, the header byte must NOT advertise the CHAT bit.
//
// Without the fix at buildPayload (`if suppressChat { masks &^= MaskChat }`),
// writeMaskHeader writes the CHAT bit but writeMaskPayloads omits the
// CHAT body — the receiving client mis-parses (reads CHAT header bit,
// expects body, consumes the next player's bytes). NAI-32 surfaces and
// retires this latent bug.
func TestBuildPayload_HeaderPayloadConsistent_ChatStripped(t *testing.T) {
	p := &fakeSource{
		masks:      MaskChat | MaskAnim,
		animID:     0x1234,
		animDelay:  5,
		chatColour: 1,
		chatEffect: 2,
		chatRights: 3,
		chatBytes:  []byte("hello"),
	}
	out := buildPayload(p, MaskChat|MaskAnim, true)

	// MaskAnim = 0x2, MaskChat = 0x40. Sum = 0x42 < 0x100 → 1-byte header.
	// After strip, header byte should be MaskAnim only = 0x2.
	if len(out) == 0 {
		t.Fatalf("buildPayload returned empty; expected at least header byte")
	}
	if out[0]&byte(MaskChat) != 0 {
		t.Errorf("header has CHAT bit set: got 0x%02x; want CHAT bit clear", out[0])
	}
	if out[0] != byte(MaskAnim) {
		t.Errorf("header byte: got 0x%02x, want 0x%02x (MaskAnim only)", out[0], MaskAnim)
	}

	// Payload must be anim-only: P2(0x1234) + P1Alt3(5).
	// P1Alt3(5) writes (-5)&0xff = 0xfb (per existing TestAnimPayload).
	want := []byte{byte(MaskAnim), 0x12, 0x34, 0xfb}
	if len(out) != len(want) {
		t.Fatalf("payload length: got %d, want %d (header + 3 anim bytes); bytes %#v", len(out), len(want), out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x (full=%#v)", i, out[i], want[i], out)
		}
	}
}
```

- [ ] **Step 1.2: Run the test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestBuildPayload_HeaderPayloadConsistent_ChatStripped -v
```

Expected: FAIL. The current `buildPayload` writes header byte = `MaskAnim | MaskChat = 0x42` because `writeMaskHeader` writes ALL bits in `masks`. Test asserts header byte == `MaskAnim = 0x2`, which fails.

- [ ] **Step 1.3: Apply the buildPayload + writeMaskPayloads fix**

Edit `pkg/rsbuf/renderer.go:122-128`:

```go
// Replace:
func buildPayload(p PlayerSource, masks int, suppressChat bool) []byte {
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, masks)
	writeMaskPayloads(buf, p, masks, suppressChat)
	// packet.Packet writes append to Data; Pos is the read cursor and stays 0.
	return append([]byte(nil), buf.Data...)
}

// With:
func buildPayload(p PlayerSource, masks int, suppressChat bool) []byte {
	if suppressChat {
		masks &^= MaskChat // CHAT bit stripped per info.rs:289-291; header AND payload omit CHAT
	}
	buf := packet.NewPacket(nil)
	writeMaskHeader(buf, masks)
	writeMaskPayloads(buf, p, masks)
	// packet.Packet writes append to Data; Pos is the read cursor and stays 0.
	return append([]byte(nil), buf.Data...)
}
```

Edit `pkg/rsbuf/mask_payload.go:15-49`:

```go
// Replace the doc-comment block + signature + body:
// writeMaskPayloads writes mask payloads in rsbuf's fixed order:
// ANIM -> SAY -> EXACT_MOVE -> FACE_ENTITY -> FACE_COORD -> SPOT_ANIM ->
// APPEARANCE -> DAMAGE -> CHAT.
//
// forceMasks is the effective mask set to write (may differ from p.Masks() for
// low-def variants). Callers requesting CHAT suppression must pre-strip
// MaskChat from forceMasks before calling (see buildPayload at renderer.go:122).
func writeMaskPayloads(buf *packet.Packet, p PlayerSource, forceMasks int) {
	if forceMasks&MaskAnim != 0 {
		writeAnim(buf, p)
	}
	if forceMasks&MaskSay != 0 {
		writeSay(buf, p)
	}
	if forceMasks&MaskExactMove != 0 {
		writeExactMove(buf, p)
	}
	if forceMasks&MaskFaceEntity != 0 {
		writeFaceEntity(buf, p)
	}
	if forceMasks&MaskFaceCoord != 0 {
		writeFaceCoord(buf, p)
	}
	if forceMasks&MaskSpotAnim != 0 {
		writeSpotAnim(buf, p)
	}
	if forceMasks&MaskAppearance != 0 {
		writeAppearance(buf, p)
	}
	if forceMasks&MaskDamage != 0 {
		writeDamage(buf, p)
	}
	if forceMasks&MaskChat != 0 {
		writeChat(buf, p)
	}
}
```

Update 5 call sites in `pkg/rsbuf/mask_payload_test.go` — drop the trailing `, false` (or `, true` if any) arg:

- Line 80: `writeMaskPayloads(buf, p, MaskAnim, false)` → `writeMaskPayloads(buf, p, MaskAnim)`
- Line 95: `writeMaskPayloads(buf, p, MaskFaceCoord, false)` → `writeMaskPayloads(buf, p, MaskFaceCoord)`
- Line 108: `writeMaskPayloads(buf, p, MaskAppearance, false)` → `writeMaskPayloads(buf, p, MaskAppearance)`
- Line 125: `writeMaskPayloads(buf, p, MaskChat, false)` → `writeMaskPayloads(buf, p, MaskChat)`
- Line 141: `writeMaskPayloads(buf, p, MaskDamage, false)` → `writeMaskPayloads(buf, p, MaskDamage)`

(Bundle 0 Step 0.3's enumerate output should match this exact list. If it doesn't, update accordingly before this step.)

- [ ] **Step 1.4: Run the failing test to verify it now passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestBuildPayload_HeaderPayloadConsistent_ChatStripped -v
```

Expected: PASS.

- [ ] **Step 1.5: Run the full pkg/rsbuf test suite (cache-busted)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -count=1
```

Expected: ALL PASS. Per `latent_bug_at_migration_boundary.md`: any test that flips RED here is investigated as a possible latent-bug surface. Particular candidates: tests pinning byte shapes against fixtures with `MaskChat` set + assertion on header byte. None are currently expected.

- [ ] **Step 1.6: Run the full repository test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: ALL PASS. (Unrelated packages should not be affected by `pkg/rsbuf` changes.)

- [ ] **Step 1.7: Commit**

```bash
git add pkg/rsbuf/renderer.go pkg/rsbuf/mask_payload.go pkg/rsbuf/mask_payload_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(rsbuf): NAI-32 Task 1 — buildPayload chat-strip consistency + drop suppressChat arg

Pre-NAI-32 buildPayload(p, masks, suppressChat=true) wrote the
CHAT bit in the mask header (writeMaskHeader does not consult
suppressChat) but omitted the CHAT body bytes (writeMaskPayloads
gated only the body write at line 46). Receiving client mis-parses:
reads CHAT header bit, expects body, consumes next player's bytes.
Bug was un-pinned by tests and undiscovered through NAI-31's smoke
because no chatting player participated in the smoke.

Fix: buildPayload now strips MaskChat from the mask set BEFORE both
writeMaskHeader and writeMaskPayloads when suppressChat is true.
Header and payload are mutually consistent.

writeMaskPayloads's suppressChat parameter is now redundant (the bit
is pre-stripped from forceMasks). Dropped the parameter and the
&& !suppressChat guard at line 46. Updated 5 call sites in
mask_payload_test.go.

Pinned forward by TestBuildPayload_HeaderPayloadConsistent_ChatStripped.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 1.8: Verify commit content matches stated diff per `implementer_commit_content_verify.md`**

Run:
```bash
git show HEAD --stat
git status
```

Expected: stat shows exactly 3 files changed (renderer.go, mask_payload.go, mask_payload_test.go); status shows clean working tree.

---

### Task 2: Dual-cache infrastructure

**Files:**
- Modify: `pkg/rsbuf/renderer.go:7-14` (Renderer struct), `:20-55` (ComputePlayers), `:58-79` (accessors)
- Test: `pkg/rsbuf/renderer_test.go` (3 new tests)

- [ ] **Step 2.1: Write the 3 failing tests**

Append to `pkg/rsbuf/renderer_test.go`:

```go
// TestComputePlayers_DualHighDef_ChatPresent pins the dual-cache
// contract for a player with CHAT in their masks: HighDefOf yields
// chat-stripped bytes (correct for self-read at writeLocalPlayer),
// HighDefWithChatOf yields chat-preserved bytes (correct for
// tracked-other read at writePlayers).
func TestComputePlayers_DualHighDef_ChatPresent(t *testing.T) {
	p := &fakeSource{
		slot:       5,
		masks:      MaskChat | MaskAnim,
		animID:     0x1234,
		animDelay:  5,
		chatColour: 1,
		chatEffect: 2,
		chatRights: 3,
		chatBytes:  []byte("yo"),
	}
	r := NewRenderer()
	r.ComputePlayers([]PlayerSource{p})

	stripped := r.HighDefOf(5)
	withChat := r.HighDefWithChatOf(5)

	if stripped == nil {
		t.Fatalf("HighDefOf(5) is nil; expected chat-stripped bytes")
	}
	if withChat == nil {
		t.Fatalf("HighDefWithChatOf(5) is nil; expected chat-preserved bytes")
	}

	// Header byte: chat-stripped should be MaskAnim only (0x2);
	// chat-preserved should be MaskAnim | MaskChat (0x42).
	if stripped[0] != byte(MaskAnim) {
		t.Errorf("HighDefOf header: got 0x%02x, want 0x%02x (MaskAnim only)", stripped[0], MaskAnim)
	}
	if withChat[0] != byte(MaskAnim|MaskChat) {
		t.Errorf("HighDefWithChatOf header: got 0x%02x, want 0x%02x (MaskAnim|MaskChat)", withChat[0], MaskAnim|MaskChat)
	}

	// Length: stripped is 1 (header) + 3 (anim) = 4 bytes.
	// With-chat is 4 + 6 (chat: colour + effect + rights + len + 2 chars) = 10 bytes.
	// Per existing TestChatPayload at mask_payload_test.go:122 — chat body for "yo" is
	// p1(1) p1(2) p1_alt2(3)=125 p1_alt1(2)=130 pdata_alt2('y','o')={7,17}.
	if len(stripped) != 4 {
		t.Errorf("HighDefOf length: got %d, want 4 (header + anim); bytes %#v", len(stripped), stripped)
	}
	if len(withChat) != 10 {
		t.Errorf("HighDefWithChatOf length: got %d, want 10 (header + anim + chat); bytes %#v", len(withChat), withChat)
	}
}

// TestComputePlayers_DualHighDef_NoChat_Identical pins that the
// dual-cache change does not drift non-CHAT outputs: when masks does
// not include MaskChat, both cache variants are byte-identical.
func TestComputePlayers_DualHighDef_NoChat_Identical(t *testing.T) {
	p := &fakeSource{slot: 5, masks: MaskAnim, animID: 100, animDelay: 2}
	r := NewRenderer()
	r.ComputePlayers([]PlayerSource{p})

	stripped := r.HighDefOf(5)
	withChat := r.HighDefWithChatOf(5)

	if stripped == nil || withChat == nil {
		t.Fatalf("both cache variants must be non-nil for masks=MaskAnim; got stripped=%v, withChat=%v", stripped, withChat)
	}
	if !bytes.Equal(stripped, withChat) {
		t.Errorf("non-CHAT masks should produce byte-identical caches:\nHighDefOf:         %#v\nHighDefWithChatOf: %#v", stripped, withChat)
	}
}

// TestComputePlayers_DualHighDef_MasksZero_BothNil pins the
// no-mask case: both cache variants are nil so encoders take the
// idle path with no orphan mask-header byte.
func TestComputePlayers_DualHighDef_MasksZero_BothNil(t *testing.T) {
	p := &fakeSource{slot: 5, masks: 0, entityMask: 0}
	r := NewRenderer()
	r.ComputePlayers([]PlayerSource{p})

	if r.HighDefOf(5) != nil {
		t.Errorf("HighDefOf(5) for masks=0: got %#v, want nil", r.HighDefOf(5))
	}
	if r.HighDefWithChatOf(5) != nil {
		t.Errorf("HighDefWithChatOf(5) for masks=0: got %#v, want nil", r.HighDefWithChatOf(5))
	}
}
```

The `bytes` import is required. Verify the import block at the top of `renderer_test.go` includes `"bytes"`. If not, add it.

- [ ] **Step 2.2: Run the failing tests to verify they fail (compile error)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestComputePlayers_DualHighDef -v
```

Expected: COMPILE ERROR — `r.HighDefWithChatOf` is undefined. (Tests cannot run.)

- [ ] **Step 2.3: Implement the dual cache**

Edit `pkg/rsbuf/renderer.go:7-14` (the `Renderer` struct):

```go
// Replace:
type Renderer struct {
	highDef     [2048][]byte
	lowDefFull  [2048][]byte // includes forced APPEARANCE + FACE_COORD
	lowDefNoApp [2048][]byte // forces FACE_COORD but NOT APPEARANCE

	npcHighDef [8192][]byte
	npcLowDef  [8192][]byte // forces FACE_COORD baseline
}

// With:
type Renderer struct {
	highDef         [2048][]byte // CHAT stripped from header AND payload (consumed by writeLocalPlayer for self per info.rs:289-291)
	highDefWithChat [2048][]byte // CHAT preserved (consumed by writePlayers for tracked others)
	lowDefFull      [2048][]byte // includes forced APPEARANCE + FACE_COORD
	lowDefNoApp     [2048][]byte // forces FACE_COORD but NOT APPEARANCE

	npcHighDef [8192][]byte
	npcLowDef  [8192][]byte // forces FACE_COORD baseline
}
```

Edit `pkg/rsbuf/renderer.go:32-37` (the high-def block in `ComputePlayers`):

```go
// Replace:
		if masks == 0 {
			r.highDef[slot] = nil
		} else {
			r.highDef[slot] = buildPayload(p, masks, true)
		}

// With:
		if masks == 0 {
			r.highDef[slot] = nil
			r.highDefWithChat[slot] = nil
		} else {
			r.highDef[slot] = buildPayload(p, masks, true)         // CHAT stripped (self per info.rs:289-291)
			r.highDefWithChat[slot] = buildPayload(p, masks, false) // CHAT preserved (tracked others)
		}
```

(The `lowDefFull` and `lowDefNoApp` blocks at lines 39-53 stay unchanged. NPC cache at lines 82-104 stays unchanged.)

Add the `HighDefWithChatOf` accessor immediately after `HighDefOf` (insert at line 64, after the existing accessor's closing brace):

```go
// HighDefWithChatOf returns the high-def mask payload bytes with CHAT
// preserved (nil if no masks). Consumed by writePlayers for
// tracked-other reads — other players' chat is preserved per upstream
// info.rs::write_blocks (only self strips CHAT per info.rs:289-291).
func (r *Renderer) HighDefWithChatOf(slot int) []byte {
	if slot < 1 || slot >= len(r.highDefWithChat) {
		return nil
	}
	return r.highDefWithChat[slot]
}
```

- [ ] **Step 2.4: Run the failing tests to verify they now pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestComputePlayers_DualHighDef -v
```

Expected: 3 PASS.

- [ ] **Step 2.5: Run the full pkg/rsbuf test suite (cache-busted)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -count=1
```

Expected: ALL PASS.

- [ ] **Step 2.6: Commit**

```bash
git add pkg/rsbuf/renderer.go pkg/rsbuf/renderer_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-32 Task 2 — dual high-def cache (highDefWithChat)

Renderer gains a parallel high-def cache slot populated alongside
highDef in ComputePlayers:
  highDef         — CHAT stripped (self read per info.rs:289-291)
  highDefWithChat — CHAT preserved (tracked-other read; new in NAI-32)

HighDefWithChatOf accessor mirrors HighDefOf shape (bounds check,
nil on OOB).

writePlayers swap to consume HighDefWithChatOf is in Task 3.
writeNewPlayers reads only the low-def caches and is unchanged.
Low-def caches stay single-variant per info.rs::lowdefinition()
(no self-vs-other CHAT branching).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 2.7: Verify commit content**

```bash
git show HEAD --stat
git status
```

Expected: stat shows 2 files changed (renderer.go, renderer_test.go); clean working tree.

---

### Task 3: writePlayers swap + un-skip CHAT-strip test + strike NAI-30-D2 doc-comment block

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go:113-124` (strike NAI-30-D2 doc block)
- Modify: `pkg/rsbuf/playerinfo.go:246` (writePlayers swap)
- Test: `pkg/rsbuf/playerinfo_test.go:572-583` (un-skip + implement TestPlayerInfo_LocalPlayer_ChatMaskStripped)

- [ ] **Step 3.1: Un-skip and implement the failing test**

**Two-types caveat per `mock_recorder_field_naming_check.md`:**
- `*pkg/rsbuf.Player` (in `pkg/rsbuf/player.go:14`) — what `b.players[N]` returns. Has `Chat *Chat`, `Masks uint32`, etc. Does NOT directly satisfy `rsbuf.PlayerSource`.
- `*modules/world.Player` (in `modules/world/player_source.go:29`) — satisfies `rsbuf.PlayerSource` via `ChatColour()` / `ChatEffect()` / `ChatRights()` / `ChatBytes()` methods reading lowercase fields.
- `*rsbuf.fakeSource` (in `pkg/rsbuf/mask_payload_test.go:10`) — the in-package fixture for `ComputePlayers` tests; satisfies `rsbuf.PlayerSource`. Existing renderer tests at `renderer_test.go:7,19,37,55` all use `*fakeSource` for cache population.

**Encoder-cache decoupling at HEAD:** the encoder reads movement state (`Tele`, `RunDir`, `WalkDir`) from `b.players[N]` (a `*rsbuf.Player`) and reads mask-payload bytes from the `Renderer`'s cache (populated separately via `r.ComputePlayers([]PlayerSource{...})`). The two paths are independent. To exercise the chat-strip path, populate the renderer cache via `*fakeSource` instances matching the Buf players' slots, and let `setupLocalPlayer` produce idle players (movement sentinels at -1 by default per `newPlayer()` in `pkg/rsbuf/player.go:63-93`) so the encoder takes the mask-only-update branch (`case hdLen > 0` at `playerinfo.go:278-283` for tracked others; analogous mask-only branch in `writeLocalPlayer`).

Edit `pkg/rsbuf/playerinfo_test.go:572-583` (the existing skipped test block — the doc-comment lines 572-580 stay; replace lines 581-583 body):

```go
// Replace existing body:
func TestPlayerInfo_LocalPlayer_ChatMaskStripped(t *testing.T) {
	t.Skip("NAI-30-D2: requires renderer cache port for per-mask suppression; audited NAI-31, deferred to NAI-32")
}

// With:
func TestPlayerInfo_LocalPlayer_ChatMaskStripped(t *testing.T) {
	b := New()
	// Self at PID=1 + tracked other at PID=2. Both at (3200, 0, 3200) — same
	// tile, well within ViewDistance. Movement sentinels stay at -1 (per
	// newPlayer() defaults), so the encoder takes the mask-only-update branch.
	setupLocalPlayer(b, 1, nil)
	setupLocalPlayer(b, 2, nil)

	// Pre-track PID=2 in self.Build.Players so writePlayers (not writeNewPlayers)
	// handles the encode for the tracked-other dual-pin assertion.
	b.players[1].Build.Players.Insert(2)

	// Renderer cache populated via fakeSource fixtures (existing pattern at
	// renderer_test.go:7,19,37,55). Distinct chat strings per slot for the
	// encoder-level bytes.Contains pin.
	fakeSelf := &fakeSource{
		slot:       1,
		masks:      MaskChat,
		chatColour: 7,
		chatEffect: 0,
		chatRights: 0,
		chatBytes:  []byte("self"),
	}
	fakeOther := &fakeSource{
		slot:       2,
		masks:      MaskChat,
		chatColour: 8,
		chatEffect: 0,
		chatRights: 0,
		chatBytes:  []byte("other"),
	}
	r := NewRenderer()
	r.ComputePlayers([]PlayerSource{fakeSelf, fakeOther})

	// Cache-layer pin: self's HighDefOf has CHAT stripped, other's
	// HighDefWithChatOf has CHAT preserved.
	selfStripped := r.HighDefOf(1)
	otherWithChat := r.HighDefWithChatOf(2)
	if selfStripped == nil {
		t.Fatalf("HighDefOf(1): nil; expected chat-stripped bytes")
	}
	if otherWithChat == nil {
		t.Fatalf("HighDefWithChatOf(2): nil; expected chat-preserved bytes")
	}
	if selfStripped[0]&byte(MaskChat) != 0 {
		t.Errorf("self HighDefOf header has CHAT bit set: got 0x%02x; want CHAT clear", selfStripped[0])
	}
	if otherWithChat[0]&byte(MaskChat) == 0 {
		t.Errorf("other HighDefWithChatOf header missing CHAT bit: got 0x%02x; want CHAT set", otherWithChat[0])
	}

	// Encoder-level dual pin: scan the encoder output for the chat strings.
	// pdata_alt2 transforms each byte b → (128 - b) & 0xff. So "self" encodes
	// as (128-'s', 128-'e', 128-'l', 128-'f') = (13, 27, 20, 26).
	// "other" encodes as (128-'o', 128-'t', 128-'h', 128-'e', 128-'r') = (17, 12, 24, 27, 18).
	pi := NewPlayerInfo()
	out := pi.Encode(b, 1, r)
	if len(out) == 0 {
		t.Fatalf("pi.Encode returned empty output")
	}

	selfChatTransformed := []byte{13, 27, 20, 26}
	otherChatTransformed := []byte{17, 12, 24, 27, 18}

	if bytes.Contains(out, selfChatTransformed) {
		t.Errorf("self chat bytes appear in encoder output: pdata_alt2('self')=%#v found in out=%#v", selfChatTransformed, out)
	}
	if !bytes.Contains(out, otherChatTransformed) {
		t.Errorf("other chat bytes missing from encoder output: pdata_alt2('other')=%#v not found in out=%#v", otherChatTransformed, out)
	}
}
```

The `bytes` import and the `*fakeSource` type are already in scope: `bytes` is imported at `playerinfo_test.go:1-8`, and `fakeSource` is in-package (defined in `mask_payload_test.go:10`).

Implementer verification step: run `go vet ./pkg/rsbuf/` to confirm the test compiles. If it fails on field-name mismatch (e.g. `Build.Players.Insert` not a method), read the current `BuildArea` struct in `pkg/rsbuf/buildarea.go` (or similar) to find the right insert/contains helpers. The pattern at `playerinfo.go:340` (`self.Build.Players.Insert(otherPid)`) is the canonical reference at HEAD.

- [ ] **Step 3.2: Run the failing test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestPlayerInfo_LocalPlayer_ChatMaskStripped -v
```

Expected: FAIL on the "other chat bytes missing from encoder output" assertion. Currently `writePlayers` line 246 reads `HighDefOf` for tracked others — which is chat-stripped — so other's chat doesn't reach the encoder output. The cache-layer pre-assertions pass, the encoder-level dual pin fails.

(If the cache-layer assertions also fail, the issue is upstream of writePlayers — verify Task 2 was applied correctly.)

- [ ] **Step 3.3: Apply the writePlayers swap**

Edit `pkg/rsbuf/playerinfo.go:246`:

```go
// Replace:
		highDef := renderer.HighDefOf(int(otherPid))

// With:
		highDef := renderer.HighDefWithChatOf(int(otherPid))
```

- [ ] **Step 3.4: Strike the NAI-30-D2 doc-comment block at lines 113-124**

Edit `pkg/rsbuf/playerinfo.go:113-124`. Delete the entire 12-line block:

```go
// Delete (lines 113-124):
// NAI-30-D2 (audited NAI-31, deferred to NAI-32): upstream
// PlayerInfo::highdefinition at info.rs:289-291 strips the CHAT mask
// bit for self only — other players' CHAT is preserved so they see
// each other's chat-history scrollback. Goscape's Renderer.ComputePlayers
// currently passes suppressChat=true to ALL three buildPayload calls
// (renderer.go:36,47,53), so CHAT is stripped for every player, not
// just self. The TS-canonical fix needs a 4th cache variant in Renderer
// (e.g. highDefWithChat) that writePlayers reads for other-player
// payloads while writeLocalPlayer continues to read the chat-stripped
// highDef. ~30-50 LOC change, deferred to the NAI-32 renderer-port
// series.
// Test pinned via TestPlayerInfo_LocalPlayer_ChatMaskStripped (t.Skip).
```

The comment block immediately before the deleted lines (the writeLocalPlayer doc-comment at lines 106-112) is preserved; the function definition at line 125 also preserved. After this edit, `writeLocalPlayer` follows directly after the upstream-cite block.

Verify the strike with:

```bash
sed -n '105,130p' pkg/rsbuf/playerinfo.go
```

Expected output ends with the upstream-cite block at lines 106-112 immediately followed by `func (pi *PlayerInfo) writeLocalPlayer(...)` at the new line 113.

- [ ] **Step 3.5: Run the failing test to verify it now passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ -run TestPlayerInfo_LocalPlayer_ChatMaskStripped -v
```

Expected: PASS.

- [ ] **Step 3.6: Run the full pkg/rsbuf test suite (cache-busted)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/... -count=1
```

Expected: ALL PASS. Per `latent_bug_at_migration_boundary.md`: any test that flips RED at this cutover is investigated as a possible latent-bug surface, NOT migration noise.

- [ ] **Step 3.7: Run the full repository test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: ALL PASS.

- [ ] **Step 3.8: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/playerinfo_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(rsbuf): NAI-32 Task 3 — writePlayers reads HighDefWithChatOf; D2 retired

writePlayers swap at playerinfo.go:246 from renderer.HighDefOf to
renderer.HighDefWithChatOf for tracked-other reads. Other players'
CHAT is now preserved through the per-tick info encoder, so
receiving clients render chat scrollback per upstream
info.rs::write_blocks. writeLocalPlayer keeps reading HighDefOf
(chat-stripped, for self per info.rs:289-291). writeNewPlayers
reads only the low-def caches and is unchanged.

NAI-30-D2 doc-comment block at playerinfo.go:113-124 struck
entirely; deviation retired (count 14 → 13). The brief inline
"// CHAT bit stripped per info.rs:289-291" cite added in Task 1's
buildPayload edit covers the canonical-source pointer.

TestPlayerInfo_LocalPlayer_ChatMaskStripped un-skipped and
implemented per ts_asymmetry_dual_pin.md: dual-pins both presence
(other's CHAT bytes appear in encoder output) AND absence (self's
CHAT bytes do NOT appear).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.9: Verify commit content**

```bash
git show HEAD --stat
git status
```

Expected: stat shows 2 files changed (playerinfo.go, playerinfo_test.go); clean working tree.

---

### Task 4: Bundle 1 close — final verification

- [ ] **Step 4.1: Cross-check spec-test coverage per `plan_test_coverage_crosscheck.md`**

Verify that each test in the spec's test strategy section has a matching task body:

| Spec test | Implemented in |
|---|---|
| `TestComputePlayers_DualHighDef_ChatPresent` | Task 2 Step 2.1 |
| `TestComputePlayers_DualHighDef_NoChat_Identical` | Task 2 Step 2.1 |
| `TestComputePlayers_DualHighDef_MasksZero_BothNil` | Task 2 Step 2.1 |
| `TestBuildPayload_HeaderPayloadConsistent_ChatStripped` | Task 1 Step 1.1 |
| `TestPlayerInfo_LocalPlayer_ChatMaskStripped` (un-skip) | Task 3 Step 3.1 |

If any test is missing or differs from the spec, document the deviation in this step's commit message.

- [ ] **Step 4.2: Verify Bundle 1 deviation accounting**

Run:
```bash
rg -n "NAI-30-D2" pkg/ modules/ cmd/
```

Expected: zero hits in production code (the doc-comment block at playerinfo.go:113-124 was struck; the t.Skip reference at playerinfo_test.go:582 was replaced by the un-skipped body).

If hits remain, retire them per `retire_deviation_grep_all_comments.md`.

- [ ] **Step 4.3: Check post-Bundle-1 working tree**

```bash
git log --oneline -10
git status
```

Expected: 3 NAI-32 commits (one per Task 1, Task 2, Task 3) on top of `6aa41e6` (spec correction); clean working tree.

---

## Bundle 2 — rs-server-225 citation sweep

Compressed cadence per `compressed_cadence.md`. Single review pass. No TDD (doc-comment-only edits). One commit.

### Task 5: 3 doc-comment edits

**Files:**
- Modify: `pkg/script/file.go:40`
- Modify: `pkg/zone/grid.go:3-4`
- Modify: `pkg/objtype/npctype.go:25,36`

- [ ] **Step 5.1: Edit `pkg/script/file.go:40`**

```go
// Replace:
//   - lookupKey is u32 (rs-server-225 had a u16 bug).

// With:
//   - lookupKey is u32 (per Engine-TS/src/engine/script/ScriptFile.ts).
```

- [ ] **Step 5.2: Edit `pkg/zone/grid.go:3-4`**

```go
// Replace (2 lines):
// Ported from /home/owner/Code/github.com/zsrv/rs-server-225/engine/zone/grid.go,
// renamed Grid → ZoneGrid for clarity in the package-qualified zone.ZoneGrid form.

// With (1 line):
// Ported from /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/zone/ZoneGrid.ts.
```

- [ ] **Step 5.3: Edit `pkg/objtype/npctype.go:25` and `:36`**

```go
// Replace at line 25:
// MoveRestrict values (mirror of rs-server-225/entity.MoveRestrict).
// With:
// MoveRestrict values (mirror of Engine-TS/src/engine/entity/MoveRestrict.ts).

// Replace at line 36:
// BlockWalk values (mirror of rs-server-225/entity.BlockWalk).
// With:
// BlockWalk values (mirror of Engine-TS/src/engine/entity/BlockWalk.ts).
```

- [ ] **Step 5.4: Verify zero `rs-server-225` hits in production source**

Run:
```bash
rg -n "rs-server-225" pkg/ modules/ cmd/
```

Expected: zero output.

- [ ] **Step 5.5: Verify build + tests stay green**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: BUILD OK; ALL TESTS PASS. (Doc-comment-only edits — should be a no-op.)

- [ ] **Step 5.6: Commit**

```bash
git add pkg/script/file.go pkg/zone/grid.go pkg/objtype/npctype.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(script,zone,objtype): NAI-32 Task 5 — replace rs-server-225 citations with Engine-TS canonical paths

3 unauthorized rs-server-225 provenance citations carried over from
NAI-31 Bundle 0's grep findings (the 4th was retired in NAI-31
Bundle 2.A.1 at pkg/gamemap/load.go). Per ts_source_canonical_path.md
and rust_source_canonical_path.md, only LostCityRS/Engine-TS
(non-pkg/rsbuf) and 2004scape/rsbuf branch 225 (only pkg/rsbuf) are
authorized canonical sources for goscape.

Sites:
- pkg/script/file.go:40 — lookupKey citation now points at
  Engine-TS/src/engine/script/ScriptFile.ts (positive canonical
  framing replaces negative bug-archaeology).
- pkg/zone/grid.go:3 — port-from citation now points at
  Engine-TS/src/engine/zone/ZoneGrid.ts; the now-redundant
  rename rationale (TS already names it ZoneGrid) dropped.
- pkg/objtype/npctype.go:25,36 — MoveRestrict + BlockWalk citations
  now point at Engine-TS/src/engine/entity/MoveRestrict.ts +
  BlockWalk.ts.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.7: Verify commit content**

```bash
git show HEAD --stat
git status
```

Expected: stat shows 3 files changed; clean working tree.

---

## Smoke gate (post-Bundle-2, pre-close)

Per `smoke_test_server_handoff.md`: user-launched server. Controller hands off; cannot launch the server from sandbox.

- [ ] **Step S.1: Hand off to user for 2-client smoke**

Tell the user:

> "Bundle 1 + Bundle 2 committed and tests green. NAI-32 needs a 2-client public-chat smoke per `smoke_test_server_handoff.md`. Please:
>
> 1. Build + run the goscape server with the latest binary.
> 2. Connect 2 Java clients with distinct usernames.
> 3. Walk both into chat range of each other (same tile is fine).
> 4. Each client says public chat (`/chat hello` or just typed text).
>
> Expected:
>   - Both clients render BOTH chat lines (own + other's). Own-chat may render via the local-display-layer immediate echo channel; other's chat MUST render via the per-tick info encoder using NAI-32's new `HighDefWithChatOf` cache.
>   - NPCs continue rendering at expected positions (NAI-31 regression-cover).
>   - Walk + zone updates continue to function.
>   - No client-side parsing crashes.
>
> Report back with: (a) screenshot or text dump of both clients showing both chat lines; (b) any crash/error messages; (c) any regression in NPC render or walk."

- [ ] **Step S.2: Smoke verdict branch**

If smoke succeeds → proceed to Close (Step C.1).
If smoke surfaces a layered bug → materialize Bundle 3 from the template below.

---

## Bundle 3 (conditional) — smoke-failure investigation template

Materialized only on smoke failure per `investigation_subspec_cadence.md`.

- [ ] **Step 3T.1: Stage 1.1 audit dispatch**

Per `audit_subagent_fabrication.md`: any audit subagent verdict that contradicts other observable evidence (existing tests pass, NAI-31's smoke succeeded for non-chat features) is treated as a SIGNAL THAT THE AUDIT IS WRONG. Controller-side independent verification before code change.

Audit input: the smoke failure mode reported by the user (client crash with byte trace, render glitch, missing chat, etc.).

Audit substages (dispatch one Explore subagent, propagate on `NO_DIVERGENCE`):
- 1.1 — `buildPayload` chat-strip path (Task 1's fix). Re-decode any reported wire bytes against the new strip-aware buildPayload.
- 1.2 — `Renderer.ComputePlayers` dual-cache populate. Verify both `highDef[slot]` and `highDefWithChat[slot]` are populated (or both nil) per masks==0 branch.
- 1.3 — `writePlayers` swap. Verify `HighDefWithChatOf` is what reaches `pi.updates`.
- 1.4 — Java client `OpPlayerInfo` parse path. Verify the receiving parser matches the new wire shape (CHAT bit + body OR no CHAT bit + no body — both consistent now).

- [ ] **Step 3T.2: Stage 2 fix dispatch**

Materialize Stage 1's verdict into a concrete fix task. Same TDD shape as Bundle 1 tasks.

- [ ] **Step 3T.3: Re-smoke**

Hand off again per Step S.1; re-evaluate.

---

## Close

- [ ] **Step C.1: Add new memory entries (if any) per `close_commit_memory_trailer.md`**

Candidate entries from execution surfaces:
- "Header-and-payload-must-be-mutually-consistent-when-stripping-mask-bits" — feedback memory if the latent header/payload pattern recurs in NPC mask-header surface or other mask-strip code paths.
- "Dual-cache pattern for self-vs-other-asymmetric mask suppression" — project memory if the dual-cache pattern recurs.
- Bundle 3 (if materialized) memory entries per investigation outcome.

Write entries via the Write tool (NOT Bash) per `memory_write_sandbox_quirk.md`. Each entry: own file under `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/<slug>.md` + 1 pointer line in `MEMORY.md`.

- [ ] **Step C.2: Compose close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(rsbuf,script,zone,objtype): NAI-32 closed — D2 retired (CHAT-self stripped, CHAT-other preserved) + rs-server-225 citation sweep

Bundle 1 (3 commits) ports upstream info.rs:289-291 self-CHAT-mask
suppression into goscape's eager-cache renderer:
  Task 1 — buildPayload chat-strip consistency + drop suppressChat arg.
           Fixed latent header/payload mismatch where header advertised
           CHAT bit but payload omitted body. Pinned by
           TestBuildPayload_HeaderPayloadConsistent_ChatStripped.
  Task 2 — dual high-def cache (highDefWithChat).
           Renderer.ComputePlayers populates both cache variants in
           lockstep; HighDefWithChatOf accessor for tracked-other
           reads. Pinned by 3 new TestComputePlayers_DualHighDef_*
           tests.
  Task 3 — writePlayers reads HighDefWithChatOf; D2 retired.
           writeLocalPlayer keeps HighDefOf (chat-stripped, self).
           writeNewPlayers unchanged (low-def caches only). NAI-30-D2
           doc-comment block at playerinfo.go:113-124 struck.
           Pinned by un-skipped TestPlayerInfo_LocalPlayer_ChatMaskStripped
           (dual-pins presence + absence per ts_asymmetry_dual_pin).

Bundle 2 (1 commit) sweeps 3 unauthorized rs-server-225 provenance
citations to LostCityRS/Engine-TS canonical paths:
  Task 5 — pkg/script/file.go, pkg/zone/grid.go, pkg/objtype/npctype.go.

Smoke (2-client public-chat exchange in chat range): user-confirmed
{PASS / FAIL+Bundle-3-followup}. {Detail per outcome.}

Net deviation count: 14 → 13.

Closes memory: {entries added at close, if any}

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Replace `{PASS / FAIL+Bundle-3-followup}` and `{Detail per outcome}` with actual values at close. Replace `{entries added at close, if any}` with actual filenames or remove the line if none added.)

- [ ] **Step C.3: Final state verification**

```bash
git log --oneline -10
rg -n "rs-server-225" pkg/ modules/ cmd/
rg -n "NAI-30-D2" pkg/ modules/ cmd/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected:
- 5 NAI-32 commits (Task 1, Task 2, Task 3, Task 5, close) on top of `6aa41e6` (spec correction).
- Zero `rs-server-225` hits in production source.
- Zero `NAI-30-D2` hits in production source.
- All tests pass.
- Smoke confirmed PASS by user.

- [ ] **Step C.4: Save post-task handoff per `post_task_handoff.md`**

Update `nai_followups.md` with the NAI-32 close summary entry per the template used for NAI-30 / NAI-31. Include: bundle execution timeline, deviation accounting, any latent-bug surfaces, smoke verdict, memory entries learned.

Provide the user a paste-ready resume prompt for the next NAI's brainstorm session — covering candidate scope topics from the updated `nai_followups.md` open-items section.
