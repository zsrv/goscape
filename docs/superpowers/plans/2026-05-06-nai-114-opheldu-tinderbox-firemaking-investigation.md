# NAI-114 — OPHELDU tinderbox-on-logs no-effect investigation (Stage 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bind H3 (downstream opcode silent-abort) to a single divergent opcode or class for the firemaking dispatch chain `[opheldu,tinderbox]` → `[label,light_logs_inv]` → loc/xp helpers, producing the audit-bound Stage 1.2 deliverable that Stage 2 will fix.

**Architecture:** Investigation sub-spec, Stage-1-only plan. Stage 1.1 is controller-only static disasm extension (probe restoration → switch-table dump + linked-script disasm + wire-order pin → investigation note commit). Stage 1.2 is Sonnet audit subagent dispatch with controller HEAD-verification. Stage 2 fix lives in a SEPARATE plan written after Stage 1.2 binding (per NAI-112 precedent). No production code changes in this plan.

**Tech Stack:** Go 1.26+; goscape `pkg/script` + `pkg/objtype` packages; Sonnet audit subagent; read-only access to `LostCityRS/Engine-TS`, `LostCityRS/Server`, `LostCityRS/Client-Java` rev-225.

**Spec:** `docs/superpowers/specs/2026-05-06-nai-114-opheldu-tinderbox-firemaking-investigation-design.md` (commit `f6e2a49`).

**Bundle 0 findings (already in spec, summarized):**
- `[opheldu,tinderbox]` IS registered (key=0x93a91, sourced from `LostCityRS/Server/.../firemaking.rs2`).
- `[opheldu,logs]` NOT registered. Fine — script keyed by tinderbox.
- Logs: id=1511, Category=212, Params={86: 1, 132: 400}.
- Script PC 18 SWITCH on `OC_CATEGORY(LAST_USEITEM)=212`; expected case-212 maps to PC 20 → `JUMP_WITH_PARAMS 7356` ([light_logs_inv]).

---

## File Structure

| Path | Role |
|---|---|
| `cmd/probe-opheldu/main.go` | Temporary disasm probe. Created in Task 1, deleted in Task 5. Never committed. |
| `docs/superpowers/investigations/2026-05-06-nai-114-stage1-bundle0-findings.md` | Stage 1.1 investigation note. Committed in Task 5. |
| `docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md` | Stage 1.2 subagent audit report. Committed in Task 9. |

No production code files modified in this plan.

---

## Task 1: Restore probe at `cmd/probe-opheldu/main.go`

**Why:** Bundle 0 deleted the probe; Stage 1.1 needs it back, extended for switch-table dump.

**Files:**
- Create: `cmd/probe-opheldu/main.go`

**Step 1.1: Write the probe**

