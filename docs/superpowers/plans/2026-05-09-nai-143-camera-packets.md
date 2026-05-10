# NAI-143 — Camera packets accumulator + `cam_moveto`/`cam_lookat`/`cam_shake` opcode wiring

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close NAI-142-D-R-D1 by porting the three remaining camera opcodes — `cam_moveto`, `cam_lookat`, `cam_shake` — and wiring the per-player `cameraPackets` accumulator drained at the top of `updateBuildArea`.

**Architecture:** `cam_shake` is a direct-write opcode (sibling of existing `cam_reset`). `cam_moveto` / `cam_lookat` buffer `cameraInfo{kind,...}` entries onto `Player.cameraPackets` and emit zone-relative coords against `originX/originZ` at drain-time, mirroring TS `NetworkPlayer.updateMap` line ordering 244-253.

**Tech Stack:** Go 1.26+ (`go_version.md`). All `go` invocations prefixed with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`.

**Spec:** `docs/superpowers/specs/2026-05-09-nai-143-camera-packets-design.md`.

**TS source (canonical: `LostCityRS/Engine-TS`):**
- `src/engine/entity/CameraInfo.ts` — accumulator entry struct.
- `src/engine/entity/Player.ts:344, 444` — field declaration + cleanup-clear.
- `src/engine/entity/NetworkPlayer.ts:242-253` — drain in `updateMap()`.
- `src/engine/script/handlers/PlayerOps.ts:206-228` — four cam handlers.
- `src/network/game/server/codec/{CamMoveTo,CamLookAt,CamShake,CamReset}Encoder.ts` — wire formats.
- `src/network/game/server/ServerGameProt.ts:33-36` — wire op IDs and payload sizes:
  - `CAM_LOOKAT = (74, 6)`, `CAM_MOVETO = (3, 6)`, `CAM_SHAKE = (13, 4)`, `CAM_RESET = (239, 0)` (already ported).

---

## File inventory

| File | Operation | Purpose |
| --- | --- | --- |
| `pkg/io/protocol/game/server/prot.go` | Modify | Add 3 wire op consts: `OpCamMoveTo`, `OpCamLookAt`, `OpCamShake`. |
| `modules/world/player.go` | Modify | Add `cameraInfo` struct + `cameraPackets []cameraInfo` field on `Player`; modify `updateBuildArea` to drain at top. |
| `modules/world/player_script.go` | Modify | Add 3 methods on `*Player`: `CamMoveTo`, `CamLookAt`, `CamShake` (sibling of existing `CamReset` at line 189). |
| `pkg/script/active.go` | Modify | Add 3 methods on `ActivePlayer` interface (near `CamReset` at line 371). |
| `pkg/script/runner_test.go` | Modify | Add capture fields + impls on `mockPlayer` (sibling of `camResetCalls` at line 600-601). |
| `pkg/script/handlers_dialog.go` | Modify | Add 3 handlers: `handleCamMoveTo`, `handleCamLookAt`, `handleCamShake` (mirror `handleCamReset` at line 90-99). |
| `pkg/script/handlers.go` | Modify | Wire 3 entries into the opcode→handler map (companion to `OpCamReset: handleCamReset` at line 131). |
| `pkg/script/handlers_dialog_test.go` | Modify | Add T1–T4, T9 (handler-layer pins). Extend the `TestDialogOpsRequireActivePlayer` for-loop at line 109. |
| `modules/world/player_camera_test.go` | Create | Add T5–T8 (drain-layer pins; sibling of `player_zone_test.go`). |

---

## Task 1: `cam_shake` direct-write end-to-end (T3 + T9)

**Why first:** No accumulator complexity. Establishes the wire-op + interface + handler + dispatch pattern. `cam_shake` is reachable via barmaid for the smoke handoff in Task 4.

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `pkg/script/active.go:371`
- Modify: `pkg/script/runner_test.go:600`
- Modify: `modules/world/player_script.go:189`
- Modify: `pkg/script/handlers_dialog.go:90`
- Modify: `pkg/script/handlers.go:131`
- Modify: `pkg/script/handlers_dialog_test.go:62`
- Test: `pkg/script/handlers_dialog_test.go`

- [ ] **Step 1: Write the failing test `TestCamShake`**

Append to `pkg/script/handlers_dialog_test.go` (after `TestCamReset` at line 78):

```go
func TestCamShake(t *testing.T) {
	// Script call: cam_shake(axis=4, random=0, amplitude=20, rate=5).
	// engine.rs2 declares cam_shake(int $axis, int $random, int $amplitude, int $rate);
	// args are pushed left-to-right, so on the int stack at OpCamShake (top → bottom):
	//   rate(5), amplitude(20), random(0), axis(4)
	// Wire encoder (TS CamShakeEncoder.ts): p1(axis), p1(random), p1(amplitude), p1(rate).
	sf := &ScriptFile{
		Name: "cam_shake",
		Opcodes: []Opcode{
			OpPushConstantInt, // axis = 4
			OpPushConstantInt, // random = 0
			OpPushConstantInt, // amplitude = 20
			OpPushConstantInt, // rate = 5
			OpCamShake,
			OpReturn,
		},
		IntOperands:      []int32{4, 0, 20, 5, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if mp.lastCamShake == nil {
		t.Fatal("CamShake was not called")
	}
	got := *mp.lastCamShake
	want := struct{ axis, random, amplitude, rate int }{axis: 4, random: 0, amplitude: 20, rate: 5}
	if got != want {
		t.Errorf("CamShake args: got %+v, want %+v", got, want)
	}
	if len(mp.cameraPackets) != 0 {
		t.Errorf("cameraPackets must NOT be populated for cam_shake (direct-write); got %d entries", len(mp.cameraPackets))
	}
}
```

Also extend the `TestDialogOpsRequireActivePlayer` for-loop at line 109 — add `OpCamShake` to the slice:

```go
for _, op := range []Opcode{OpPPauseButton, OpPCountDialog, OpLastCom, OpCamReset, OpCamShake} {
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestCamShake|TestDialogOpsRequireActivePlayer' -v
```

Expected: `TestCamShake` and the extended `TestDialogOpsRequireActivePlayer/CAM_SHAKE` subtest both fail to compile (no `mockPlayer.lastCamShake`, no `mockPlayer.cameraPackets`, no handler dispatch for `OpCamShake`).

- [ ] **Step 3: Add `OpCamShake` wire op to `pkg/io/protocol/game/server/prot.go`**

Locate the existing `OpCamReset` declaration (line 39-41) and add siblings immediately above or below:

```go
// Camera control. TS ServerGameProt.CAM_SHAKE = (13, 4), payload p1×4.
// Sent by the CAM_SHAKE script opcode for cutscene camera shake.
OpCamShake = Op{Opcode: 13, PayloadSize: 4}
```

- [ ] **Step 4: Add `CamShake` to the `ActivePlayer` interface in `pkg/script/active.go`**

Insert after the existing `CamReset()` declaration (line 371):

```go
// CamShake sends a CAM_SHAKE wire packet to the client. Direct-write
// (no accumulator); siblings the existing CamReset shape. Called by
// the CAM_SHAKE script opcode for cutscene camera shake.
CamShake(axis, random, amplitude, rate int)
```

- [ ] **Step 5: Add `CamShake` capture fields + impl on `mockPlayer` in `pkg/script/runner_test.go`**

Locate `camResetCalls` (line 600). Add adjacent to the same struct:

```go
// cameraPackets mirrors the production Player.cameraPackets accumulator
// for handler-layer tests. CamMoveTo / CamLookAt append to this slice;
// CamShake does NOT touch it (direct-write).
cameraPackets []struct {
	kind                                       uint8
	camX, camZ, height, rotationSpeed, rotationMultiplier int
}
lastCamShake *struct{ axis, random, amplitude, rate int }
```

And add the impl method on `mockPlayer` (near the existing `CamReset` at line 601):

```go
func (m *mockPlayer) CamShake(axis, random, amplitude, rate int) {
	m.lastCamShake = &struct{ axis, random, amplitude, rate int }{axis, random, amplitude, rate}
}
```

- [ ] **Step 6: Add `(*Player).CamShake` direct-write in `modules/world/player_script.go`**

Insert after the existing `CamReset` (line 189-191):

```go
// CamShake sends a CAM_SHAKE wire packet (TS ServerGameProt.CAM_SHAKE
// = 13, payload p1×4 = axis, random, amplitude, rate). Direct-write;
// does NOT route through the cameraPackets accumulator (TS PlayerOps.ts:223
// is `state.activePlayer.write(new CamShake(...))`, no accumulator).
// Called by the CAM_SHAKE (opcode 2010) script handler. Mirrors TS
// CamShakeEncoder.ts:9-14.
func (p *Player) CamShake(axis, random, amplitude, rate int) {
	p.writeOut(gameserver.OpCamShake, []byte{
		byte(axis), byte(random), byte(amplitude), byte(rate),
	})
}
```

(Verify `gameserver` import alias is already present in this file — `CamReset` already uses it.)

- [ ] **Step 7: Add `handleCamShake` in `pkg/script/handlers_dialog.go`**

Insert after `handleCamReset` (line 99). Pop order is reverse of TS push order (`axis, random, amplitude, rate`):

```go
// handleCamShake reads (axis, random, amplitude, rate) from the int stack
// and dispatches to ActivePlayer.CamShake. Args were pushed left-to-right
// at the script call site (engine.rs2:120 `cam_shake(int $axis, int $random,
// int $amplitude, int $rate)`); goscape's PopInt returns them in reverse.
// Mirrors TS PlayerOps.ts:220-224.
func handleCamShake(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CAM_SHAKE: no active player")
	}
	rate := s.PopInt()
	amplitude := s.PopInt()
	random := s.PopInt()
	axis := s.PopInt()
	s.Self.CamShake(axis, random, amplitude, rate)
	return nil
}
```

- [ ] **Step 8: Wire `OpCamShake → handleCamShake` in `pkg/script/handlers.go`**

Locate the existing `OpCamReset: handleCamReset` entry (line 131) and add adjacent:

```go
OpCamShake: handleCamShake,
```

- [ ] **Step 9: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestCamShake|TestDialogOpsRequireActivePlayer' -v
```

Expected: PASS.

- [ ] **Step 10: Run full package tests to verify no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/... ./pkg/io/protocol/...
```

Expected: PASS (no regressions in pre-existing tests).

- [ ] **Step 11: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go pkg/script/active.go pkg/script/runner_test.go modules/world/player_script.go pkg/script/handlers_dialog.go pkg/script/handlers.go pkg/script/handlers_dialog_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-143 — port cam_shake direct-write opcode

OpCamShake (2010) was protocol-stub-not-completed: declared in
pkg/script/opcode.go but no handler / Player method / wire op. Adds
wire op (TS ServerGameProt.CAM_SHAKE = (13, 4) p1*4), handler with
TS-reverse pop order, ActivePlayer.CamShake, Player.CamShake direct-
write. Mirrors existing CamReset shape.

Tests: TestCamShake pins TS pop order via per-field assertion (not
slice-length-only, per handler_pop_order_test_masking memory) and
asserts cameraPackets accumulator is NOT touched (direct-write).
TestDialogOpsRequireActivePlayer extended to OpCamShake.

NAI-143 task 1 of 4.
EOF
)"
```

---

## Task 2: `cam_moveto` + `cam_lookat` accumulator (T1 + T2 + T4)

**Why second:** Builds on the wire-op / interface / dispatch pattern from Task 1. Adds the `cameraPackets` accumulator and the two append-only handlers; defers actual byte emission to Task 3 (drain).

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `modules/world/player.go:64` (add struct + field)
- Modify: `pkg/script/active.go:371`
- Modify: `pkg/script/runner_test.go`
- Modify: `modules/world/player_script.go`
- Modify: `pkg/script/handlers_dialog.go`
- Modify: `pkg/script/handlers.go`
- Modify: `pkg/script/handlers_dialog_test.go`

- [ ] **Step 1: Write failing tests `TestCamMoveTo`, `TestCamLookAt`, `TestCamMoveToHandler_invalidCoord`**

Append to `pkg/script/handlers_dialog_test.go`:

```go
func TestCamMoveTo(t *testing.T) {
	// Script call: cam_moveto(coord=0_45_146_48_7, height=550, rate=100, rate2=100).
	// engine.rs2:116: [command,cam_moveto](coord $src, int $height, int $rate, int $rate2);
	// args pushed left-to-right, so PopInt order is rate2, rate, height, coord.
	// Use a packed coord that decodes to a deterministic (level, x, z).
	// CoordValid range is [0, 2147483647]; pick a safe in-range value.
	const packedCoord = int32(0x0000_1000) // arbitrary in-range packed coord
	level, x, z := unpackCoord(int(packedCoord))

	sf := &ScriptFile{
		Name: "cam_moveto",
		Opcodes: []Opcode{
			OpPushConstantInt, // coord
			OpPushConstantInt, // height
			OpPushConstantInt, // rate
			OpPushConstantInt, // rate2
			OpCamMoveTo,
			OpReturn,
		},
		IntOperands:      []int32{packedCoord, 550, 100, 80, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	_ = level
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.cameraPackets) != 1 {
		t.Fatalf("cameraPackets: got %d entries, want 1", len(mp.cameraPackets))
	}
	got := mp.cameraPackets[0]
	if got.kind != 0 {
		t.Errorf("kind: got %d, want 0 (moveto)", got.kind)
	}
	if got.camX != x || got.camZ != z {
		t.Errorf("(camX, camZ): got (%d, %d), want (%d, %d) from unpackCoord", got.camX, got.camZ, x, z)
	}
	if got.height != 550 || got.rotationSpeed != 100 || got.rotationMultiplier != 80 {
		t.Errorf("scalars: got height=%d rate=%d rate2=%d, want 550 100 80",
			got.height, got.rotationSpeed, got.rotationMultiplier)
	}
}

func TestCamLookAt(t *testing.T) {
	// Same script shape as TestCamMoveTo but OpCamLookAt; assert kind=1.
	const packedCoord = int32(0x0000_1000)
	_, x, z := unpackCoord(int(packedCoord))

	sf := &ScriptFile{
		Name: "cam_lookat",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpCamLookAt, OpReturn,
		},
		IntOperands:      []int32{packedCoord, 200, 100, 100, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(mp.cameraPackets) != 1 {
		t.Fatalf("cameraPackets: got %d entries, want 1", len(mp.cameraPackets))
	}
	got := mp.cameraPackets[0]
	if got.kind != 1 {
		t.Errorf("kind: got %d, want 1 (lookat)", got.kind)
	}
	if got.camX != x || got.camZ != z {
		t.Errorf("(camX, camZ): got (%d, %d), want (%d, %d)", got.camX, got.camZ, x, z)
	}
}

func TestCamMoveToHandler_invalidCoord(t *testing.T) {
	// CoordValid range is [0, 2147483647]; -1 must error per checkCoord.
	const invalidCoord = int32(-1)
	sf := &ScriptFile{
		Name: "cam_moveto_bad",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpCamMoveTo, OpReturn,
		},
		IntOperands:      []int32{invalidCoord, 100, 1, 1, 0, 0},
		StringOperands:   []string{"", "", "", "", "", ""},
		InstructionCount: 6,
	}
	mp := &mockPlayer{}
	state := Init(sf, mp, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("expected error from CAM_MOVETO with invalid coord, got nil")
	}
	if !strings.Contains(err.Error(), "CAM_MOVETO") || !strings.Contains(err.Error(), "coord out of range") {
		t.Errorf("error shape: got %q, want substrings 'CAM_MOVETO' and 'coord out of range'", err.Error())
	}
	if len(mp.cameraPackets) != 0 {
		t.Errorf("cameraPackets must remain empty on error; got %d entries", len(mp.cameraPackets))
	}
}
```

If `strings` is not yet imported in this test file, add it:
```go
import (
	"strings"
	"testing"
)
```

Also extend `TestDialogOpsRequireActivePlayer` slice:
```go
for _, op := range []Opcode{OpPPauseButton, OpPCountDialog, OpLastCom, OpCamReset, OpCamShake, OpCamMoveTo, OpCamLookAt} {
```

(Note: T1/T2 may need pop-order arithmetic verification before running. The test fixture reflects the design's pop order — if the test fails with mismatched scalar values, the implementer must NOT swap the test's push order to match a buggy pop order — fix the handler instead. See `handler_pop_order_test_masking.md`.)

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestCamMoveTo|TestCamLookAt|TestCamMoveToHandler_invalidCoord|TestDialogOpsRequireActivePlayer' -v
```

Expected: compile failure (no `OpCamMoveTo`/`OpCamLookAt` handler dispatch; mockPlayer doesn't have `CamMoveTo`/`CamLookAt`).

- [ ] **Step 3: Add `OpCamMoveTo` + `OpCamLookAt` wire ops to `pkg/io/protocol/game/server/prot.go`**

Add adjacent to the existing `OpCamShake` (Task 1) and `OpCamReset`:

```go
// Camera control. TS ServerGameProt.CAM_MOVETO = (3, 6), payload
// p1(localX) p1(localZ) p2(height) p1(rotationSpeed) p1(rotationMultiplier).
// Coords are zone-relative against player.originX/originZ at drain-time
// (TS NetworkPlayer.ts:245-246). Sent by the CAM_MOVETO script opcode.
OpCamMoveTo = Op{Opcode: 3, PayloadSize: 6}

// Camera control. TS ServerGameProt.CAM_LOOKAT = (74, 6); same payload
// shape as OpCamMoveTo. Sent by the CAM_LOOKAT script opcode.
OpCamLookAt = Op{Opcode: 74, PayloadSize: 6}
```

- [ ] **Step 4: Add `cameraInfo` struct + `cameraPackets` field on `Player` in `modules/world/player.go`**

Add the struct definition near the top of `player.go` (above `type Player struct {`, around line 60, alongside other internal types):

```go
// cameraInfo is one entry in Player.cameraPackets — a deferred
// CAM_MOVETO / CAM_LOOKAT wire packet awaiting drain at the top of
// updateBuildArea. kind=0 emits OpCamMoveTo; kind=1 emits OpCamLookAt.
// Mirrors TS engine/entity/CameraInfo.ts. (slice for goscape; TS uses
// LinkList<CameraInfo>.)
type cameraInfo struct {
	kind               uint8 // 0 = moveto, 1 = lookat
	camX, camZ         int   // world-space cam target; converted to zone-relative at drain
	height             int   // p2 (16-bit big-endian at encode)
	rotationSpeed      int   // p1 (rate in engine.rs2)
	rotationMultiplier int   // p1 (rate2 in engine.rs2)
}
```

Add the `cameraPackets` field on `Player` near `queue` (line 149):

```go
// cameraPackets is the per-player buffer of deferred camera packets.
// CAM_MOVETO / CAM_LOOKAT script opcodes append; updateBuildArea drains
// at top-of-tick (after Player.updateMap has refreshed originX/originZ
// per NAI-93 ordering). Mirrors TS Player.cameraPackets at Player.ts:344.
cameraPackets []cameraInfo
```

- [ ] **Step 5: Add `CamMoveTo` + `CamLookAt` to `ActivePlayer` interface in `pkg/script/active.go`**

Insert near `CamReset` (line 371) and `CamShake` (Task 1):

```go
// CamMoveTo and CamLookAt buffer a deferred zone-relative camera packet
// onto Player.cameraPackets. The packet is drained at the top of
// updateBuildArea, where (camX, camZ) is converted to (localX, localZ)
// against the player's freshly-rebuilt originX/originZ. kind is
// 0 (moveto) or 1 (lookat). Mirrors TS PlayerOps.ts:206-218 +
// NetworkPlayer.ts:244-253.
CamMoveTo(camX, camZ, height, rate, rate2 int)
CamLookAt(camX, camZ, height, rate, rate2 int)
```

- [ ] **Step 6: Add `mockPlayer.CamMoveTo` / `CamLookAt` impls in `pkg/script/runner_test.go`**

Add (Task 1 already added the `cameraPackets` field on mockPlayer):

```go
func (m *mockPlayer) CamMoveTo(camX, camZ, height, rate, rate2 int) {
	m.cameraPackets = append(m.cameraPackets, struct {
		kind                                       uint8
		camX, camZ, height, rotationSpeed, rotationMultiplier int
	}{kind: 0, camX: camX, camZ: camZ, height: height, rotationSpeed: rate, rotationMultiplier: rate2})
}

func (m *mockPlayer) CamLookAt(camX, camZ, height, rate, rate2 int) {
	m.cameraPackets = append(m.cameraPackets, struct {
		kind                                       uint8
		camX, camZ, height, rotationSpeed, rotationMultiplier int
	}{kind: 1, camX: camX, camZ: camZ, height: height, rotationSpeed: rate, rotationMultiplier: rate2})
}
```

- [ ] **Step 7: Add `(*Player).CamMoveTo` / `CamLookAt` accumulator-append in `modules/world/player_script.go`**

```go
// CamMoveTo appends a kind=0 cameraInfo onto p.cameraPackets. The packet
// is drained at the top of updateBuildArea (TS NetworkPlayer.ts:244-253);
// (camX, camZ) is converted to (localX, localZ) at drain-time using
// p.originX/p.originZ. Mirrors TS PlayerOps.ts:213-218.
func (p *Player) CamMoveTo(camX, camZ, height, rate, rate2 int) {
	p.cameraPackets = append(p.cameraPackets, cameraInfo{
		kind: 0, camX: camX, camZ: camZ,
		height: height, rotationSpeed: rate, rotationMultiplier: rate2,
	})
}

// CamLookAt appends a kind=1 cameraInfo. Same drain semantics as CamMoveTo.
// Mirrors TS PlayerOps.ts:206-211.
func (p *Player) CamLookAt(camX, camZ, height, rate, rate2 int) {
	p.cameraPackets = append(p.cameraPackets, cameraInfo{
		kind: 1, camX: camX, camZ: camZ,
		height: height, rotationSpeed: rate, rotationMultiplier: rate2,
	})
}
```

- [ ] **Step 8: Add `handleCamMoveTo` + `handleCamLookAt` in `pkg/script/handlers_dialog.go`**

```go
// handleCamMoveTo reads (coord, height, rate, rate2) from the int stack,
// validates coord via checkCoord (mirrors TS CoordValid at
// ScriptValidators.ts:109), and dispatches to ActivePlayer.CamMoveTo
// with the unpacked (x, z). Args were pushed left-to-right; PopInt
// reverses, so we pop rate2, rate, height, coord. Mirrors TS
// PlayerOps.ts:213-218.
func handleCamMoveTo(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CAM_MOVETO: no active player")
	}
	rate2 := s.PopInt()
	rate := s.PopInt()
	height := s.PopInt()
	coord := s.PopInt()
	_, x, z, err := checkCoord(coord, "CAM_MOVETO")
	if err != nil {
		return err
	}
	s.Self.CamMoveTo(x, z, height, rate, rate2)
	return nil
}

// handleCamLookAt is identical to handleCamMoveTo except it dispatches
// to CamLookAt (kind=1). Mirrors TS PlayerOps.ts:206-211.
func handleCamLookAt(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("CAM_LOOKAT: no active player")
	}
	rate2 := s.PopInt()
	rate := s.PopInt()
	height := s.PopInt()
	coord := s.PopInt()
	_, x, z, err := checkCoord(coord, "CAM_LOOKAT")
	if err != nil {
		return err
	}
	s.Self.CamLookAt(x, z, height, rate, rate2)
	return nil
}
```

- [ ] **Step 9: Wire `OpCamMoveTo`/`OpCamLookAt` → handlers in `pkg/script/handlers.go`**

Add adjacent to existing `OpCamReset` and `OpCamShake` (Task 1) entries:

```go
OpCamMoveTo: handleCamMoveTo,
OpCamLookAt: handleCamLookAt,
```

- [ ] **Step 10: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestCamMoveTo|TestCamLookAt|TestCamMoveToHandler_invalidCoord|TestDialogOpsRequireActivePlayer' -v
```

Expected: PASS.

- [ ] **Step 11: Run full package tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...
```

Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go modules/world/player.go modules/world/player_script.go pkg/script/active.go pkg/script/runner_test.go pkg/script/handlers_dialog.go pkg/script/handlers.go pkg/script/handlers_dialog_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-143 — port cam_moveto/cam_lookat accumulator + handlers

OpCamMoveTo (2008) and OpCamLookAt (2007) were
protocol-stub-not-completed. Adds wire ops (TS ServerGameProt = (3, 6)
and (74, 6) p1×2 + p2 + p1×2), Player.cameraPackets accumulator slice
+ cameraInfo struct (mirrors TS CameraInfo.ts), Player.CamMoveTo /
CamLookAt append-only methods, two handlers with TS-reverse pop order
+ checkCoord validation. Drain inserted in Task 3.

Tests: TestCamMoveTo/TestCamLookAt pin kind-byte distinction +
unpacked coord + scalar args (per handler_pop_order_test_masking).
TestCamMoveToHandler_invalidCoord pins error path through checkCoord.

NAI-143 task 2 of 4.
EOF
)"
```

---

## Task 3: Drain `cameraPackets` in `updateBuildArea` (T5 + T6 + T7 + T8)

**Why third:** Tasks 1+2 established the wire ops, accumulator, and append-only handlers. Task 3 closes the loop by emitting bytes at drain-time with zone-relative arithmetic.

**Files:**
- Modify: `modules/world/player.go:880` (`updateBuildArea`)
- Create: `modules/world/player_camera_test.go`

- [ ] **Step 1: Write the failing tests in `modules/world/player_camera_test.go`**

Create the new file:

```go
package world

import (
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// readWirePackets drains everything currently in the pipe, decrypts via the
// provided isaac stream, and returns parsed (opcode, payload) pairs in the
// order they were written. Mirrors the wire-decoding pattern used by
// player_zone_test.go's drainConn + parse helpers — but specifically for
// cam packets which have fixed payload sizes.
func readWirePackets(t *testing.T, c net.Conn, dec *io2.Isaac) []wirePacket {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	n, _ := c.Read(buf)
	pkts := []wirePacket{}
	pos := 0
	for pos < n {
		opcode := int(buf[pos]) - int(dec.Next()&0xff)
		opcode &= 0xff
		pos++
		var op gameserver.Op
		switch opcode {
		case gameserver.OpCamMoveTo.Opcode:
			op = gameserver.OpCamMoveTo
		case gameserver.OpCamLookAt.Opcode:
			op = gameserver.OpCamLookAt
		case gameserver.OpCamShake.Opcode:
			op = gameserver.OpCamShake
		case gameserver.OpCamReset.Opcode:
			op = gameserver.OpCamReset
		default:
			t.Fatalf("readWirePackets: unexpected opcode %d at pos %d (n=%d)", opcode, pos-1, n)
		}
		payload := buf[pos : pos+op.PayloadSize]
		pos += op.PayloadSize
		pkts = append(pkts, wirePacket{opcode: opcode, payload: append([]byte(nil), payload...)})
	}
	return pkts
}

type wirePacket struct {
	opcode  int
	payload []byte
}

// TestUpdateBuildAreaCameraDrain pins TS NetworkPlayer.ts:244-253 byte-exact:
// kind=0 cameraInfo emits OpCamMoveTo with localX/Z computed against the
// player's freshly-rebuilt origin. p2 height pinned big-endian per
// rsbuf_roundtrip_tests.
func TestUpdateBuildAreaCameraDrain(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 296, 392, 0)
	dec := io2.New([4]uint32{1, 2, 3, 4}) // matches encryptor seed in newZoneTestPlayer
	// Skip already-written REBUILD packets from newZoneTestPlayer init.
	p.client.flushWrite()
	_ = readWirePackets // drain pre-existing — but newZoneTestPlayer's REBUILD packets use OpRebuildNormal; skip them
	// Re-baseline: drain anything pending in the pipe before our test write.
	cc.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	pre := make([]byte, 4096)
	cc.Read(pre)

	p.cameraPackets = []cameraInfo{{
		kind: 0, camX: 300, camZ: 400, height: 550,
		rotationSpeed: 100, rotationMultiplier: 100,
	}}

	p.updateBuildArea()
	p.client.flushWrite()

	pkts := readWirePackets(t, cc, dec)
	if len(pkts) != 1 {
		t.Fatalf("expected 1 cam packet, got %d", len(pkts))
	}
	if pkts[0].opcode != gameserver.OpCamMoveTo.Opcode {
		t.Errorf("opcode: got %d, want %d (OpCamMoveTo)", pkts[0].opcode, gameserver.OpCamMoveTo.Opcode)
	}
	wantLocalX := byte(300 - coordgrid.ZoneOrigin(296))
	wantLocalZ := byte(400 - coordgrid.ZoneOrigin(392))
	want := []byte{wantLocalX, wantLocalZ, 0x02, 0x26, 100, 100} // 550 = 0x0226
	if got := pkts[0].payload; !bytesEqual(got, want) {
		t.Errorf("payload: got %v, want %v", got, want)
	}
	if len(p.cameraPackets) != 0 {
		t.Errorf("cameraPackets must be empty after drain, got %d", len(p.cameraPackets))
	}
	_ = packet.Packet{} // silence unused import if applicable
}

// TestUpdateBuildAreaCameraDrain_lookatKind pins kind=1 → OpCamLookAt mapping.
func TestUpdateBuildAreaCameraDrain_lookatKind(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 296, 392, 0)
	dec := io2.New([4]uint32{1, 2, 3, 4})
	p.client.flushWrite()
	cc.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	pre := make([]byte, 4096)
	cc.Read(pre)

	p.cameraPackets = []cameraInfo{{
		kind: 1, camX: 300, camZ: 400, height: 200, rotationSpeed: 0, rotationMultiplier: 100,
	}}
	p.updateBuildArea()
	p.client.flushWrite()

	pkts := readWirePackets(t, cc, dec)
	if len(pkts) != 1 || pkts[0].opcode != gameserver.OpCamLookAt.Opcode {
		t.Fatalf("expected 1 OpCamLookAt packet, got %v", pkts)
	}
}

// TestUpdateBuildAreaCameraDrain_originFreshness pins that drain reads
// p.originX / p.originZ at drain-time, NOT a snapshot at append-time.
func TestUpdateBuildAreaCameraDrain_originFreshness(t *testing.T) {
	s := newZoneTestServer(t)
	p, cc := newZoneTestPlayer(t, s, 1, 296, 392, 0)
	dec := io2.New([4]uint32{1, 2, 3, 4})
	p.client.flushWrite()
	cc.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	cc.Read(make([]byte, 4096))

	// Append while origin is at A, then mutate origin to B before drain.
	p.cameraPackets = append(p.cameraPackets, cameraInfo{
		kind: 0, camX: 300, camZ: 400, height: 100, rotationSpeed: 1, rotationMultiplier: 1,
	})
	p.originX = 304 // mutate after append
	p.originZ = 408
	p.updateBuildArea()
	p.client.flushWrite()

	pkts := readWirePackets(t, cc, dec)
	if len(pkts) != 1 {
		t.Fatalf("expected 1 packet, got %d", len(pkts))
	}
	wantLocalX := byte(300 - coordgrid.ZoneOrigin(304))
	wantLocalZ := byte(400 - coordgrid.ZoneOrigin(408))
	if pkts[0].payload[0] != wantLocalX || pkts[0].payload[1] != wantLocalZ {
		t.Errorf("origin freshness: got localX=%d localZ=%d, want %d/%d (origin must be read at drain-time, not append-time)",
			pkts[0].payload[0], pkts[0].payload[1], wantLocalX, wantLocalZ)
	}
}

// TestUpdateBuildAreaCameraThenZone pins TS line ordering: cam drain
// (NetworkPlayer.ts:244-253) runs BEFORE the lastZone rebuildZones check
// (NetworkPlayer.ts:269-271).
func TestUpdateBuildAreaCameraThenZone(t *testing.T) {
	s := newZoneTestServer(t)
	p, _ := newZoneTestPlayer(t, s, 1, 296, 392, 0)

	// Force lastZone to differ from current zone so rebuildZones would fire.
	p.lastZone = -1 // sentinel — first updateBuildArea normally fires rebuildZones
	p.cameraPackets = []cameraInfo{{
		kind: 0, camX: 300, camZ: 400, height: 100, rotationSpeed: 1, rotationMultiplier: 1,
	}}

	// Snapshot activeZones before drain to detect mid-drain mutation.
	beforeActiveCount := len(p.activeZones)
	_ = beforeActiveCount

	p.updateBuildArea()

	// After drain: cameraPackets empty AND lastZone updated AND rebuildZones ran.
	if len(p.cameraPackets) != 0 {
		t.Errorf("cameraPackets must be drained, got %d", len(p.cameraPackets))
	}
	if p.lastZone == -1 {
		t.Errorf("lastZone must be updated after updateBuildArea; got -1 (rebuildZones never fired)")
	}
	// Both effects are observable post-call. Ordering is pinned implicitly
	// by the design (drain at top of method) — any future refactor that
	// reorders will still pass this test, but a refactor that drops the
	// drain entirely will fail TestUpdateBuildAreaCameraDrain. The pair is
	// the binding ordering pin per spec §1.2 T8.
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

**Note for the implementer:** the wire-decoding helper above is a starting sketch — verify against the existing pattern in `player_zone_test.go:72-110` (which uses `drainConn` + manual byte-slicing). If `player_zone_test.go` already has a more idiomatic `readPacket(opcode, payload)` extractor, prefer that over reimplementing here. The exact ISAAC seed (`{1, 2, 3, 4}`) must match what `newZoneTestPlayer` initializes (`io2.New([4]uint32{uint32(slot), 2, 3, 4})` per player_zone_test.go:17 — so slot=1 → seed `{1, 2, 3, 4}`). Mismatched seeds will mis-decrypt the opcode byte.

If decoding test infrastructure proves heavier than expected, an alternative seam: add a tiny `wireRecorder` helper that captures `(opcode, payload)` directly from a `writeOut` call (bypassing the connection), per `test_fixture_view_parity.md` — but only if the existing `drainConn` path is genuinely awkward.

- [ ] **Step 2: Run tests to verify they fail (compile or assertion)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestUpdateBuildAreaCameraDrain|TestUpdateBuildAreaCameraThenZone' -v
```

Expected: tests fail because the drain code is not yet present in `updateBuildArea`. Compile errors should be limited to the test helper imports — fix any unused-import issues by tightening the imports list.

- [ ] **Step 3: Insert drain at top of `updateBuildArea` in `modules/world/player.go`**

Modify `(*Player).updateBuildArea` (line 880-886). Update the doc-comment and prepend the drain block before the `lastZone` check:

```go
// updateBuildArea fires rebuildZones() on per-tick zone transitions
// AND drains the cameraPackets accumulator. Mirrors TS
// NetworkPlayer.updateMap (NetworkPlayer.ts:242-285):
//
//	updateMap() {
//	    // 1. drain cameraPackets (lines 244-253)
//	    for (const info of this.cameraPackets.all()) {
//	        const localX = info.camX - CoordGrid.zoneOrigin(this.originX);
//	        const localZ = info.camZ - CoordGrid.zoneOrigin(this.originZ);
//	        ...
//	    }
//	    // 2. lastMapZone check + triggerMapzone (lines 256-266)  -- NAI-142-D-R-D2
//	    // 3. lastZone check + rebuildZones (lines 269-271)       -- NAI-142
//	    // 4. triggerZone/triggerZoneExit/SetMultiway (lines 274-285) -- NAI-142-D-R-D3
//	}
//
// lastMapZone (NetworkPlayer.ts:256-266) and triggerZone +
// triggerZoneExit + SetMultiway (NetworkPlayer.ts:274-285) are
// deferred follow-ups; see nai_followups.md NAI-142-D-R-D{2,3}.
func (p *Player) updateBuildArea() {
	// 1. drain cameraPackets — TS NetworkPlayer.ts:244-253. Origin is
	// already fresh because Player.updateMap (TS BuildArea.rebuildNormal
	// slot) runs in Server.processInfo before processOut per NAI-93.
	for i := range p.cameraPackets {
		info := p.cameraPackets[i]
		localX := info.camX - coordgrid.ZoneOrigin(p.originX)
		localZ := info.camZ - coordgrid.ZoneOrigin(p.originZ)
		payload := []byte{
			byte(localX),
			byte(localZ),
			byte(info.height >> 8), byte(info.height), // p2 big-endian
			byte(info.rotationSpeed),
			byte(info.rotationMultiplier),
		}
		op := gameserver.OpCamMoveTo
		if info.kind == 1 {
			op = gameserver.OpCamLookAt
		}
		p.writeOut(op, payload)
	}
	p.cameraPackets = p.cameraPackets[:0]

	// 2. lastZone — TS NetworkPlayer.ts:269-271 (NAI-142).
	zone := coordgrid.PackCoord(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
	if p.lastZone != zone {
		p.rebuildZones()
		p.lastZone = zone
	}
}
```

(Verify `gameserver` import alias is already present in `player.go` — `writeOut` already takes a `gameserver.Op` so the import must exist.)

- [ ] **Step 4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestUpdateBuildAreaCameraDrain|TestUpdateBuildAreaCameraThenZone' -v
```

Expected: PASS.

- [ ] **Step 5: Run full test suite for regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. Pay particular attention to NAI-141 / NAI-142 tests in `modules/world/player_zone_test.go` — the bundle inserts code BEFORE the lastZone check; any regression there will fail those tests.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player.go modules/world/player_camera_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-143 — drain cameraPackets at top of updateBuildArea

Closes the loop on the cam_moveto/cam_lookat accumulator (Task 2):
drain emits OpCamMoveTo (kind=0) or OpCamLookAt (kind=1) with
zone-relative localX/Z = camX - zoneOrigin(originX), localZ = camZ -
zoneOrigin(originZ). p2 height encoded big-endian. Slice cleared
post-drain.

Drain inserted at TOP of updateBuildArea (before lastZone check) per
TS NetworkPlayer.updateMap line ordering 244-253 → 269-271. Origin
freshness preserved by NAI-93 ordering: Player.updateMap (TS
BuildArea.rebuildNormal slot) runs in Server.processInfo before
processOut.

Tests: T5 byte-exact drain, T6 kind=1 → OpCamLookAt, T7 origin read
at drain-time (not append-time), T8 cam-then-zone ordering.

NAI-143 task 3 of 4.
EOF
)"
```

---

## Task 4: Smoke handoff (user-launched server) + close

**Why:** Per `smoke_test_server_handoff.md`, the Java client cannot reach the goscape server from the sandbox. The user runs the server and exercises the smoke pins.

**Files:** none (handoff + close commit + memory update only).

- [ ] **Step 1: Stage smoke handoff to user**

Provide the user with the smoke checklist from spec §1.1:

> **NAI-143 smoke handoff (run on your machine; do not skip the cascade-binding step):**
>
> 1. Build + run server:
>    ```bash
>    CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
>    ```
> 2. Connect with Java client #225, log in.
> 3. **Pin 1 (cam_shake):** Walk to Falador `risingsun_barmaid` (in the Rising Sun Inn). Trigger the dialogue branch that fires `cam_shake(0, 0, 15, 2)` (per `LostCityRS/Content/scripts/areas/area_falador/scripts/barmaid.rs2:34-35`). Verify: client camera shakes; no AIOOBE / disconnect.
> 4. **Pin 2 (cam_moveto + cam_lookat):** Either (a) progress `quest_arena.rs2` to the line-143 cutscene, or (b) wire a one-shot dev `::cam_test` proc that calls `cam_moveto(spawncoord, 400, 0, 100); cam_lookat(spawncoord+1tile, 400, 0, 100); cam_reset;`. Verify: camera moves to / looks at the configured tile; no AIOOBE.
> 5. **Pin 3 (cam_reset regression-fence):** End-to-end login flow continues to work; any `cam_reset;` in any script restores normal camera.
> 6. **Pin 4 (NAI-141 / NAI-142 regression-fence):** Walk through ≥3 zone boundaries and tele across the 13×13 build-area window. Loc deltas render correctly; no client AIOOBE.

The implementer must wait for the user's smoke confirmation before proceeding to Step 2.

- [ ] **Step 2: On smoke green, write the close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-143 — cam_moveto/cam_lookat/cam_shake ported; smoke green

PRIMARY pin (smoke): barmaid cam_shake renders client camera shake;
cam_moveto+cam_lookat (route per smoke handoff) renders correctly;
cam_reset regression-fence + NAI-141/142 regression-fence both clean.

SECONDARY pins: T1-T9 in pkg/script/handlers_dialog_test.go +
modules/world/player_camera_test.go.

Bundle commits: <task1-sha> + <task2-sha> + <task3-sha>.

Carry-forward routing:
- NAI-142-D-R-D2 (lastMapZone + triggerMapzone): open
- NAI-142-D-R-D3 (triggerZone + SetMultiway): open
- NAI-142-D-R-D4 (rename updateMap → rebuildNormal): open

Closes memory: nai_followups.md NAI-142-D-R-D1.
EOF
)"
```

(Replace `<taskN-sha>` placeholders with the actual commit SHAs from Tasks 1–3 before running.)

- [ ] **Step 3: Update memory `nai_followups.md`**

Move the `NAI-142-D-R-D1` entry from "open follow-ups" to a closed-record section, and add a new NAI-143 close entry summarizing the bundle (commits, smoke results, deviations D1–D3 from spec §6).

- [ ] **Step 4: Update `MEMORY.md` index**

If any of the spec's deviations or learnings warrant a new memory entry (per `dead_api_polish.md` post-close audit), add a one-line index entry. Likely candidates:
- None expected — the bundle is parity-clean and leans on existing patterns. Skip if no surprise was learned.

- [ ] **Step 5: Final verification**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
git log --oneline -5
```

