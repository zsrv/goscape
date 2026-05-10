# NAI-145 zone triggers + SetMultiway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bundle-port TS `NetworkPlayer.updateMap` (`Engine-TS/src/engine/entity/NetworkPlayer.ts:255-287`) — both the `lastMapZone` block (NAI-142-D-R-D2) and the zone-block enrichment with `SetMultiway` emission + `triggerZone`/`triggerZoneExit` dispatch (NAI-142-D-R-D3) — into goscape's `(*Player).updateBuildArea`, atop the field + four trigger methods on `Player`.

**Architecture:** Add `Player.lastMapZone int = -1` field. Four cohesive trigger methods (`triggerMapzone`, `triggerMapzoneExit`, `triggerZone`, `triggerZoneExit`) live in a new `modules/world/player_zone_triggers.go` file, each calling `scriptProvider.GetByName(<key>)` then `p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)`. Add `OpSetMultiway = Op{254, 1}` to `pkg/io/protocol/game/server/prot.go`. Enrich `updateBuildArea` with a mapzone block (between the camera drain and the existing lastZone block) and SetMultiway + trigger dispatches inside the existing lastZone block. No new helpers; emission is inline `writeOut` matching the `OpCamShake` precedent.

**Tech Stack:** Go 1.26+; `pkg/script` (Provider, ScriptFile, QueueEngine); `pkg/coordgrid` (PackCoord/UnpackCoord); `pkg/gamemap` (IsMulti); `pkg/io/protocol/game/server` (Op constants); `modules/world` (Player, updateBuildArea, writeOut). Tests use existing fixtures `newTestServer`, `newTestPlayer`, `newZoneTestServer`, `newZoneTestPlayer` plus existing `captureCamWire` pattern.

---

## File Structure

| File | Status | Responsibility |
|---|---|---|
| `modules/world/player.go` | Modify | Add `lastMapZone int` field; init to `-1` at construction; rewrite `updateBuildArea` body and doc-comment to add mapzone block + zone-block enrichment. |
| `modules/world/player_zone_triggers.go` | Create | Four trigger methods (`triggerMapzone`, `triggerMapzoneExit`, `triggerZone`, `triggerZoneExit`), each ~10 LOC. |
| `modules/world/player_zone_triggers_test.go` | Create | Unit tests T1-T4: trigger fire / silent / key-shape arithmetic for the four methods. |
| `modules/world/player_build_area_test.go` | Create | Integration tests T5-T7 for `updateBuildArea`: mapzone first-tick fire, SetMultiway transition emit, no SetMultiway when both sides false. Includes a focused wire-capture helper that scans for `OpSetMultiway` opcode. |
| `pkg/io/protocol/game/server/prot.go` | Modify | Add `OpSetMultiway = Op{Opcode: 254, PayloadSize: 1}` constant. |

---

## Task 1: lastMapZone field + four trigger methods (RED → GREEN)

**Files:**
- Create: `modules/world/player_zone_triggers.go`
- Create: `modules/world/player_zone_triggers_test.go`
- Modify: `modules/world/player.go` (field block at line 358, construction at line 562)

### Step 1.1: Write the failing tests in `player_zone_triggers_test.go`

- [ ] **Step 1.1: Create the test file with four red tests**

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestTriggerMapzone_RegisteredScriptEnqueued (T1) — register
// [mapzone,0_50_60]; calling triggerMapzone(50<<6, 60<<6) must enqueue
// exactly one engine-queue entry pointing at the registered script.
func TestTriggerMapzone_RegisteredScriptEnqueued(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[mapzone,0_50_60]", LookupKey: 0x12340001}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerMapzone(50<<6, 60<<6)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (engine triggers must NOT route to primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v", got, sf)
	}
	if got := p.engineQueue[0].Type; got != script.QueueEngine {
		t.Errorf("p.engineQueue[0].Type: got %v, want QueueEngine", got)
	}
}

// TestTriggerMapzone_UnregisteredKeySilent (T2) — no script registered
// for the computed key; trigger must be a silent no-op.
func TestTriggerMapzone_UnregisteredKeySilent(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerMapzone(50<<6, 60<<6)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0", len(p.queue))
	}
	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0 (no script registered → silent no-op)", len(p.engineQueue))
	}
}

// TestTriggerZone_KeyShape_Bitmath (T3) — at world coords
// (x=3214, z=3398, level=0), TS bit-math:
//   mx = 3214 >> 6      = 50
//   mz = 3398 >> 6      = 53
//   lx = ((3214&0x3f)>>3)<<3 = ((14)>>3)<<3 = 1<<3 = 8
//   lz = ((3398&0x3f)>>3)<<3 = ((6)>>3)<<3  = 0<<3 = 0
// Expected key: [zone,0_50_53_8_0]. Pin both the literal key string AND
// that triggerZone successfully resolves it to the registered script.
func TestTriggerZone_KeyShape_Bitmath(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[zone,0_50_53_8_0]", LookupKey: 0x12340002}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerZone(0, 3214, 3398)

	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (key [zone,0_50_53_8_0] must resolve)", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v (key shape mismatch — recompute bit-math)", got, sf)
	}
}

// TestTriggerZoneExit_NoUnderscore_KeyShape (T4) — pins the absence of
// an underscore between `zoneexit` and the level segment. Same bit-math
// as T3; expected key is [zoneexit,0_50_53_8_0]. If a port accidentally
// emits [zoneexit_0_50_53_8_0] or [zone_exit,...], this test fails.
func TestTriggerZoneExit_NoUnderscore_KeyShape(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[zoneexit,0_50_53_8_0]", LookupKey: 0x12340003}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.triggerZoneExit(0, 3214, 3398)

	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (key [zoneexit,0_50_53_8_0] must resolve)", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v (key shape mismatch — check for stray underscore after `zoneexit`)", got, sf)
	}
}
```

- [ ] **Step 1.2: Run tests, verify they fail to compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestTrigger' -v`

