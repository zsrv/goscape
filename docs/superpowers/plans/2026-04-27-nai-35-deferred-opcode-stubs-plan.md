# NAI-35 Implementation Plan — deferred opcode stubs (NPC_PARAM, MAP_PLAYERCOUNT, NPC_HUNTALL, HUNTALL, HUNTNEXT, MAP_FINDSQUARE)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the six deferred opcode stubs surfaced by NAI-33 / NAI-34, retire NAI-33-D1 (`huntvis` dead-field), and unblock the wider content-script call surface (NPC_PARAM has 202 callers; MAP_FINDSQUARE has 77; HUNTALL family has ~50 combined).

**Architecture:** Multi-task feature-port. Path A extension of NAI-33's `NpcIterator` (adds HuntAll mode + activates the deferred huntvis filter). New `pkg/script/player_iterator.go` mirroring the NpcIterator template (HuntAll-only constructor; Distance/Zone modes deferred per NAI-35-D2 to avoid speculative dead-API). Cross-package surface: `pkg/script` ↔ `modules/world` ↔ `pkg/pathfinder/routefinder` ↔ `pkg/zone`.

**Tech Stack:** Go 1.26+ (use modern Go syntax via the `use-modern-go` skill). Reference TS: `LostCityRS/Engine-TS` only per `ts_source_canonical_path.md`.

**Spec:** `docs/superpowers/specs/2026-04-27-nai-35-deferred-opcode-stubs-design.md` (commit `cdfc96e`).

---

## File Structure

| Path | Action | Net production lines | Owning task |
|---|---|---|---|
| `pkg/script/handlers_config.go` | Add `handleNpcParam` (alongside `handleNcParam:280`) | +14 | T1 |
| `pkg/script/handlers_npc.go` | Add `handleNpcHuntAll` | +24 | T3 |
| `pkg/script/handlers_player.go` | Add `handleHuntAll` and `handleHuntNext` | +50 | T4, T5 |
| `pkg/script/handlers_map.go` (new) | Add `handleMapPlayerCount`, `handleMapFindSquare` | +135 | T2, T6 |
| `pkg/script/handlers.go` | Register 6 new dispatch entries | +6 | each task |
| `pkg/script/npc_iterator.go` | Add `NpcIteratorHuntAll` mode + `NewHuntAllNpcIterator` constructor; activate huntvis branch in `passesFilter`; drop NAI-33-D1 deferred-comments | ±25 | T3, T7 |
| `pkg/script/player_iterator.go` (new) | `PlayerIterator` struct + `Stale` + `passesFilter` + `Next` + `advanceZone` + `NewHuntAllPlayerIterator` constructor; `PlayerLookupForIter` interface contract | +130 | T4 |
| `pkg/script/state.go` | Add `playerIterator *PlayerIterator` field; extend `PlayerLookup` interface with `ZonePlayers` | +5 | T2 (interface) + T4 (state) |
| `modules/world/player_script_lookup.go` | Add `ZonePlayers` impl delegating to `Zone.PlayersSafe` | +18 | T2 |
| `pkg/pathfinder/routefinder/linevalidator.go` | (Existing — no change; T6 calls `HasLineOfSight` / `HasLineOfWalk` directly) | 0 | (none) |
| `pkg/script/findsquare_type.go` (new) | `MapFindSquareType` constants + `checkFindSquareType` validator | +18 | T6 |
| `pkg/script/handlers_*_test.go` | Add per-task handler tests | +120 | each task |
| `pkg/script/npc_iterator_test.go` | Add huntvis-active `passesFilter` pins; HuntAll-mode iterator end-to-end | +35 | T3 |
| `pkg/script/player_iterator_test.go` (new) | 7-test suite (Layer 1 mechanics) mirroring NpcIterator | +60 | T4 |
| `pkg/script/handlers_map_test.go` (new) | Test fixtures for MAP_PLAYERCOUNT + MAP_FINDSQUARE | +85 | T2, T6 |

**Aggregate**: ~395 net production + ~200 test (per spec).

---

## Plan-author preflight resolutions

The spec listed 10 preflight items. Resolved by plan-author against HEAD `afdc28b` before plan-write:

| # | Spec preflight item | Resolution |
|---|---|---|
| 1 | Configs.NpcType API shape | `s.Configs.NpcType(typeID) → *objtype.NpcType` with `.Params` field of type `objtype.ParamMap`. Confirmed at handlers_config.go:286-290 (handleNcParam). |
| 2 | PopInt convention | `s.PopInt()` returns top-of-stack first. TS `popInts(N)` returns array `[c1, …, cN]` where `c1` was deepest at pop time → goscape pop-by-one is REVERSE order. Confirmed by handleEnum at handlers_config.go:69-72 (key, enumID, outputType, inputType pop in that order). |
| 3 | IsLineOfSight wrapper status | `(LineValidator).HasLineOfSight` already exists at `pkg/pathfinder/routefinder/linevalidator.go:19` with same signature as `HasLineOfWalk` (line 31). T6 calls these directly; **no new wrapper needed**. |
| 4 | Members/F2P world flag | `WorldVars.MapMembers() int` already in script-VM at `pkg/script/state.go:45`. T6 calls `s.World.MapMembers() != 0` to determine `freeWorld`. **NAI-35-D3 reduces to "use existing accessor" — no infrastructure work.** |
| 5 | NAI-33-D1 / S7f-D1 comment sites | Confirmed sites: `pkg/script/npc_iterator.go:39-44` (huntvis field comment), `:69-71` (passesFilter "carryover" comment), `:79` (intentional-omission comment), `pkg/script/state.go:64-66` (NpcLookup huntvis doc-comment). T7 enumerates these. |
| 6 | PlayerLookup interface body | `pkg/script/state.go:27-29` — currently has only `LookupPlayerByUID(uid int) ActivePlayer`. T2 extends with `ZonePlayers(level, zoneX, zoneZ int) []ActivePlayer`. |
| 7 | Compiled-bytecode call-site inventory | Verified by source-grep against `LostCityRS/Content/scripts/`: npc_param=202, huntall=35, npc_huntall=18, map_findsquare=77, map_playercount=4. |
| 8 | HUNTNEXT push-shape | Mirrors `handleNpcFindNext` at handlers_npc.go:605-622: nil/done → `s.PushInt(0)`; hit → `s.Self = p; s.Pointers \|= PtrActivePlayer; s.PushInt(1)`. Pattern confirmed at handlers_player.go:715-716 (P_FINDUID). |
| 9 | `checkNotNull` + `checkHuntVis` helper locations | `checkCoord` at handlers_npc.go:11; `checkNotNull` (NOT `checkNumberNotNull` — corrected) used widely; `checkHuntVis` at handlers_npc.go:32. Signature: `(v int, op string) error`. |
| 10 | `Configs.NpcType` Params accessor | `*objtype.NpcType.Params` is `objtype.ParamMap` (lowercase `params` is internal field; `Params` is exported). Mirrors handleNcParam line 290 exactly. |

---

## Task 1 — NPC_PARAM (OpNpcParam=2529)

**Files:**
- Modify: `pkg/script/handlers_config.go` (add `handleNpcParam` after `handleNcParam:291`)
- Modify: `pkg/script/handlers.go` (register dispatch entry)
- Test: `pkg/script/handlers_config_test.go`

**Reference**: TS `Engine-TS/src/engine/script/handlers/NpcOps.ts:132-141`. Goscape mirror of `handleNcParam` minus the popped npcID — instead reads from `s.ActiveNpc.NpcType()`.

- [ ] **Step 1: Write the failing test for int-param push from active-npc**

Add to `pkg/script/handlers_config_test.go` (alongside existing `TestHandleNcParam_*`):

```go
func TestHandleNpcParam_IntParam(t *testing.T) {
	t.Parallel()
	const npcTypeID = 7
	const paramID = 42
	const expected = 1234

	configs := &fakeConfigs{
		params: map[int]*objtype.ParamType{
			paramID: {ID: paramID, Type: objtype.ScriptVarTypeInt, DefaultInt: 0},
		},
		npcs: map[int]*objtype.NpcType{
			npcTypeID: {Params: objtype.ParamMap{uint32(paramID): uint32(expected)}},
		},
	}
	s := newTestScriptState()
	s.Configs = configs
	s.ActiveNpc = &fakeActiveNpc{npcType: npcTypeID}

	s.PushInt(paramID)

	if err := handleNpcParam(s); err != nil {
		t.Fatalf("handleNpcParam: %v", err)
	}
	got := s.PopInt()
	if got != expected {
		t.Errorf("pushed int: got %d, want %d", got, expected)
	}
}

func TestHandleNpcParam_StringParam(t *testing.T) {
	t.Parallel()
	const npcTypeID = 7
	const paramID = 42
	const expected = "hello"

	configs := &fakeConfigs{
		params: map[int]*objtype.ParamType{
			paramID: {ID: paramID, Type: objtype.ScriptVarTypeString, DefaultString: ""},
		},
		npcs: map[int]*objtype.NpcType{
			npcTypeID: {Params: objtype.ParamMap{uint32(paramID): expected}},
		},
	}
	s := newTestScriptState()
	s.Configs = configs
	s.ActiveNpc = &fakeActiveNpc{npcType: npcTypeID}

	s.PushInt(paramID)

	if err := handleNpcParam(s); err != nil {
		t.Fatalf("handleNpcParam: %v", err)
	}
	got := s.PopString()
	if got != expected {
		t.Errorf("pushed string: got %q, want %q", got, expected)
	}
}

func TestHandleNpcParam_NoActiveNpc(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.Configs = &fakeConfigs{}
	s.ActiveNpc = nil
	s.PushInt(0)

	err := handleNpcParam(s)
	if err == nil || !strings.Contains(err.Error(), "NPC_PARAM") {
		t.Errorf("expected NPC_PARAM-tagged error, got %v", err)
	}
}

func TestHandleNpcParam_NoConfigs(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.Configs = nil
	s.ActiveNpc = &fakeActiveNpc{npcType: 1}
	s.PushInt(0)

	err := handleNpcParam(s)
	if err == nil || !strings.Contains(err.Error(), "NPC_PARAM") {
		t.Errorf("expected NPC_PARAM-tagged error, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcParam -v`

Expected: FAIL with `undefined: handleNpcParam`.

- [ ] **Step 3: Write minimal implementation**

Append to `pkg/script/handlers_config.go` (immediately after `handleNcParam` ending at line 291):

