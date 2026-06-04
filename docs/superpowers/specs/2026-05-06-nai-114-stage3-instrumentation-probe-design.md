# NAI-114 Stage 3 — OPHELDU instrumentation probe (Path A vs Path B binding)

**Date:** 2026-05-06
**Predecessor:** [NAI-114 spec](2026-05-06-nai-114-opheldu-tinderbox-firemaking-investigation-design.md) (Stage 2 closed code-side; cascade-binding invalidated by smoke).
**Status:** Stage 3 instrumentation sub-spec — re-investigates the cascade attribution that Stage 1.2 audit got wrong.

---

## 1. Symptom (post-Stage-2)

After NAI-114 Stage 2 (commits `340081c`, `d164963`, `03bff85`) shipped the MAP_LOCADDUNSAFE port — TS-faithful, 12 unit tests, full suite green — the user-launched smoke shows **no observable change**:

- Player on Tutorial Island uses tinderbox on logs in inventory.
- 5 OPHELDU packets (opcode 130, 12-byte payload) received by server (`14:56:21.728` onward).
- **Zero** `level=WARN msg="script execute error"` lines in server stdout (no script abort).
- **Zero** `script_name=...` debug entries after OPHELDU events (only `[proc,open_and_close_door]` shows up earlier from the door step).
- No fire loc, no inventory change, no animation 733, no chatbox advance.

The Stage 1.2 audit (HIGH confidence) bound H3 = MAP_LOCADDUNSAFE statically (bytecode disasm + opcode-handler grep). The audit's own caveat 1 explicitly flagged that the warn-emission claim was unverified at smoke. Live smoke contradicts the cascade theory: **the script chain isn't reaching MAP_LOCADDUNSAFE — it never abort-warns, with or without the fix.**

Per spec §6 fail-routing of NAI-114 ("symptom unchanged → Stage 3 re-brainstorm per `smoke_unchanged_means_multiple_blockers`"), this sub-spec re-investigates.

## 2. Hypothesis space

The audit's elimination of the stat-gate path was unsound: `audit §261 caveat 2` argued "absence of chatbox suggests script reaches GOSUB 7358" — but per `java_client_coord_chat_suppression`, Tutorial Island suppresses chat MES regardless of source. Both paths produce identical Tutorial-Island-observable symptoms. We need server-side instrumentation, not client-observable signals.

**Plausible bindings:**

- **Path A1 — script not registered.** `[opheldu,tinderbox]` not present in `script.Provider.byName`. Could be content-cache miss, decode failure, or naming mismatch.
- **Path A2 — trigger-lookup ID mismatch.** Script registered, but `GetByTriggerSpecific(TriggerOpHeldU, tinderboxObjID, -1)` returns nil because the script's `LookupKey` was computed against a different obj ID than `s.objTypes.Configs[tinderboxObjID].ConfigType.ID`. Same for category arms. Result: 4-arm fallback all miss → handler sends `"Nothing interesting happens."` MES → suppressed by Tutorial Island chat box.
- **Path B — script ran but exited early on a gate.** Examples: members gate (handler line 365-368), Firemaking-stat check at PC 11 of `[label,light_logs_inv]`, or some other gate before GOSUB 7358 to `[proc,area_allow_loc_add]`. MES "You need a Firemaking level…" → suppressed by Tutorial Island chat box.
- **Path D — cascade re-binding to a different missing handler.** Script ran, hit a non-MAP_LOCADDUNSAFE missing handler, aborted. Should produce a `script execute error` warn — **but** the warn is absent from the user's log, ruling this out unless logging is itself broken (LOW probability).

The smoke discriminator we need: **did the 4-arm fallback dispatch a script, and if so, which one?** A single instrumentation pass binds all paths in one trip.

## 3. Scope

### In scope

- Add boot-time INFO log enumerating all `[opheldu,*]` script names registered (binds Path A1).
- Add pre-dispatch DEBUG log in `handleOpHeldU` showing input obj/useObj resolution (ID + name + category) just before the 4-arm fallback (binds Path A2 inputs).
- Add per-arm DEBUG log inside the 4-arm fallback showing `arm`, `key`, `hit` (true/false) for each of arms (a)/(b)/(c)/(d) (binds Path A2 mechanism).
- Add dispatch-or-miss DEBUG log: either `"opheldu dispatch"` with `sf.Name` + `swapped` flag immediately before `s.runScript(...)`, OR `"opheldu fallback miss"` if all 4 arms missed (binds Path A overall vs Path B).
- All instrumentation lives in:
  - **One new file** `modules/world/debug_nai114.go` containing the boot-time helper + a single per-call probe helper invoked from `handleOpHeldU`.
  - **Inline log calls** in `handler_opheld.go` (additive only — no restructuring of the existing 4-arm chain; existing tests must remain green with zero modification).
- One transient commit `chore(debug): NAI-114 Stage 3 — opheldu trigger-lookup instrumentation`. Reverted at NAI-114 Stage 4 close.

