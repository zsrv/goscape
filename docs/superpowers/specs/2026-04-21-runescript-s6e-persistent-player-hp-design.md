# Sub-spec RuneScript S6e: Persistent Player HP (TS-Faithful) — Design

**Status:** Draft → ready for plan
**Scope:** Eliminate the duplicated `Player.curHP` / `Player.baseHP` fields. Route current and base HP through `levels[PlayerStatHitpoints]` and `baseLevels[PlayerStatHitpoints]` — the existing skill-array fields, which already are the source of truth for HP in the TS engine. Add `Player.Damage(amount, dmgType int)` matching TS `Player.applyDamage`. Add `Player.ResetHP()` Go-side helper. Delete the test-only `Player.ShowHit` API. Seed Hitpoints to 10 in `processLogins` (matches TS `PlayerLoading.ts:49-51`). Export a `PlayerStatHitpoints` constant from `pkg/objtype`.

**Out of scope:** HP save/restore across logout (login proto has a `bytes save` field but no serialization code; deferred to a future sub-spec). Death handling, respawn-trigger wiring, HP regeneration, combat formulas — future combat sub-spec. Other 20 PlayerStat constants beyond Hitpoints (added when consumers appear). The `OpDamage` (2015) script handler — wires straightforwardly to `Player.Damage` after S6e but stays out of scope here. `Npc` HP plumbing (already shipped in S6d).

---

## Rationale

The S6d final review flagged Player HP as "the parallel anti-pattern" to NPC HP — `player_masks.go:73-74` clears `curHP` and `baseHP` to `-1` every tick the same way the pre-S6d Npc did. But fact-checking against TS `Player.ts:1860-1873` reveals a deeper issue: **the TS Player has no separate `curHP` field at all**. Current HP IS `this.levels[PlayerStat.HITPOINTS]`. The wire encoder (`World.ts:1023-1024`) writes `levels[HITPOINTS]` and `baseLevels[HITPOINTS]` directly. Goscape's separate `p.curHP` and `p.baseHP` fields are a duplication that wouldn't exist if the goscape Player had been ported faithfully from TS.

The right S6e move is therefore not "mirror S6d" — it's "remove the duplication and route everything through the skill array, like TS." Healing potions, regen, stat boost/drain all already write to `p.levels[3]`, so HP becomes correctly affected by all of them without any extra wiring.

A pre-existing bug surfaces during the work: `processLogins` doesn't seed `baseLevels[Hitpoints]` or `levels[Hitpoints]`. They start at 0. This was invisible while the parallel `curHP`/`baseHP` fields were ephemeral and never read meaningfully. With the duplication removed, login seeding becomes mandatory — matching TS `PlayerLoading.ts:49-51` (`baseLevels[HITPOINTS] = 10; levels[HITPOINTS] = 10`).

## Architecture

```
pkg/objtype/
└── npctype.go                  (modify) — add PlayerStatHitpoints constant alongside NpcStat*

modules/world/
├── player.go                   (modify) — delete curHP/baseHP fields; remove their `-1` init
├── player_source.go            (modify) — CurHP/BaseHP getters now derive from levels[]/baseLevels[]
├── player_masks.go             (modify) — DELETE Player.ShowHit;
│                                            ResetMasks no longer touches HP fields (gone);
│                                            new Player.Damage(amount, dmgType);
│                                            new Player.ResetHP()
├── player_masks_test.go        (modify) — rewrite ShowHit-based tests to use Damage;
│                                            add HP-persistence-across-ResetMasks tests
└── tick.go                     (modify) — processLogins seeds baseLevels[Hitpoints]=10, levels[Hitpoints]=10

(plus tick test file with login-seeding test)
```

Total: **~270 LOC** (production + tests).

## Components

### 1. `pkg/objtype/npctype.go` — add `PlayerStatHitpoints`

Append after the existing `NpcStat*` block:

```go
// PlayerStat* are indices into Player.levels and Player.baseLevels for
// player-skill slots. Only Hitpoints is exported here; other stats
// (Attack, Defence, Strength, Ranged, Prayer, Magic, Cooking, ...) get
// added as their first consumer ships. Index values match TS
// PlayerStat enum (PlayerStat.ts) — Hitpoints is 3, sharing the slot
// with NpcStatHitpoints since both represent the same skill index.
const (
	PlayerStatHitpoints = 3
)
```