```go
// handleNpcParam (NPC_PARAM, opcode 2529) reads a param from the
// ACTIVE npc's NpcType (vs. NC_PARAM which pops an explicit npcID).
// Pop order: paramID. Mirrors TS NpcOps.ts:132-141 — checkedHandler
// (ActiveNpc) + ParamHelper.getIntParam / getStringParam.
func handleNpcParam(s *ScriptState) error {
	if err := requireConfigs(s, "NPC_PARAM"); err != nil {
		return err
	}
	if s.ActiveNpc == nil {
		return fmt.Errorf("NPC_PARAM: no active npc")
	}
	paramID := s.PopInt()
	npcID := s.ActiveNpc.NpcType()
	nt := s.Configs.NpcType(npcID)
	if nt == nil {
		return fmt.Errorf("NPC_PARAM: unknown npc id %d", npcID)
	}
	return paramLookup(s, nt.Params, paramID)
}
```

- [ ] **Step 4: Register dispatch entry**

In `pkg/script/handlers.go`, find the section near line 214 where `OpNcParam: handleNcParam,` lives and add immediately below:

```go
	OpNpcParam:    handleNpcParam,
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcParam -v`

Expected: PASS (4 sub-tests).

Run cross-package green: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_config.go pkg/script/handlers.go pkg/script/handlers_config_test.go
git commit --no-gpg-sign -m "feat(script): NAI-35 Task 1 — NPC_PARAM handler (opcode 2529)

Mirrors handleNcParam shape minus the popped npcID; reads npcType from
s.ActiveNpc.NpcType() and delegates to paramLookup. Closes NPC_PARAM
stub-not-completed (declared at opcode.go:266 since pre-NAI-33).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2 — MAP_PLAYERCOUNT (OpMapPlayerCount=1015)

**Files:**
- Create: `pkg/script/handlers_map.go`
- Create: `pkg/script/handlers_map_test.go`
- Modify: `pkg/script/state.go` (extend `PlayerLookup` interface)
- Modify: `pkg/script/handlers.go` (register dispatch)
- Modify: `modules/world/player_script_lookup.go` (impl `ZonePlayers`)

**Reference**: TS `Engine-TS/src/engine/script/handlers/ServerOps.ts:27-45`. Iterates zones in coord-rect; counts players whose `(x,z)` is inside the rect.

- [ ] **Step 1: Extend PlayerLookup interface**

In `pkg/script/state.go`, replace the existing PlayerLookup definition at lines 27-29:

```go
// PlayerLookup is the player-resolution surface for FINDUID / P_FINDUID
// (UID-keyed lookup) and for zone-rect player enumeration used by
// MAP_PLAYERCOUNT (NAI-35).
type PlayerLookup interface {
	// LookupPlayerByUID resolves a UID to an ActivePlayer if a player
	// with that UID is currently logged in. Returns nil on miss.
	LookupPlayerByUID(uid int) ActivePlayer

	// ZonePlayers returns all players in the zone at (level, zoneX, zoneZ)
	// where (zoneX, zoneZ) are coord-grid (NOT zone-index) coords. Mirrors
	// NpcLookup.ZoneNpcs shape. Used by MAP_PLAYERCOUNT (NAI-35).
	// Empty/nil slice on miss. Filters by Player.IsValid() at the
	// implementor level (matches Zone.PlayersSafe semantics).
	ZonePlayers(level, zoneX, zoneZ int) []ActivePlayer
}
```

- [ ] **Step 2: Write the failing test**

Create `pkg/script/handlers_map_test.go`:

```go
package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// fakePlayerLookup is a test-only PlayerLookup that returns pre-seeded
// players for ZonePlayers calls. ZonePlayers is keyed by absolute
// coord-grid (not zone-index).
type fakePlayerLookup struct {
	uidLookup map[int]ActivePlayer
	zonePool  map[zoneKey][]ActivePlayer
}

type zoneKey struct{ level, zoneX, zoneZ int }

func (f *fakePlayerLookup) LookupPlayerByUID(uid int) ActivePlayer {
	return f.uidLookup[uid]
}

func (f *fakePlayerLookup) ZonePlayers(level, zoneX, zoneZ int) []ActivePlayer {
	return f.zonePool[zoneKey{level, zoneX, zoneZ}]
}

// fakeActivePlayerXZ is a minimal ActivePlayer test stub exposing
// X/Z coords for rect-filter tests.
type fakeActivePlayerXZ struct {
	ActivePlayer
	x, z int
}

func (p *fakeActivePlayerXZ) X() int { return p.x }
func (p *fakeActivePlayerXZ) Z() int { return p.z }

func TestHandleMapPlayerCount_EmptyRect(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.PlayerLookup = &fakePlayerLookup{}

	c1 := coordgrid.PackCoord(0, 100, 100)
	c2 := coordgrid.PackCoord(0, 110, 110)
	s.PushInt(c1)
	s.PushInt(c2)

	if err := handleMapPlayerCount(s); err != nil {
		t.Fatalf("handleMapPlayerCount: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("count: got %d, want 0", got)
	}
}

func TestHandleMapPlayerCount_SinglePlayerInRect(t *testing.T) {
	t.Parallel()
	p := &fakeActivePlayerXZ{x: 105, z: 105}
	lookup := &fakePlayerLookup{
		zonePool: map[zoneKey][]ActivePlayer{
			{0, 105 >> 3, 105 >> 3}: {p},
		},
	}
	s := newTestScriptState()
	s.PlayerLookup = lookup

	c1 := coordgrid.PackCoord(0, 100, 100)
	c2 := coordgrid.PackCoord(0, 110, 110)
	s.PushInt(c1)
	s.PushInt(c2)

	if err := handleMapPlayerCount(s); err != nil {
		t.Fatalf("handleMapPlayerCount: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("count: got %d, want 1", got)
	}
}

func TestHandleMapPlayerCount_PlayerAtRectBoundary(t *testing.T) {
	t.Parallel()
	// Place player exactly on from.x boundary (inclusive per TS line 36).
	p := &fakeActivePlayerXZ{x: 100, z: 105}
	lookup := &fakePlayerLookup{
		zonePool: map[zoneKey][]ActivePlayer{
			{0, 100 >> 3, 105 >> 3}: {p},
		},
	}
	s := newTestScriptState()
	s.PlayerLookup = lookup

	c1 := coordgrid.PackCoord(0, 100, 100)
	c2 := coordgrid.PackCoord(0, 110, 110)
	s.PushInt(c1)
	s.PushInt(c2)

	_ = handleMapPlayerCount(s)
	if got := s.PopInt(); got != 1 {
		t.Errorf("inclusive-boundary count: got %d, want 1", got)
	}
}

func TestHandleMapPlayerCount_PlayerOutsideRect(t *testing.T) {
	t.Parallel()
	// Player at (95, 95) is outside [(100,100), (110,110)].
	p := &fakeActivePlayerXZ{x: 95, z: 95}
	lookup := &fakePlayerLookup{
		zonePool: map[zoneKey][]ActivePlayer{
			{0, 95 >> 3, 95 >> 3}: {p},
		},
	}
	s := newTestScriptState()
	s.PlayerLookup = lookup

	c1 := coordgrid.PackCoord(0, 100, 100)
	c2 := coordgrid.PackCoord(0, 110, 110)
	s.PushInt(c1)
	s.PushInt(c2)

	_ = handleMapPlayerCount(s)
	if got := s.PopInt(); got != 0 {
		t.Errorf("count: got %d, want 0", got)
	}
}

func TestHandleMapPlayerCount_CrossLevelRectIgnoresToLevel(t *testing.T) {
	t.Parallel()
	// NAI-35-D1: TS uses from.level only; to.level is silently ignored.
	// Player on level 1 with from.level=0 should NOT be counted.
	p := &fakeActivePlayerXZ{x: 105, z: 105}
	lookup := &fakePlayerLookup{
		zonePool: map[zoneKey][]ActivePlayer{
			{1, 105 >> 3, 105 >> 3}: {p}, // level 1
			{0, 105 >> 3, 105 >> 3}: nil, // level 0 (empty)
		},
	}
	s := newTestScriptState()
	s.PlayerLookup = lookup

	c1 := coordgrid.PackCoord(0, 100, 100) // from.level = 0
	c2 := coordgrid.PackCoord(1, 110, 110) // to.level = 1 (ignored)
	s.PushInt(c1)
	s.PushInt(c2)

	_ = handleMapPlayerCount(s)
	if got := s.PopInt(); got != 0 {
		t.Errorf("cross-level count (D1): got %d, want 0", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleMapPlayerCount -v`

Expected: FAIL with `undefined: handleMapPlayerCount`.

- [ ] **Step 4: Write the implementation**

Create `pkg/script/handlers_map.go`:

```go
// Package script — handlers for ServerOps map opcodes (MAP_PLAYERCOUNT,
// MAP_FINDSQUARE) ported as part of NAI-35.
package script

import (
	"fmt"
	"math/rand"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// handleMapPlayerCount (MAP_PLAYERCOUNT, opcode 1015) pops two coords
// (rect bounds) and pushes the count of players whose (x, z) falls
// inside the rect on from.level. Mirrors TS ServerOps.ts:27-45.
//
// Pop order: top-of-stack is c2; c1 is below. TS popInts(2) returns
// [c1, c2]. NAI-35-D1: TS uses from.level for inner getZone with no
// to.level validation; cross-level rect silently iterates only
// from.level zones. goscape mirrors.
func handleMapPlayerCount(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()

	fromLevel, fromX, fromZ, err := checkCoord(c1, "MAP_PLAYERCOUNT")
	if err != nil {
		return err
	}
	_, toX, toZ, err := checkCoord(c2, "MAP_PLAYERCOUNT")
	if err != nil {
		return err
	}

	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}

	count := 0
	// Zone iteration: from floor(fromX/8) to ceil(toX/8), inclusive.
	// Goscape's >> 3 is integer floor; TS uses Math.floor / Math.ceil.
	// (fromX >> 3) is floor; (toX + 7) >> 3 is the equivalent of ceil
	// for positive coords.
	for zx := fromX >> 3; zx <= (toX+7)>>3; zx++ {
		for zz := fromZ >> 3; zz <= (toZ+7)>>3; zz++ {
			for _, p := range s.PlayerLookup.ZonePlayers(fromLevel, zx<<3, zz<<3) {
				if p.X() >= fromX && p.X() <= toX && p.Z() >= fromZ && p.Z() <= toZ {
					count++
				}
			}
		}
	}
	s.PushInt(count)
	return nil
}

// _ retained import for math/rand which is used by handleMapFindSquare
// (Task 6 in the same file). Kept here to avoid an unused-import error
// during the in-progress staged-merge of NAI-35; remove if Task 2
// commits before Task 6 begins.
var _ = rand.Intn

// _ similarly retained — fmt used by Task 6's error-tagged validators.
var _ = fmt.Errorf
```

