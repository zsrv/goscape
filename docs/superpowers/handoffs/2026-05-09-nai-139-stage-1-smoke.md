# NAI-139 Stage 1 — tutorial-completion cascade smoke

**Date:** 2026-05-09
**Spec:** `docs/superpowers/specs/2026-05-09-nai-139-tutorial-completion-cascade-design.md` @ `8182166`
**Audit:** `docs/superpowers/specs/2026-05-09-nai-139-stage-1-audit.md` @ `2d05bf0`
**Verdict commit:** `b8e547e` (Stage 1 audit-clean — 0 blockers)
**Routing path:** [A — audit-clean] per audit-verdict commit

## §1 PRIMARY criteria results (per spec §1:78-86)

User-driven smoke: fresh tutorial-stage account → completed Tutorial Island → Magic Instructor "Yes, go to mainland" → `[label,tutorial_complete]` executed → arrived at Lumbridge spawn (3222, 3222, level 0).

| # | Criterion (spec §1) | Result | Observation |
|---|--------------------|--------|-------------|
| 0 | Teleport regression check (NAI-111 sanity): Lumbridge spawn 3222, 3222, level 0 reached cleanly | **PASS** | `p_telejump` gate held; clean teleport. |
| 1 | Inventory: exactly 18 items in `tutorial.rs2:304-321` slot order with exact counts (bronze_axe×1 … bodyrune×2) | **PASS** | All 18 items present in declared order with declared counts. |
| 2 | Worn: all equipment slots empty | **PASS** | No equipped items. |
| 3 | Bank: exactly `coins×25` (Lumbridge → Varrock west bank or NodeDebug fallback) | **PASS** | Verified per user. |
| 4 | Stats: Hitpoints 10/1154 XP; trained skills (cook/fish/mine/smith/wood/fire/range/magic) at base ≤3; untrained at level 1 / 0 XP | **PASS** | `~stat_reset_all` resets temporary boosts to base, leaves XP — observed correctly. |
| 5 | Tabs: all 14 tabs clickable AND populated (not blank, not error) | **PASS** | All tabs render and are populated. **SECONDARY note** below on Quest Journal coloring. |
| 6 | Weapon-category UI: attack-tab reflects empty rhand (likely "Unarmed") | **PASS** | Reflects empty rhand correctly. |

**PRIMARY verdict:** **6/6 PASS** → NAI-139 cascade audit-clean **and** smoke-bound. Theory + observation both clean (rare path per spec §3:141).

## §2 SECONDARY observations (informational, non-blocking; per spec §1:87-89)

### S1 — Quest Journal tab colors diverge from canonical 225 client expectation

- **Symptom:** Quest names in the Quest Journal tab render in wrong colors:
  - Not-started quests appear **black** (expected: **red**).
  - In-progress quests appear **orange** (expected: **yellow**).
- **Why this is SECONDARY:** Spec §1 criterion 5 requires tabs to be "clickable AND populated (not blank, not error)" — both hold. Color choice is a content/client-rendering surface, not a §1 strict criterion.
- **Routing:** NAI-N+1 (open as NAI-140 when prioritized) per `smoke_surfaces_adjacent_divergences`. Out of scope for in-scope-stretch — this surface is `~initquestlist` / quest-state varbits + interface color attribute resolution, distinct from the cascade audit surface (handler dispatch + proc resolution for tutorial-completion ops).
- **Investigation seed for NAI-140:**
  - TS source: `LostCityRS/Content/scripts/login_logout/login.rs2` `~update_questlist` body + the underlying interface color resolution.
  - Audit lens: quest varbit→color mapping at the interface-attribute layer; whether the client's quest-state→color table is keyed off a varp/varbit goscape leaves at default.
  - Surface candidates: `pkg/script/handlers_interface.go` color-attribute writers; varbit emit for quest-state; `~update_questlist` proc reachability.

## §3 Server log

No `script not protected`, `proc not found`, opcode-dispatch warnings, panics, or stack traces reported during the cascade. No log excerpts to capture.

## §4 Closure

- NAI-139 closes as **PRIMARY met; cascade audit-clean + smoke-bound** per spec §3:141 and the verdict commit's routing path A.
- SECONDARY S1 (quest-color) routes to NAI-N+1 (NAI-140 candidate) — captured in the close commit body and in `nai_followups.md`.
- No Stage 2 fix required; no follow-up smoke needed at this level.