### 2. `modules/world/player.go` — delete duplicated fields

Find the two struct fields (around line 194):
```go
curHP, baseHP   int
```
or whatever exact form they take. Delete them entirely.

In `newPlayer`, remove the two `curHP: -1, baseHP: -1` initializer lines. The `levels` and `baseLevels` arrays are already zero-initialized by the struct literal; HP-specific seeding now happens in `processLogins`.

### 3. `modules/world/player_source.go` — getters derive from skill arrays

Replace:
```go
func (p *Player) CurHP() int  { return p.curHP }
func (p *Player) BaseHP() int { return p.baseHP }
```

With:
```go
func (p *Player) CurHP() int  { return int(p.levels[objtype.PlayerStatHitpoints]) }
func (p *Player) BaseHP() int { return int(p.baseLevels[objtype.PlayerStatHitpoints]) }
```

The wire encoder at `pkg/rsbuf/mask_payload.go:95-100` continues to call these accessors — no encoder change needed.

### 4. `modules/world/player_masks.go` — delete ShowHit, slim ResetMasks, add Damage + ResetHP

**Delete `Player.ShowHit`** entirely. It's test-only (current callers: `player_masks_test.go:44, 76`). Tests get rewritten to use `Damage`.

**`Player.Damage(amount, dmgType int)`** — TS-faithful with one deliberate Go-side defensive deviation (negative-amount clamp), matching the existing `*Npc.Damage` convention from S6c:

```go
// Damage applies `amount` damage of `dmgType` to the player this tick,
// flagging MaskDamage so the player-info encoder emits the hitsplat. HP
// decrements via levels[Hitpoints] (the single source of truth — no
// separate curHP field as of S6e). On overkill (amount > current HP),
// damageAmt clamps to the pre-hit HP so the wire shows only damage
// actually dealt — matches TS Player.applyDamage (Player.ts:1860-1873).
//
// Negative amount coerces to 0 defensively. This deviates from TS where
// negative amount would heal the player (`current - (-3) = current + 3`
// passes the overkill check and writes back). The TS path is almost
// certainly an unintended consequence of unsigned-input assumptions; we
// match the *Npc.Damage convention from S6c instead.
//
// This is a pure output op — no death / auto-retaliate / aggro logic.
// Death/respawn/regen belong in a future combat sub-spec.
func (p *Player) Damage(amount, dmgType int) {
	if amount < 0 {
		amount = 0
	}
	current := int(p.levels[objtype.PlayerStatHitpoints])
	p.damageAmt = min(amount, current)
	p.damageType = dmgType
	next := current - amount
	if next < 0 {
		next = 0
	}
	p.levels[objtype.PlayerStatHitpoints] = uint8(next)
	p.masks |= rsbuf.MaskDamage
}
```

**`Player.ResetHP()`** — Go-side helper (no direct TS counterpart; TS does HP refill inline in death/respawn code). Restores levels[Hitpoints] from baseLevels[Hitpoints] — RS2 convention is "respawn fills you to base, not to boosted-max":

```go
// ResetHP restores levels[Hitpoints] to baseLevels[Hitpoints] — the
// player's "full HP" state. Called by respawn paths and certain script
// triggers in future sub-specs. Boost/drain effects on Hitpoints are
// wiped (RS2 convention: respawn fills to base, not boosted-max).
//
// No direct TS counterpart — TS performs HP refill inline within death
// handling. This Go-side helper makes the intent reusable.
func (p *Player) ResetHP() {
	p.levels[objtype.PlayerStatHitpoints] = p.baseLevels[objtype.PlayerStatHitpoints]
}
```

**`ResetMasks` (existing)** — delete the two lines that cleared curHP/baseHP (now-deleted fields). Keep the rest:

