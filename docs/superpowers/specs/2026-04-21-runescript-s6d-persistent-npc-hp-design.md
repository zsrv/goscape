# Sub-spec RuneScript S6d: Persistent NPC HP Storage — Design

**Status:** Draft → ready for plan
**Scope:** Promote `*Npc.curHP` and `*Npc.baseHP` from per-tick ephemeral state (cleared by `ResetMasks` each tick to sentinel `-1`) to persistent NPC state. Seed both at construction from `NpcType.Stats[NpcStatHitpoints]`. Drop the lazy baseHP seeding from `*Npc.Damage`. Add a public `*Npc.ResetHP()` helper for future respawn / AI paths. Export the currently-unexported `npcStat*` constants from `pkg/objtype`.
**Out of scope:** Player HP (`player_masks.go` has the same ephemeral-reset pattern; parallel cleanup for a future sub-spec). Death handling, respawn trigger wiring, HP regeneration, combat formulas — all deferred to the NPC AI / combat sub-spec that will be S6d's primary consumer. Player-side `ShowHit` equivalent. Any changes to the wire encoder (`pkg/rsbuf/npc_mask_payload.go`).

---

## Rationale

S6c's final review flagged persistent NPC HP as "the biggest owed debt." Current behavior:

- `NewNpc` initializes `curHP = -1, baseHP = -1`.
- `ResetMasks` resets both to `-1` every tick.
- `*Npc.Damage` lazily seeds `baseHP` from `NpcType.Stats[3]` via an `if n.baseHP < 0` guard.

Consequences:

1. Scripts querying `NPC_STAT(0)` (wired to `curHP` per S6a) on tick N+1 after damage on tick N see `-1`, not the real current HP.
2. A fresh NPC calling `Damage(3, 0)` on tick 1 produces `damageAmt=3, curHP=0` — the NPC has no HP to lose because it was never seeded.
3. The lazy `if baseHP < 0` seed leaks implementation detail (ResetMasks's sentinel) into the damage op, and would be clobbered by any future AI sub-spec that wants mutable persistent baseHP.

After S6d:

- `NewNpc` seeds both from `NpcType.Stats[NpcStatHitpoints]` (defaulting to 0 if typ is nil or Stats is too short).
- `ResetMasks` leaves curHP / baseHP untouched. Only per-tick hitsplat fields (`damageAmt`, `damageType`) get cleared.
- `*Npc.Damage` drops the lazy baseHP logic — it's always set at construction.
- `*Npc.ResetHP()` public method lets respawn / AI paths re-fill HP from the current type.
- `pkg/objtype.NpcStatHitpoints` (and siblings) exported so consumers can reference the stat slot by name.

No new user-visible behavior on the wire — the encoder continues to emit `damageAmt, damageType, curHP, baseHP` only when `NpcMaskDamage` is set, and those values are now *more accurate* rather than different-shape.

## Architecture

```
pkg/objtype/
└── npctype.go                  (modify) — export NpcStat* constants (6 names)

modules/world/
├── npc.go                      (modify) — NewNpc seeds curHP + baseHP; add initialHP helper
├── npc_masks.go                (modify) — *Npc.Damage drops lazy init; ResetMasks slimmed;
│                                            new *Npc.ResetHP helper
└── npc_masks_test.go           (modify) — update helper doc; add 7 new tests
```

Total: **~190 LOC** (production + tests).

## Components

### 1. `pkg/objtype/npctype.go` — export stat constants

Replace:
```go
const (
	npcStatAttack    = 0
	npcStatDefence   = 1
	npcStatStrength  = 2
	npcStatHitpoints = 3
	npcStatRanged    = 4
	npcStatMagic     = 5
)
```
With:
```go
// NpcStat* are indices into NpcType.Stats for combat-relevant attributes.
const (
	NpcStatAttack    = 0
	NpcStatDefence   = 1
	NpcStatStrength  = 2
	NpcStatHitpoints = 3
	NpcStatRanged    = 4
	NpcStatMagic     = 5
)
```

Update the one internal reference at `npctype.go:155` (`t.Stats[npcStatHitpoints] = dat.G2()`) to the capitalized form.

### 2. `modules/world/npc.go` — seed HP at construction

Find the `NewNpc` constructor. Replace the `curHP: -1, baseHP: -1` lines with:
```go
curHP:  initialHP(typ),
baseHP: initialHP(typ),
```

Add the helper in the same file (next to `NewNpc`):
```go
// initialHP returns the max HP stored in an NpcType, defaulting to 0 when
// typ is nil or Stats doesn't cover the Hitpoints slot. This runs at NPC
// construction and at ResetHP.
func initialHP(typ *objtype.NpcType) int {
	if typ == nil || len(typ.Stats) <= objtype.NpcStatHitpoints {
		return 0
	}
	hp := int(typ.Stats[objtype.NpcStatHitpoints])
	if hp < 0 {
		return 0
	}
	return hp
}
```

### 3. `modules/world/npc_masks.go` — simplify Damage, slim ResetMasks, add ResetHP

**Damage** — drop the lazy baseHP block:
```go
func (n *Npc) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	prevHP := n.curHP
	if amount > prevHP {
		n.damageAmt = prevHP
	} else {
		n.damageAmt = amount
	}
	n.damageType = dmgType
	n.curHP -= amount
	if n.curHP < 0 {
		n.curHP = 0
	}
	n.masks |= rsbuf.NpcMaskDamage
}
```

Note: `prevHP` guard doesn't need a `>= 0` check anymore — curHP is always seeded at construction, never `-1`. The overkill clamp still matches TS `Npc.applyDamage` exactly.

**ResetMasks** — stop clearing curHP/baseHP:
```go
// ResetMasks clears mask bits + ephemeral per-tick state. Persistent fields
// (animID, faceEntity, faceSquareX/Z, changeTypeID, curHP, baseHP) are
// retained across ticks. damageAmt / damageType are per-tick hitsplat
// payload and get reset.
func (n *Npc) ResetMasks() {
	n.masks = 0
	n.sayText = nil
	n.damageAmt = -1
	n.damageType = -1
	n.spotanimID = -1
	n.spotanimHeight = -1
	n.spotanimDelay = -1
}
```

**ResetHP** — new public method:
```go
// ResetHP re-seeds curHP + baseHP from the NPC's current NpcType. Called by
// respawn paths (on NPC death-and-respawn) and by AI sub-spec code that
// needs to restore max HP on some trigger. Safe on nil typ.
func (n *Npc) ResetHP() {
	hp := initialHP(n.typ)
	n.curHP = hp
	n.baseHP = hp
}
```

### 4. `modules/world/npc_masks_test.go` — update helper + new tests

The existing `npcWithHP(t, maxHP, curHP)` helper still builds an NpcType, calls `NewNpc`, then manually overrides `npc.curHP`. After S6d, `NewNpc` produces the correct baseHP directly — the manual override is only needed when the test wants a *different starting curHP* than max.

Update its doc comment:
```go
// npcWithHP builds an Npc whose NpcType.Stats[NpcStatHitpoints] = maxHP,
// then overrides curHP if needed. NewNpc seeds both curHP and baseHP from
// Stats[NpcStatHitpoints] as of S6d, so the override is only meaningful
// when the caller wants a starting curHP distinct from max.
```

**New tests (7):**

```go
func TestNewNpcSeedsHPFromStats(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	typ.Stats = []uint16{0, 0, 0, 20, 0, 0} // Hitpoints slot
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	if npc.curHP != 20 {
		t.Errorf("curHP: got %d, want 20", npc.curHP)
	}
	if npc.baseHP != 20 {
		t.Errorf("baseHP: got %d, want 20", npc.baseHP)
	}
}

func TestNewNpcWithNilTypeSeedsZeroHP(t *testing.T) {
	npc := NewNpc(0, 0, 3222, 3218, 0, nil)
	if npc.curHP != 0 {
		t.Errorf("curHP: got %d, want 0", npc.curHP)
	}
	if npc.baseHP != 0 {
		t.Errorf("baseHP: got %d, want 0", npc.baseHP)
	}
}

func TestNewNpcWithShortStatsSeedsZeroHP(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	// No Stats seeding at all — default-empty slice.
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	if npc.curHP != 0 || npc.baseHP != 0 {
		t.Errorf("curHP/baseHP: got %d/%d, want 0/0", npc.curHP, npc.baseHP)
	}
}

func TestNpcDamagePersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	if npc.curHP != 7 {
		t.Fatalf("pre-reset curHP: got %d, want 7", npc.curHP)
	}
	npc.ResetMasks()
	if npc.curHP != 7 {
		t.Errorf("post-reset curHP: got %d, want 7 (persistent)", npc.curHP)
	}
	if npc.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1 (per-tick cleared)", npc.damageAmt)
	}
}

func TestNpcBaseHPPersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	npc.ResetMasks()
	if npc.baseHP != 10 {
		t.Errorf("post-reset baseHP: got %d, want 10 (persistent)", npc.baseHP)
	}
}

func TestNpcResetHP(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(7, 1)
	if npc.curHP != 3 {
		t.Fatalf("pre-reset curHP: got %d, want 3", npc.curHP)
	}
	npc.ResetHP()
	if npc.curHP != 10 {
		t.Errorf("curHP after ResetHP: got %d, want 10", npc.curHP)
	}
	if npc.baseHP != 10 {
		t.Errorf("baseHP after ResetHP: got %d, want 10", npc.baseHP)
	}
}

func TestNpcResetHPWithNilType(t *testing.T) {
	npc := NewNpc(0, 0, 3222, 3218, 0, nil)
	// Should not panic and should leave HP at 0 (no typ to read from).
	npc.ResetHP()
	if npc.curHP != 0 || npc.baseHP != 0 {
		t.Errorf("after ResetHP on nil-typ npc: got %d/%d, want 0/0", npc.curHP, npc.baseHP)
	}
}
```

## Data flow (happy path — damage across two ticks)

**Tick N:**
1. `NewNpc(..., typ)` created during world load. `typ.Stats[NpcStatHitpoints] = 10`, so `n.curHP = 10, n.baseHP = 10`.
2. Script runs `NPC_DAMAGE(3, 0)` on this NPC. `Damage`: `damageAmt = 3, damageType = 0, curHP = 7, baseHP stays 10, NpcMaskDamage flagged`.
3. NPC-info encoder emits hitsplat payload with `{dmgAmt=3, dmgType=0, cur=7, base=10}`.
4. End of tick: `ResetMasks` clears mask bits, `damageAmt = -1, damageType = -1`. `curHP = 7` and `baseHP = 10` preserved.

**Tick N+1:**
5. Another script runs `NPC_STAT(0)`. Reads `n.curHP = 7`. Pushes 7. Scripts can now see real post-damage HP (current behavior would push -1 or 0 depending on encoder).
6. Another script runs `NPC_DAMAGE(2, 0)`. `Damage`: `prevHP = 7, amount = 2, not overkill, damageAmt = 2, curHP = 5, baseHP = 10, mask re-flagged`.
7. Encoder emits `{dmgAmt=2, dmgType=0, cur=5, base=10}`.

**Later — respawn or script command:**
8. `ResetHP()` called — `curHP = 10, baseHP = 10`. NPC is "full HP" again.

## Edge cases

| # | Case | Handling |
|---|---|---|
| 1 | `NewNpc` with `typ == nil` | `initialHP` returns 0 → both HPs start at 0. Matches current test-fixture behavior. |
| 2 | `typ != nil` but `Stats` slice too short | `initialHP` guard returns 0. Safe. |
| 3 | `NpcType.Stats[NpcStatHitpoints] == 0` | HP starts at 0; `Damage` clamps. NPCs with 0 max HP are non-combatant scenery. |
| 4 | Damage overflow when curHP is 0 | Already handled; curHP clamps at 0. |
| 5 | `ResetHP()` on an NPC whose typ is stale (post-NPC_CHANGETYPE) | Uses current `n.typ` pointer. S6c noted that ChangeType doesn't re-resolve `n.typ`, so ResetHP would re-fill using the OLD type's Stats. Acceptable trade; future NPC_CHANGETYPE fix will address both together. |
| 6 | Wire encoder reads curHP/baseHP when `NpcMaskDamage` is NOT set | Doesn't happen — encoder only emits the damage block when the flag is on. Persistence is wire-invisible outside damage ticks. |
| 7 | Script reads `NPC_STAT(0)` on fresh unhit NPC | Now returns the seeded max HP. Previously returned -1 or 0 depending on tick phase. This is the intended new behavior. |
| 8 | `ShowHit(amount, dmgType, cur, base)` — legacy script-op path | Still works; overwrites curHP/baseHP directly. Test-only today; future AI code can use Damage instead. No changes needed. |

## Key design calls

- **NPC HP is now persistent; PLAYER HP is unchanged.** `player_masks.go:73-74` resets `curHP = -1, baseHP = -1` in `Player.ResetMasks` — exactly the same anti-pattern. That parallel cleanup is deliberately out of scope for S6d to keep the diff focused on the NPC side (different consumers, different test surface, different integration with login flow). Flagged as a future follow-up.
- **`NpcStat*` constants get promoted to exported as part of this sub-spec.** Combines cleanly with the new HP-seeding code (both reference the same slot) and retires the S6c-flagged magic `3`. Other internal readers get the capitalized form in the same commit.
- **`ResetHP` is the public API shape the AI sub-spec will need.** Ships now as scaffolding even though no production caller exists — the tests exercise it, and it's the obvious contract for "refill HP to max." Trivial to remove if it turns out not to fit.
- **`ShowHit` stays untouched.** Currently test-only (grep confirms no production callers). Future code can prefer `Damage` for the script-ish semantic; ShowHit remains available for wire-format tests that want to set exact cur/base values.
- **No wire-encoder changes.** The NPC info mask payload already reads curHP/baseHP via `NpcSource.CurHP()/BaseHP()`. Those accessors stay identical; they just return real values instead of `-1` after this change.
- **Damage's `prevHP >= 0` guard removed.** Current overkill clamp has `if amount > prevHP && prevHP >= 0` — the `prevHP >= 0` existed because `curHP` started at `-1` and we didn't want to report `-1` damage. With seeded HP, `prevHP` is always ≥ 0, so the check simplifies.

## Gotchas

- **`NpcType.Stats` is `[]uint16`, nil-able.** The `initialHP` guard must handle both nil slice and short slice. This was the Task 2 S6c bug; don't repeat it.
- **`ResetMasks` still clears `damageAmt / damageType`.** Don't accidentally move those alongside curHP — they're genuinely per-tick and must reset.
- **Test assertions that check `npc.curHP != -1` (if any exist)** will start failing because that's no longer the post-ResetMasks state. Grep before committing to confirm none exist. None found in the current tree; still flag for future safety.
- **`NewNpc` ctor signature doesn't change.** Callers (handler_opnpc, test fixtures, npc_registry) pass the same args. Only the stored field values change.
- **`NpcSource` interface is unchanged.** `CurHP()` / `BaseHP()` accessors still return ints from the same fields; the wire encoder doesn't care whether they came from persistent state or per-tick reset.