### Out of scope

- **Path B sub-binding** (which gate triggered the early exit). If smoke binds B, Stage 4 will need a second instrumentation pass — defer to Stage 4 brainstorm.
- **Production-quality logging.** This is throwaway probe code. No log levels tuning, no field stability guarantees, no test coverage of the log-line format.
- **Fixing the cascade.** Stage 3 binds; Stage 4 fixes.
- **Reverting the probe.** Handled at Stage 4 close (or fold into a single Stage-3+4 close depending on outcome).

## 4. Stage 1 — instrumentation implementation

### 4.1 Site 1: boot-time `[opheldu,*]` enumeration

**Hook point:** `modules/world/server.go` after the `s.scriptProvider.Load(...)` call completes (need to grep for the call site at plan-write time).

**Helper:** `func logOpHeldUScriptInventory(p *script.Provider, log *slog.Logger)` in `modules/world/debug_nai114.go`. Iterates `p` (using either an exported `Names()` accessor — to be added if needed — or via test-only direct field access if Provider is in same package).

**Provider iteration access:** `script.Provider.byName` is unexported. Two options:
- (a) Add `func (p *Provider) Names() []string` to `pkg/script/provider.go` (3 LOC). Useful beyond this probe.
- (b) Walk via `script.Provider.Count()` + `script.Provider.GetByID(id uint32)` for each id 0..Count()-1, reading `f.Name`.

**Recommended:** (a). Small, useful, plan codifies the addition.

**Output (single line at startup, INFO level):**

```
level=INFO msg="opheldu script registry" count=N names="[opheldu,tinderbox],[opheldu,logs],..."
```

Filter: any name starting with `[opheldu,`.

**Expected outcome examples:**
- `count=0 names=""` → Path A1 confirmed; content cache missing firemaking scripts entirely.
- `count=1 names="[opheldu,tinderbox]"` → script registered; binding shifts to A2 / B.
- `count=4 names="[opheldu,tinderbox],[opheldu,logs],[opheldu,_],..."` → multiple opheldu scripts; arm matching depends on which key the lookup actually hits.

### 4.2 Site 2: pre-dispatch context log (in `handler_opheld.go`)

Insert at the existing line 369 (just before `// 4-arm trigger fallback (TS OpHeldUHandler.ts:96-117); first hit wins.`):

```go
s.log.Debug("opheldu trigger probe context",
    "tick", s.currentTick,
    "obj", obj, "obj_name", objType.ConfigType.DebugName, "obj_config_id", objType.ConfigType.ID, "obj_category", objType.Category,
    "useObj", useObj, "useObj_name", useObjType.ConfigType.DebugName, "useObj_config_id", useObjType.ConfigType.ID, "useObj_category", useObjType.Category)
```

Single line per OPHELDU event reaching this point. Confirms which obj IDs the lookup actually receives (ruling out wire-decode bugs upstream).

### 4.3 Site 3: per-arm hit-trace + dispatch log (in `handler_opheld.go`)

**Constraint:** additive only. No restructuring of the 4-arm chain. Insert one DEBUG log call after each arm's lookup, then one final dispatch-or-miss DEBUG just before the existing `s.runScript(sf, p, nil, true, nil, nil)` (line 398) and `p.MessageGame("Nothing interesting happens.")` (line 394).

**After arm (a) at line 371:**

```go
s.log.Debug("opheldu arm probe", "arm", "a", "key", "type", "trigger", "OPHELDU", "type_id", objType.ConfigType.ID, "hit", sf != nil)
```

**After arm (b) at line 374** (note: arm (b) UNCONDITIONALLY swaps lastItem/lastUseItem regardless of hit; log captures both):

```go
s.log.Debug("opheldu arm probe", "arm", "b", "key", "type", "trigger", "OPHELDU", "type_id", useObjType.ConfigType.ID, "hit", sf != nil)
```

**After arm (c) at line 382:**

```go
s.log.Debug("opheldu arm probe", "arm", "c", "key", "category", "trigger", "OPHELDU", "category_id", objType.Category, "hit", sf != nil)
```

(Wrap in a guard if (c) was skipped due to `objType.Category == -1`; log `"skipped", true` instead.)

**After arm (d) at line 386:**

```go
s.log.Debug("opheldu arm probe", "arm", "d", "key", "category", "trigger", "OPHELDU", "category_id", useObjType.Category, "hit", sf != nil)
```

(Same skip-guard.)

**Before `s.runScript(...)` at line 398:**

```go
s.log.Debug("opheldu dispatch", "tick", s.currentTick, "script", sf.Name)
```

**Before `p.MessageGame("Nothing interesting happens.")` at line 394:**

```go
s.log.Debug("opheldu fallback miss — sending 'Nothing interesting happens.'", "tick", s.currentTick)
```

### 4.4 Stage 1 build + test verification