```go
// ResetMasks clears mask bits + ephemeral per-tick state. Persistent
// HP lives in levels[Hitpoints]/baseLevels[Hitpoints] (S6e); this
// method no longer touches it. damageAmt / damageType remain per-tick
// hitsplat payload.
func (p *Player) ResetMasks() {
	p.masks = 0
	p.tele = false
	p.jump = false
	p.sayText = nil
	// ... whatever other ephemeral fields exist; keep them as-is ...
	p.damageAmt = -1
	p.damageType = -1
	// p.curHP = -1   ← deleted
	// p.baseHP = -1  ← deleted
	// (rest of ResetMasks unchanged)
}
```

The exact body shape depends on what's already in player_masks.go's ResetMasks; only the two HP-reset lines change.

### 5. `modules/world/tick.go` — seed Hitpoints in `processLogins`

In `processLogins` (around line 73), inside the per-player loop after `addPlayer` succeeds, add:

```go
// Seed Hitpoints to 10 (RS2 default starting HP) before any code
// reads p.levels[PlayerStatHitpoints]. Matches TS PlayerLoading.ts:49-51.
// Full skill initialization (all 21 skills with persisted XP) is a
// future sub-spec; S6e covers Hitpoints only because the persistent-HP
// design requires it.
p.baseLevels[objtype.PlayerStatHitpoints] = 10
p.levels[objtype.PlayerStatHitpoints] = 10
```

Place this near the other per-player initialisation (after `p.invs = ...`, before the LOGIN trigger fires at the bottom of the loop).

## Data flow (happy path — damage across two ticks)

**Tick N (login + first damage):**
1. `processLogins` seeds `baseLevels[3] = 10, levels[3] = 10`.
2. Script runs `Player.Damage(3, 0)` (e.g. via PLAYER_DAMAGE op when wired). `current = 10, damage = 3, not overkill, damageAmt = 3, levels[3] = 7, MaskDamage flagged`.
3. Player-info encoder emits hitsplat block with `damageAmt=3, damageType=0, CurHP()=7, BaseHP()=10`.
4. End of tick: `ResetMasks` clears mask bits + damageAmt/damageType. levels[3] = 7 preserved.

**Tick N+1:**
5. Script runs `STAT(3)` (pre-existing in S6a as `Player.Stat`). Returns `int(p.levels[3]) = 7`. Real persistent HP, not -1 or 0.
6. Another `Damage(2, 0)` runs. current = 7, damageAmt = 2, levels[3] = 5.
7. Encoder emits `{2, 0, 5, 10}`.

**Boost interaction:**
8. Player drinks Saradomin brew, which writes `levels[3] = 14` (above base 10). curHP is now effectively 14.
9. `Damage(3, 0)` runs. current = 14, damageAmt = 3, levels[3] = 11. Encoder emits `{3, 0, 11, 10}`. Note: cur > base on the wire — the client displays this correctly (boosted state).
10. Boost wears off: separate timer logic decrements `levels[3]` by 1 each tick until back at baseLevels[3]. Already wired via existing skill mechanisms; no S6e work needed.

**Respawn:**
11. `Player.ResetHP()` called by future death/respawn path. `levels[3] = baseLevels[3] = 10`. Player at full base HP again.

## Edge cases

| # | Case | Handling |
|---|---|---|
| 1 | Damage on a fresh-login player | `levels[3]=10` from login seed; standard Damage path. |
| 2 | Damage > current HP (overkill) | `damageAmt = min(amount, current)`. levels[3] floors at 0. Matches TS Player.ts:1865-1867. |
| 3 | Negative damage amount | Clamped to 0; mask still flagged (zero hitsplat — debug signal). Defensive deviation from TS. |
| 4 | Damage when `levels[3] == 0` | `damageAmt = min(amount, 0) = 0`; levels[3] stays 0; mask flips. Wire emits 0 damage on 0/N HP. |
| 5 | ResetHP when `baseLevels[3] == 0` | levels[3] becomes 0. Login seeding (= 10) prevents this in normal flow. |
| 6 | Boosted Hitpoints (e.g., levels[3]=14, baseLevels[3]=10), then Damage(3) | current=14, damageAmt=3, levels[3]=11, baseLevels unchanged. Wire `cur=11, base=10` — TS-correct (boosted state visible). |
| 7 | Drained Hitpoints (e.g., levels[3]=7, baseLevels[3]=10), then Damage | Standard path with current=7. ResetHP restores to base = 10. |
| 8 | Existing tests reading `p.curHP` / `p.baseHP` | Compile error (`undefined field`). Each must be rewritten to read `p.levels[3]` / `p.baseLevels[3]`. Same wire-byte values; same test intent. |
| 9 | Wire encoder reads `CurHP() / BaseHP()` when `MaskDamage` is NOT set | Doesn't happen — encoder only emits damage block when mask is on. Persistence is wire-invisible outside damage events. |
| 10 | `processLogins` runs before `levels` array exists | Cannot — `levels [21]uint8` is allocated by the struct literal in `newPlayer`. No race. |