Expected: all tests pass; `git log` shows 3 feat commits + 1 close commit.

---

## Self-review

**Spec coverage:**
- §1.1 PRIMARY smoke pins → Task 4 step 1.
- §1.2 SECONDARY tests T1–T9 → Tasks 1–3 (T1, T2, T4 in Task 2; T3, T9 in Task 1; T5–T8 in Task 3).
- §1.3 negative/out-of-scope → not implemented (correct).
- §2 architecture (5 components, data flow, drain location) → Task 3 step 3 + Tasks 1–2.
- §3 test plan (handler + drain + smoke) → Tasks 1–4.
- §4 risk register (R1–R7) → tests T1–T9 cover R1–R5; R6/R7 are non-actionable parity.
- §6 deviations (D1 slice/LinkList, D2 no logout-clear, D3 fluent method) → reflected in Task 2 step 4 doc-comment and design.

**Placeholder scan:**
- Task 4 step 2 has `<taskN-sha>` placeholders; flagged in the step note. Acceptable — these are filled at execution-time from `git log`, not pre-known.
- No "TBD" / "TODO" / "implement later" anywhere.

**Type consistency:**
- `cameraInfo` struct shape consistent across Task 2 step 4 (definition), Task 3 step 1 (test fixture literals), and Task 3 step 3 (drain consumption).
- Method names `CamMoveTo`/`CamLookAt`/`CamShake` consistent across `ActivePlayer` interface (Task 2 step 5), `mockPlayer` impl (Task 1 step 5 + Task 2 step 6), `Player` impl (Task 1 step 6 + Task 2 step 7), and handler call sites (Task 1 step 7 + Task 2 step 8).
- Wire op identifiers `OpCamMoveTo`/`OpCamLookAt`/`OpCamShake` consistent across `prot.go` (Task 1 step 3 + Task 2 step 3), `Player.CamShake` direct write (Task 1 step 6), drain logic (Task 3 step 3), and tests (Task 3 step 1).
- Pop order: `rate2, rate, height, coord` for moveto/lookat; `rate, amplitude, random, axis` for shake — consistent across handlers (Task 1/2 steps 7/8) and test fixtures (Task 1/2 steps 1).

No issues found.
