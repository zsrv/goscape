# NAI-120 — Combat-init path missing-handler enumeration (Stage 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce the audit-bound list of every missing opcode handler / var registration / opcode declaration along the combat-init chain reachable from `[opnpc2,_]` / `[apnpc2,_]` within `LostCityRS/Content/scripts/skill_combat/scripts/player/*.rs2` (8 inner-ring files), with TS-source signature for each missing handler. This Stage-1-only plan bounds enumeration; Stage 2 (per-file TDD ports) lives in a SEPARATE plan written after Bundle 1 audit binds.

**Architecture:** Investigation sub-spec, Stage-1-only plan per NAI-114 precedent. Bundle 0 (Tasks 1-6) is controller-driven static enumeration: grep tokens from rs2 sources, build a cross-reference matrix against `pkg/script/opcode.go` and `handlers*.go`, verify §9 risk-register entries at HEAD, commit findings note. Bundle 1 (Tasks 7-9) is one Sonnet Explore audit subagent dispatch + controller HEAD-verification + audit note commit + Stage 2 resume prompt. **No production code changes in this plan.**

**Tech Stack:** Go 1.26+; goscape `pkg/script` package; ripgrep for token extraction; Sonnet Explore audit subagent; read-only access to `LostCityRS/Engine-TS` (TS handlers) + `LostCityRS/Content/scripts/skill_combat/` (rs2 sources).

**Spec:** `docs/superpowers/specs/2026-05-07-nai-120-combat-init-path-investigation-design.md` (commit `99c71a9`).

**Predecessor:** NAI-119 close commit `a042d2b`; smoke residual #2 ("no handler for MAP_MULTIWAY (opcode 1014)" at pc=1 of `[proc,player_in_combat_check]` when attacking tutorial rats).

**Inner-ring files** (8 total, ~1195 source lines):

| File | Lines | Notable labels/procs |
|---|---|---|
| `LostCityRS/Content/scripts/skill_combat/scripts/player/player_combat.rs2` | 149 | entry labels `player_combat_start_ap`/`player_combat_start`/`player_combat_start_ap_nomulti`/`player_combat_start_nomulti`; `[proc,player_in_combat_check]`/`player_in_combat_check2`/`player_npc_hit_roll`/`player_attack_roll_specific`/`player_defence_roll_specific`/`.player_defence_roll_specific`/`player_attackrange` |
| `LostCityRS/Content/scripts/skill_combat/scripts/player/player_melee.rs2` | 59 | `[label,player_melee_attack]` |
| `LostCityRS/Content/scripts/skill_combat/scripts/player/player_ranged.rs2` | 144 | `[label,player_ranged_attack]` |
| `LostCityRS/Content/scripts/skill_combat/scripts/player/player_magic.rs2` | 280 | `[label,player_magic_attack]` |
| `LostCityRS/Content/scripts/skill_combat/scripts/player/auto_cast.rs2` | 60 | `[proc,player_autocast_enabled]` |
| `LostCityRS/Content/scripts/skill_combat/scripts/player/auto_retaliate.rs2` | 46 | (TBD per token extraction) |
| `LostCityRS/Content/scripts/skill_combat/scripts/player/player_attackstyles.rs2` | 201 | (TBD) |
| `LostCityRS/Content/scripts/skill_combat/scripts/player/player_combat_stat.rs2` | 256 | (TBD) |

---

## File Structure

| Path | Role |
|---|---|
| `docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md` | Bundle 0 deliverable: token extraction matrix, cross-reference table, var-registry findings, frontier list, §9 risk-register HEAD verification. Committed in Task 6. |
| `docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md` | Bundle 1 deliverable: per-(D)/(U)/(V)-entry TS-source audit report from Sonnet subagent + controller spot-check verdicts. Committed in Task 9. |

No production code files modified in this plan.

---

## Task 1: Token extraction from inner-ring `.rs2` files (controller-only)

**Why:** Bundle 0 §4.1 — produce the raw token universe. Every distinct intrinsic name / proc-call / label-jump / var read / constant must enter the cross-reference matrix.

**Files:**
- Read-only: 8 inner-ring `.rs2` files listed above.
- Scratch: `$TMPDIR/nai-120-tokens/` (deleted after Task 6).

**Step 1.1: Create scratch dir**

```bash
mkdir -p $TMPDIR/nai-120-tokens
```

Expected: silent success.

**Step 1.2: Extract function-call-shape tokens (intrinsics + proc calls)**