Expected: COMPILATION FAILURE — `p.triggerMapzone undefined`, `p.triggerZone undefined`, `p.triggerZoneExit undefined`. The four methods don't yet exist.

### Step 1.3: Add `lastMapZone` field

- [ ] **Step 1.3: Modify `modules/world/player.go` field block**

Locate the field block around line 358 (immediately after the `lastZone` doc-comment + declaration). Append a new field declaration:

```go
// lastMapZone is the previously-witnessed packed mapzone coord
// (level=0, mapsquareX<<6, mapsquareZ<<6) used by updateBuildArea
// to detect per-tick mapzone (64-tile-grid) transitions. Sentinel
// -1 forces the first updateBuildArea call to fire triggerMapzone
// without firing triggerMapzoneExit (matches TS Player.ts:379
// `lastMapZone: number = -1` + NetworkPlayer.ts:259 `!== -1` guard).
lastMapZone int
```

Find it via `grep -n "lastZone int" modules/world/player.go`.

- [ ] **Step 1.4: Initialize `lastMapZone: -1` at construction**

Locate the player struct literal at `modules/world/player.go:562` (the line `lastZone: -1,`). Insert a new line directly after:

```go
lastZone:       -1, // NAI-142: sentinel; first updateBuildArea fires rebuildZones
lastMapZone:    -1, // NAI-145: sentinel; first updateBuildArea fires triggerMapzone (no exit)
```

Verify the surrounding struct literal alignment matches the existing style (tab-aligned colons).

### Step 1.5: Create `player_zone_triggers.go`

- [ ] **Step 1.5: Create the trigger-methods file**

```go
package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/script"
)

// triggerMapzone fires the [mapzone,0_X_Z] cache script when content
// is registered for the entered 64-tile mapzone. Mirrors TS
// Player.ts:561-567 (NAI-142-D-R-D2). Silent no-op when the
// scriptProvider returns nil — EnqueueScriptFile nil-guards sf
// internally (player_script.go:70-72).
//
// EnqueueScriptFile (file-based variant) matches TS
// `enqueueScript(trigger, ENGINE)` shape exactly — closer than the
// ID-roundtrip EnqueueScriptArgs form. Aligned with changeStat /
// advanceStat precedent (player_script.go:587-615).
func (p *Player) triggerMapzone(x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	name := fmt.Sprintf("[mapzone,0_%d_%d]", x>>6, z>>6)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerMapzoneExit fires the [mapzoneexit,0_X_Z] cache script for
// the exited 64-tile mapzone. Mirrors TS Player.ts:569-574. NOTE:
// exit key has NO underscore between `mapzoneexit` and the level
// segment — verified against LostCityRS/Content 2026-05-09 (17
// [mapzoneexit,...] real declarations).
func (p *Player) triggerMapzoneExit(x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	name := fmt.Sprintf("[mapzoneexit,0_%d_%d]", x>>6, z>>6)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerZone fires the [zone,L_MX_MZ_LX_LZ] cache script for the
// entered 8-tile zone. Mirrors TS Player.ts:576-585. The 5-segment
// key encodes mapsquare (MX,MZ) plus zone-local 8-tile-aligned
// offset within the mapsquare (LX,LZ).
func (p *Player) triggerZone(level, x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	mx := x >> 6
	mz := z >> 6
	lx := ((x & 0x3f) >> 3) << 3
	lz := ((z & 0x3f) >> 3) << 3
	name := fmt.Sprintf("[zone,%d_%d_%d_%d_%d]", level, mx, mz, lx, lz)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}

// triggerZoneExit fires the [zoneexit,L_MX_MZ_LX_LZ] cache script
// for the exited 8-tile zone. Mirrors TS Player.ts:587-596. NO
// underscore between `zoneexit` and the level segment — verified
// against LostCityRS/Content 2026-05-09 (5 [zoneexit,...] real
// declarations).
func (p *Player) triggerZoneExit(level, x, z int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	mx := x >> 6
	mz := z >> 6
	lx := ((x & 0x3f) >> 3) << 3
	lz := ((z & 0x3f) >> 3) << 3
	name := fmt.Sprintf("[zoneexit,%d_%d_%d_%d_%d]", level, mx, mz, lx, lz)
	sf := p.client.server.scriptProvider.GetByName(name)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}
```

- [ ] **Step 1.6: Run tests, verify all four pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestTrigger' -v`

Expected: PASS for `TestTriggerMapzone_RegisteredScriptEnqueued`, `TestTriggerMapzone_UnregisteredKeySilent`, `TestTriggerZone_KeyShape_Bitmath`, `TestTriggerZoneExit_NoUnderscore_KeyShape`.

- [ ] **Step 1.7: Run the full modules/world test suite — regression fence**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`

Expected: PASS. The new field add + new methods are additive; nothing else should break.

- [ ] **Step 1.8: Commit**

```bash
git add modules/world/player.go modules/world/player_zone_triggers.go modules/world/player_zone_triggers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-145 T1 — lastMapZone field + 4 zone-trigger methods

Port TS Player.ts:561-596 trigger family (triggerMapzone /
triggerMapzoneExit / triggerZone / triggerZoneExit). Each calls
scriptProvider.GetByName + EnqueueScriptFile(sf, 0, nil, nil,
QueueEngine), matching the TS enqueueScript(trigger, ENGINE) shape and
the existing changeStat/advanceStat precedent.