**Required — extend `ActivePlayer` interface with `X()`/`Z()` accessors**:

Plan-author verified (HEAD afdc28b): `ActivePlayer` does NOT expose `X()`/`Z()` today; world-side `Player.Coords() (x, z, level int)` exists at `modules/world/player.go:437` but the script VM has no per-axis accessor. This Task adds them.

In `pkg/script/active.go` (alongside `ActivePlayer.UID() int` at line 281), add:

```go
	// X returns the player's current absolute world X coord. Used by
	// MAP_PLAYERCOUNT (NAI-35-T2) for rect-filter checks and by
	// PlayerIterator.passesFilter (NAI-35-T4).
	X() int

	// Z returns the player's current absolute world Z coord. Used by
	// MAP_PLAYERCOUNT (NAI-35-T2) and PlayerIterator.passesFilter
	// (NAI-35-T4).
	Z() int
```

In `modules/world/player.go`, add (alongside `Coords()` at line 437):

```go
// X is the script-VM ActivePlayer.X accessor. NAI-35.
func (p *Player) X() int { return p.x }

// Z is the script-VM ActivePlayer.Z accessor. NAI-35.
func (p *Player) Z() int { return p.z }
```

- [ ] **Step 5: Add ZonePlayers impl on the world side**

In `modules/world/player_script_lookup.go`, find the existing `serverPlayerLookup` (or equivalent) struct and add the method:

```go
// ZonePlayers returns all players in the zone at (level, zoneX, zoneZ)
// where (zoneX, zoneZ) are coord-grid coords. Filters by Player.IsValid()
// via Zone.PlayersSafe(false). Mirrors TS Zone.getAllPlayersSafe.
// NAI-35.
func (l *serverPlayerLookup) ZonePlayers(level, zoneX, zoneZ int) []script.ActivePlayer {
	if l.world == nil {
		return nil
	}
	z := l.world.zoneMap.Get(level, zoneX, zoneZ)
	if z == nil {
		return nil
	}
	out := make([]script.ActivePlayer, 0, 4)
	for p := range z.PlayersSafe(false) {
		if pp, ok := p.(*Player); ok {
			out = append(out, pp)
		}
	}
	return out
}
```

- [ ] **Step 6: Register dispatch entry**

In `pkg/script/handlers.go`, find the ServerOps section (near `OpDistance`/`OpInZone` if present, or near other `OpMap*` entries) and add:

```go
	OpMapPlayerCount: handleMapPlayerCount,
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleMapPlayerCount -v`

Expected: PASS (5 sub-tests).

Run cross-package green: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/handlers_map.go pkg/script/handlers_map_test.go pkg/script/handlers.go pkg/script/state.go modules/world/player_script_lookup.go pkg/script/active.go
git commit --no-gpg-sign -m "feat(script,world): NAI-35 Task 2 — MAP_PLAYERCOUNT handler (opcode 1015)

Iterates zones in coord-rect via new PlayerLookup.ZonePlayers (mirror of
NpcLookup.ZoneNpcs). World-side impl delegates to Zone.PlayersSafe.

NAI-35-D1: TS uses from.level for inner getZone with no to.level
validation; cross-level rect silently iterates from.level zones only.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3 — NPC_HUNTALL (OpNpcHuntAll=2526) + NAI-33-D1 retirement preparation

**Files:**
- Modify: `pkg/script/npc_iterator.go` (add HuntAll mode + constructor + activate huntvis filter)
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcHuntAll`)
- Modify: `pkg/script/handlers.go` (register dispatch)
- Modify: `pkg/script/npc_iterator_test.go` (add huntvis-active passesFilter tests)
- Modify: `pkg/script/handlers_npc_test.go` (add handler tests)

**Reference**: TS `Engine-TS/src/engine/script/handlers/NpcOps.ts:325-333`. Sets `state.npcIterator = new NpcHuntAllCommandIterator(...)` — but per Path A (chosen at brainstorm), goscape extends the existing `NpcIterator` struct rather than introducing a sibling type. The deferred `huntvis` field at npc_iterator.go:44 (NAI-33-D1) becomes a live consumer.

- [ ] **Step 1: Add NpcIteratorHuntAll mode constant**

Modify `pkg/script/npc_iterator.go` lines 12-15. Replace:

```go
const (
	NpcIteratorDistance NpcIteratorMode = iota
	NpcIteratorZone
)
```

With:

```go
const (
	NpcIteratorDistance NpcIteratorMode = iota
	NpcIteratorZone
	NpcIteratorHuntAll // NAI-35: distance-bounded iteration with active huntvis filter
)
```

- [ ] **Step 2: Write the failing test for NewHuntAllNpcIterator constructor**

Add to `pkg/script/npc_iterator_test.go` (alongside existing constructor tests):

```go
func TestNewHuntAllNpcIterator_Construction(t *testing.T) {
	t.Parallel()
	const tick = 10
	const level, x, z = 0, 3200, 3200
	const distance = 8
	const huntvis = objtype.HuntVisLineOfSight

	it := NewHuntAllNpcIterator(nil, tick, level, x, z, distance, huntvis)

	if it.mode != NpcIteratorHuntAll {
		t.Errorf("mode: got %d, want NpcIteratorHuntAll(%d)", it.mode, NpcIteratorHuntAll)
	}
	if it.creationTick != tick {
		t.Errorf("creationTick: got %d, want %d", it.creationTick, tick)
	}
	if it.distance != distance {
		t.Errorf("distance: got %d, want %d", it.distance, distance)
	}
	if it.huntvis != huntvis {
		t.Errorf("huntvis: got %d, want %d", it.huntvis, huntvis)
	}
	if it.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (no type filter)", it.typeID)
	}
	// HuntAll uses the same zone-cursor bounds-math as Distance.
	expectedRadius := 1 + distance/8
	expectedCenterX := x >> 3
	expectedCenterZ := z >> 3
	if it.minZoneX != expectedCenterX-expectedRadius {
		t.Errorf("minZoneX: got %d, want %d", it.minZoneX, expectedCenterX-expectedRadius)
	}
	if it.maxZoneX != expectedCenterX+expectedRadius {
		t.Errorf("maxZoneX: got %d, want %d", it.maxZoneX, expectedCenterX+expectedRadius)
	}
	if it.curZoneX != it.maxZoneX || it.curZoneZ != it.maxZoneZ {
		t.Errorf("cursor: got (%d,%d), want (%d,%d)", it.curZoneX, it.curZoneZ, it.maxZoneX, it.maxZoneZ)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNewHuntAllNpcIterator -v`

Expected: FAIL with `undefined: NewHuntAllNpcIterator`.

- [ ] **Step 4: Add NewHuntAllNpcIterator constructor**

In `pkg/script/npc_iterator.go`, after `NewZoneNpcIterator` (ending around line 137), add:

```go
// NewHuntAllNpcIterator constructs an iterator that walks NPCs in zones
// within `distance` of (level, x, z), filtered by huntvis (now ACTIVE
// per NAI-35-T3 — closes NAI-33-D1) and no typeID filter (-1). Mirrors
// TS NpcHuntAllCommandIterator at ScriptIterators.ts (path differs from
// NpcIterator; the goscape Path-A extension reuses NpcIterator with
// HuntAll mode rather than introducing a sibling type).
//
// Bounds math identical to NewDistanceNpcIterator. HuntAll mode is
// distinguished only by passesFilter activating huntvis-based
// LoS/LoW filtering on each candidate NPC.
func NewHuntAllNpcIterator(lookup NpcLookup, tick, level, x, z, distance, huntvis int) *NpcIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &NpcIterator{
		mode:         NpcIteratorHuntAll,
		creationTick: tick,
		lookup:       lookup,
		level:        level,
		x:            x,
		z:            z,
		distance:     distance,
		huntvis:      huntvis,
		typeID:       -1,
		minZoneX:     centerX - radius,
		maxZoneX:     centerX + radius,
		minZoneZ:     centerZ - radius,
		maxZoneZ:     centerZ + radius,
		curZoneX:     centerX + radius,
		curZoneZ:     centerZ + radius,
	}
}
```

- [ ] **Step 5: Run constructor test to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNewHuntAllNpcIterator -v`

Expected: PASS.

- [ ] **Step 6: Write failing tests for huntvis-active passesFilter (LoW + LoS)**

Add to `pkg/script/npc_iterator_test.go`:

```go
func TestPassesFilter_HuntAllMode_HuntVisOff_AdmitsInRange(t *testing.T) {
	t.Parallel()
	const level, x, z = 0, 3200, 3200
	it := NewHuntAllNpcIterator(nil, 0, level, x, z, 8, objtype.HuntVisOff)
	npc := &fakeActiveNpc{x: x + 1, z: z + 1, level: level}

	if !it.passesFilter(npc) {
		t.Error("HuntVisOff in-range NPC should pass filter")
	}
}

func TestPassesFilter_HuntAllMode_HuntVisOff_RejectsOutsideDistance(t *testing.T) {
	t.Parallel()
	const level, x, z = 0, 3200, 3200
	it := NewHuntAllNpcIterator(nil, 0, level, x, z, 4, objtype.HuntVisOff)
	npc := &fakeActiveNpc{x: x + 100, z: z + 100, level: level}

	if it.passesFilter(npc) {
		t.Error("NPC beyond distance should fail filter regardless of huntvis")
	}
}

func TestPassesFilter_HuntAllMode_LineOfSight_RejectsBlocked(t *testing.T) {
	t.Parallel()
	// Place NPC in range with a wall between origin and NPC (collision-flag
	// fixture). Use the NpcIterator's collision-aware passesFilter path.
	// This test requires plumbing LineValidator into the iterator OR using
	// a stub. Per plan-author preflight, the simplest TDD approach is a
	// stub-injection point.
	t.Skip("LineOfSight filter wiring — implementer adds a LineValidator stub field on NpcIterator and wires; see Step 8")
}

func TestPassesFilter_HuntAllMode_LineOfWalk_AdmitsClear(t *testing.T) {
	t.Parallel()
	t.Skip("LineOfWalk filter wiring — paired with LoS test above; see Step 8")
}
```