For each of the 8 files, run:

```bash
cd /home/owner/Code/github.com/LostCityRS/Content/scripts/skill_combat/scripts/player

for f in player_combat.rs2 player_melee.rs2 player_ranged.rs2 player_magic.rs2 auto_cast.rs2 auto_retaliate.rs2 player_attackstyles.rs2 player_combat_stat.rs2; do
  # Strip line comments, extract bare-word tokens followed by '('.
  sed 's,//.*$,,' "$f" \
    | rg -o '\b[a-z_][a-z_0-9]*\b\s*\(' \
    | sed 's,\s*(,,' \
    | sort -u > "$TMPDIR/nai-120-tokens/${f%.rs2}.calls"
done
```

Expected: 8 `.calls` files in `$TMPDIR/nai-120-tokens/`, each a sorted-unique list of call-shape tokens.

**Step 1.3: Extract var reads (`%name`)**

```bash
cd /home/owner/Code/github.com/LostCityRS/Content/scripts/skill_combat/scripts/player

for f in player_combat.rs2 player_melee.rs2 player_ranged.rs2 player_magic.rs2 auto_cast.rs2 auto_retaliate.rs2 player_attackstyles.rs2 player_combat_stat.rs2; do
  sed 's,//.*$,,' "$f" \
    | rg -o '%[a-z_][a-z_0-9]*' \
    | sort -u > "$TMPDIR/nai-120-tokens/${f%.rs2}.vars"
done
```

Expected: 8 `.vars` files.

**Step 1.4: Extract proc/label references (`~proc` / `@label`)**

```bash
cd /home/owner/Code/github.com/LostCityRS/Content/scripts/skill_combat/scripts/player

for f in player_combat.rs2 player_melee.rs2 player_ranged.rs2 player_magic.rs2 auto_cast.rs2 auto_retaliate.rs2 player_attackstyles.rs2 player_combat_stat.rs2; do
  sed 's,//.*$,,' "$f" \
    | rg -o '[~@][a-z_][a-z_0-9.]*' \
    | sort -u > "$TMPDIR/nai-120-tokens/${f%.rs2}.refs"
done
```

Expected: 8 `.refs` files.

**Step 1.5: Extract constants (`^name`)**

```bash
cd /home/owner/Code/github.com/LostCityRS/Content/scripts/skill_combat/scripts/player

for f in player_combat.rs2 player_melee.rs2 player_ranged.rs2 player_magic.rs2 auto_cast.rs2 auto_retaliate.rs2 player_attackstyles.rs2 player_combat_stat.rs2; do
  sed 's,//.*$,,' "$f" \
    | rg -o '\^[a-z_][a-z_0-9]*' \
    | sort -u > "$TMPDIR/nai-120-tokens/${f%.rs2}.consts"
done
```

Expected: 8 `.consts` files.

**Step 1.6: Build union token sets across files**

```bash
cd $TMPDIR/nai-120-tokens
cat *.calls  | sort -u > all.calls
cat *.vars   | sort -u > all.vars
cat *.refs   | sort -u > all.refs
cat *.consts | sort -u > all.consts
wc -l all.calls all.vars all.refs all.consts
```

Expected: four numeric counts. Anticipated rough magnitudes: calls ≈ 50-100 unique, vars ≈ 30-60, refs ≈ 20-40, consts ≈ 20-50.

**Step 1.7: Sanity scan — manually skim each `all.*` file for surprises**

```bash
cat $TMPDIR/nai-120-tokens/all.calls
cat $TMPDIR/nai-120-tokens/all.vars
cat $TMPDIR/nai-120-tokens/all.refs
cat $TMPDIR/nai-120-tokens/all.consts
```

Expected: every entry looks like a reasonable rs2 token. No false positives from string-literal slop, no truncated tokens. If something looks off (e.g., a partial word, an embedded operator), refine the regex and re-run.

**No commit at Task 1.** Output is scratch; consumed by Tasks 2-4.

---

## Task 2: Cross-reference matrix construction (controller-only)

**Why:** Bundle 0 §4.2 — every (call) token must be classified W / D / U / F against `pkg/script/opcode.go` and `handlers*.go`.

**Files:**
- Read-only: `pkg/script/opcode.go`, `pkg/script/handlers.go`, `pkg/script/handlers_*.go`.
- Output: prose appendix in scratch (`$TMPDIR/nai-120-tokens/matrix.md`); finalized in Task 6.