Add Player.lastMapZone field (-1 sentinel) for the upcoming
NetworkPlayer.ts:255-266 mapzone-transition block — wiring lands in T2.

T1-T4 unit tests pin: registered-key resolution, silent-no-op,
[zone,0_50_53_8_0] bit-math, and absence of underscore in the
[zoneexit,...] key.

Spec: docs/superpowers/specs/2026-05-10-nai-145-zone-triggers-setmultiway-design.md
Plan: docs/superpowers/plans/2026-05-10-nai-145-zone-triggers-setmultiway.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: OpSetMultiway opcode + updateBuildArea wiring + integration tests T5-T7

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `modules/world/player.go` (`updateBuildArea` body and doc-comment, lines ~894-953)
- Create: `modules/world/player_build_area_test.go`

### Step 2.1: Write the failing integration tests

- [ ] **Step 2.1: Create `modules/world/player_build_area_test.go` with three red tests**

```go
package world

import (
	"net"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/script"
)

// captureBuildAreaWire reads everything currently in the pipe (with a
// 1-second deadline), then walks the byte stream decrypting each
// opcode via the supplied parallel ISAAC decryptor and stepping past
// each known op's PayloadSize. Returns all decoded opcodes; tests
// assert presence/absence of OpSetMultiway by scanning the slice.
//
// Must be called BEFORE the action that causes the write (net.Pipe
// is synchronous — both sides must be ready concurrently). dec must
// already be advanced past any pre-existing bytes that newZoneTestPlayer
// emitted before this goroutine launched. In practice newZoneTestPlayer
// emits no wire packets (rebuildScenery / rebuildZones at fixture-
// construction time write zone state but not raw packets), so dec
// starts at stream offset 0.
//
// updateBuildArea may emit (in order, all optional): cam packets
// (OpCamMoveTo / OpCamLookAt / OpCamShake), zone-update packets from
// rebuildZones (OpUpdateZonePartialFollows / FullFollows /
// PartialEnclosed), and OpSetMultiway. The walker handles each.
// Variable-length packets (PayloadSize == -1 / -2) read their length
// prefix, then advance.
func captureBuildAreaWire(t *testing.T, cc net.Conn, dec *io2.Isaac) <-chan []int {
	t.Helper()
	ch := make(chan []int, 1)
	go func() {
		buf := make([]byte, 16384)
		cc.SetReadDeadline(time.Now().Add(time.Second))
		n, _ := cc.Read(buf)
		buf = buf[:n]

		var opcodes []int
		pos := 0
		for pos < n {
			encByte := int(buf[pos])
			key := int(dec.GetNext() & 0xff)
			opcode := (encByte - key) & 0xff
			pos++
			opcodes = append(opcodes, opcode)

			payloadSize, ok := buildAreaOpPayloadSize(opcode)
			if !ok {
				t.Errorf("captureBuildAreaWire: unknown opcode %d at byte pos %d (n=%d, opcodes-so-far=%v)",
					opcode, pos-1, n, opcodes)
				ch <- opcodes
				return
			}

			switch payloadSize {
			case -1:
				if pos >= n {
					t.Errorf("captureBuildAreaWire: truncated -1 length prefix at pos %d", pos)
					ch <- opcodes
					return
				}
				size := int(buf[pos])
				pos += 1 + size
			case -2:
				if pos+1 >= n {
					t.Errorf("captureBuildAreaWire: truncated -2 length prefix at pos %d", pos)
					ch <- opcodes
					return
				}
				size := (int(buf[pos]) << 8) | int(buf[pos+1])
				pos += 2 + size
			default:
				pos += payloadSize
			}
		}
		ch <- opcodes
	}()
	return ch
}

// buildAreaOpPayloadSize maps every opcode that updateBuildArea may
// emit to its payload size. Anything else returns ok=false → walker
// fails the test with a clear "unknown opcode" message.
func buildAreaOpPayloadSize(opcode int) (int, bool) {
	switch opcode {
	case int(gameserver.OpCamMoveTo.Opcode):
		return gameserver.OpCamMoveTo.PayloadSize, true
	case int(gameserver.OpCamLookAt.Opcode):
		return gameserver.OpCamLookAt.PayloadSize, true
	case int(gameserver.OpCamShake.Opcode):
		return gameserver.OpCamShake.PayloadSize, true
	case int(gameserver.OpSetMultiway.Opcode):
		return gameserver.OpSetMultiway.PayloadSize, true
	case int(gameserver.OpUpdateZonePartialFollows.Opcode):
		return gameserver.OpUpdateZonePartialFollows.PayloadSize, true
	case int(gameserver.OpUpdateZoneFullFollows.Opcode):
		return gameserver.OpUpdateZoneFullFollows.PayloadSize, true
	case int(gameserver.OpUpdateZonePartialEnclosed.Opcode):
		return gameserver.OpUpdateZonePartialEnclosed.PayloadSize, true
	}
	return 0, false
}

// containsOpcode returns true if any element in opcodes equals target.
func containsOpcode(opcodes []int, target int) bool {
	for _, op := range opcodes {
		if op == target {
			return true
		}
	}
	return false
}

// TestUpdateBuildArea_FirstTickMapzoneFires_ExitDoesNot (T5) — fresh
// player at (3200, 3200, 0). lastMapZone starts -1 → the first
// updateBuildArea call must enqueue triggerMapzone (entry script)
// but NOT triggerMapzoneExit (exit gated on lastMapZone != -1).
// After the call, lastMapZone is updated to the packed coord.
func TestUpdateBuildArea_FirstTickMapzoneFires_ExitDoesNot(t *testing.T) {
	s := newZoneTestServer(t)
	s.scriptProvider = script.NewProvider()
	enterSf := &script.ScriptFile{Name: "[mapzone,0_50_50]", LookupKey: 0xa1}
	exitSf := &script.ScriptFile{Name: "[mapzoneexit,0_50_50]", LookupKey: 0xa2}
	s.scriptProvider.Register(enterSf)
	s.scriptProvider.Register(exitSf)

	p, _ := newZoneTestPlayer(t, s, 1, 3200, 3200, 0)

	if p.lastMapZone != -1 {
		t.Fatalf("setup: lastMapZone got %d, want -1", p.lastMapZone)
	}

	p.updateBuildArea()

	// Filter engineQueue to entries whose Script Name starts with [mapzone or [mapzoneexit.
	var enterCount, exitCount int
	for _, req := range p.engineQueue {
		if req.Script == enterSf {
			enterCount++
		}
		if req.Script == exitSf {
			exitCount++
		}
	}
	if enterCount != 1 {
		t.Errorf("triggerMapzone fire count: got %d, want 1", enterCount)
	}
	if exitCount != 0 {
		t.Errorf("triggerMapzoneExit fire count: got %d, want 0 (lastMapZone was -1, exit gated)", exitCount)
	}
	if p.lastMapZone == -1 {
		t.Error("lastMapZone must be updated after the transition; still -1")
	}
}

// TestUpdateBuildArea_SetMultiwayEmitOnEntry (T6) — fresh player
// (lastZone=-1) standing on a tile marked multi-combat. The first
// updateBuildArea call decodes lastWasMulti=false (UnpackCoord(-1)
// → defensive Position → IsMulti map-miss) and nowIsMulti=true →
// transition fires; OpSetMultiway with payload [0x01] hits the wire.
func TestUpdateBuildArea_SetMultiwayEmitOnEntry(t *testing.T) {
	s := newZoneTestServer(t)
	// SetMulti requires gamemap to be initialized — newZoneTestServer
	// does not init gamemap by default; do it explicitly here.
	s.gamemap = newTestGamemap(t)

	p, cc := newZoneTestPlayer(t, s, 1, 3200, 3200, 0)
	s.gamemap.SetMulti(p.x, p.z, p.level, true)

	dec := io2.New([4]uint32{1, 2, 3, 4}) // matches slot=1 encryptor seed
	received := captureBuildAreaWire(t, cc, dec)

	p.updateBuildArea()
	p.client.flushWrite()

	opcodes := <-received
	if !containsOpcode(opcodes, int(gameserver.OpSetMultiway.Opcode)) {
		t.Errorf("OpSetMultiway not emitted; opcodes seen=%v (expected opcode 254)", opcodes)
	}
}

// TestUpdateBuildArea_NoSetMultiwayWhenBothFalse (T7) — fresh player
// in a non-multi tile with no SetMulti call. lastWasMulti=false (from
// -1 sentinel) AND nowIsMulti=false → no transition; no OpSetMultiway
// on the wire. Other packets (zone updates from rebuildZones) may
// appear; pin only the absence of opcode 254.
func TestUpdateBuildArea_NoSetMultiwayWhenBothFalse(t *testing.T) {
	s := newZoneTestServer(t)
	s.gamemap = newTestGamemap(t)

	p, cc := newZoneTestPlayer(t, s, 1, 3200, 3200, 0)
	// Deliberately NOT calling SetMulti — lookup returns false.

	dec := io2.New([4]uint32{1, 2, 3, 4})
	received := captureBuildAreaWire(t, cc, dec)

	p.updateBuildArea()
	p.client.flushWrite()

	opcodes := <-received
	if containsOpcode(opcodes, int(gameserver.OpSetMultiway.Opcode)) {
		t.Errorf("OpSetMultiway must NOT be emitted when both sides false; opcodes seen=%v", opcodes)
	}
}

// newTestGamemap constructs a fresh GameMap suitable for SetMulti +
// IsMulti tests. Mirrors the inline init pattern used by
// world_zone_test.go (s.gamemap = gamemap.New(discardLogger())).
func newTestGamemap(t *testing.T) *gamemapPkg.GameMap {
	t.Helper()
	return gamemapPkg.New(discardLogger())
}
```