**Non-obvious rules:**

- **Healing potions / regen / boost-drain "just work."** They already write `p.levels[3]`. With curHP duplication gone, HP automatically reflects them. Zero new wiring.
- **Combat scripts can read current HP via `STAT(3)`** (already wired in S6a). They get the real persistent value.
- **`ResetHP` does NOT touch `baseLevels`.** Drain effects on baseLevels (rare in RS2) persist across respawn unless explicit logic restores them.
- **Wire `cur` byte can exceed `base` byte.** When the player is HP-boosted, `levels[3] > baseLevels[3]`. The client renders this correctly as "boosted HP bar." Tests should not assume cur ≤ base.

## Testing strategy

### Tests to update (existing tests touching deleted fields)

In `modules/world/player_masks_test.go`, find every `p.ShowHit(...)` call and every `p.curHP` / `p.baseHP` reference. Rewrite via mechanical substitution:

| Old | New |
|---|---|
| `p.ShowHit(10, 1, 40, 50)` | `p.baseLevels[3] = 50; p.levels[3] = 50; p.Damage(10, 1)` |
| `p.curHP != 40` | `p.levels[3] != 40` |
| `p.baseHP != 50` | `p.baseLevels[3] != 50` |
| `p.damageAmt != 10` | `p.damageAmt != 10` (unchanged) |

Same wire bytes emitted in both shapes; test intent preserved.

### New tests in `modules/world/player_masks_test.go`

```go
func TestPlayerDamageDecrementsHitpointsAndSetsMask(t *testing.T)
// Setup: levels[3]=50, baseLevels[3]=50.
// Damage(10, 1).
// Assert: levels[3]==40, damageAmt==10, damageType==1, masks&MaskDamage != 0.

func TestPlayerDamageClampsAtZero(t *testing.T)
// Setup: levels[3]=2.
// Damage(5, 1).
// Assert: levels[3]==0, damageAmt==2 (clamped to pre-hit) — matches TS overkill.

func TestPlayerDamageNegativeAmountClampsToZero(t *testing.T)
// Setup: levels[3]=50.
// Damage(-3, 1).
// Assert: levels[3]==50 (no heal), damageAmt==0, mask still flipped.
// Documents the defensive deviation from TS.

func TestPlayerHPPersistsAcrossResetMasks(t *testing.T)
// Damage(3, 1) → levels[3]==47; ResetMasks(); assert levels[3]==47 still
// (persistent), damageAmt==-1 (per-tick cleared).

func TestPlayerResetHP(t *testing.T)
// Setup: baseLevels[3]=50, levels[3]=30.
// ResetHP(). Assert: levels[3]==50, baseLevels[3]==50 (unchanged).

func TestPlayerCurHPAndBaseHPDeriveFromLevels(t *testing.T)
// Setup: baseLevels[3]=50, levels[3]=35.
// Assert: p.CurHP()==35, p.BaseHP()==50. Verifies the getter rewrite.

func TestPlayerDamageWithBoostedHitpoints(t *testing.T)
// Setup: baseLevels[3]=10, levels[3]=14 (boosted).
// Damage(3, 1) → levels[3]==11, damageAmt==3, baseLevels[3]==10.
// ResetHP() → levels[3]==10 (restored to base, not boosted-max).
```

### New test for login seeding

In a tick-related test file (existing `tick_*_test.go` or a new `tick_logins_test.go`):