**Step 2.1: Build the call-token → goscape-Op mapping**

For each entry in `$TMPDIR/nai-120-tokens/all.calls`, determine the corresponding goscape opcode constant. Authoritative TS mapping at `Engine-TS/src/engine/script/ScriptOpcode.ts` (uppercase-name → numeric ID). Goscape mirrors via `pkg/script/opcode.go` constants (`OpFoo Opcode = N`).

Convention: rs2 token `foo` → TS `ScriptOpcode.FOO` → goscape `OpFoo`. Exceptions are rare; verify by grep.

```bash
cd /home/owner/Code/github.com/zsrv/goscape

# For each token, produce a one-line classification.
while IFS= read -r tok; do
  upper=$(printf '%s' "$tok" | tr '[:lower:]' '[:upper:]')
  # Strip leading uppercase + assemble PascalCase candidate.
  pascal=$(printf '%s' "$tok" | awk -F_ '{for(i=1;i<=NF;i++) printf "%s%s",toupper(substr($i,1,1)),substr($i,2)}')
  decl=$(rg -n "Op${pascal}\s+Opcode\s*=" pkg/script/opcode.go | head -1)
  disp=$(rg -n "Op${pascal}:" pkg/script/handlers.go pkg/script/handlers_*.go | head -1)
  if [ -n "$decl" ] && [ -n "$disp" ]; then status=W
  elif [ -n "$decl" ]; then status=D
  else status=U
  fi
  printf '%-30s %-5s decl=%s  disp=%s\n' "$tok" "$status" "${decl:-NONE}" "${disp:-NONE}"
done < $TMPDIR/nai-120-tokens/all.calls > $TMPDIR/nai-120-tokens/matrix.calls
cat $TMPDIR/nai-120-tokens/matrix.calls
```

Expected: one line per call-token with W / D / U classification + opcode.go decl line + handlers.go dispatch line.

**Step 2.2: Manual reconciliation pass**

For each row whose status ≠ W, re-verify by reading `opcode.go` and `handlers*.go` directly — the regex above can miss e.g. tokens whose goscape PascalCase name differs from the auto-derivation (e.g., `npc_uid` → `OpNpcUid`, but `inv_getobj` might be `OpInvGetObj` or `OpInventoryGetObj` — verify per case).

For each (D) or (U) row, also grep `Engine-TS/src/engine/script/ScriptOpcode.ts` to confirm the TS opcode name + numeric ID:

```bash
rg "<UPPER_NAME>" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/ScriptOpcode.ts
```

If the rg auto-derivation missed a wired opcode (status appears (D) or (U) but is actually wired under a different name), correct the row to (W) and note the actual goscape-side name.

**Step 2.3: Separate proc-references from intrinsic calls**

`all.calls` includes `~proc` calls treated as bare-word `proc` (the `~` was stripped by the call-shape regex — confirm). Cross-reference each surviving (D)/(U) entry against `all.refs` to identify:

- Pure intrinsics (token is a goscape opcode name): real (D)/(U) candidates.
- Proc/label calls (token corresponds to a `[proc,name]` or `[label,name]` definition somewhere in `LostCityRS/Content/`): these are NOT goscape opcodes; they're script-id targets reached via GOSUB_WITH_PARAMS / JUMP_WITH_PARAMS. Mark as **(P)** — proc/label reference, not opcode-missing.

For each (P) candidate, grep `LostCityRS/Content/scripts/` for the body:

```bash
rg -l "\[proc,<name>\]|\[label,<name>\]" /home/owner/Code/github.com/LostCityRS/Content/scripts/
```

If body lives in inner ring → mark (P, in-ring) — covered transitively by Stage 2 reachability.
If body lives outside inner ring → mark **(F)** Frontier — out of NAI-120; record in frontier list.

**Step 2.4: Append matrix to `$TMPDIR/nai-120-tokens/matrix.md`**

Final matrix structure (markdown table):

```markdown
## Call-token classification

| Token | Status | Goscape Op | opcode.go | handlers.go | TS ScriptOpcode | Notes |
|---|---|---|---|---|---|---|
| map_multiway | D | OpMapMultiway | opcode.go:88 | NONE | MAP_MULTIWAY=1014 | confirmed missing per smoke |
| add | W | OpAdd | opcode.go:434 | handlers.go:27 | ADD=4600 | |
| ... | ... | ... | ... | ... | ... | ... |
```