- [ ] **Step 7: Run tests to verify the un-skipped ones fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPassesFilter_HuntAllMode -v`

Expected: PASS-or-FAIL on first two (they exercise existing distance + new HuntAll mode constant — should PASS already since Distance-mode passesFilter already handles distance check), SKIP on the LoS/LoW pair.

If TEST 1 (`HuntVisOff_AdmitsInRange`) FAILS: the `passesFilter` body has not yet been extended to include HuntAll mode in the "non-Zone gating" branch. Fix in Step 8.

- [ ] **Step 8: Activate huntvis filter in passesFilter (closes NAI-33-D1)**

Modify `pkg/script/npc_iterator.go` lines 68-84 (`passesFilter` body). Replace the entire function with:

```go
// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// HuntAll mode (NAI-35-T3) activates the huntvis branch — ZONE mode
// remains unfiltered (matches TS line 329-335). Distance mode keeps the
// pre-NAI-35 behavior (huntvis-validated-but-not-consumed; this preserves
// NAI-33's intent for the Distance-mode iterators which have no LoS/LoW
// content-script consumers).
func (it *NpcIterator) passesFilter(npc ActiveNpc) bool {
	if it.mode == NpcIteratorZone {
		return true // ZONE mode: no per-NPC filtering per TS line 329-335
	}
	if coordgrid.DistanceToSW(it.x, it.z, npc.NpcX(), npc.NpcZ()) > it.distance {
		return false
	}
	if it.mode == NpcIteratorHuntAll {
		switch it.huntvis {
		case objtype.HuntVisOff:
			// no LoS/LoW gate
		case objtype.HuntVisLineOfSight:
			if !it.npcVisibleViaLineOfSight(npc) {
				return false
			}
		case objtype.HuntVisLineOfWalk:
			if !it.npcVisibleViaLineOfWalk(npc) {
				return false
			}
		}
	}
	if it.typeID >= 0 && npc.NpcType() != it.typeID {
		return false
	}
	return true
}

// npcVisibleViaLineOfSight returns true when there's an unobstructed
// line-of-sight from the iterator's center coord to the NPC. Stub for
// production wiring: the real impl plumbs a LineValidator (NAI-35-T3
// follow-up if production wiring is missing). For tests, override via
// LineValidator field on NpcIterator (added below).
func (it *NpcIterator) npcVisibleViaLineOfSight(npc ActiveNpc) bool {
	if it.lineValidator == nil {
		// No validator wired — pessimistically allow. Production must
		// wire this; tests inject a stub.
		return true
	}
	return it.lineValidator.HasLineOfSight(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 0, 0, 0)
}

// npcVisibleViaLineOfWalk: see npcVisibleViaLineOfSight.
func (it *NpcIterator) npcVisibleViaLineOfWalk(npc ActiveNpc) bool {
	if it.lineValidator == nil {
		return true
	}
	return it.lineValidator.HasLineOfWalk(it.level, it.x, it.z, npc.NpcX(), npc.NpcZ(), 1, 0, 0, 0)
}
```

Add to NpcIterator struct (lines 30-56) a new field after `huntvis int` (line 44):

```go
	// lineValidator is the LoS/LoW validator used by HuntAll-mode
	// passesFilter when huntvis ∈ {LineOfSight, LineOfWalk}. nil = no
	// validator wired (test stub or pre-wiring); production sets this
	// at iterator-construction time. NAI-35-T3.
	lineValidator LineValidator
```

Add to `pkg/script/npc_iterator.go` (top, after imports), the LineValidator interface:

```go
// LineValidator is the script-VM bridge for LoS/LoW checks during
// HuntAll-mode passesFilter. Mirrors the shape of
// pkg/pathfinder/routefinder.LineValidator's HasLineOfSight /
// HasLineOfWalk methods. World-side wiring delegates to the
// existing routefinder primitive. NAI-35-T3.
type LineValidator interface {
	HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool
	HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool
}
```

Update `NewHuntAllNpcIterator` to accept a `LineValidator` parameter:

```go
func NewHuntAllNpcIterator(lookup NpcLookup, lv LineValidator, tick, level, x, z, distance, huntvis int) *NpcIterator {
	// ... existing body ...
	// Add: lineValidator: lv,
}
```

Drop `// huntvis filter intentionally omitted — NAI-33-D1 carryover` comment from passesFilter.

Drop `// kept (rather than dropped) for retirement readiness` block from the `huntvis` field declaration. Replace with: `// huntvis is the LoS/LoW gate level, consumed only in HuntAll mode (NAI-35-T3). Distance mode validates but does not filter, by design.`

- [ ] **Step 9: Run all NpcIterator tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNpcIterator -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPassesFilter -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNewHuntAllNpcIterator -v`

Expected: PASS for all non-skipped tests.

- [ ] **Step 10: Write the failing test for handleNpcHuntAll**

Add to `pkg/script/handlers_npc_test.go`:

```go
func TestHandleNpcHuntAll_StoresHuntAllIterator(t *testing.T) {
	t.Parallel()
	const level, x, z = 0, 3200, 3200
	const distance = 8
	const huntvis = objtype.HuntVisLineOfSight

	s := newTestScriptState()
	s.Npcs = &fakeNpcLookup{}
	s.World = &fakeWorld{tick: 42}

	s.PushInt(coordgrid.PackCoord(level, x, z))
	s.PushInt(distance)
	s.PushInt(huntvis)

	if err := handleNpcHuntAll(s); err != nil {
		t.Fatalf("handleNpcHuntAll: %v", err)
	}
	if s.npcIterator == nil {
		t.Fatal("npcIterator: nil — should be set")
	}
	if s.npcIterator.mode != NpcIteratorHuntAll {
		t.Errorf("mode: got %d, want NpcIteratorHuntAll", s.npcIterator.mode)
	}
	if s.npcIterator.huntvis != huntvis {
		t.Errorf("huntvis: got %d, want %d", s.npcIterator.huntvis, huntvis)
	}
	if s.npcIterator.creationTick != 42 {
		t.Errorf("creationTick: got %d, want 42", s.npcIterator.creationTick)
	}
}

func TestHandleNpcHuntAll_NilNpcsDegrades(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.Npcs = nil
	s.World = &fakeWorld{tick: 1}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(8)
	s.PushInt(objtype.HuntVisOff)

	if err := handleNpcHuntAll(s); err != nil {
		t.Fatalf("handleNpcHuntAll: %v", err)
	}
	if s.npcIterator != nil {
		t.Error("npcIterator should remain nil when Npcs is nil")
	}
}

func TestHandleNpcHuntAll_InvalidHuntVisRejected(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.Npcs = &fakeNpcLookup{}
	s.World = &fakeWorld{tick: 1}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(8)
	s.PushInt(99) // invalid huntvis

	err := handleNpcHuntAll(s)
	if err == nil || !strings.Contains(err.Error(), "NPC_HUNTALL") {
		t.Errorf("expected NPC_HUNTALL-tagged error for invalid huntvis, got %v", err)
	}
}
```

- [ ] **Step 11: Run handler test to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcHuntAll -v`

Expected: FAIL with `undefined: handleNpcHuntAll`.

- [ ] **Step 12: Implement handleNpcHuntAll**

Append to `pkg/script/handlers_npc.go` (after `handleNpcFindAll` at line 566):

```go
// handleNpcHuntAll (NPC_HUNTALL, opcode 2526) pops [coord, distance,
// huntvis] and stores a HuntAll-mode NpcIterator in s.npcIterator
// (consumed by NPC_FINDNEXT 2520). Mirrors TS NpcOps.ts:325-333.
//
// Pop order (top-of-stack first): huntvis, distance, coord.
// Validation: checkCoord, checkNotNull(distance), checkHuntVis.
// Nil-Npcs degrades silently (matches NPC_FINDALL convention).
//
// NAI-35-T3: closes NAI-33-D1 (huntvis field becomes live consumer
// of LoS/LoW filtering via passesFilter HuntAll branch).
func handleNpcHuntAll(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "NPC_HUNTALL")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "NPC_HUNTALL"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "NPC_HUNTALL"); err != nil {
		return err
	}

	if s.Npcs == nil {
		return nil
	}
	// LineValidator wiring: production-side comes from world impl;
	// here we use s.LineValidator (a new ScriptState field — Step 13).
	s.npcIterator = NewHuntAllNpcIterator(
		s.Npcs, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis,
	)
	return nil
}
```

- [ ] **Step 13: Add LineValidator field to ScriptState**

In `pkg/script/state.go`, after the `Npcs NpcLookup` field at line 121, add:

```go
	// LineValidator is the LoS/LoW bridge for HuntAll-mode
	// NpcIterator/PlayerIterator passesFilter. Callers set this after
	// Init when iterator-using opcodes (NPC_HUNTALL / HUNTALL) are in
	// the call set. Nil = no validator (HuntAll mode falls back to
	// pessimistic-allow). NAI-35.
	LineValidator LineValidator
```

- [ ] **Step 14: Register dispatch entry**

In `pkg/script/handlers.go`, near the existing iterator-family registrations (lines 356-359), add:

```go
	OpNpcHuntAll:  handleNpcHuntAll,
```

- [ ] **Step 15: Run all NPC_HUNTALL tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcHuntAll -v`

Expected: PASS (3 sub-tests).

Run cross-package green: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages PASS.

- [ ] **Step 16: Commit**

```bash
git add pkg/script/npc_iterator.go pkg/script/state.go pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/npc_iterator_test.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "feat(script): NAI-35 Task 3 — NPC_HUNTALL handler (opcode 2526) + activate huntvis filter

Path A extension: NpcIterator gains NpcIteratorHuntAll mode + new
NewHuntAllNpcIterator constructor; passesFilter HuntAll branch consumes
the previously-deferred huntvis field for LoS/LoW gating. Closes
NAI-33-D1 (huntvis dead-field deferral retired structurally; comment
cleanup in T7).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4 — HUNTALL (OpHuntAll=2031, player variant)

**Files:**
- Create: `pkg/script/player_iterator.go`
- Create: `pkg/script/player_iterator_test.go`
- Modify: `pkg/script/handlers_player.go` (add `handleHuntAll`)
- Modify: `pkg/script/handlers.go` (register dispatch)
- Modify: `pkg/script/state.go` (add `playerIterator *PlayerIterator` field)
- Modify: `pkg/script/handlers_player_test.go` (add handler tests)

**Reference**: TS `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1215-1223`. Mirrors NPC_HUNTALL handler shape; sets `s.playerIterator` (new field).

- [ ] **Step 1: Create PlayerIterator file**

Create `pkg/script/player_iterator.go`:

```go
package script

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// PlayerIteratorMode mirrors NpcIteratorMode but for the player
// iterator family. Currently only HuntAll mode has a script-VM
// consumer (HUNTNEXT, NAI-35-T5); Distance/Zone modes are deferred
// per NAI-35-D2 to avoid speculative dead-API.
type PlayerIteratorMode int

const (
	PlayerIteratorHuntAll PlayerIteratorMode = iota
)

// PlayerIterator is the script-VM iterator state for the player
// iterator family (currently HUNTALL only). Lifetime: single-tick.
// Created by HUNTALL; consumed by HUNTNEXT.
//
// Mirrors NpcIterator template (pkg/script/npc_iterator.go) closely:
// same lazy zone-walking shape, same Stale check, same exhaustion
// semantics. PlayerLookupForIter provides the per-zone snapshot.
type PlayerIterator struct {
	mode         PlayerIteratorMode
	creationTick int
	lookup       PlayerLookup
	lineValid    LineValidator

	level    int
	x, z     int
	distance int
	huntvis  int

	minZoneX, maxZoneX int
	minZoneZ, maxZoneZ int
	curZoneX, curZoneZ int
	started            bool

	zonePlayers []ActivePlayer
	zoneIdx     int
}

// Stale reports whether the iterator was created in a prior tick.
// HUNTNEXT MUST check this before calling Next. Strict greater-than
// per `iterator_state_pattern.md` element 3 (TS-faithful).
func (it *PlayerIterator) Stale(currentTick int) bool {
	return currentTick > it.creationTick
}

// passesFilter applies the per-player filter chain. HuntAll mode:
// distance + huntvis (LoS/LoW). Distance and Zone modes are
// not yet implemented (NAI-35-D2).
func (it *PlayerIterator) passesFilter(p ActivePlayer) bool {
	if coordgrid.DistanceToSW(it.x, it.z, p.X(), p.Z()) > it.distance {
		return false
	}
	switch it.huntvis {
	case objtype.HuntVisOff:
		return true
	case objtype.HuntVisLineOfSight:
		if it.lineValid == nil {
			return true
		}
		return it.lineValid.HasLineOfSight(it.level, it.x, it.z, p.X(), p.Z(), 1, 0, 0, 0)
	case objtype.HuntVisLineOfWalk:
		if it.lineValid == nil {
			return true
		}
		return it.lineValid.HasLineOfWalk(it.level, it.x, it.z, p.X(), p.Z(), 1, 0, 0, 0)
	}
	return true
}

// NewHuntAllPlayerIterator constructs a HuntAll-mode iterator. Mirrors
// NewHuntAllNpcIterator bounds-math (centerX = x>>3, radius = 1+distance/8,
// cursor at maxZone). NAI-35-T4.
func NewHuntAllPlayerIterator(lookup PlayerLookup, lv LineValidator, tick, level, x, z, distance, huntvis int) *PlayerIterator {
	centerX := x >> 3
	centerZ := z >> 3
	radius := 1 + distance/8
	return &PlayerIterator{
		mode:         PlayerIteratorHuntAll,
		creationTick: tick,
		lookup:       lookup,
		lineValid:    lv,
		level:        level,
		x:            x,
		z:            z,
		distance:     distance,
		huntvis:      huntvis,
		minZoneX:     centerX - radius,
		maxZoneX:     centerX + radius,
		minZoneZ:     centerZ - radius,
		maxZoneZ:     centerZ + radius,
		curZoneX:     centerX + radius,
		curZoneZ:     centerZ + radius,
	}
}

// Next advances and returns the next matching player. Returns
// (nil, false) on exhaustion. Caller MUST check Stale first when the
// single-tick lifetime invariant matters; HUNTNEXT does this.
func (it *PlayerIterator) Next() (ActivePlayer, bool) {
	if it.lookup == nil {
		return nil, false
	}
	for {
		for it.zoneIdx < len(it.zonePlayers) {
			p := it.zonePlayers[it.zoneIdx]
			it.zoneIdx++
			if it.passesFilter(p) {
				return p, true
			}
		}
		if !it.advanceZone() {
			return nil, false
		}
		it.zonePlayers = it.lookup.ZonePlayers(it.level, it.curZoneX*8, it.curZoneZ*8)
		it.zoneIdx = 0
	}
}

// advanceZone walks outer-X-desc / inner-Z-desc per the NpcIterator
// reference impl. Returns false on exhaustion.
func (it *PlayerIterator) advanceZone() bool {
	if !it.started {
		it.started = true
		return true
	}
	it.curZoneZ--
	if it.curZoneZ < it.minZoneZ {
		it.curZoneZ = it.maxZoneZ
		it.curZoneX--
		if it.curZoneX < it.minZoneX {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Add playerIterator field to ScriptState**

In `pkg/script/state.go`, after `npcIterator *NpcIterator` at line 173, add:

```go
	// playerIterator holds the active player-iterator state. Set by
	// HUNTALL; consumed by HUNTNEXT. Single-tick lifetime — Stale()
	// check at HUNTNEXT against s.World.CurrentTick(). Nil = no active
	// iterator. NAI-35-T4.
	playerIterator *PlayerIterator
```

- [ ] **Step 3: Write failing tests for PlayerIterator mechanics**

Create `pkg/script/player_iterator_test.go`:

```go
package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestPlayerIterator_Stale_StrictGreaterThan(t *testing.T) {
	t.Parallel()
	it := &PlayerIterator{creationTick: 10}
	if it.Stale(10) {
		t.Error("currentTick == creationTick: should NOT be stale (TS-faithful)")
	}
	if !it.Stale(11) {
		t.Error("currentTick > creationTick: should be stale")
	}
	if it.Stale(9) {
		t.Error("currentTick < creationTick: should NOT be stale (per iterator_state_pattern element 3)")
	}
}

func TestNewHuntAllPlayerIterator_Construction(t *testing.T) {
	t.Parallel()
	const tick = 42
	const level, x, z = 0, 3200, 3200
	const distance = 16
	const huntvis = objtype.HuntVisLineOfWalk

	it := NewHuntAllPlayerIterator(nil, nil, tick, level, x, z, distance, huntvis)

	if it.mode != PlayerIteratorHuntAll {
		t.Errorf("mode: got %d, want PlayerIteratorHuntAll", it.mode)
	}
	if it.creationTick != tick {
		t.Errorf("creationTick: got %d, want %d", it.creationTick, tick)
	}
	if it.distance != distance {
		t.Errorf("distance: got %d, want %d", it.distance, distance)
	}
	if it.huntvis != huntvis {
		t.Errorf("huntvis: got %d, want %d", it.huntvis, huntvis)
	}
	expectedRadius := 1 + distance/8
	if it.maxZoneX != (x>>3)+expectedRadius {
		t.Errorf("maxZoneX: got %d, want %d", it.maxZoneX, (x>>3)+expectedRadius)
	}
	if it.curZoneX != it.maxZoneX || it.curZoneZ != it.maxZoneZ {
		t.Errorf("cursor: got (%d,%d), want (%d,%d)", it.curZoneX, it.curZoneZ, it.maxZoneX, it.maxZoneZ)
	}
}

func TestPlayerIterator_PassesFilter_HuntVisOff_AdmitsInRange(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisOff)
	p := &fakeActivePlayerXZ{x: 3201, z: 3201}
	if !it.passesFilter(p) {
		t.Error("HuntVisOff in-range: should pass")
	}
}

func TestPlayerIterator_PassesFilter_OutsideDistanceRejected(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 4, objtype.HuntVisOff)
	p := &fakeActivePlayerXZ{x: 3300, z: 3300}
	if it.passesFilter(p) {
		t.Error("beyond distance: should fail regardless of huntvis")
	}
}

func TestPlayerIterator_NilLookup_NextReturnsFalse(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisOff)
	p, ok := it.Next()
	if ok || p != nil {
		t.Errorf("nil lookup: got (%v, %t), want (nil, false)", p, ok)
	}
}

func TestPlayerIterator_AdvanceZone_StartedFalseFirstCall(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisOff)
	if !it.advanceZone() {
		t.Error("first advanceZone should return true (cursor at maxZone)")
	}
	if !it.started {
		t.Error("advanceZone should set started=true")
	}
}

func TestPlayerIterator_AdvanceZone_ExhaustReturnsFalse(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 0, objtype.HuntVisOff)
	// distance=0 → radius=1; bounds = [centerX-1, centerX+1] × [centerZ-1, centerZ+1] = 9 cells.
	// First advanceZone: started=true, return true (cursor at max).
	// Subsequent advanceZones must drain 9 cells then return false.
	count := 0
	for it.advanceZone() {
		count++
		if count > 100 {
			t.Fatal("advanceZone never returned false (infinite loop)")
		}
	}
	if count != 9 {
		t.Errorf("advanceZone walk: got %d cells, want 9", count)
	}
}
```

- [ ] **Step 4: Run iterator tests to verify pass (no impl needed — file written in Step 1)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPlayerIterator -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestNewHuntAllPlayerIterator -v`

Expected: PASS (7 sub-tests).

- [ ] **Step 5: Write failing test for handleHuntAll**

Add to `pkg/script/handlers_player_test.go`:

```go
func TestHandleHuntAll_StoresHuntAllPlayerIterator(t *testing.T) {
	t.Parallel()
	const level, x, z = 0, 3200, 3200
	const distance = 8
	const huntvis = objtype.HuntVisLineOfWalk

	s := newTestScriptState()
	s.PlayerLookup = &fakePlayerLookup{}
	s.World = &fakeWorld{tick: 100}

	s.PushInt(coordgrid.PackCoord(level, x, z))
	s.PushInt(distance)
	s.PushInt(huntvis)

	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll: %v", err)
	}
	if s.playerIterator == nil {
		t.Fatal("playerIterator: nil — should be set")
	}
	if s.playerIterator.mode != PlayerIteratorHuntAll {
		t.Errorf("mode: got %d, want PlayerIteratorHuntAll", s.playerIterator.mode)
	}
	if s.playerIterator.huntvis != huntvis {
		t.Errorf("huntvis: got %d, want %d", s.playerIterator.huntvis, huntvis)
	}
	if s.playerIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100", s.playerIterator.creationTick)
	}
}

func TestHandleHuntAll_NilLookupDegrades(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.PlayerLookup = nil
	s.World = &fakeWorld{tick: 1}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(8)
	s.PushInt(objtype.HuntVisOff)

	if err := handleHuntAll(s); err != nil {
		t.Fatalf("handleHuntAll: %v", err)
	}
	if s.playerIterator != nil {
		t.Error("playerIterator should remain nil when PlayerLookup is nil")
	}
}

func TestHandleHuntAll_InvalidHuntVisRejected(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.PlayerLookup = &fakePlayerLookup{}
	s.World = &fakeWorld{tick: 1}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(8)
	s.PushInt(99)

	err := handleHuntAll(s)
	if err == nil || !strings.Contains(err.Error(), "HUNTALL") {
		t.Errorf("expected HUNTALL-tagged error, got %v", err)
	}
}
```