```go
// Stage 1.1 disasm probe for NAI-114. Standalone; deleted after Task 5.
// Recreates Bundle 0 enumeration + adds switch-table dump and full
// disasm of linked scripts (7942, 7359, 7357, 2120, 6460, 7904).
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

func main() {
	cacheRoot := "data/pack"
	if len(os.Args) > 1 {
		cacheRoot = os.Args[1]
	}

	prov := script.NewProvider()
	if err := prov.Load(cacheRoot + "/server"); err != nil {
		fmt.Fprintln(os.Stderr, "script load:", err)
		os.Exit(1)
	}

	params, err := objtype.LoadParamTypes(cacheRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "params load:", err)
		os.Exit(1)
	}
	otc, err := objtype.LoadObjTypes(cacheRoot, params)
	if err != nil {
		fmt.Fprintln(os.Stderr, "obj load:", err)
		os.Exit(1)
	}

	const trigger = script.TriggerOpHeldU // 145
	fmt.Printf("== TriggerOpHeldU = %d ==\n", trigger)
	fmt.Printf("Loaded scripts: %d   ObjTypes: %d\n\n", prov.Count(), len(otc.Configs))

	// Pass 1: dump [opheldu,tinderbox] disasm.
	const tinderName = "[opheldu,tinderbox]"
	tinder := prov.GetByName(tinderName)
	if tinder == nil {
		fmt.Fprintf(os.Stderr, "[opheldu,tinderbox] not loaded — abort\n")
		os.Exit(1)
	}
	fmt.Println("---- DISASM [opheldu,tinderbox] ----")
	fmt.Println(script.Disassemble(tinder))

	// Pass 2: dump SWITCH operand tables (script-level, indexed by switch id).
	fmt.Println("---- SWITCH TABLES for [opheldu,tinderbox] ----")
	for switchID, table := range tinder.SwitchTables {
		fmt.Printf("switch[%d] (%d cases):\n", switchID, len(table))
		// Sort by case key for deterministic output.
		keys := make([]int, 0, len(table))
		for k := range table {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		for _, k := range keys {
			fmt.Printf("  case %5d → PC offset %d\n", k, table[int32(k)])
		}
	}

	// Pass 3: disasm linked scripts by ID.
	fmt.Println()
	for _, id := range []uint32{6460, 7904, 7356, 7360, 7359, 7357, 7942, 2120, 2130} {
		sf := prov.GetByID(id)
		if sf == nil {
			fmt.Printf("script id %d: NOT loaded\n", id)
			continue
		}
		fmt.Printf("---- DISASM script id %d (%s) ----\n", id, sf.Name)
		fmt.Println(script.Disassemble(sf))
		fmt.Println()
	}

	// Pass 4: cache state for logs / newbielogs / tinderbox.
	for _, name := range []string{"logs", "newbielogs", "tinderbox"} {
		id, ok := otc.ConfigNames[name]
		if !ok {
			fmt.Printf("%s: NOT in ConfigNames\n", name)
			continue
		}
		ot := otc.Configs[id]
		fmt.Printf("%s id=%d Category=%d Members=%v\n", name, id, ot.Category, ot.Members)
		if ot.Params != nil {
			pkeys := make([]int, 0, len(ot.Params))
			for k := range ot.Params {
				pkeys = append(pkeys, int(k))
			}
			sort.Ints(pkeys)
			for _, k := range pkeys {
				fmt.Printf("  param[%d] = %v\n", k, ot.Params[int32(k)])
			}
		}
	}

	// Pass 5: enumerate every distinct opcode appearing in the captured chain.
	fmt.Println()
	fmt.Println("---- OPCODE INVENTORY (chain-wide) ----")
	opSet := map[script.Op]bool{}
	for _, sf := range []*script.ScriptFile{
		tinder,
		prov.GetByID(6460), prov.GetByID(7904), prov.GetByID(7356),
		prov.GetByID(7360), prov.GetByID(7359), prov.GetByID(7357),
		prov.GetByID(7942), prov.GetByID(2120), prov.GetByID(2130),
	} {
		if sf == nil {
			continue
		}
		for _, op := range sf.Opcodes {
			opSet[op] = true
		}
	}
	opNames := make([]string, 0, len(opSet))
	for op := range opSet {
		name := op.String()
		if strings.HasPrefix(name, "Op(") {
			name = "*** UNKNOWN " + name + " ***"
		}
		opNames = append(opNames, name)
	}
	sort.Strings(opNames)
	for _, n := range opNames {
		fmt.Println("  " + n)
	}
}
```

- [ ] **Step 1.1: Create the probe file** (write the code above to `cmd/probe-opheldu/main.go`).

- [ ] **Step 1.2: Verify it builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./cmd/probe-opheldu`

Expected: build succeeds (no output, exit 0).

**Verified at plan-write time:** `pkg/script/file.go:11,34` — `type SwitchTable map[int32]int32` and `ScriptFile.SwitchTables []SwitchTable` (slice indexed by switch-ID; switch-ID is the operand of the `SWITCH` opcode at PC 18).

**Step 1.3 (optional adjustment):** if `ot.Params` is `ParamMap` with a different key type than `int32`, adjust the type assertions. `rg -n "type ParamMap" pkg/objtype/` to confirm.

---

## Task 2: Run probe and capture findings

**Files:**
- No file changes (output captured to a transient file then transcribed into the investigation note).

- [ ] **Step 2.1: Run the probe and capture output**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/probe-opheldu data/pack > /tmp/claude/nai114-probe.out 2>&1
```