```go
func TestProcessLoginsSeedsHitpoints(t *testing.T) {
    s := newTestServer(t)
    p, _ := newTestPlayer(t)
    p.client.server = s
    s.newPlayers = []*Player{p}
    s.processLogins()
    if p.baseLevels[objtype.PlayerStatHitpoints] != 10 {
        t.Errorf("baseLevels[Hitpoints]: got %d, want 10",
            p.baseLevels[objtype.PlayerStatHitpoints])
    }
    if p.levels[objtype.PlayerStatHitpoints] != 10 {
        t.Errorf("levels[Hitpoints]: got %d, want 10",
            p.levels[objtype.PlayerStatHitpoints])
    }
}
```

### Cross-spec compatibility

- S6a `STAT(3)` reads `p.levels[3]` — now returns real persistent HP. Existing S6a tests use mocks or pre-seeded levels arrays; should still pass.
- S6c/S6d Npc HP tests untouched (different code path).
- S6b OPNPC dispatch tests untouched (no HP).

**Total test LOC: ~120** (7 new behavior tests + 1 login test + ~30 LOC of test rewrites).

## LOC estimate

| File | LOC |
|---|---|
| `pkg/objtype/npctype.go` (diff) | +6 |
| `modules/world/player.go` (diff) | -3 (delete fields + init) |
| `modules/world/player_source.go` (diff) | +2 (rewrite getters) |
| `modules/world/player_masks.go` (diff) | -10/+30 (delete ShowHit, slim ResetMasks, add Damage + ResetHP) |
| `modules/world/player_masks_test.go` (diff) | ~+90 (rewrites + 7 new tests) |
| `modules/world/tick.go` (diff) | +5 (login seed) |
| login-seed test (new or diff) | +25 |
| **Total** | **~145 production + ~115 test = ~260 LOC** |

## Key design calls

- **Match TS structure (skill array as source of truth), but match goscape's S6c convention on negative-damage clamp.** The TS structural decision (no separate curHP field) is correct and worth porting. The TS literal behavior on negative damage (heals the player) is almost certainly an unintended consequence of unsigned-input assumptions and worth deliberately deviating from. The two are independent calls.
- **`Player.ShowHit` deletion is safe.** Grep confirms only test-file callers. Tests rewrite to use `Damage` with the exact same wire-byte intent. No production caller breaks.
- **Login seeding is mandatory, not optional.** Pre-S6e, the missing seeding was invisible because `curHP`/`baseHP` were ephemeral. Post-S6e, players would log in at 0/0 HP without it. The fix lands in this sub-spec because the work uncovered the bug.
- **Only `PlayerStatHitpoints` exported.** YAGNI on the other 20 PlayerStat constants. Each can be added when its first consumer arrives — same approach as the S6c/S6d incremental constant exports.
- **`baseLevels` is NOT touched by Damage.** Only `levels[3]` decrements. `baseLevels` represents the player's max-HP-from-skill and only changes via skill advancement (gain XP → base level rises). Damage doesn't drain base.
- **`ResetHP` is Go-side abstraction; no TS counterpart.** Justified because we expect respawn / death / certain script triggers to need it, and a named method is cleaner than scattering `p.levels[3] = p.baseLevels[3]` at call sites. If it never gains a non-test caller, drop in a later sub-spec.

## Gotchas

- **`uint8` cast in Damage.** `levels` is `[21]uint8`. After `current - amount`, the result fits in uint8 only because of the floor-at-0 clamp. Order matters: clamp before cast.
- **`min()` is a Go 1.21+ builtin.** Project already uses it (S6d Damage). No import needed.
- **`processLogins` per-player loop ordering.** Place the HP seeding alongside other per-player init (after `addPlayer`, before LOGIN trigger). The LOGIN script can read `STAT(3)` and should see 10, not 0.
- **Player struct has `lastLevels [21]uint8` for stat-update diffing** (per `player.go:447-450`). The login seeding doesn't touch `lastLevels`, so the next `processStats` pass will see `levels[3]=10 != lastLevels[3]=0` and fire an UpdateStat wire op — the client correctly receives the initial Hitpoints value. No extra work needed.
- **Tests using `&Player{}` direct construction** (no `newPlayer`) start with all-zero arrays. They must seed `levels[3]` and `baseLevels[3]` explicitly before calling `Damage` if they want non-zero behavior.