- [ ] **Step 6: Run handler test to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleHuntAll -v`

Expected: FAIL with `undefined: handleHuntAll`.

- [ ] **Step 7: Implement handleHuntAll**

Append to `pkg/script/handlers_player.go`:

```go
// handleHuntAll (HUNTALL, opcode 2031) pops [coord, distance, huntvis]
// and stores a HuntAll-mode PlayerIterator in s.playerIterator
// (consumed by HUNTNEXT 2032). Mirrors TS PlayerOps.ts:1215-1223.
//
// Pop order (top-of-stack first): huntvis, distance, coord.
// Validation: checkCoord, checkNotNull(distance), checkHuntVis.
// Nil-PlayerLookup degrades silently (matches HUNTALL/NPC_HUNTALL
// convention).
func handleHuntAll(s *ScriptState) error {
	checkVis := s.PopInt()
	distance := s.PopInt()
	coord := s.PopInt()

	level, x, z, err := checkCoord(coord, "HUNTALL")
	if err != nil {
		return err
	}
	if err := checkNotNull(distance, "HUNTALL"); err != nil {
		return err
	}
	if err := checkHuntVis(checkVis, "HUNTALL"); err != nil {
		return err
	}

	if s.PlayerLookup == nil {
		return nil
	}
	s.playerIterator = NewHuntAllPlayerIterator(
		s.PlayerLookup, s.LineValidator, s.World.CurrentTick(),
		level, x, z, distance, checkVis,
	)
	return nil
}
```

- [ ] **Step 8: Register dispatch entry**

In `pkg/script/handlers.go`, near the player-handler registrations, add:

```go
	OpHuntAll:     handleHuntAll,
```

- [ ] **Step 9: Run all HUNTALL tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleHuntAll -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS for all.

- [ ] **Step 10: Commit**

```bash
git add pkg/script/player_iterator.go pkg/script/player_iterator_test.go pkg/script/state.go pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-35 Task 4 — HUNTALL handler (opcode 2031, player variant)

New pkg/script/player_iterator.go mirroring NpcIterator template.
HuntAll-only constructor — Distance/Zone modes deferred per NAI-35-D2
(no PLAYER_FINDALL family consumers in TS at HEAD).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5 — HUNTNEXT (OpHuntNext=2032)

**Files:**
- Modify: `pkg/script/handlers_player.go` (add `handleHuntNext`)
- Modify: `pkg/script/handlers.go` (register dispatch)
- Modify: `pkg/script/handlers_player_test.go` (add tests)

**Reference**: TS `Engine-TS/src/engine/script/handlers/PlayerOps.ts:1226-1233`. Mirrors `handleNpcFindNext` shape; uses `s.Self = p; s.Pointers |= PtrActivePlayer; s.PushInt(1)` per the P_FINDUID convention at handlers_player.go:715-716.

- [ ] **Step 1: Write failing tests for handleHuntNext**

Add to `pkg/script/handlers_player_test.go`:

```go
func TestHandleHuntNext_NilIteratorPushesZero(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.playerIterator = nil
	s.World = &fakeWorld{tick: 0}

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got %d, want 0", got)
	}
}

func TestHandleHuntNext_StaleIteratorReturnsError(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.World = &fakeWorld{tick: 5}
	s.playerIterator = &PlayerIterator{creationTick: 3} // tick > creationTick → stale

	err := handleHuntNext(s)
	if err == nil || !strings.Contains(err.Error(), "HUNTNEXT") {
		t.Errorf("expected HUNTNEXT-tagged stale error, got %v", err)
	}
}

func TestHandleHuntNext_HitSetsSelfAndPushesOne(t *testing.T) {
	t.Parallel()
	p := &fakeActivePlayerXZ{x: 3201, z: 3201}
	lookup := &fakePlayerLookup{
		zonePool: map[zoneKey][]ActivePlayer{
			{0, (3200 + 8) >> 3, (3200 + 8) >> 3}: {p}, // matches HuntAll cursor start
		},
	}
	s := newTestScriptState()
	s.World = &fakeWorld{tick: 10}
	s.playerIterator = NewHuntAllPlayerIterator(lookup, nil, 10, 0, 3200, 3200, 32, objtype.HuntVisOff)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if s.Self != p {
		t.Errorf("Self: got %v, want %v", s.Self, p)
	}
	if s.Pointers&PtrActivePlayer == 0 {
		t.Errorf("Pointers: PtrActivePlayer not set")
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("hit: got %d, want 1", got)
	}
}

func TestHandleHuntNext_ExhaustionPushesZero(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.World = &fakeWorld{tick: 10}
	// Iterator with empty zone-pool → Next returns (nil, false).
	s.playerIterator = NewHuntAllPlayerIterator(&fakePlayerLookup{}, nil, 10, 0, 3200, 3200, 32, objtype.HuntVisOff)

	if err := handleHuntNext(s); err != nil {
		t.Fatalf("handleHuntNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("exhaustion: got %d, want 0", got)
	}
}

func TestHandleHuntNext_ExhaustionDoesNotClearIterator(t *testing.T) {
	t.Parallel()
	s := newTestScriptState()
	s.World = &fakeWorld{tick: 10}
	s.playerIterator = NewHuntAllPlayerIterator(&fakePlayerLookup{}, nil, 10, 0, 3200, 3200, 32, objtype.HuntVisOff)

	_ = handleHuntNext(s)
	_ = s.PopInt()
	if s.playerIterator == nil {
		t.Error("iterator should NOT be cleared on exhaustion (matches iterator_state_pattern element 7)")
	}
	// Subsequent call also pushes 0.
	_ = handleHuntNext(s)
	if got := s.PopInt(); got != 0 {
		t.Errorf("repeat-after-exhaustion: got %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run tests to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleHuntNext -v`

Expected: FAIL with `undefined: handleHuntNext`.

- [ ] **Step 3: Implement handleHuntNext**

Append to `pkg/script/handlers_player.go`:

```go
// handleHuntNext (HUNTNEXT, opcode 2032) advances the active
// PlayerIterator and either sets active_player + pushes 1 on hit, or
// pushes 0 on miss / nil-iterator. Mirrors TS PlayerOps.ts:1226-1233
// and the analogous NPC handler at handlers_npc.go:605 (handleNpcFindNext).
//
// Active-player slot pattern (Pointers + Self) mirrors P_FINDUID at
// handlers_player.go:715-716. Stale check uses strict-greater-than per
// iterator_state_pattern element 3.
//
// Exhaustion does NOT clear s.playerIterator (matches NPC_FINDNEXT
// behavior; iterator_state_pattern element 7). NAI-35-T5.
func handleHuntNext(s *ScriptState) error {
	it := s.playerIterator
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("HUNTNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	p, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	s.Self = p
	s.Pointers |= PtrActivePlayer
	s.PushInt(1)
	return nil
}
```

- [ ] **Step 4: Register dispatch entry**

In `pkg/script/handlers.go`, add (next to `OpHuntAll` registration from Task 4):

```go
	OpHuntNext:    handleHuntNext,
```

- [ ] **Step 5: Run all HUNTNEXT tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleHuntNext -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS (5 sub-tests + cross-package green).

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_player.go pkg/script/handlers.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "feat(script): NAI-35 Task 5 — HUNTNEXT handler (opcode 2032)

Mirrors handleNpcFindNext shape; consumes s.playerIterator. Active-
player slot pattern (s.Self = p; s.Pointers |= PtrActivePlayer; PushInt(1))
mirrors P_FINDUID at handlers_player.go:715-716.

Pairs with HUNTALL (Task 4) — closes the dead-API risk per
dead_api_polish.md by ensuring HUNTALL has an in-VM consumer.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6 — MAP_FINDSQUARE (OpMapFindSquare=1009)

**Files:**
- Modify: `pkg/script/handlers_map.go` (add `handleMapFindSquare`)
- Create: `pkg/script/findsquare_type.go`
- Modify: `pkg/script/handlers.go` (register dispatch)
- Modify: `pkg/script/handlers_map_test.go` (add tests)

**Reference**: TS `Engine-TS/src/engine/script/handlers/ServerOps.ts:254-374`. Six structural branches: `maxRadius < 10` (random-50) × {NONE, LINEOFWALK, LINEOFSIGHT}; `>= 10` (west-bias iteration) × {NONE, LINEOFWALK, LINEOFSIGHT}.

- [ ] **Step 1: Add MapFindSquareType constants**

Create `pkg/script/findsquare_type.go`:

```go
package script

import "fmt"

// MapFindSquareType selects the line-of-walk / line-of-sight gate for
// MAP_FINDSQUARE (opcode 1009). Mirrors TS MapFindSquareType enum.
type MapFindSquareType int

const (
	MapFindSquareNone        MapFindSquareType = 0
	MapFindSquareLineOfWalk  MapFindSquareType = 1
	MapFindSquareLineOfSight MapFindSquareType = 2
)

// checkFindSquareType validates that v is in {0, 1, 2}. Mirrors TS
// FindSquareValid (ScriptValidators.ts).
func checkFindSquareType(v int, op string) error {
	switch MapFindSquareType(v) {
	case MapFindSquareNone, MapFindSquareLineOfWalk, MapFindSquareLineOfSight:
		return nil
	default:
		return fmt.Errorf("%s: invalid find-square type %d", op, v)
	}
}

// checkNumberPositive validates v > 0. Mirrors TS NumberPositive.
func checkNumberPositive(v int, op string) error {
	if v <= 0 {
		return fmt.Errorf("%s: expected positive number, got %d", op, v)
	}
	return nil
}
```

- [ ] **Step 2: Add MapBlocked / GameMap surface to ScriptState (or use World accessor)**

The TS handler calls `World.gameMap.isFreeToPlay(x, z)` and `isMapBlocked(x, z, level)`. Goscape's equivalent: `s.World.MapMembers()` for the F2P-world flag; for `isMapBlocked` and `isFreeToPlay(x,z)`, we need a world-side surface accessible from the script VM.

**Design choice**: extend `WorldVars` interface (state.go:34) with two methods, OR pipe through a new `MapValidator` interface. Per `interface_at_cyclic_import_boundary.md`, prefer minimal interface at the lower-level package. The script VM only needs:
- `s.World.MapMembers() int` (already exists at state.go:45)
- `s.World.IsMapBlocked(level, x, z int) bool` (NEW)
- `s.World.IsFreeToPlay(x, z int) bool` (NEW)