Expected: exit 0; `/tmp/claude/nai114-probe.out` populated.

- [ ] **Step 2.2: Read the captured output and extract**

Read `/tmp/claude/nai114-probe.out`. Capture mentally (or to scratch):

1. **SWITCH table for `[opheldu,tinderbox]`** — the case → PC-offset map. Specifically: does case **212** (logs category) appear, and to what PC offset?
2. **Disasm of `[label,light_logs_inv]`** (id 7356) — full body verbatim.
3. **Disasm of GOSUB targets** id 6460, 7904, 7359, 7357, 7942, 2120, 2130 — full bodies.
4. **Opcode inventory** — any `*** UNKNOWN Op(NN) ***` entries? Flag each as "candidate-missing-opcode".
5. **Logs/newbielogs/tinderbox cache state** — confirm Bundle 0 numbers (logs Category=212, Params={86:1, 132:400}; newbielogs Category=-1).

---

## Task 3: Pin Java client OPHELDU wire ordering

**Why:** Spec §7 R1 — confirm whether `obj` field is the click-target (logs) or the use-item (tinderbox). Affects which arm of the 4-arm fallback hits.

**Files:**
- No file changes; reads only.

- [ ] **Step 3.1: Read TS decoder**

Read: `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/network/game/client/codec/OpHeldUDecoder.ts`.

Capture: the field-decode order. The `OpHeldU` model (`src/network/game/client/model/OpHeldU.ts`) names the fields `obj/slot/com/useObj/useSlot/useCom`. The decoder reads them from the wire in some order. **Note which wire byte position corresponds to which field, and which is described as the "target" vs the "use-item".**

- [ ] **Step 3.2: Read TS handler comment**

Read: `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/network/game/client/handler/OpHeldUHandler.ts:14-15` (destructuring line) plus comment if any.

- [ ] **Step 3.3: Read Java client encoder**

Run: `rg -n "OPHELDU|opheldu" /home/owner/Code/github.com/LostCityRS/Client-Java/src/main/java/ | head -20`

Open the file that emits opcode 130 (OPHELDU) on the wire. Capture: which Java-side variable goes into the first wire field, and what's its meaning (target item that was clicked, or use-item that was selected first)?

- [ ] **Step 3.4: Reach a verdict**

Answer this question for the investigation note:

> When player drags tinderbox onto logs, the wire packet has `obj = ____ (logs/tinderbox)` and `useObj = ____ (logs/tinderbox)`.

This determines whether arm (a) or arm (b) hits in `handleOpHeldU` — and therefore whether the script body sees `LAST_USEITEM = logs` (post-arm-(b)-swap) or `LAST_USEITEM = tinderbox` (no swap).

---

## Task 4: Walk goscape's SWITCH opcode handler against the probe's switch-table dump

**Why:** if Bundle 0's SWITCH table dump shows case 212 → PC 20 BUT goscape's SWITCH handler decodes the table differently (wrong endianness, wrong bounds, wrong case-key parsing), H3.a binds.

**Files:**
- No file changes; reads only.

- [ ] **Step 4.1: Locate goscape's SWITCH handler**

Run: `rg -n "OpSwitch\b|case OpSwitch|TriggerSwitch" pkg/script/`

- [ ] **Step 4.2: Read the SWITCH handler body**

Read the file:line that owns `OpSwitch` dispatch. Capture: how it pops the stack, looks up the case in `f.SwitchTables[switchID]`, and computes the new PC.

- [ ] **Step 4.3: Read TS reference**

Run: `rg -n "OPCODES\.SWITCH|switch.*case" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/lostcity/engine/script/handlers/CoreOps.ts | head -20`