Add the missing import alias at the top: change `import` block to include `gamemapPkg "github.com/zsrv/goscape/pkg/gamemap"`. Final import block:

```go
import (
	"net"
	"testing"
	"time"

	gamemapPkg "github.com/zsrv/goscape/pkg/gamemap"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 2.2: Run tests, verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateBuildArea_(FirstTickMapzone|SetMultiwayEmit|NoSetMultiwayWhen)' -v`

Expected: COMPILATION FAILURE — `gameserver.OpSetMultiway undefined`. Also runtime FAIL once compilation passes — `lastMapZone` updates and SetMultiway emission don't yet exist.

### Step 2.3: Add the OpSetMultiway opcode constant

- [ ] **Step 2.3: Modify `pkg/io/protocol/game/server/prot.go`**

Locate the section near other state-flag opcodes (around line 65-77; the file groups `OpUpdateStat`, `OpUpdateRunEnergy`, `OpUpdateRunWeight`, etc.). Add `OpSetMultiway` adjacent to other "tell client to update overlay state" opcodes. Concrete placement: after `OpUpdateRunWeight` (line 72) so it's grouped with the player-state-update opcodes.

```go
// OpSetMultiway tells the client to show or hide the multi-combat
// overlay icon (top-right of the chatbox). Sent on transitions across
// multi-combat zone boundaries from updateBuildArea. 1-byte payload
// (pbool): 0 to hide overlay (left a multi zone), 1 to show overlay
// (entered a multi zone). Mirrors TS ServerGameProt.SET_MULTIWAY
// (opcode 254, size 1) and SetMultiwayEncoder (`buf.pbool(message.hidden)`)
// at Engine-TS/src/network/game/server/codec/SetMultiwayEncoder.ts.
OpSetMultiway = Op{Opcode: 254, PayloadSize: 1}
```