In `pkg/script/state.go`, append to `WorldVars` interface (after `MapLive() int` at line 47):

```go
	// IsMapBlocked reports whether the tile at (level, x, z) blocks
	// walking. Used by MAP_FINDSQUARE for candidate-square rejection.
	// NAI-35.
	IsMapBlocked(level, x, z int) bool

	// IsFreeToPlay reports whether the tile at (x, z) is in an F2P-zone.
	// Used by MAP_FINDSQUARE for free-world filtering. NAI-35.
	IsFreeToPlay(x, z int) bool
```

In `modules/world/script_vars.go` (or wherever `WorldVars` is implemented), add:

```go
// IsMapBlocked delegates to pkg/pathfinder/collision/flagmap. NAI-35.
// Plan-author verified (HEAD afdc28b): `pkg/pathfinder/collision/flag.go:41`
// declares `FlagBlockWalk` (NOT `BLOCK_WALK`). Use the actual constant.
// FlagFloorBlocked combines BlockWalk + GroundDecor; a tile is "blocked"
// for MAP_FINDSQUARE purposes whenever the walk flag is set.
func (w *worldVars) IsMapBlocked(level, x, z int) bool {
	flag := w.world.collisionFlags.Get(x, z, level)
	return flag&collision.FlagBlockWalk != 0
}

// IsFreeToPlay delegates to pkg/gamemap/multimap.go. NAI-35.
func (w *worldVars) IsFreeToPlay(x, z int) bool {
	return w.world.gameMap.IsFreeToPlay(x, z)
}
```