Open the matching TS handler. Capture: how TS pops the stack, looks up the case, and computes the new PC.

- [ ] **Step 4.4: Compare**

If the two implementations match line-by-line in semantics (lookup logic, default-fallthrough, PC arithmetic), H3.a is statically refuted. If they differ, H3.a binds.

---

## Task 5: Write Stage 1.1 investigation note + commit + delete probe

**Files:**
- Create: `docs/superpowers/investigations/2026-05-06-nai-114-stage1-bundle0-findings.md`
- Delete: `cmd/probe-opheldu/main.go`

- [ ] **Step 5.1: Write the investigation note**

Create the file with this structure:

```markdown
# NAI-114 — Stage 1.1 Bundle 0 findings

**Date:** 2026-05-06
**Spec:** docs/superpowers/specs/2026-05-06-nai-114-opheldu-tinderbox-firemaking-investigation-design.md
**Stage:** 1.1 — controller disasm extension; static-only; no production change.

## 1. SWITCH table for `[opheldu,tinderbox]` PC 18

<!-- From Task 2 step 2.2 item 1. Enumerate every (case, PC-offset) pair.
     Mark whether case 212 is present and what PC-offset it maps to. -->

| Case key | PC offset | Routes to (PC of target instruction) |
|---|---|---|
| ... | ... | ... |

**Verdict:** case 212 is [PRESENT/ABSENT]. If PRESENT: routes to PC [N] which is [LAST_USESLOT + JUMP_WITH_PARAMS 7356 / different]. If ABSENT: SWITCH default fires → PC 26 → GOSUB 2130 ([proc,displaymessage] arg=0).

## 2. Disasm: `[label,light_logs_inv]` (id 7356)

<!-- Full bytecode dump. Include line numbers. -->

## 3. Disasm: chained scripts

### id 6460 (`[label,?_logs_param]` — oc_param=1 path)
<!-- bytecode -->

### id 7904 (newbielogs path)
<!-- bytecode -->

### id 7359 (likely loc_add fire helper)
<!-- bytecode -->

### id 7357 (likely firemaking xp grant)
<!-- bytecode -->

### id 7942 (likely fire_make helper)
<!-- bytecode -->

### id 2120
<!-- bytecode -->

### id 2130 ([proc,displaymessage])
<!-- bytecode -->

## 4. Java client wire ordering

<!-- From Task 3 step 3.4. -->

When player drags tinderbox onto logs:
- Wire `obj` = ____  (target item — clicked-on)
- Wire `useObj` = ____  (use-item — held)

**Therefore:** in `handleOpHeldU`, arm (a) lookup `[opheldu, ____]` [HITS/MISSES]; arm (b) lookup `[opheldu, ____]` [HITS]. After dispatch: `LAST_USEITEM = ____`.

## 5. SWITCH opcode-handler walk (goscape vs TS)

<!-- From Task 4. -->

- Goscape SWITCH handler: `pkg/script/handlers_*.go:LINE` — [paste body or summary]
- TS SWITCH handler: `Engine-TS/src/lostcity/engine/script/handlers/CoreOps.ts:LINE` — [paste body or summary]
- **Diff verdict:** [match / divergence at X]

## 6. Opcode inventory (chain-wide)

<!-- From Task 2 step 2.2 item 4. List every distinct opcode appearing in the chain. Flag any unknown Op(NN). -->

```
ADD
ANIM
BRANCH
BRANCH_EQUALS
...
```

**Unknown opcodes:** [none / list]

## 7. Bundle 0 hypothesis-status update

| H | Status | Evidence |
|---|---|---|
| H3.a (SWITCH case-212 mismatch) | [LIVE / REFUTED] | [from §1 + §5] |
| H3.b (opcode in 7356 silent-abort) | LIVE — pending Stage 1.2 audit | full opcode walk in §6 |
| H3.c (chain opcode silent-abort) | LIVE — pending Stage 1.2 audit | §3 + §6 |
| H3.d (ENUM(105,...) loader gap) | LIVE-low priority | gated behind §1 default-path firing |

## 8. Stage 1.2 dispatch readiness

Subagent inputs ready: spec, this note, opcode inventory, candidate-missing-opcodes (if any).
```