Save to `$TMPDIR/nai-120-tokens/matrix.md`. This file becomes a section of the Task 6 commit deliverable.

**No commit at Task 2.**

---

## Task 3: Var-registry path discovery + per-var presence check (controller-only)

**Why:** Bundle 0 §4.3 — `%name` reads need both (a) the PUSH_VAR family opcode wired AND (b) the named var registered in goscape's var registry. Per `mock_recorder_field_naming_check`, controller must NOT convention-infer the registry path; grep-verify before classifying.

**Files:**
- Read-only: `pkg/script/`, `pkg/world/`, `pkg/configs/`, plus any package whose name suggests vars.
- Scratch: `$TMPDIR/nai-120-tokens/vars.md`.

**Step 3.1: Discover goscape's var-registry path**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
rg -l "VarPType|VarSType|VarNType|varp|varn|VarPlayer|VarServer|VarNpc" pkg/ modules/ cmd/ | head -20
```

Expected: a small set of files (likely under `pkg/configs/` or `pkg/script/configs.go`). Read each candidate to identify the canonical loader + lookup function.

**Step 3.2: Re-verify PUSH_VAR* / POP_VAR* / VARBIT-family dispatch at HEAD**

```bash
rg -n "OpPushVarp|OpPopVarp|OpPushVarn|OpPopVarn|OpPushVars|OpPopVars|OpPushVarbit|OpPopVarbit" pkg/script/handlers.go pkg/script/handlers_*.go
rg -n "handlePushVarn|handlePopVarn" pkg/script/handlers_vars.go
```

Expected at HEAD `a042d2b`:
- `OpPushVarp / OpPopVarp / OpPushVars / OpPopVars` — fully wired.
- `OpPushVarn / OpPopVarn` — wired but flagged "stub until S6" at `pkg/script/handlers.go:207-208`. Read the actual handler bodies; classify whether stub semantics suffice for inner-ring NPC-stat reads (e.g., does the stub return zero/null in a TS-faithful manner, or does it error?).
- `OpPushVarbit / OpPopVarbit` — Bundle 0 task confirms.

If PUSH_VARN's stub blocks reads, escalate to (D) candidate — needs real impl in Stage 2 if any inner-ring `%npc_*` var is read.

**Step 3.3: Per-var classification pass**

For each entry in `$TMPDIR/nai-120-tokens/all.vars` (e.g., `%lastcombat`, `%npc_aggressive_player`):

1. Determine var type by name prefix convention + TS ground truth:
   - rs2 vars don't carry an explicit type prefix; ground truth lives in `Engine-TS/src/lostcity/`. Grep TS:
     ```bash
     rg -n "VarPType.*<name>|VarSType.*<name>|VarNType.*<name>" /home/owner/Code/github.com/LostCityRS/Engine-TS/data/ /home/owner/Code/github.com/LostCityRS/Engine-TS/src/
     ```
     Or search `Engine-TS` data files for the var declaration.
2. Grep the goscape registry (path discovered in Step 3.1) for the var name:
   ```bash
   rg -n "<varname>" <registry-paths>
   ```
3. Classify:
   - **(V-W)** Var registered in goscape with matching type → wired (assuming PUSH_VAR* opcode wired).
   - **(V-D)** Var declared in TS but not in goscape registry → port required (small task, registry-only).
   - **(V-U)** Var not even declared in TS at the path checked → escalate; may indicate a deeper config-pack-loading gap.

**Step 3.4: Append var classification to `$TMPDIR/nai-120-tokens/vars.md`**

```markdown
## Var classification