After instrumentation:
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` all PASS — additive-only constraint preserves all existing handler_opheld_test.go assertions (no behavior change).

## 5. Stage 2 — smoke handoff + outcome routing

User re-launches goscape, connects via Java client rev-225, walks Tutorial Island fire-making step (tinderbox on logs).

### 5.1 Decision matrix

| Boot log shows | Smoke shows | Diagnosis | Stage 4 routing |
|---|---|---|---|
| `count=0` | n/a | A1: content not loaded | Stage 4 spec: investigate content cache + script.Load paths. Out-of-tree of NAI-114; possibly a separate NAI cycle. |
| `count≥1` with `[opheldu,tinderbox]` | All 4 arms `hit=false`, then `opheldu fallback miss` | A2: trigger-lookup ID mismatch | Stage 4 spec: compare `LookupKey` computation between script-load and runtime lookup; check `objType.ConfigType.ID` vs script-bound ID. |
| `count≥1` | `opheldu dispatch script=[opheldu,tinderbox]`, then `script execute error … no handler for X` | D: cascade rebound | Stage 4 spec: port handler X (or add to NAI-115 cascade list). |
| `count≥1` | `opheldu dispatch script=[opheldu,tinderbox]`, then no `script execute error` | B: script ran clean; early-exit gate | Stage 4 brainstorm: add second instrumentation pass to bind which gate triggered. |
| `count≥1` | `opheldu dispatch script=[opheldu,tinderbox]`, then `script execute error … no handler for MAP_LOCADDUNSAFE` | (impossible — handler now registered) | Investigate handler registration regression. |

### 5.2 Smoke pass criteria for Stage 3

Stage 3 is **diagnostic-only**; no user-visible game change is expected. Stage 3 pass = the instrumentation lines appear in server stdout in the expected shape, and the decision matrix unambiguously selects exactly one row.

## 6. Risk register

| # | Risk | Probability | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Adding `Names()` accessor to `script.Provider` triggers test-coverage drift in pkg/script | LOW | One new test or none | Plan codifies a single trivial unit test covering the new accessor. |
| R2 | Additive-only constraint requires careful ordering — log AFTER each arm's lookup (per Site 3 wording) requires care that `sf` is read at the right scope | MED | Wrong sf state in log | Plan prescribes literal insertion points by line number; controller HEAD-verifies before implementer dispatch. |
| R3 | `s.log` may not be reachable from `handleOpHeldU` (handler signature is `(p *Player, payload []byte)`). Need to verify. | MED | Compile fail; need to thread logger via `s := p.client.server` then `s.log` | Plan-author HEAD-verifies the logger field name on `*Server` before codifying. Same access pattern as the existing handler comments referencing `s.objTypes`, `s.scriptProvider` — likely `s.log`. |
| R4 | Boot-time enumeration runs before scripts are loaded if hook point is wrong | LOW | `count=0` reported regardless of cache state → false A1 binding | Plan-author HEAD-verifies the `scriptProvider.Load` call site and inserts the logging hook strictly after Load returns success. |
| R5 | User changes Tutorial Island position between smoke runs | LOW | Different chat-suppression box behavior; doesn't affect server-side instrumentation | Smoke shape doesn't depend on client chat state — instrumentation is server-stdout-only. |
| R6 | Path B confirmation requires another instrumentation pass | MED | Stage 4 needs a second probe sub-spec before fix | Acceptable; per §3 out-of-scope. Stage 4 brainstorm decides cadence. |

## 7. Tech stack & deliverables

- **Go 1.26+** per `go_version`.
- **TS source:** `LostCityRS/Engine-TS` per `ts_source_canonical_path` (Stage 3 doesn't touch TS-faithful code paths).

**Commit sequence:**

1. `docs(spec): NAI-114 Stage 3 — instrumentation probe design` ← this commit.
2. `docs(plan): NAI-114 Stage 3 — implementation plan` ← from `writing-plans`.
3. `chore(debug): NAI-114 Stage 3 — opheldu trigger-lookup instrumentation` ← single transient implementation commit.
4. (Stage 4 spec → fix → close, separately.)

**Memory updates on Stage 3 close (deferred to Stage 4 close):**

- One memory entry on the audit-vs-smoke contradiction pattern: "static-only audits with HIGH confidence still need smoke verification when the symptom is chat-suppressed". Candidate name: `static_audit_needs_smoke_when_chat_suppressed.md`. Defer write to Stage 4 close where final cascade attribution is known.
- Update `cascade_theory_smoke_binding.md` with this NAI-114 example (smoke binds against the cascade).

---

## 8. Self-review

1. **Placeholder scan:** none.
2. **Internal consistency:** §4 instrumentation sites map 1:1 to §5 decision matrix rows. No contradictions.
3. **Scope check:** focused on a single instrumentation pass. Stage 4 fix is excluded. Decomposition not needed.
4. **Ambiguity check:** §4.1 calls out the `Names()` accessor decision (option a vs b) with a recommendation. §4.3 calls out the additive-only constraint. §3 enumerates paths A1 / A2 / B / D distinctly.