- [ ] **Step 5.2: Fill in the placeholders from probe + reads**

Replace every `<!-- ... -->` and `____` with the actual content from Task 2 + Task 3 + Task 4. Use the captured output from `/tmp/claude/nai114-probe.out`.

- [ ] **Step 5.3: Stage and commit (note only)**

```bash
git add docs/superpowers/investigations/2026-05-06-nai-114-stage1-bundle0-findings.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-114 Stage 1.1 — Bundle 0 findings + procs disasm

Captures full bytecode disasm for the firemaking dispatch chain
[opheldu,tinderbox] → SWITCH → [light_logs_inv] (7356) plus GOSUB
helpers (6460, 7904, 7359, 7357, 7942, 2120, 2130). Pins Java client
OPHELDU wire ordering. Walks goscape SWITCH opcode handler against
TS reference. Updates H3.a binding status; H3.b/c/d remain pending
Stage 1.2 audit.

Stage 1.1 deliverable per docs/superpowers/specs/2026-05-06-nai-114-...

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.4: Delete the probe**

```bash
rm -rf cmd/probe-opheldu
```

(Probe was never staged; this is a working-tree-only cleanup.)

- [ ] **Step 5.5: Verify clean state**

Run: `git status`

Expected: working tree clean (apart from sandbox-injected `crw-rw-rw-` dotfiles + pre-existing `test_typed_nil.go` + `.claude/` — all untracked, none staged).

---

## Task 6: Decision gate — H3.a binding outcome

**Files:** none.

- [ ] **Step 6.1: Read `docs/superpowers/investigations/2026-05-06-nai-114-stage1-bundle0-findings.md` §1 + §5.**

- [ ] **Step 6.2: Branch:**

- **If §1 reports case 212 routes to PC 20 (logs path) AND §5 reports SWITCH handler matches TS:** H3.a REFUTED. Skip to Task 7 (Stage 1.2 dispatch for H3.b/c).
- **If §1 reports case 212 ABSENT (default fires) AND §5 reports SWITCH handler matches TS:** H3.a binds at content layer (cache table is incomplete or upstream data drift). Skip Task 7; route Stage 2 fix planning to a NEW Stage 2 plan with shape "content-cache investigation" — this plan ends here with §10 handoff.
- **If §5 reports SWITCH handler diverges from TS:** H3.a binds at goscape SWITCH-decode layer. Skip Task 7; route Stage 2 fix planning to a NEW Stage 2 plan with shape "fix SWITCH opcode handler" — this plan ends here with §10 handoff.

---

## Task 7: Dispatch Sonnet audit subagent for H3.b/H3.c

**Files:** none in this repo; subagent writes audit report in Task 8.

- [ ] **Step 7.1: Dispatch the subagent**

Use the `Agent` tool. Parameters:

- `description`: `NAI-114 Stage 1.2 opcode audit`
- `subagent_type`: `general-purpose`
- `model`: `sonnet`
- `prompt` (verbatim — paste this in full, no `<!-- -->` placeholders):

```
You are the Stage 1.2 audit subagent for NAI-114, an investigation sub-spec on goscape's
OPHELDU "tinderbox on logs" no-effect bug at Tutorial Island.

Read these documents in order BEFORE doing anything else:

1. /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/specs/2026-05-06-nai-114-opheldu-tinderbox-firemaking-investigation-design.md (the spec — sections 2 + 3 are most relevant)
2. /home/owner/Code/github.com/zsrv/goscape/docs/superpowers/investigations/2026-05-06-nai-114-stage1-bundle0-findings.md (Stage 1.1 findings — opcode inventory in §6, full disasm in §2-3)

