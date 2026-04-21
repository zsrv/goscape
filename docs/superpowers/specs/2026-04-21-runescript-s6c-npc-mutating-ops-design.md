# Sub-spec RuneScript S6c: NPC Mutating Ops Batch — Design

**Status:** Draft → ready for plan
**Scope:** Expose 4 NPC-mutating script opcodes — `NPC_ANIM`, `NPC_FACESQUARE`, `NPC_CHANGETYPE`, `NPC_DAMAGE`. Ships the `ActiveNpc` interface extension (4 methods), 4 handlers, one new `*Npc.Damage` method on the world side (managing HP decrement + mask flag). No new mask bits or encoder changes — rides entirely on existing S6b-era plumbing.
**Out of scope:** NPC_SETMODE (no AI consumer yet — deferred to an AI sub-spec), NPC_SPOTANIM / NPC_FACEENTITY (no corresponding script opcodes exist in goscape's opcode table — they are engine-internal only), NPC_TELE / NPC_WALK / NPC_STATADD / NPC_STATHEAL / NPC_STATSUB (partial state coverage — NPC stat array beyond HP not yet wired per S6a), NPC_DEL / AI-death triggers / auto-retaliate / combat defense calculation / HP regeneration (all belong in a future NPC AI + combat sub-spec).

---

## Goal

After S6c:

- A `[opnpc1,rat]` script can call `~npc_anim(<attack_seq>) ~npc_facesquare(<player_x>, <player_z>) ~npc_damage(3, 0)` and the player sees the rat animate, turn, and take a 3-damage hit on the wire.
- Transformation scripts can call `~npc_changetype(<new_id>)` to morph the NPC mid-interaction (e.g., curse-triggered polymorph).
- NPC HP is tracked server-side: `NPC_DAMAGE(3, 0)` decrements `curHP` and clamps at 0.

Every handler gates on `ActiveNpc != nil` with the same `requireActiveNpc` helper as S6a/S6b.

## Architecture

```
pkg/script/
├── active.go                 (modify) — ActiveNpc gains 4 methods
├── handlers_npc.go           (modify) — 4 new handlers
├── handlers.go               (modify) — 4 registrations under "// S6c: NPC mutating ops batch."
└── handlers_npc_test.go      (modify) — mockNpc extended; 5 new tests

modules/world/
├── npc_masks.go              (modify) — new Damage(amount, dmgType) method
├── npc_masks_test.go         (modify or create) — 3 tests for Damage HP semantics
├── npc_script.go             (untouched — *Npc already satisfies Animate/FaceCoord/ChangeType)
└── script_test.go            (modify) — one new E2E test (ANIM + SAY compound-mask emission)
```

No new files. No new mask bits (all 4 ops reuse existing S6b-era encoder entries: `NpcMaskAnim 0x02`, `NpcMaskDamage 0x10`, `NpcMaskChangeType 0x20`, `NpcMaskFaceCoord 0x80`).

## Components

### 1. `ActiveNpc` interface extension — `pkg/script/active.go`

Append 4 methods inside the `ActiveNpc` interface, immediately after `Say(text []byte)`:

```go
	// Animate schedules sequence `id` with client-side `delay` on the NPC's
	// primary animation slot this tick. id = -1 clears.
	Animate(id, delay int)

	// FaceCoord rotates the NPC to face absolute square (x, z). Wire coords
	// are doubled + 1 (face-center convention).
	FaceCoord(x, z int)

	// ChangeType morphs the NPC to `newType`. The client swaps the model on
	// the next NPC-info flush; server-side fields beyond typeId are not
	// re-initialized (stats, category, etc. still reference the old config).
	// The script op NPC_CHANGETYPE also carries a `duration` parameter for
	// timed revert, but S6c discards it (method takes type only); future
	// AI sub-spec wires a revert timer.
	ChangeType(newType int)

	// Damage applies `amount` damage of `dmgType` to the NPC this tick,
	// flagging NpcMaskDamage. Decrements curHP (clamped at 0). Does NOT
	// trigger death handling or auto-retaliate — those belong in a future
	// NPC AI sub-spec.
	Damage(amount, dmgType int)
```

### 2. `*Npc.Damage` — new method in `modules/world/npc_masks.go`

```go
// Damage applies `amount` damage of `dmgType` to the NPC this tick, flagging
// NpcMaskDamage so the NPC-info encoder emits the hitsplat. curHP decrements
// by amount (clamped at 0); baseHP is set from the NpcType's stored max HP
// if available, otherwise left at its current value. Negative `amount` is
// coerced to 0 defensively (a script bug shouldn't heal the NPC).
//
// This method is a pure output op — it does NOT fire ai_death, remove the
// NPC, or adjust target / aggression state. Scripts that need death logic
// should test NPC_STAT(0) after NPC_DAMAGE and fire their own despawn.
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	n.damageAmt = amount
	n.damageType = dmgType
	n.curHP -= amount
	if n.curHP < 0 {
		n.curHP = 0
	}
	if n.typ != nil {
		if hp := int(n.typ.Stats[npcStatHitpoints]); hp > 0 {
			n.baseHP = hp
		}
	}
	n.masks |= rsbuf.NpcMaskDamage
}
```

Notes:
- `npcStatHitpoints` is `3` — the index into `NpcType.Stats` established in `pkg/objtype/npctype.go`.
- `n.damageAmt`, `n.damageType`, `n.curHP`, `n.baseHP` all already exist on `*Npc`.
- `ResetMasks` already clears `damageAmt = -1, damageType = -1, curHP = -1, baseHP = -1` at tick-end — our values get written fresh each call.

### 3. 4 handlers — `pkg/script/handlers_npc.go`

Append after `handleNpcSay`:

```go
// handleNpcAnim pops (delay, id) in TS order (id on top) and schedules the
// animation on the active NPC this tick.
func handleNpcAnim(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_ANIM"); err != nil {
		return err
	}
	delay := s.PopInt()
	id := s.PopInt()
	s.ActiveNpc.Animate(id, delay)
	return nil
}

// handleNpcFaceSquare pops (z, x) in TS order (x on top) and rotates the NPC
// to face absolute square (x, z).
func handleNpcFaceSquare(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_FACESQUARE"); err != nil {
		return err
	}
	coord := s.PopInt()
	x, z := coord>>14&0x3fff, coord&0x3fff
	s.ActiveNpc.FaceCoord(x, z)
	return nil
}

// handleNpcChangeType pops (duration, newType) in TS order (duration on top)
// and morphs the NPC. Duration = ticks until the NPC reverts to its original
// type; 0 = permanent. S6c ignores duration (timed revert deferred to a
// future sub-spec) and always applies a permanent change; NPC_CHANGETYPE
// with a non-zero duration will silently NOT revert.
func handleNpcChangeType(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
		return err
	}
	_ = s.PopInt() // duration — deferred; see spec §6c Gotchas
	newType := s.PopInt()
	s.ActiveNpc.ChangeType(newType)
	return nil
}

// handleNpcDamage pops (type, amount) in TS order (amount on top) and
// applies damage. The concrete Npc impl manages HP; this handler stays thin.
func handleNpcDamage(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DAMAGE"); err != nil {
		return err
	}
	amount := s.PopInt()
	dmgType := s.PopInt()
	s.ActiveNpc.Damage(amount, dmgType)
	return nil
}
```

**Pop-order note** (verified against `LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts`):
- **NPC_FACESQUARE**: single packed coord pop (`level<<28 | x<<14 | z`). Handler unpacks x/z at call site.
- **NPC_ANIM**: pops (seq, delay) with `delay` on top.
- **NPC_CHANGETYPE**: pops (newType, duration) with `duration` on top. S6c discards duration — see Gotchas.
- **NPC_DAMAGE**: pops (type, amount) with `amount` on top.

All four orderings are tested explicitly so a future refactor can't silently swap them.

### 4. Registration — `pkg/script/handlers.go`

Add after the existing `OpNpcSay: handleNpcSay` entry:

```go
	// S6c: NPC mutating ops batch.
	OpNpcAnim:       handleNpcAnim,
	OpNpcFaceSquare: handleNpcFaceSquare,
	OpNpcChangeType: handleNpcChangeType,
	OpNpcDamage:     handleNpcDamage,
```

### 5. mockNpc extension — `pkg/script/handlers_npc_test.go`

Extend fields:

```go
	animCalls       []struct{ id, delay int }
	faceCoordCalls  []struct{ x, z int }
	changeTypeCalls []int
	damageCalls     []struct{ amount, dmgType int }
```

Methods:

```go
func (m *mockNpc) Animate(id, delay int)        { m.animCalls = append(m.animCalls, struct{ id, delay int }{id, delay}) }
func (m *mockNpc) FaceCoord(x, z int)           { m.faceCoordCalls = append(m.faceCoordCalls, struct{ x, z int }{x, z}) }
func (m *mockNpc) ChangeType(newType int)       { m.changeTypeCalls = append(m.changeTypeCalls, newType) }
func (m *mockNpc) Damage(amount, dmgType int)   { m.damageCalls = append(m.damageCalls, struct{ amount, dmgType int }{amount, dmgType}) }
```

## Data flow (NPC_ANIM tick)

1. Script running with `ActiveNpc = npc` executes `OpNpcAnim` with int stack top = `(id, delay)`.
2. `handleNpcAnim` pops `delay`, then `id`; calls `s.ActiveNpc.Animate(id, delay)`.
3. `*Npc.Animate` sets `n.animID = id, n.animDelay = delay, n.masks |= NpcMaskAnim`.
4. Later in the tick, `rsbuf.npcMaskPayload` encoder sees the bit, writes the anim block (u2 id, u1 delay).
5. `ResetMasks` clears the bit + fields at tick end.

Identical shape for NPC_FACESQUARE / NPC_CHANGETYPE / NPC_DAMAGE — they only differ in the fields touched.

## Edge cases

| # | Case | Handling |
|---|---|---|
| 1 | Any op with `ActiveNpc == nil` | `requireActiveNpc` error → script aborts with `"<OP>: no active npc"` |
| 2 | NPC_DAMAGE with negative amount | Clamp to 0 defensively; don't heal |
| 3 | NPC_DAMAGE with amount > curHP | `curHP` floors at 0; no death trigger (S6c explicitly not AI) |
| 4 | NPC_DAMAGE when `npc.typ` is nil | `baseHP` left at current value; no panic |
| 5 | NPC_ANIM with id = -1 | Delegates to existing `Npc.Animate(-1, 0)` — cleared animation |
| 6 | NPC_CHANGETYPE to an unknown type id | Method just stores the id; encoder emits it; client-side issue only |
| 7 | Multiple mutating ops in same script tick | `masks |=` accumulates all bits; encoder emits all payloads in-order |

**Non-obvious:** E2E test asserts case 7 — running `NPC_ANIM` and `NPC_SAY` in one script must leave both `NpcMaskAnim` and `NpcMaskSay` set, proving the encoder handles compound writes.

## Testing strategy

### Unit tests — `pkg/script/handlers_npc_test.go` (diff)

| Test | Assertion |
|---|---|
| `TestNpcAnim` | `push id + push delay + NPC_ANIM` → `mockNpc.animCalls == [{id, delay}]` |
| `TestNpcFaceSquare` | `push packed(x, z) + NPC_FACESQUARE` → `mockNpc.faceCoordCalls == [{x, z}]` |
| `TestNpcChangeType` | `push newType + NPC_CHANGETYPE` → `mockNpc.changeTypeCalls == [newType]` |
| `TestNpcDamage` | `push type + push amount + NPC_DAMAGE` (amount on top per TS) → `mockNpc.damageCalls == [{amount, type}]` |
| `TestNpcChangeTypeDiscardsDuration` | `push newType + push duration + NPC_CHANGETYPE` → `mockNpc.changeTypeCalls == [newType]`; asserts duration is popped but ignored |
| `TestNpcMutatingOpsRequireActiveNpc` | Table-driven over all 4 ops; each returns `"<OP>: no active npc"` error with nil ActiveNpc |

### HP integration — `modules/world/npc_masks_test.go` (create if absent)

```go
func TestNpcDamageDecrementsHPAndSetsMask(t *testing.T)
// NpcType with Stats[3] = 10; curHP = 10.
// Damage(3, 1). Assert curHP == 7, baseHP == 10, damageAmt == 3, damageType == 1,
// masks & NpcMaskDamage != 0.

func TestNpcDamageClampsAtZero(t *testing.T)
// curHP = 2. Damage(5, 1). Assert curHP == 0 (not -3), damageAmt == 5.

func TestNpcDamageNegativeAmountClampsToZero(t *testing.T)
// curHP = 10. Damage(-3, 1). Assert curHP == 10, damageAmt == 0.
```

### E2E — `modules/world/script_test.go` (diff)

```go
func TestOpNpc1FiresScriptAndEmitsAnimPlusSay(t *testing.T)
// Register [opnpc1, type=7]: push 42 + push 3 + NPC_ANIM + push "cluck" + NPC_SAY + RETURN.
// Fire OPNPC1 click with NPC placed adjacent; run processInteraction once.
// Assert:
//   npc.animID == 42, npc.animDelay == 3
//   npc.sayText == "cluck"
//   npc.masks & NpcMaskAnim != 0
//   npc.masks & NpcMaskSay != 0
//   p.target == nil, p.interactionFired == true
```

Proves compound masks flow cleanly through a single-tick script.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/script/active.go` (diff) | +20 |
| `pkg/script/handlers_npc.go` (diff) | +55 |
| `pkg/script/handlers.go` (diff) | +7 |
| `pkg/script/handlers_npc_test.go` (diff) | +120 |
| `modules/world/npc_masks.go` (diff) | +22 |
| `modules/world/npc_masks_test.go` (new or diff) | +65 |
| `modules/world/script_test.go` (diff) | +55 |
| **Total** | **~344 LOC** |

## Key design calls

- **Thin handlers; the Npc method owns the semantic.** `handleNpcDamage` is five pop/delegate lines; HP arithmetic lives in `*Npc.Damage`. This makes the handler file a uniform wire-to-method bridge and keeps the non-trivial logic where future combat sub-specs will extend it.
- **Pop order is tested explicitly for every multi-arg op.** A future refactor can't silently swap `(amount, type)` in NPC_DAMAGE without a test failing. Small defensive shield against a real bug class.
- **No new mask bits.** All 4 ops reuse existing `NpcMask*` constants and encoder entries shipped before S6b. The encoder is unchanged; only the condition-to-emit side wires up.
- **NPC_CHANGETYPE doesn't re-initialize cached type state.** `n.typ` points at the *old* config after ChangeType; subsequent `NPC_NAME` / `NPC_CATEGORY` reads will return the new typeID but the old cached config. Fixing this would require re-looking-up through `Configs` on every read — out of scope; document the caveat.
- **`baseHP` is refreshed from `NpcType.Stats[npcStatHitpoints]` on every Damage call.** Cheap (~2 pointer derefs) and keeps the wire accurate even if earlier ticks left `baseHP` stale via `ResetMasks`. Alternative (lazy-init baseHP once at Npc construction) is cleaner but beyond S6c scope.
- **Scope-corrected from β.** Initial design proposed 6 handlers including NPC_SPOTANIM and NPC_FACEENTITY; fact-check showed no corresponding script opcodes exist in `pkg/script/opcode.go`. The `*Npc.SpotAnim` and `*Npc.SetFaceEntity` methods remain engine-internal (used by processInteraction). Real scope: 4 ops.

## Gotchas

- **`NpcType.Hitpoints` is NOT a field.** Max HP lives in `NpcType.Stats[npcStatHitpoints]` — index 3. Use the constant from `pkg/objtype/npctype.go` (unexported; the reference is `npcStatHitpoints = 3`).
- **`NPC_FACESQUARE` pop-order assumption.** Spec writes the handler as if it pops a single packed coord; verify against TS `NpcOps.ts` during implementation. If TS pops two separate ints, drop the unpack and pop x + z directly.
- **`ResetMasks` zeroes `curHP / baseHP` to -1 at tick end.** `Damage` re-asserts baseHP from the type config; curHP must be set by AI/combat logic or by scripts before the next Damage call, otherwise the wire sends `cur=-1` which the client may render as full or empty depending on version.
- **`ActiveNpc` interface now has 16 methods** (after S6a=11, S6b=+1, S6c=+4). Still cohesive — all are "what scripts can read/write on an NPC." No split warranted yet; revisit around 30.
- **NPC_CHANGETYPE duration is popped-and-discarded.** Cache scripts calling `npc_changetype <id>, 0` (permanent change) behave correctly. `npc_changetype <id>, 50` (50-tick revert) stays permanently changed — the revert timer is unimplemented in S6c. Future AI sub-spec ships a revert timer. The opcode handler itself is correctly shaped (pops both args, TS-matching) so the revert can be added without a handler-signature change.
