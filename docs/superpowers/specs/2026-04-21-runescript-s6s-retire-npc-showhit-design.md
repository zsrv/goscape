# S6s — Retire `Npc.ShowHit` Design

> **Sub-spec context:** Twentieth runescript sub-spec. Tiny symmetry cleanup. `Player.ShowHit` was retired in S6e (replaced by `Player.Damage` + `Player.ResetHP`). `Npc.Damage` + `Npc.ResetHP` shipped but `Npc.ShowHit` remained as legacy API. This sub-spec deletes it.

> **TS-faithfulness gate:** Moves closer to TS. TS has `Npc.applyDamage` + `Npc.resetHP`; no `ShowHit` equivalent. **Zero new deviations.**

> **Scope:** Single-task, 2-file change.

## 1. Goal

Delete the legacy `(n *Npc) ShowHit(amount, dmgType, cur, base int)` method at `modules/world/npc_masks.go:16-22`. Update the one caller in `npc_test.go:64` to use `Damage(amount, dmgType)` instead.

## 2. Architecture

One-step sweep:

1. **Delete** `Npc.ShowHit` method body (`npc_masks.go:16-22`, 7 lines)
2. **Update** the single caller in `TestNpcResetMasksClearsEphemerals` (`npc_test.go:64`) from `n.ShowHit(10, 1, 40, 50)` to `n.Damage(10, 1)`

### 2.1 Why the test still passes

`TestNpcResetMasksClearsEphemerals` asserts:
- `n.masks != 0` before reset → `Damage(10, 1)` still sets `NpcMaskDamage` ✓
- `n.damageAmt != -1` after Reset clears it to `-1` — tautology
- `n.animID != 123` after ResetMasks — unaffected by our change

`Damage(10, 1)` with the test's default `curHP=0` computes `damageAmt = min(10, 0) = 0`. `0 != -1`, so the `ResetMasks` test assertion still holds. If we wanted to test specifically that `damageAmt` was `10` (as the old `ShowHit` path implied), we could seed `n.curHP = 50` before calling `Damage`. But since this test is about `ResetMasks` behavior, not damage semantics, the simpler form suffices.

## 3. File Map

| File | Action | Lines |
|---|---|---|
| `modules/world/npc_masks.go` | Modify | Delete 7-line `ShowHit` method (lines 16-22) |
| `modules/world/npc_test.go` | Modify | Line 64: `n.ShowHit(10, 1, 40, 50)` → `n.Damage(10, 1)` |

## 4. Deviations

**Zero new deviations.** No closures — this is pure API retirement matching TS.

## 5. Task Split

**Single task.** ~10 LOC net change (7-line delete + 1-line modify).

Commit: `refactor(world): retire Npc.ShowHit legacy API (S6s)`