Your job: produce an opcode-coverage matrix walking the firemaking dispatch chain
([opheldu,tinderbox] → SWITCH → [label,light_logs_inv] id 7356 → GOSUB chain ids 7359
/ 7357 / 7942 / 2120 / 2130) and bind H3.b or H3.c (downstream opcode silent-abort) to
ONE specific divergent opcode (or class).

Repos available read-only:
- /home/owner/Code/github.com/zsrv/goscape (this project; goscape Go engine)
- /home/owner/Code/github.com/LostCityRS/Engine-TS (TS engine — canonical reference)
- /home/owner/Code/github.com/LostCityRS/Server (content scripts; firemaking.rs2)
- /home/owner/Code/github.com/LostCityRS/Client-Java (Java client rev-225)

Method:
1. From Stage 1.1 findings §6, extract the chain-wide opcode inventory.
2. For EACH unique opcode in the inventory:
   a. Find the goscape handler. Grep `pkg/script/handlers_*.go` for the opcode constant
      name (e.g. for `OC_PARAM`, search `OpOcParam` or similar; check `pkg/script/opcodes.go`
      for the canonical constant). Capture file:line.
   b. Find the TS handler. The TS handlers live in `Engine-TS/src/lostcity/engine/script/
      handlers/` split by area (CoreOps, NumberOps, ObjOps, PlayerOps, InvOps, MapOps,
      WorldOps, etc.). Capture file:line.
   c. Read both handler bodies. Note any of:
      - missing in goscape (no handler registered for this opcode)
      - silent-abort pattern (returns nil but TS does work)
      - wrong return value / wrong stack mutation
      - wrong side-effect (e.g. INV_DROPSLOT removes wrong slot, ANIM doesn't propagate masks)
      - wrong pop count
3. Build the opcode-coverage matrix per spec §4.2 deliverable item 1:

   | Opcode | TS handler (file:line) | Goscape handler (file:line) | Behavior diff | Bound to symptom? |

4. From the matrix, identify the SINGLE opcode (or smallest class) most likely responsible
   for the firemaking script reaching the end of [light_logs_inv] WITHOUT producing a
   visible fire / animation / log-removal / xp-grant.

   Score candidates by:
   - position in the chain (earlier abort = higher impact)
   - severity (missing > silent-abort > wrong-value > minor diff)
   - alignment with all four observed-missing effects (an opcode that aborts before
     INV_DROPSLOT, ANIM, GOSUB 7359, GOSUB 7357 explains all symptoms; one that aborts
     after INV_DROPSLOT would leave logs gone)

5. Recommend a Stage 2 fix shape (port new handler / fix existing handler / fix decoder /
   fix loader) with LOC estimate.

Constraints:
- Cite TS file:line and goscape file:line for EVERY claim. No "by analogy" reasoning.
- If unable to bind H3 with high confidence after a full opcode walk: report inconclusive,
  name the top-3 candidates, and recommend Stage 1.3 controller instrumentation.
- If you encounter an opcode whose goscape handler grep returns no result (apparent
  missing handler), DOUBLE-CHECK by re-greping with alternative naming conventions
  (`OpXxx`, `xxxHandler`, `handleXxx`, `op_xxx`, etc.) before concluding "missing".
- Do NOT modify any files. Read-only audit.

Deliverable: write your final report to:
/home/owner/Code/github.com/zsrv/goscape/docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md

Structure:
# NAI-114 — Stage 1.2 opcode-coverage audit
## 1. Audit method
## 2. Opcode-coverage matrix (full)
## 3. SWITCH-decode audit verdict (re-derive from Stage 1.1 §5 OR re-do if §5 was skipped)
## 4. H3 binding verdict (single named opcode/class; cite file:line; symptom-alignment scoring)
## 5. Fix shape recommendation (shape A/B/C/D + LOC estimate)
## 6. Confidence level (high/medium/low) + open uncertainties

When done, end your response with the file path of your report and a one-paragraph
executive summary of the H3 binding.
```

- [ ] **Step 7.2: Wait for subagent to complete and read the report**

Read `/home/owner/Code/github.com/zsrv/goscape/docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md`.

If the subagent reports inconclusive: continue to Task 8 to verify what it DID claim, then proceed to §10 handoff with note that Stage 1.3 instrumentation may be needed (NAI-112 precedent).

---

## Task 8: Controller HEAD-verification of audit claims

**Why:** per `audit_subagent_fabrication.md` + `verify_implementer_claims.md` — subagents fabricate; verify before treating as binding.

**Files:** none modified; verification is read-only + go-test runs.

- [ ] **Step 8.1: Re-grep every cited goscape file:line**

For each (file, line, claim) row in the audit's matrix:

```bash
rg -n "<exact-symbol-or-shape>" <cited-file>
```

Compare returned line number(s) against audit's claim. If mismatch → flag in scratch as "audit-stale" or "audit-wrong".

- [ ] **Step 8.2: Re-grep every cited TS file:line**

Same protocol against `/home/owner/Code/github.com/LostCityRS/Engine-TS/`.

- [ ] **Step 8.3: For audit's named "missing handler" claims, exhaustive grep**

For each "missing" opcode:

```bash
rg -n "<OpcodeName>|<opcode_name_snake>|handle<OpcodeName>" /home/owner/Code/github.com/zsrv/goscape/pkg/script/
```

If grep returns a hit anywhere → audit was wrong; flag and route to subagent re-prompt or re-derive controller-side.

- [ ] **Step 8.4: For audit's named "behavior diff" claim, run targeted go test**

If the audit names a specific handler (e.g. "OpStatRandom in handlers_player.go:NNN diverges in Y"):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "<TestNamePattern>" -count=1 -v ./pkg/script/...
```

If existing tests pass against the supposedly-divergent handler → audit's behavior-diff claim is statically suspect. Read the handler body manually and compare to TS.

- [ ] **Step 8.5: Reach a verification verdict**

For the audit's H3 binding (its single named opcode/class):

- **CONFIRMED:** all cited file:lines verify; manual handler-body read confirms the diff. Proceed to Task 9.
- **PARTIALLY CONFIRMED:** some claims verify, others stale. Update audit doc inline with controller corrections; proceed to Task 9 noting "controller-corrected".
- **REFUTED:** binding doesn't hold. Re-dispatch the subagent (return to Task 7 with corrected prompt) OR re-derive controller-side without subagent — choose based on remaining context budget.

---

## Task 9: Commit Stage 1.2 audit + verification verdict

**Files:**
- Modify: `docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md` (append controller verification section if any corrections were made).

- [ ] **Step 9.1: If Task 8 made corrections, append `## 7. Controller HEAD-verification` to the audit doc**

Structure:

```markdown
## 7. Controller HEAD-verification (post-subagent)

**Verified at HEAD:** <commit-sha-of-Task-9-base>

### Confirmed claims
- <list>

### Stale / corrected claims
- <list with file:line→file:line corrections>

### Refuted claims
- <list>

**Final H3 binding (post-verification):** <single-opcode-or-class>; cite file:line.
**Fix-shape recommendation (post-verification):** <shape>; <LOC estimate>.
```

- [ ] **Step 9.2: Stage and commit**

```bash
git add docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-114 Stage 1.2 — opcode-coverage audit + H3 binding

Sonnet audit subagent walks the firemaking dispatch chain
[opheldu,tinderbox] → SWITCH → [light_logs_inv] (7356) → GOSUB chain
against goscape pkg/script/handlers_*.go vs Engine-TS handlers.
Binds H3 to <single opcode/class>; cites <file:line> and <ts-file:line>.
Controller HEAD-verifies every cited claim per audit_subagent_fabrication
+ verify_implementer_claims; corrections recorded in §7 if any.

Stage 1.2 deliverable per docs/superpowers/specs/2026-05-06-nai-114-...

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Edit the commit-body `<single opcode/class>` and `<file:line>` placeholders to reflect actual Stage 1.2 binding before committing.)

---

## §10. Handoff to Stage 2 plan

**Stage 1 of NAI-114 ends here.** Stage 2 fix planning lives in a SEPARATE plan doc:

- **Filename:** `docs/superpowers/plans/2026-05-06-nai-114-stage2-<fix-shape>.md` (where `<fix-shape>` is e.g. `port-stat-random-handler`, `fix-switch-decoder`, `fix-content-loader`, etc.).
- **Inputs:** this plan's Stage 1.2 audit report (`docs/superpowers/investigations/2026-05-06-nai-114-stage1-audit.md`); spec §5 fix shapes A/B/C/D.
- **Cadence:** TDD per `superpowers:test-driven-development`; one RED-GREEN-REFACTOR per opcode handler; defensive-gate doc-comment labels per `defensive_gate_doc_comment_label`; TS-citation per `true_to_ts_gate`.

**Resume protocol** (per `superpowers_clear_between_spec_and_impl`):

After this Stage 1 plan's tasks complete (i.e., commit at Task 9 lands), the executing session should:

1. Update `nai_followups.md` index entries (Stage 1 progress note; not a close).
2. **NOT** auto-write the Stage 2 plan in the same session — let the user `/clear`.
3. Emit a paste-ready resume prompt for the user (per `post_task_handoff`):

```
NAI-114 Stage 1 complete (commit <Task-9-sha>). Stage 1.2 audit binds H3 to
<single opcode/class> with <high/medium/low> confidence.

Next: write the Stage 2 fix plan via brainstorming (skip — spec §5 shape is
already locked) → writing-plans. Save to:
  docs/superpowers/plans/2026-05-06-nai-114-stage2-<shape>.md

Pre-flight greps:
  rg -n "<bound-opcode-name>" pkg/script/
  rg -n "<bound-opcode-name>" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/lostcity/engine/script/handlers/

Use TDD; pin the test fixture per scriptstate_test_fixture_idioms; defensive-gate
doc-comments per defensive_gate_doc_comment_label.

Stage 2 closes with user-launched smoke per spec §6.
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Plan task |
|---|---|
| §2.5 control-flow reconstruction | Task 2.2 + Task 5.1 §1 + §2 |
| §3 in-scope: Stage 1.1 controller disasm extension | Task 1 + Task 2 + Task 5 |
| §3 in-scope: Stage 1.2 Sonnet audit subagent | Task 7 |
| §3 in-scope: controller HEAD-verification | Task 8 |
| §4.1 Stage 1.1 deliverable shape | Task 5.1 (template) |
| §4.2 Stage 1.2 deliverable shape | Task 7.1 (subagent prompt) |
| §4.3 controller HEAD-verification protocol | Task 8 |
| §7 R1 (wire-order pin) | Task 3 |
| §7 R2 (subagent fabrication mitigation) | Task 8.1-8.5 |

Stage 2 + smoke + close are out of this plan's scope (separate plan per §10).

**Placeholder scan:** the audit-doc template has `<!-- ... -->` and `____` placeholders intended to be filled at Task 5.2 from probe output. Step 5.2 explicitly enumerates the fill-in step. Task 7.1 prompt and Task 9.2 commit body have `<single opcode/class>` placeholders intended to be filled at write-time from the actual Stage 1.2 binding. These are not plan failures — they're parameterized handoff points. Marked clearly.

**Type/symbol consistency:** `tinder.SwitchTables` referenced in Task 1 — flagged as adjustment hint if field name differs (Task 1.2 step). `prov.GetByID(uint32(id))` matches `pkg/script/provider.go:172`. `prov.GetByName("[opheldu,tinderbox]")` matches `provider.go:156`. `script.LookupKeyForType` / `LookupKeyForCategory` / `LookupKeyForGlobal` — all confirmed exported during Bundle 0. `objtype.LoadParamTypes(cacheRoot)` — confirmed.