| Var | Type | TS source (file:line) | Goscape registry (file:line) | Status |
|---|---|---|---|---|
| %lastcombat | VarPType | <ts-path>:N | <gs-path>:N or NONE | V-W or V-D |
| ... | ... | ... | ... | ... |
```

**No commit at Task 3.**

---

## Task 4: Frontier resolution + §9 risk-register HEAD verification (controller-only)

**Why:** Bundle 0 §4.4 + §9. Confirm every (F) candidate's body location, and finish HEAD-verifying §9 risk-register entries deferred at spec-write (R3 PUSH_VARBIT, R4 enum constants, R5 frontier confirms, R6 p_aprange, R8 npc_uid/uid/mes).

**Step 4.1: Resolve every (P) entry's body location**

For each (P) row from Task 2.3, grep:

```bash
NAME=<proc-or-label-name>
rg -l "\[proc,${NAME}\]|\[label,${NAME}\]" /home/owner/Code/github.com/LostCityRS/Content/scripts/
```

If hit ∈ `LostCityRS/Content/scripts/skill_combat/scripts/player/*.rs2` → (P, in-ring), covered transitively.
Else → upgrade to **(F) Frontier**, record file path. Frontier list goes into the Task 6 commit.

**Step 4.2: §9 R6 — verify `p_aprange`**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
rg -n "OpPAprange|OpPAprange|p_aprange|PAprange|PARange" pkg/script/opcode.go pkg/script/handlers.go pkg/script/handlers_*.go
```

Expected: a declared opcode + dispatch entry. If missing, record as a confirmed (D)/(U) — a known Stage 2 port. (Note: `p_aprange` is rs2-name; goscape PascalCase candidate `OpPAprange` per convention.)

**Step 4.3: §9 R8 — verify npc_uid / uid / mes**

```bash
rg -n "OpNpcUid|OpUid|OpMes\b" pkg/script/opcode.go pkg/script/handlers.go pkg/script/handlers_*.go
```

Expected: all three declared + dispatched. Record line numbers in `$TMPDIR/nai-120-tokens/risk.md`.

**Step 4.4: §9 R3 — verify PUSH_VARBIT / POP_VARBIT dispatch**

```bash
rg -n "OpPushVarbit:|OpPopVarbit:" pkg/script/handlers.go pkg/script/handlers_*.go
```

If dispatched → wired; record line. If declared (`opcode.go:52-53`) but no dispatch → (D), Stage 2 port required IF any inner-ring var is varbit-typed (Task 3 already classifies this).

**Step 4.5: §9 R4 — enum/inv-pack constants spot-check**

For at least one constant from each family found in `all.consts` (e.g., `^stab_style`, `^wearpos_rhand`, `^style_ranged_longrange`):

```bash
NAME=<const-name>
rg -n "${NAME}" /home/owner/Code/github.com/zsrv/goscape/pkg/configs/ /home/owner/Code/github.com/zsrv/goscape/pkg/objtype/ /home/owner/Code/github.com/zsrv/goscape/data/pack/
```

Goal: confirm the constant resolves at content-pack-load time. If a constant is referenced in rs2 but its enum/inv pack isn't loaded by goscape's bootstrap, that's a separate (and serious) gap — record as **(C-MISSING)** and route to NAI-121+.

**Step 4.6: Append risk-register results to `$TMPDIR/nai-120-tokens/risk.md`**

Markdown table:

```markdown
## §9 risk register — Bundle 0 final HEAD verification

| Item | Status at HEAD a042d2b | Evidence |
|---|---|---|
| R1 ADD wired | ✅ | opcode.go:434 + handlers.go:27 (verified at spec-write) |
| R2 BRANCH_* family wired | ✅ | opcode.go:39-43,55-56 + handlers.go:21-23 (representative) |
| R3 PUSH_VARP/S/N wired | ✅ (PUSH_VARN flagged stub-until-S6) | handlers.go:203-208; stub semantics: <verdict from Step 3.2> |
| R3 PUSH_VARBIT wired? | ✅/⚠ <fill> | opcode.go:52-53; dispatch: <fill> |
| R4 enum/inv pack constants | ✅/⚠ <fill> | <evidence per Step 4.5> |
| R5 frontier resolutions | ✅ | <count> (P) entries resolved; <count> upgraded to (F) |
| R6 p_aprange wired | ✅/⚠ <fill> | <evidence per Step 4.2> |
| R7 Gosub/Jump wired | ✅ | handlers.go:30,83,346-347 (verified at spec-write) |
| R8 NpcCoord/MapClock/NpcUid/Mes wired | ✅/⚠ <fill> | <evidence per Step 4.3> |
```

**No commit at Task 4.**

---

## Task 5: Assemble Bundle 0 deliverable note (controller-only)

**Why:** Consolidate Tasks 1-4 outputs into one investigation note for commit. Per the spec's §4.5 deliverable criteria.

**Files:**
- Create: `docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md`

**Step 5.1: Assemble the investigation note**

Structure (template — fill from `$TMPDIR/nai-120-tokens/`):

```markdown
# NAI-120 — Bundle 0 controller pre-flight findings

**Date:** 2026-05-07
**Spec:** `docs/superpowers/specs/2026-05-07-nai-120-combat-init-path-investigation-design.md` (commit `99c71a9`)
**HEAD at pre-flight:** `a042d2b` (NAI-119 close)

This is the Bundle 0 deliverable per spec §4.5. Bundle 1 (audit subagent dispatch) consumes it.

## 1. Token universe (Task 1)

- `all.calls`: <N> unique tokens.
- `all.vars`:  <N> unique tokens.
- `all.refs`:  <N> unique tokens.
- `all.consts`: <N> unique tokens.

Per-file extraction tables: <inline summary or list-of-files>.

## 2. Call-token classification (Task 2)

<paste $TMPDIR/nai-120-tokens/matrix.md>

### Summary

- (W) Wired:        <count>
- (D) Declared-only: <count>  ← Stage 2 dispatch entry only
- (U) Undeclared:   <count>  ← Stage 2 declare + handler + dispatch
- (P, in-ring):     <count>  ← covered transitively
- (F) Frontier:     <count>  ← out of NAI-120; routes to NAI-121+

### Frontier list

| Proc/label | Body file | Routing |
|---|---|---|
| <name> | <path> | NAI-121 candidate / parked |

## 3. Var classification (Task 3)

<paste $TMPDIR/nai-120-tokens/vars.md>

### Goscape var-registry path discovered

- <file:line> for VarPType registry
- <file:line> for VarSType registry
- <file:line> for VarNType registry

### PUSH_VARN stub semantics

<verdict from Task 3.2: does the stub suffice for inner-ring NPC-stat reads, or does it need real impl?>

## 4. §9 risk register — final HEAD verification (Task 4)

<paste $TMPDIR/nai-120-tokens/risk.md>

## 5. Bundle 1 audit input

The (D)/(U)/(V-D)/(V-U) subset of §2 + §3 is the audit input. Bundle 1's subagent receives:
- This findings note (pinned at the commit hash from Task 6)
- The 8 inner-ring `.rs2` files (read-only)
- `Engine-TS/src/engine/script/handlers/` (read-only)
- The goscape var-registry path discovered in §3

Bundle 1 produces per-entry TS-source signature audit at `docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md`.

## 6. Stage 2 bundle assignment hypothesis (refines after Bundle 1)

Per spec §6.1, anticipated decomposition (subject to Bundle 1's dependency edges):

- Bundle 2A — `player_combat.rs2` missing handlers
- Bundle 2B — `player_melee.rs2` missing handlers
- Bundle 2C — `player_ranged.rs2` missing handlers
- Bundle 2D — `player_magic.rs2` missing handlers
- Bundle 2E — small-files merge (`auto_cast`, `auto_retaliate`, `player_attackstyles`, `player_combat_stat`)

Total expected production LOC: 200-800 + comparable test LOC. Multi-session.
```

**Step 5.2: Sanity-read the assembled note**

Verify:
- Every (D)/(U) entry has both opcode.go and handlers.go evidence.
- Every (F) entry has a body path.
- Every (V-D)/(V-U) entry has TS source citation.
- No "TBD" / "TODO" markers in the actual content; only template placeholders that have been filled.

**No commit yet** (Task 6 commits).

---

## Task 6: Commit Bundle 0 findings + clean up scratch (controller-only)

**Files:**
- Create-and-commit: `docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md`
- Delete: `$TMPDIR/nai-120-tokens/`

**Step 6.1: Stage and commit**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
git add docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md

git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-120 — Bundle 0 controller pre-flight findings

Static enumeration of every distinct opcode / var / proc-call /
label-jump / constant referenced by the 8 inner-ring combat-init
.rs2 files (skill_combat/scripts/player/, ~1195 lines). Cross-
referenced against pkg/script/opcode.go + handlers*.go at HEAD
a042d2b; classified W/D/U/P-in-ring/F per spec §4.2.

Includes:
- Per-file token extraction tables.
- Cross-reference matrix (call-token classification).
- Var classification with goscape registry path discovery.
- §9 risk-register final HEAD verification.
- Frontier list for NAI-121+ routing.
- Stage 2 bundle assignment hypothesis (refines after Bundle 1).

Bundle 1 audit subagent consumes this note as input; per-entry
TS-source signatures land at
docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md
in the next task.

Spec: docs/superpowers/specs/2026-05-07-nai-120-combat-init-path-investigation-design.md (commit 99c71a9).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Step 6.2: Verify commit landed**

```bash
git log -1 --stat
```

Expected: one new file under `docs/superpowers/investigations/`, no other changes.

**Step 6.3: Clean up scratch**

```bash
rm -rf $TMPDIR/nai-120-tokens/
```

Expected: silent success.

---

## Task 7: Bundle 1 audit subagent dispatch (controller dispatches one Sonnet Explore subagent)

**Why:** Spec §5. Independent verification of Bundle 0's (D)/(U)/(V-D)/(V-U) entries against TS source. Per `audit_subagent_fabrication`, controller must dispatch but not trust blindly.

**Step 7.1: Compose the subagent prompt**

The prompt must be self-contained (subagent has no conversation history). Template:

```
You are auditing the missing-handler list for NAI-120 (combat-init
path port). Read this file as your input:

  /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md

For EACH (D), (U), (V-D), (V-U) entry in the matrix, produce a
per-entry markdown stanza with these fields:

1. **Token** + classification.
2. **TS impl location:** exact file:line range in
   /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/
   (or the var-data path for V-* entries).
3. **TS impl verbatim:** copy the full handler body or var
   declaration, no summarization.
4. **Pop/push signature:** what types it consumes/produces (int /
   string / coord / etc.).
5. **Side effects:** any state mutations (player/npc/world fields,
   timers, animations, world events).
6. **Goscape sibling pattern:** identify the closest already-wired
   handler in pkg/script/handlers*.go that shares structure (e.g.,
   for MAP_MULTIWAY, the sibling pattern is MAP_BLOCKED at
   ServerOps.ts:N → handlers_server.go:N).
7. **Edge cases:** null handling, OOB, nullity sentinels (-1 vs 0
   vs nil) — TS-faithful or known-divergent?
8. **Test-case skeletons:** 3-5 unit-test bullet points (input
   state → expected push/pop/side-effect).

Output to:

  /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md

Constraints:
- Read-only against TS source. Do not write any code outside the
  audit note.
- Cite TS source verbatim with file:line. Do NOT summarize TS
  behavior — paste the actual TS body.
- For each entry, also identify dependency edges: "handler X needs
  ScriptState field Y, port Y first" or "handler X reads NPC field
  Z, plumb Z first".
- After all entries, append a "Stage 2 bundle ordering" section
  listing the dependency edges and recommending sequential dispatch
  order.
- If Bundle 0's classification is wrong (e.g., a (D) entry is
  actually (W) under a different goscape name you discovered),
  flag it explicitly in the entry.

Subagent constraints:
- Sonnet model (per superpowers_code_reviewer_model cap).
- Explore agent type — read-only, no Edit/Write/NotebookEdit.
- Cite verbatim; do not paraphrase TS bodies.
```

**Step 7.2: Dispatch via Agent tool**

```
subagent_type: Explore
model: sonnet
description: NAI-120 Bundle 1 audit
prompt: <Step 7.1 template>
```

**Step 7.3: Wait for subagent completion**

The subagent writes the audit note directly. When it returns, verify the file exists:

```bash
ls -la /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md
```

Expected: file exists, non-empty.

**No commit at Task 7.** (The audit note is uncommitted; Task 9 commits after spot-check.)

---

## Task 8: Controller spot-check audit verdicts (controller-only)

**Why:** Per `audit_subagent_fabrication` (NAI-31 near-miss precedent), audit subagents can return confident-but-wrong diagnoses. Controller must independently verify before trusting Stage 2 dispatch.

**Step 8.1: Pick 3 audit entries to spot-check**

Select:
- One (D) entry whose TS impl is the simplest (e.g., MAP_MULTIWAY).
- One (D) or (V-D) entry with non-trivial side effects (e.g., a per-NPC-stat reader).
- One entry whose dependency-edge claim is non-obvious (e.g., "handler X needs new ScriptState field Y").

**Step 8.2: For each picked entry**

1. Open the cited TS file:line directly via Read.
2. Confirm the verbatim citation matches actual file contents (catches paraphrasing-passed-as-quote).
3. Confirm the pop/push signature matches the actual TS code.
4. Confirm the dependency-edge claim by grepping goscape's `pkg/script/state.go` (or equivalent) for the named field — exists or not.

**Step 8.3: If any spot-check fails**

If the audit fabricates a citation or mis-frames a signature:
- Append a "Controller spot-check addendum" section to the audit note documenting the discrepancy.
- Re-grep TS source independently for the disputed entry; provide controller's authoritative version.
- Flag whether the entry is salvageable (replace with controller verdict) or whether Bundle 1 needs re-dispatch.

**Step 8.4: Frontier sanity**

Independently grep one of the (F) entries against `LostCityRS/Content/scripts/` to confirm Bundle 0's frontier classification is correct (defends against a rs2 file-rename or a typo'd grep).

**No commit at Task 8.**

---

## Task 9: Commit Bundle 1 audit + emit Stage 2 resume prompt (controller-only)

**Files:**
- Commit: `docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md`

**Step 9.1: Stage and commit**

```bash
cd /home/owner/Code/github.com/zsrv/goscape
git add docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md

git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-120 — Bundle 1 Stage 1 audit verdict

Sonnet Explore subagent audited every (D)/(U)/(V-D)/(V-U) entry
from Bundle 0 against Engine-TS handler source. Per-entry stanza:
TS file:line, verbatim TS impl, pop/push signature, side effects,
goscape sibling pattern, edge cases, test-case skeletons.

Includes "Stage 2 bundle ordering" section with dependency edges
across the per-file Stage 2 bundles.

Controller spot-checked <N> verdicts independently against TS
source per audit_subagent_fabrication; <addendum-note-or-clean>.

Spec: docs/superpowers/specs/2026-05-07-nai-120-combat-init-path-investigation-design.md (commit 99c71a9).
Bundle 0: <commit-from-task-6>.

Stage 2 plan to be written separately per investigation_subspec_cadence
(NAI-114 precedent).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**Step 9.2: Verify commit**

```bash
git log -2 --oneline
```

Expected: two new investigation commits at top of log.

**Step 9.3: Emit Stage 2 resume prompt**

Display to user the paste-ready resume prompt for the next session (Stage 2 plan-author session). Do NOT invoke writing-plans for Stage 2 in this session — let the user `/clear` first per `superpowers_clear_between_spec_and_impl`.

Resume prompt template (substitute `<bundle1-commit>` after Step 9.1):

```
NAI-120 Stage 1 closed. Bundle 0 findings: docs/superpowers/investigations/2026-05-07-nai-120-bundle0-findings.md (commit <bundle0-commit>). Bundle 1 audit: docs/superpowers/investigations/2026-05-07-nai-120-bundle1-audit.md (commit <bundle1-commit>).

Audit binds <N> missing handlers across <M> Stage 2 bundles (see Bundle 1 §"Stage 2 bundle ordering"). Dependency edges: <one-line summary>.

Next: write Stage 2 plan covering Bundles 2A..2E. Spec is unchanged at docs/superpowers/specs/2026-05-07-nai-120-combat-init-path-investigation-design.md (commit 99c71a9) — §6 specifies cadence, §6.4 specifies test seeds. Use writing-plans skill. Per-bundle TDD shape: T1 RED (per-handler unit tests with ScriptState fixtures, scriptstate_test_fixture_idioms applied) → T2 GREEN (handler + dispatch) → T3 verify (controller fresh `go test ./...`) → T4 review (Sonnet code-reviewer).

Bundle 1 surfaced <K> frontier items (see Bundle 0 §2 "Frontier list" + §6) — record in nai_followups.md at NAI-120 final close, not during Stage 2.

Apply: controller_preflight (re-verify each plan task premise pre-dispatch), verify_implementer_claims (T3 30s protocol), plan_runnable_test_fixtures (mentally execute every fixture), risk_register_premise_grep (any "X already exists" claim in spec/audit/Bundle 0 that Stage 2 leans on must be re-grepped at HEAD before plan-author commits).
```

**Step 9.4: Stop**

Per `superpowers_clear_between_spec_and_impl`: do NOT proceed to Stage 2 planning in this session. End the turn after emitting the resume prompt.

---

## Self-review notes (controller-only, pre-Task-1)

Before Task 1 dispatch, controller verifies:

- **Spec coverage:** every Bundle 0 §4 sub-section maps to a Task (4.1→Task 1, 4.2→Task 2, 4.3→Task 3, 4.4→Task 4.1, 4.5→Task 5/6). Every Bundle 1 §5 deliverable maps (audit prompt at Task 7, spot-check at Task 8, commit at Task 9). §9 risk register R3-R8 deferred items map to Task 4. ✅
- **Placeholder scan:** matrix template uses `<fill>` markers but only inside the in-progress `risk.md` table that gets filled during Task 4; final committed note has no placeholders. ✅
- **Type consistency:** classification tags (W / D / U / P / F / V-W / V-D / V-U / C-MISSING) used consistently across Tasks 2, 3, 4, 5. ✅