### Step 2.4: Modify `updateBuildArea` body and doc-comment

- [ ] **Step 2.4a: Replace the doc-comment block above `updateBuildArea`**

Locate `func (p *Player) updateBuildArea()` (currently around line 924). Replace the doc-comment block (lines ~894-923) with:

```go
// updateBuildArea drains deferred camera packets, then handles per-tick
// mapzone (64-tile-grid) and zone (8-tile-grid) transitions. Mirrors TS
// NetworkPlayer.updateMap (NetworkPlayer.ts:243-287) end-to-end.
//
// Step 1 — camera drain (NetworkPlayer.ts:244-253, NAI-143):
//
//	for (const info of this.cameraPackets.all()) {
//	    const localX = info.camX - CoordGrid.zoneOrigin(this.originX);
//	    const localZ = info.camZ - CoordGrid.zoneOrigin(this.originZ);
//	    // ... write CamMoveTo or CamLookAt ...
//	}
//
// Origin freshness is preserved by NAI-93 ordering: Player.updateMap
// (TS BuildArea.rebuildNormal slot) runs in Server.processInfo before
// processOut, so p.originX/Z are already anchored to the current
// rebuild position when this drain fires.
//
// Step 2 — lastMapZone transition (NetworkPlayer.ts:255-266, NAI-145):
//
//	const mapZone = CoordGrid.packCoord(0, (this.x >> 6) << 6, (this.z >> 6) << 6);
//	if (this.lastMapZone !== mapZone) {
//	    if (this.lastMapZone !== -1) {
//	        const { x, z } = CoordGrid.unpackCoord(this.lastMapZone);
//	        this.triggerMapzoneExit(x, z);
//	    }
//	    this.triggerMapzone((this.x >> 6) << 6, (this.z >> 6) << 6);
//	    this.lastMapZone = mapZone;
//	}
//
// Step 3 — lastZone transition (NetworkPlayer.ts:268-287, NAI-142 +
// NAI-145):
//
//	const zone = CoordGrid.packCoord(this.level, (this.x >> 3) << 3, (this.z >> 3) << 3);
//	if (this.lastZone !== zone) {
//	    this.buildArea.rebuildZones();
//	    const lastWasMulti = World.gameMap.isMulti(this.lastZone);
//	    const nowIsMulti = World.gameMap.isMulti(zone);
//	    if (lastWasMulti != nowIsMulti) {
//	        this.write(new SetMultiway(nowIsMulti));
//	    }
//	    if (this.lastZone !== -1) {
//	        const { level, x, z } = CoordGrid.unpackCoord(this.lastZone);
//	        this.triggerZoneExit(level, x, z);
//	    }
//	    this.triggerZone(this.level, (this.x >> 3) << 3, (this.z >> 3) << 3);
//	    this.lastZone = zone;
//	}
//
// First-tick semantics for both blocks: the -1 sentinels suppress
// triggerMapzoneExit / triggerZoneExit on the very first updateBuildArea
// call. SetMultiway emission still fires on first tick if the player
// spawns into a multi-combat tile (UnpackCoord(-1) yields a defensive
// position whose IsMulti lookup map-misses to false → transition
// detected). Matches TS World.gameMap.isMulti(-1) → false behavior.
```

- [ ] **Step 2.4b: Replace the body of `updateBuildArea`**

The existing body (lines ~924-953) is:

```go
func (p *Player) updateBuildArea() {
	// 1. drain cameraPackets — TS NetworkPlayer.ts:244-253. ...
	for i := range p.cameraPackets {
		info := p.cameraPackets[i]
		localX := info.camX - coordgrid.ZoneOrigin(p.originX)
		localZ := info.camZ - coordgrid.ZoneOrigin(p.originZ)
		payload := []byte{
			byte(localX),
			byte(localZ),
			byte(info.height >> 8), byte(info.height),
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

Replace it with:

```go
func (p *Player) updateBuildArea() {
	// 1. drain cameraPackets — TS NetworkPlayer.ts:244-253 (NAI-143).
	for i := range p.cameraPackets {
		info := p.cameraPackets[i]
		localX := info.camX - coordgrid.ZoneOrigin(p.originX)
		localZ := info.camZ - coordgrid.ZoneOrigin(p.originZ)
		payload := []byte{
			byte(localX),
			byte(localZ),
			byte(info.height >> 8), byte(info.height),
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

	// 2. lastMapZone transition — TS NetworkPlayer.ts:255-266 (NAI-145
	// / NAI-142-D-R-D2).
	mapZone := coordgrid.PackCoord(0, (p.x>>6)<<6, (p.z>>6)<<6)
	if p.lastMapZone != mapZone {
		if p.lastMapZone != -1 {
			prev := coordgrid.UnpackCoord(p.lastMapZone)
			p.triggerMapzoneExit(prev.X, prev.Z)
		}
		p.triggerMapzone((p.x>>6)<<6, (p.z>>6)<<6)
		p.lastMapZone = mapZone
	}

	// 3. lastZone transition — TS NetworkPlayer.ts:268-287 (NAI-142
	// rebuildZones + NAI-145 SetMultiway/zone-trigger enrichment).
	zone := coordgrid.PackCoord(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
	if p.lastZone != zone {
		p.rebuildZones()

		// SetMultiway emit on multi-flag transition. lastZone=-1 first-tick
		// unpacks to {Level:3, X:0x3FFF, Z:0x3FFF} → IsMulti map-miss →
		// false, matching TS World.gameMap.isMulti(-1) → false. The
		// gamemap nil-guard is goscape defensive (TS skips this check —
		// World.gameMap is always present in TS but test fixtures here
		// often omit it).
		var lastWasMulti, nowIsMulti bool
		if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
			gm := p.client.server.gamemap
			prev := coordgrid.UnpackCoord(p.lastZone)
			lastWasMulti = gm.IsMulti(prev.X, prev.Z, prev.Level)
			nowIsMulti = gm.IsMulti(p.x, p.z, p.level)
		}
		if lastWasMulti != nowIsMulti {
			var hidden byte
			if nowIsMulti {
				hidden = 1
			}
			p.writeOut(gameserver.OpSetMultiway, []byte{hidden})
		}

		if p.lastZone != -1 {
			prev := coordgrid.UnpackCoord(p.lastZone)
			p.triggerZoneExit(prev.Level, prev.X, prev.Z)
		}
		p.triggerZone(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
		p.lastZone = zone
	}
}
```

- [ ] **Step 2.5: Run integration tests, verify all three pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateBuildArea_(FirstTickMapzone|SetMultiwayEmit|NoSetMultiwayWhen)' -v`

Expected: PASS for `TestUpdateBuildArea_FirstTickMapzoneFires_ExitDoesNot`, `TestUpdateBuildArea_SetMultiwayEmitOnEntry`, `TestUpdateBuildArea_NoSetMultiwayWhenBothFalse`.

- [ ] **Step 2.6: Run the full updateBuildArea-adjacent test family — regression fence**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateBuildArea|TestTrigger|TestShouldRebuild|TestUpdateZones' -v`

Expected: PASS. Existing camera-drain tests (`TestUpdateBuildAreaCameraDrain_*`) and zone-update tests must still pass. Note: those tests also use `newZoneTestPlayer` which now starts with `lastMapZone == -1`, so the camera-drain test will now ALSO trigger the mapzone block and enqueue triggerMapzone. **The wire output is unchanged** because triggerMapzone enqueues to engineQueue (not the wire). If a cam test fails, inspect: the failure is informative.

- [ ] **Step 2.7: Run the full repo test suite — final regression fence**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: PASS across all packages and clean vet.

- [ ] **Step 2.8: Commit**

Use a HEREDOC-via-tmpfile pattern to avoid nested-heredoc shell issues:

```bash
# Write commit message to a tmp file (no nested heredoc needed):
cat > "$TMPDIR/nai145-t2-msg.txt" <<'MSG_EOF'
feat(player): NAI-145 T2 — OpSetMultiway + updateBuildArea zone-trigger wiring

Port the lastMapZone block (NetworkPlayer.ts:255-266, NAI-142-D-R-D2)
and enrich the existing lastZone block with SetMultiway emit +
triggerZone/triggerZoneExit dispatch (NetworkPlayer.ts:268-287,
NAI-142-D-R-D3).

Add OpSetMultiway = Op{Opcode: 254, PayloadSize: 1} matching
ServerGameProt.SET_MULTIWAY. Emission is inline writeOut — single
call site, no helper, mirroring OpCamShake direct-write precedent.

First-tick semantics: lastZone=-1 unpacks to a defensive position
whose IsMulti map-misses to false, matching TS isMulti(-1) → false.
Player spawning into a multi-combat tile emits SetMultiway(1) on
the first tick. Mapzone/zone exit triggers stay gated on the -1
sentinel.

T5/T6/T7 integration tests pin: first-tick mapzone-enter fires +
mapzone-exit suppressed, SetMultiway emit on multi-tile entry, and
no SetMultiway when both sides are non-multi.

Spec: docs/superpowers/specs/2026-05-10-nai-145-zone-triggers-setmultiway-design.md
Plan: docs/superpowers/plans/2026-05-10-nai-145-zone-triggers-setmultiway.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
MSG_EOF

git add pkg/io/protocol/game/server/prot.go modules/world/player.go modules/world/player_build_area_test.go
git commit --no-gpg-sign -F "$TMPDIR/nai145-t2-msg.txt"
```

---

## Task 3: Reviewer fixups (Sonnet-only)

**Files:** TBD per reviewer feedback. This task is a placeholder; the controller dispatches a Sonnet-model `superpowers:code-reviewer` agent (per memory `superpowers_code_reviewer_model`) after Task 2's commit lands. Reviewer scope: TS-fidelity at NetworkPlayer.ts:255-287 and Player.ts:561-596, key-shape correctness, first-tick semantics, gamemap nil-guard label, integration test coverage.

- [ ] **Step 3.1: Dispatch the reviewer**

Use Agent tool with `subagent_type=feature-dev:code-reviewer`, `model=sonnet`. Brief: "NAI-145 implementation review at HEAD (commits T1 + T2). Scope: TS parity vs LostCityRS/Engine-TS NetworkPlayer.ts:255-287 + Player.ts:561-596. Verify: (a) lastMapZone -1 init in player.go construction, (b) four trigger methods at modules/world/player_zone_triggers.go match TS bit-math + key shapes (no underscore in mapzoneexit/zoneexit), (c) OpSetMultiway opcode 254 size 1, (d) updateBuildArea step ordering matches TS line-by-line (camera drain → mapzone block → zone block; within zone block: rebuildZones → SetMultiway emit → triggerZoneExit → triggerZone), (e) gamemap nil-guard doc-comment labels it 'goscape defensive; TS skips this check' per defensive_gate_doc_comment_label memory, (f) tests T1-T7 cover the spec test strategy. Filter to Critical / Important issues; ignore stylistic nits."

- [ ] **Step 3.2: Address reviewer findings**

For each Critical / Important finding: write a focused fix commit. Keep each fixup under ~30 LOC; if scope exceeds that, push to NAI-146 follow-up tracker per `nai_followups` memory.

- [ ] **Step 3.3: Re-run the full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: PASS + clean vet.

- [ ] **Step 3.4: Close commit**

Use the same tmpfile pattern as Step 2.8 to avoid nested heredocs:

```bash
cat > "$TMPDIR/nai145-close-msg.txt" <<'MSG_EOF'
chore(close): NAI-145 — zone triggers + SetMultiway; smoke deferred

Bundle close for NAI-142-D-R-D2 (lastMapZone + triggerMapzone/Exit)
and NAI-142-D-R-D3 (triggerZone/Exit + SetMultiway). TS source map
at NetworkPlayer.ts:255-287 + Player.ts:561-596 ported end-to-end.

Smoke deferred per cascade_theory_smoke_binding (foundational
secondary infra). Bind on next D2/D3-adjacent observable symptom —
candidates: wilderness multi-zone boundary crossing (overlay flip),
[mapzone,...] / [zone,...] content-script fires.

Carry-forward (still open): DEVIATION-NAI-144-D4 (canAccess≈!Busy),
NAI-144-D-MoveClickRequestSetter (movement gate inert at HEAD).

Closes memory:
- nai_followups (NAI-145 entry)
- spec_followup_tracker_freshness (NAI-144 §9 EnqueueScriptArgs
  prescription corrected to EnqueueScriptFile per TS Player.ts:565
  enqueueScript shape and existing changeStat/advanceStat precedent)

Spec: docs/superpowers/specs/2026-05-10-nai-145-zone-triggers-setmultiway-design.md
Plan: docs/superpowers/plans/2026-05-10-nai-145-zone-triggers-setmultiway.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
MSG_EOF

git commit --allow-empty --no-gpg-sign -F "$TMPDIR/nai145-close-msg.txt"
```

---

## Self-review notes

**Spec coverage:**
- §1 scope — Task 1 (field + triggers) + Task 2 (opcode + wiring) cover both D2 and D3.
- §2 TS source map — every TS line-range from the table is referenced in either Step 1.5 (trigger methods) or Step 2.4 (updateBuildArea body + doc-comment).
- §3 architecture — Step 1.3 (field), Step 1.5 (methods), Step 2.3 (opcode), Step 2.4 (wiring) cover all four sub-sections.
- §4 tests — T1, T2, T3, T4 in Step 1.1; T5, T6, T7 in Step 2.1.
- §5 risk register — R1 (gamemap nil-guard) addressed by inline check in Step 2.4b + doc-comment label; R2 (first-tick lastWasMulti) pinned by T6/T7; R3 (key-shape drift) pinned by T3/T4; R4 (EnqueueScriptFile vs Args) baked into Step 1.5 method bodies; R5 (bit-arithmetic) pinned by T3/T4 + Step 1.5 explicit operator-precedence.
- §6 memories — applied at the steps that invoke them (defensive label in Step 2.4b doc-comment).
- §7 smoke — Task 3 close commit notes deferral + bind candidates.
- §8 cadence — 3 tasks (T1 trigger methods, T2 opcode + wiring, T3 reviewer fixups) collapses spec's 4-task split because OpSetMultiway is a single-line constant and doesn't warrant its own task. The collapse keeps each task self-contained (T1 = unit-testable in isolation; T2 = integration-testable in isolation; T3 = review).

**Type / signature consistency:**
- `triggerMapzone(x, z int)` / `triggerMapzoneExit(x, z int)` / `triggerZone(level, x, z int)` / `triggerZoneExit(level, x, z int)` — same signatures used in Step 1.1 tests, Step 1.5 implementations, and Step 2.4b updateBuildArea call sites.
- `EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)` — same call shape in all four trigger methods + matches existing `changeStat`/`advanceStat` at player_script.go:592, 614.
- `OpSetMultiway` — declared in Step 2.3, referenced in Step 2.1 helper + Step 2.4b emission.
- `lastMapZone int` — added in Step 1.3 field block, initialized in Step 1.4 construction, read/written in Step 2.4b updateBuildArea body, asserted in Step 2.1 T5 test.

**No placeholders:** every code block above shows the actual code to write or the actual command to run. Task 3 is intentionally placeholder-shaped (reviewer feedback unknowable in advance) but each Step 3.N has a concrete action with concrete commands.

- [ ] **Step 2.4b: Replace the body of `updateBuildArea`**

The existing body (lines ~924-953) is:

```go
func (p *Player) updateBuildArea() {
	// 1. drain cameraPackets — TS NetworkPlayer.ts:244-253. ...
	for i := range p.cameraPackets {
		info := p.cameraPackets[i]
		localX := info.camX - coordgrid.ZoneOrigin(p.originX)
		localZ := info.camZ - coordgrid.ZoneOrigin(p.originZ)
		payload := []byte{
			byte(localX),
			byte(localZ),
			byte(info.height >> 8), byte(info.height),
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
	if p.lastZone \!= zone {
		p.rebuildZones()
		p.lastZone = zone
	}
}
```

Replace it with:

```go
func (p *Player) updateBuildArea() {
	// 1. drain cameraPackets — TS NetworkPlayer.ts:244-253 (NAI-143).
	for i := range p.cameraPackets {
		info := p.cameraPackets[i]
		localX := info.camX - coordgrid.ZoneOrigin(p.originX)
		localZ := info.camZ - coordgrid.ZoneOrigin(p.originZ)
		payload := []byte{
			byte(localX),
			byte(localZ),
			byte(info.height >> 8), byte(info.height),
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

	// 2. lastMapZone transition — TS NetworkPlayer.ts:255-266 (NAI-145
	// / NAI-142-D-R-D2).
	mapZone := coordgrid.PackCoord(0, (p.x>>6)<<6, (p.z>>6)<<6)
	if p.lastMapZone \!= mapZone {
		if p.lastMapZone \!= -1 {
			prev := coordgrid.UnpackCoord(p.lastMapZone)
			p.triggerMapzoneExit(prev.X, prev.Z)
		}
		p.triggerMapzone((p.x>>6)<<6, (p.z>>6)<<6)
		p.lastMapZone = mapZone
	}

	// 3. lastZone transition — TS NetworkPlayer.ts:268-287 (NAI-142
	// rebuildZones + NAI-145 SetMultiway/zone-trigger enrichment).
	zone := coordgrid.PackCoord(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
	if p.lastZone \!= zone {
		p.rebuildZones()

		// SetMultiway emit on multi-flag transition. lastZone=-1 first-tick
		// unpacks to {Level:3, X:0x3FFF, Z:0x3FFF} → IsMulti map-miss →
		// false, matching TS World.gameMap.isMulti(-1) → false. The
		// gamemap nil-guard is goscape defensive (TS skips this check —
		// World.gameMap is always present in TS but test fixtures here
		// often omit it).
		var lastWasMulti, nowIsMulti bool
		if p.client \!= nil && p.client.server \!= nil && p.client.server.gamemap \!= nil {
			gm := p.client.server.gamemap
			prev := coordgrid.UnpackCoord(p.lastZone)
			lastWasMulti = gm.IsMulti(prev.X, prev.Z, prev.Level)
			nowIsMulti = gm.IsMulti(p.x, p.z, p.level)
		}
		if lastWasMulti \!= nowIsMulti {
			var hidden byte
			if nowIsMulti {
				hidden = 1
			}
			p.writeOut(gameserver.OpSetMultiway, []byte{hidden})
		}

		if p.lastZone \!= -1 {
			prev := coordgrid.UnpackCoord(p.lastZone)
			p.triggerZoneExit(prev.Level, prev.X, prev.Z)
		}
		p.triggerZone(p.level, (p.x>>3)<<3, (p.z>>3)<<3)
		p.lastZone = zone
	}
}
```

- [ ] **Step 2.5: Run integration tests, verify all three pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateBuildArea_(FirstTickMapzone|SetMultiwayEmit|NoSetMultiwayWhen)' -v`

Expected: PASS for `TestUpdateBuildArea_FirstTickMapzoneFires_ExitDoesNot`, `TestUpdateBuildArea_SetMultiwayEmitOnEntry`, `TestUpdateBuildArea_NoSetMultiwayWhenBothFalse`.

- [ ] **Step 2.6: Run the full updateBuildArea-adjacent test family — regression fence**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestUpdateBuildArea|TestTrigger|TestShouldRebuild|TestUpdateZones' -v`

Expected: PASS. Existing camera-drain tests (`TestUpdateBuildAreaCameraDrain_*`) and zone-update tests must still pass. If `TestUpdateBuildAreaCameraDrain_moveto` or related cam tests fail because the wire stream now has additional packets (lastMapZone block fires for the first time on those fixtures), inspect: those tests also use `newZoneTestPlayer` (lastMapZone starts -1), so the camera-drain test will now ALSO trigger triggerMapzone enqueue. **The wire output should still be the same** — triggerMapzone enqueues to engineQueue, it does not write to the wire. Verify this is the case; if the test fails, the failure is informative.

- [ ] **Step 2.7: Run the full repo test suite — final regression fence**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS across all packages. Also run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — expected clean.

- [ ] **Step 2.8: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go modules/world/player.go modules/world/player_build_area_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(player): NAI-145 T2 — OpSetMultiway + updateBuildArea zone-trigger wiring

Port the lastMapZone block (NetworkPlayer.ts:255-266, NAI-142-D-R-D2)
and enrich the existing lastZone block with SetMultiway emit +
triggerZone/triggerZoneExit dispatch (NetworkPlayer.ts:268-287,
NAI-142-D-R-D3).

Add OpSetMultiway = Op{Opcode: 254, PayloadSize: 1} matching
ServerGameProt.SET_MULTIWAY. Emission is inline writeOut — single
call site, no helper, mirroring OpCamShake direct-write precedent.

First-tick semantics: lastZone=-1 unpacks to a defensive position
whose IsMulti map-misses to false, matching TS isMulti(-1) → false.
Player spawning into a multi-combat tile emits SetMultiway(1) on
the first tick. Mapzone/zone exit triggers stay gated on the -1
sentinel.

T5/T6/T7 integration tests pin: first-tick mapzone-enter fires +
mapzone-exit suppressed, SetMultiway emit on multi-tile entry, and
no SetMultiway when both sides are non-multi.

Spec: docs/superpowers/specs/2026-05-10-nai-145-zone-triggers-setmultiway-design.md
Plan: docs/superpowers/plans/2026-05-10-nai-145-zone-triggers-setmultiway.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