(Plan-author resolved: `pkg/pathfinder/collision/flag.go:41` declares `FlagBlockWalk`; this is the actual constant. `FlagFloorBlocked = FlagBlockWalk | FlagGroundDecor` per line 59 — but for MAP_FINDSQUARE's "tile blocks walking" check, `FlagBlockWalk` alone is sufficient.)

- [ ] **Step 3: Write failing tests**

Add to `pkg/script/handlers_map_test.go`:

```go
// fakeWorldMap extends fakeWorld with MAP_FINDSQUARE primitives.
type fakeWorldMap struct {
	fakeWorld
	blockedTiles map[blockKey]bool
	f2pTiles     map[xzKey]bool
	members      int
}
type blockKey struct{ level, x, z int }
type xzKey struct{ x, z int }

func (w *fakeWorldMap) IsMapBlocked(level, x, z int) bool {
	return w.blockedTiles[blockKey{level, x, z}]
}
func (w *fakeWorldMap) IsFreeToPlay(x, z int) bool {
	return w.f2pTiles[xzKey{x, z}]
}
func (w *fakeWorldMap) MapMembers() int {
	return w.members
}

func TestHandleMapFindSquare_NoneType_FindsFreeSquareWithinRadius(t *testing.T) {
	t.Parallel()
	s := newTestScriptStateWithSeededRand(t, 1)
	s.World = &fakeWorldMap{members: 1} // members world, F2P irrelevant

	const level, x, z = 0, 3200, 3200
	s.PushInt(coordgrid.PackCoord(level, x, z))
	s.PushInt(1) // minRadius
	s.PushInt(5) // maxRadius
	s.PushInt(int(MapFindSquareNone))

	if err := handleMapFindSquare(s); err != nil {
		t.Fatalf("handleMapFindSquare: %v", err)
	}
	got := s.PopInt()
	pos := coordgrid.UnpackCoord(got)
	if pos.Level != level {
		t.Errorf("level: got %d, want %d", pos.Level, level)
	}
	dx := pos.X - x
	if dx < 0 {
		dx = -dx
	}
	dz := pos.Z - z
	if dz < 0 {
		dz = -dz
	}
	if dx > 5 || dz > 5 {
		t.Errorf("found coord (%d,%d) outside maxRadius=5 from origin (%d,%d)", pos.X, pos.Z, x, z)
	}
}

func TestHandleMapFindSquare_AllBlocked_ReturnsOriginCoord(t *testing.T) {
	t.Parallel()
	s := newTestScriptStateWithSeededRand(t, 2)
	// Block the entire 11×11 tile region around origin.
	blocked := map[blockKey]bool{}
	for dx := -5; dx <= 5; dx++ {
		for dz := -5; dz <= 5; dz++ {
			blocked[blockKey{0, 3200 + dx, 3200 + dz}] = true
		}
	}
	s.World = &fakeWorldMap{members: 1, blockedTiles: blocked}

	const c = 3200
	origCoord := coordgrid.PackCoord(0, c, c)
	s.PushInt(origCoord)
	s.PushInt(1)
	s.PushInt(5)
	s.PushInt(int(MapFindSquareNone))

	_ = handleMapFindSquare(s)
	got := s.PopInt()
	if got != origCoord {
		t.Errorf("all-blocked: got %d, want origCoord %d (TS line 373 fall-through)", got, origCoord)
	}
}

func TestHandleMapFindSquare_F2PTileRejectedInFreeWorld(t *testing.T) {
	t.Parallel()
	s := newTestScriptStateWithSeededRand(t, 3)
	// freeWorld = !members; members=0 means free world.
	// F2P map = pure subset; non-F2P tiles rejected.
	s.World = &fakeWorldMap{members: 0, f2pTiles: map[xzKey]bool{}}

	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(1)
	s.PushInt(5)
	s.PushInt(int(MapFindSquareNone))

	_ = handleMapFindSquare(s)
	got := s.PopInt()
	// All candidate squares are non-F2P → fall through to origin.
	if got != coordgrid.PackCoord(0, 3200, 3200) {
		t.Errorf("free-world all-non-F2P: got %d, expected origin coord", got)
	}
}

func TestHandleMapFindSquare_TypeValidationRejectsInvalid(t *testing.T) {
	t.Parallel()
	s := newTestScriptStateWithSeededRand(t, 4)
	s.World = &fakeWorldMap{members: 1}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(1)
	s.PushInt(5)
	s.PushInt(99)

	err := handleMapFindSquare(s)
	if err == nil || !strings.Contains(err.Error(), "MAP_FINDSQUARE") {
		t.Errorf("expected MAP_FINDSQUARE-tagged error, got %v", err)
	}
}

func TestHandleMapFindSquare_NumberPositiveValidation(t *testing.T) {
	t.Parallel()
	s := newTestScriptStateWithSeededRand(t, 5)
	s.World = &fakeWorldMap{members: 1}
	s.PushInt(coordgrid.PackCoord(0, 3200, 3200))
	s.PushInt(0) // minRadius = 0 should be rejected
	s.PushInt(5)
	s.PushInt(int(MapFindSquareNone))

	err := handleMapFindSquare(s)
	if err == nil || !strings.Contains(err.Error(), "MAP_FINDSQUARE") {
		t.Errorf("expected MAP_FINDSQUARE-tagged positive error, got %v", err)
	}
}
```

(Plan-author preflight: `newTestScriptStateWithSeededRand` may need to be added as a test helper; current `newTestScriptState` may not seed `rand`. Define it as `s := newTestScriptState(); s.rand = rand.New(rand.NewSource(int64(seed)))` if `ScriptState` accepts a rand source — otherwise use a package-level `setRandSource(seed)` helper for tests.)

- [ ] **Step 4: Run tests to verify fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleMapFindSquare -v`

Expected: FAIL with `undefined: handleMapFindSquare`.

- [ ] **Step 5: Implement handleMapFindSquare**

Append to `pkg/script/handlers_map.go`:

```go
// handleMapFindSquare (MAP_FINDSQUARE, opcode 1009) finds a free
// walkable square near origin, optionally gated by line-of-walk or
// line-of-sight. Mirrors TS ServerOps.ts:254-374.
//
// Pop order (top-of-stack first): type, maxRadius, minRadius, coord.
// On hit: pushes packed coord. On exhaustion: pushes the input coord
// (TS line 373 fall-through).
//
// NAI-35-D4: uses Go's math/rand (TS uses Math.random); behaviorally
// equivalent for non-deterministic per-call random; tests seed.
func handleMapFindSquare(s *ScriptState) error {
	typeArg := s.PopInt()
	maxRadius := s.PopInt()
	minRadius := s.PopInt()
	coord := s.PopInt()

	if err := checkNumberPositive(minRadius, "MAP_FINDSQUARE"); err != nil {
		return err
	}
	if err := checkNumberPositive(maxRadius, "MAP_FINDSQUARE"); err != nil {
		return err
	}
	if err := checkFindSquareType(typeArg, "MAP_FINDSQUARE"); err != nil {
		return err
	}
	level, originX, originZ, err := checkCoord(coord, "MAP_FINDSQUARE")
	if err != nil {
		return err
	}

	if s.World == nil {
		s.PushInt(coord)
		return nil
	}
	freeWorld := s.World.MapMembers() == 0
	findType := MapFindSquareType(typeArg)

	rng := s.RandSource()

	if maxRadius < 10 {
		// Random-50-attempts branch.
		for i := 0; i < 50; i++ {
			distX := rng.Intn(2*maxRadius+1) - maxRadius
			distZ := rng.Intn(2*maxRadius+1) - maxRadius
			distance := absMax(distX, distZ)
			if distance < minRadius || distance > maxRadius {
				continue
			}
			randomX := originX + distX
			randomZ := originZ + distZ
			if freeWorld && !s.World.IsFreeToPlay(randomX, randomZ) {
				continue
			}
			ok := false
			switch findType {
			case MapFindSquareNone:
				ok = !s.World.IsMapBlocked(level, randomX, randomZ)
			case MapFindSquareLineOfWalk:
				ok = isLineOfWalk(s, level, randomX, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, randomX, randomZ)
			case MapFindSquareLineOfSight:
				ok = isLineOfSight(s, level, randomX, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, randomX, randomZ)
			}
			if ok {
				s.PushInt(coordgrid.PackCoord(level, randomX, randomZ))
				return nil
			}
		}
	} else {
		// West-bias iteration branch (imps).
		for x := originX - maxRadius; x <= originX+maxRadius; x++ {
			distX := x - originX
			distZ := rng.Intn(2*maxRadius+1) - maxRadius
			distance := absMax(distX, distZ)
			if distance < minRadius || distance > maxRadius {
				continue
			}
			randomZ := originZ + distZ
			if freeWorld && !s.World.IsFreeToPlay(x, randomZ) {
				continue
			}
			ok := false
			switch findType {
			case MapFindSquareNone:
				ok = !s.World.IsMapBlocked(level, x, randomZ) &&
					!coordgrid.IsWithinDistanceSW(x, randomZ, originX, originZ, minRadius)
			case MapFindSquareLineOfWalk:
				ok = isLineOfWalk(s, level, x, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, x, randomZ) &&
					!coordgrid.IsWithinDistanceSW(x, randomZ, originX, originZ, minRadius)
			case MapFindSquareLineOfSight:
				ok = isLineOfSight(s, level, x, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, x, randomZ) &&
					!coordgrid.IsWithinDistanceSW(x, randomZ, originX, originZ, minRadius)
			}
			if ok {
				s.PushInt(coordgrid.PackCoord(level, x, randomZ))
				return nil
			}
		}
	}

	s.PushInt(coord)
	return nil
}

// isLineOfWalk / isLineOfSight delegate to the script-VM's
// LineValidator if wired; pessimistic-allow if unwired (matches the
// NpcIterator passesFilter HuntAll-mode behavior).
func isLineOfWalk(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
}

func isLineOfSight(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
}

// absMax returns max(|a|, |b|).
func absMax(a, b int) int {
	aa, bb := a, b
	if aa < 0 {
		aa = -aa
	}
	if bb < 0 {
		bb = -bb
	}
	if aa > bb {
		return aa
	}
	return bb
}
```

(Plan-author preflight: confirm `coordgrid.IsWithinDistanceSW` and `coordgrid.UnpackCoord` exist in `pkg/coordgrid/`. Both are referenced. If `IsWithinDistanceSW` is absent, it's a small port of TS `CoordGrid.isWithinDistanceSW` — same package as `DistanceToSW` already verified at `pkg/script/npc_iterator.go:76`.)

- [ ] **Step 6: Add RandSource accessor to ScriptState**

In `pkg/script/state.go`, add a field and accessor:

```go
	// rng is the random source for non-deterministic opcodes
	// (RANDOM, MAP_FINDSQUARE). Tests seed via SetRandSource. Defaults
	// to crypto-stable global rand if unset.
	rng *rand.Rand
```

…and methods:

```go
// RandSource returns the rand.Rand used for non-deterministic opcodes.
// Lazy-initialized to a new source on first access if unset.
func (s *ScriptState) RandSource() *rand.Rand {
	if s.rng == nil {
		s.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return s.rng
}

// SetRandSource overrides the rand source. Tests use this for
// deterministic outcomes.
func (s *ScriptState) SetRandSource(seed int64) {
	s.rng = rand.New(rand.NewSource(seed))
}
```

Adjust imports as needed (`math/rand`, `time`).

- [ ] **Step 7: Register dispatch entry**

In `pkg/script/handlers.go`, add (next to `OpMapPlayerCount` from Task 2):

```go
	OpMapFindSquare: handleMapFindSquare,
```

- [ ] **Step 8: Run all MAP_FINDSQUARE tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleMapFindSquare -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS (5 sub-tests + cross-package green).

- [ ] **Step 9: Commit**

```bash
git add pkg/script/handlers_map.go pkg/script/findsquare_type.go pkg/script/state.go pkg/script/handlers.go pkg/script/handlers_map_test.go modules/world/script_vars.go
git commit --no-gpg-sign -m "feat(script,world): NAI-35 Task 6 — MAP_FINDSQUARE handler (opcode 1009)

Six structural branches per TS ServerOps.ts:254-374 (random-50 +
west-bias × {NONE, LINEOFWALK, LINEOFSIGHT}). Adds WorldVars surface
methods IsMapBlocked + IsFreeToPlay; deterministic rand source via
SetRandSource for tests.

NAI-35-D3 resolved: F2P/members flag uses existing WorldVars.MapMembers().
NAI-35-D4 instrumentation: math/rand vs Math.random; behaviorally
equivalent.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7 — Close polish + NAI-33-D1 retirement

**Files:**
- Modify: `pkg/script/npc_iterator.go` (drop NAI-33-D1 deferred-comment block)
- Modify: `pkg/script/state.go` (drop NAI-33-D1 carryover comment in NpcLookup huntvis doc-comment)
- Modify: any other files surfaced by the deviation-tag grep
- Modify: relevant `nai_followups.md` entries (memory)

- [ ] **Step 1: Enumerate all stale NAI-33-D1 / S7f-D1 sites**

Run from repo root:

```bash
rg "NAI-33-D1|S7f-D1" pkg/ modules/
```

Expected output: a list of every doc-comment + inline-code reference. Per `retire_deviation_grep_all_comments.md`, the implementer MUST update each.

Anticipated sites (verify by grep):
- `pkg/script/npc_iterator.go:39-44` (huntvis field comment block)
- `pkg/script/npc_iterator.go:69-71` (passesFilter "carryover" header)
- `pkg/script/state.go:64-66` (NpcLookup huntvis doc-comment)

- [ ] **Step 2: Update npc_iterator.go huntvis field comment**

Replace the existing comment block at lines 39-44:

```go
	// huntvis is stored at construction (validated upstream by handlers
	// via checkHuntVis) but NOT consumed by passesFilter today — see
	// NAI-33-D1 deviation. Field kept (rather than dropped) for
	// retirement readiness: when LoS/LoW filtering lands, passesFilter
	// only needs to start reading huntvis; no constructor surface change.
	huntvis int
```

With:

```go
	// huntvis is the LoS/LoW filter level (HuntVisOff/LineOfSight/
	// LineOfWalk). Consumed by passesFilter ONLY in HuntAll mode
	// (NAI-35-T3). Distance and Zone modes validate but do not filter,
	// preserving NAI-33's intent for non-hunt iterator consumers.
	huntvis int
```

- [ ] **Step 3: Update passesFilter header comment**

In `pkg/script/npc_iterator.go`, the comment at lines 68-71 (immediately above `passesFilter`) should already have been updated in Task 3 Step 8. Verify it now reads (correctly) something like:

```go
// passesFilter applies the per-NPC filter chain in TS line 345-356 order.
// HuntAll mode (NAI-35-T3) activates the huntvis branch — ZONE mode
// remains unfiltered (matches TS line 329-335). Distance mode keeps the
// pre-NAI-35 behavior...
```

If any "carryover" / "intentionally omitted" / "NAI-33-D1" reference remains, drop it.

- [ ] **Step 4: Update state.go NpcLookup huntvis doc-comment**

In `pkg/script/state.go`, find the existing comment around lines 63-66:

```go
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*)
// but the current impl does not filter on it (deviation S7f-D1).
// Callers must still validate via checkHuntVis.
```

Replace with:

```go
// huntvis accepts HuntVisOff/LineOfSight/LineOfWalk (pkg/objtype.HuntVis*).
// FindClosestNpcByType / FindClosestNpcByCategory currently validate
// huntvis but do not filter on it; HuntAll-mode iterators
// (NewHuntAllNpcIterator, NewHuntAllPlayerIterator, NAI-35) DO filter.
// Callers must still validate via checkHuntVis.
```

- [ ] **Step 5: Run all tests to verify nothing broke**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages PASS.

- [ ] **Step 6: Sanity-check by re-grepping**

Run from repo root:

```bash
rg "NAI-33-D1|S7f-D1" pkg/ modules/
```

Expected: no matches OR only intentional retirement-record references (e.g., a single comment in `npc_iterator.go` documenting "NAI-35-T3 retired NAI-33-D1").

- [ ] **Step 7: Update memory deviation registry**

Edit `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. In the "From NAI-33" section (or wherever NAI-33-D1 is recorded), update the D1 entry status from "deferred" to "closed by NAI-35-T3" with commit reference.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/npc_iterator.go pkg/script/state.go
git commit --no-gpg-sign -m "polish(script): NAI-35 Task 7 — NAI-33-D1 retirement + final review

Drop deferred-comment annotations on NpcIterator.huntvis field +
passesFilter + NpcLookup doc-comment now that HuntAll-mode iterators
(NAI-35-T3, T4) consume huntvis.

Closes memory: NAI-33-D1 (huntvis dead-field deferral).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## NAI-35 close commit

After Task 7 lands, create one final close commit summarizing the sub-spec:

```bash
git commit --no-gpg-sign --allow-empty -m "chore(script,world): NAI-35 closed — six deferred opcode stubs ported

Closes NAI-34 follow-up #4 (NPC_PARAM, MAP_PLAYERCOUNT, HUNTALL,
MAP_FINDSQUARE) + sibling stubs NPC_HUNTALL, HUNTNEXT folded in per
audit_full_method_against_ts.md and dead_api_polish.md.

Sub-spec metrics: 7 implementation tasks, ~395 net production LOC +
~200 test LOC. Multi-task feature-port cadence per runescript_cadence.md.

Deviations active: NAI-35-D1 (cross-level rect uses from.level),
NAI-35-D2 (PlayerIterator HuntAll-only), NAI-35-D4 (math/rand vs
Math.random — instrumentation only).
Deviations retired: NAI-33-D1 (huntvis dead-field, by NAI-35-T3),
NAI-35-D3 (resolved at plan-write — used existing WorldVars.MapMembers).

Closes memory: NAI-33-D1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Smoke gate (post-close, user-driven)

Per `smoke_test_server_handoff.md`: Claude's sandboxed binary is unreachable from the host Java client. After NAI-35 close, ASK THE USER to launch the server. Smoke checklist (per spec section "Smoke gate"):

1. NPC_PARAM smoke — Lumbridge area; confirm no `no handler for opcode 2529` WARN in server log.
2. HUNTALL smoke — Al-Kharid; aggressive warriors fire approach behavior.
3. NPC_HUNTALL smoke — Barbarian Village beer barrels behavior visible.
4. MAP_FINDSQUARE smoke — Wizards' Tower imp visibly wanders.
5. MAP_PLAYERCOUNT smoke — optional via `[mes,debug_map_playercount]`.
6. No regressions in NAI-33 fishing-spot relocate (the original NAI-33 smoke gate must still hold).

If any smoke fails, surface as a Bundle 3 conditional follow-up per `investigation_subspec_cadence.md`.

---

## Post-close memory updates

Per `post_task_handoff.md`:

1. Save any non-derivable lessons learned during NAI-35 to memory (e.g., if D2's "deferred player-iterator modes" causes a real-world friction, capture as feedback memory).
2. Mark NAI-35 entry closed in `nai_followups.md`.
3. Provide the user with a paste-ready resume prompt for the next task (likely NPC_WALK opcode 2542 from NAI-34 follow-up #3, or one of the NAI-35-D2/D3/D4 tracker entries).
