# NAI-126 — NPC_DEL handler + paramtype DefaultInt sign-extension + modernization sweep

**Status:** spec — draft 1
**Date:** 2026-05-08
**Predecessor:** NAI-125 close (`a5be9b6`); cascade-bound to the `[proc,npc_death]` smoke surfaced by NAI-125 close (2026-05-08).
**Cadence:** subagent-driven-development — three independent bundles dispatched as separate task sequences; one Sonnet code-reviewer pass at end; user-launched smoke binds Bundle 1 PRIMARY only.
**Tech stack:** Go 1.26+.

## §0 — One-line summary

Close the `[proc,npc_death]` cascade-tail surfaced by NAI-125 close smoke (2026-05-08) by porting `NPC_DEL` (opcode 2510), and converge two queued NAI-124 carryovers — paramtype `DefaultInt` sign-extension and a small modernization sweep. Bundle 1 (NPC_DEL) is the smoke-bound PRIMARY; Bundles 2 and 3 are independent.

## §1 — Bundle 1: NPC_DEL handler (PRIMARY, smoke-bound)

### §1.1 — Symptom and binding evidence

**Smoke (NAI-125 close, `a5be9b6`, 2026-05-08):**
- Tutorial Island fresh char kills giant rat (post-NAI-125 — NPC_ARRIVEDELAY now dispatched).
- Server log emits `WARN: script "[proc,npc_death]": no handler for NPC_DEL (opcode 2510) at pc=31`.
- Whatever ops follow `NPC_DEL` at pc≥32 in `npc_death` (loot drop / respawn timer / kill-count XP / etc.) are not running because `Execute` aborts at the first unknown opcode (`pkg/script/runner.go:55-77`).

**Verification at HEAD (`a5be9b6`):**

```
$ rg "OpNpcDel\b" pkg/ modules/
pkg/script/opcode.go:247:	OpNpcDel               Opcode = 2510
pkg/script/opcode.go:893:	case OpNpcDel:
pkg/script/opcode.go:894:		return "NPC_DEL"
```

No dispatch entry in `pkg/script/handlers.go:402-415`. No handler function. No reference in any test.

The producer-side surface (`Server.removeNpc`) is wired and consumed by `NpcLifecycleDespawn` lifecycle ticking:

```
$ rg "func \(s \*Server\) removeNpc" modules/world/
modules/world/npc_registry.go:181:func (s *Server) removeNpc(n *Npc, duration int) {
```

`Server.removeNpc` already does zone-leave, rsbuf remove, dead-flag, collision toggle, and lifecycle-branched lifecycleTick scheduling. Bundle 1 is the wiring layer above it.

### §1.2 — TS reference (verbatim)

`Engine-TS/src/engine/script/handlers/NpcOps.ts:78-80`:

```typescript
[ScriptOpcode.NPC_DEL]: checkedHandler(ActiveNpc, state => {
    World.removeNpc(state.activeNpc, check(state.activeNpc.type, NpcTypeValid).respawnrate);
}),
```

`World.removeNpc(npc, duration)` reference at `Engine-TS/src/engine/World.ts:1296-1319`.

`NpcTypeValid` at `Engine-TS/src/engine/script/ScriptValidators.ts` — validates that the type ID maps to a `NpcType` config; throws on miss. In goscape, `*Npc.typ` is established at `NewNpc` and is non-nil for every live NPC; `requireActiveNpc` (gated on `Pointers` flag) is the goscape analogue of `checkedHandler(ActiveNpc, ...)`. **No additional NpcTypeValid validator** is added; `Respawnrate()` reads `n.typ.RespawnRate` directly.

### §1.3 — Surface

| Layer | File | Change |
|---|---|---|
| Interface | `pkg/script/active.go` | Add `Respawnrate() int` to `ActiveNpc` (sibling of `NpcCategory()`). |
| Interface | `pkg/script/state.go` | Add `RemoveNpc(npc ActiveNpc, duration int)` to `WorldVars` (sibling of `RemoveObj`). |
| Handler | `pkg/script/handlers_npc.go` | New `handleNpcDel` between `handleNpcDamage` (line 300) and `handleNpcDelay` (line 319). |
| Dispatch | `pkg/script/handlers.go` | New entry `OpNpcDel: handleNpcDel,` between `OpNpcDamage` (line 407) and `OpNpcDelay` (line 408). |
| Impl | `modules/world/npc_script.go` | New `(n *Npc) Respawnrate() int { return int(n.typ.RespawnRate) }`. (`*Npc` already has the other ActiveNpc methods here.) |
| Adapter | `modules/world/server_varp.go` | New `(w worldVarsView) RemoveNpc(npc script.ActiveNpc, duration int)` — type-asserts `*Npc`, calls `w.s.removeNpc(realNpc, duration)`. (Mirrors `RemoveObj` at `:130`.) |
| Test fixture | `pkg/script/handlers_npc_test.go` | Extend `mockNpc` with `respawnrate int` field + `Respawnrate()` getter. |
| Test fixture | `pkg/script/handlers_vars_test.go` | Extend `mockWorld` with `RemoveNpc(npc ActiveNpc, duration int) {}` no-op stub (overrideable per existing `RemoveObj` pattern). |

### §1.4 — Handler body

```go
// pkg/script/handlers_npc.go (between handleNpcDamage and handleNpcDelay)

// handleNpcDel (NPC_DEL, opcode 2510) removes the active NPC. The
// duration passed to World.RemoveNpc is the active NPC type's
// respawnrate; Server.removeNpc scales it by player count and writes
// it to lifecycleTick (RESPAWN-lifecycle) or schedules registry
// cleanup (DESPAWN-lifecycle, currently dead-bool model — see
// modules/world/npc_registry.go:181 and TODO(NAI-19)).
//
// Mirrors TS NpcOps.ts:78-80:
//
//   [ScriptOpcode.NPC_DEL]: checkedHandler(ActiveNpc, state => {
//       World.removeNpc(state.activeNpc, check(state.activeNpc.type, NpcTypeValid).respawnrate);
//   }),
//
// DEVIATION-NAI-126-D1: nil-World defensive guard (goscape defensive;
// TS skips this check — World is always present in a running engine).
// Mirrors handleObjDel at handlers_obj.go:122-124. Retire when an
// upstream invariant proves s.World is non-nil for any executing
// script.
func handleNpcDel(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_DEL"); err != nil {
		return err
	}
	if s.World == nil {
		return fmt.Errorf("NPC_DEL: no world surface")
	}
	s.World.RemoveNpc(s.ActiveNpc, s.ActiveNpc.Respawnrate())
	return nil
}
```

### §1.5 — Adapter body

```go
// modules/world/server_varp.go (after RemoveObj at ~line 130)

// RemoveNpc implements script.WorldVars.RemoveNpc. Type-asserts the
// script-side ActiveNpc to *Npc and calls the existing Server.removeNpc.
// Mirrors RemoveObj. Type-assert miss is a silent no-op (matches
// RemoveObj behavior); production NPC pointers are always *Npc.
func (w worldVarsView) RemoveNpc(npc script.ActiveNpc, duration int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	w.s.removeNpc(realNpc, duration)
}
```

### §1.6 — `*Npc.Respawnrate` impl

```go
// modules/world/npc_script.go (after NpcCategory)

// Respawnrate returns the NPC type's respawnrate config field
// (uint16 widened to int). Read by NPC_DEL — passed as the duration
// arg to World.RemoveNpc. Mirrors TS check(state.activeNpc.type,
// NpcTypeValid).respawnrate at NpcOps.ts:79.
func (n *Npc) Respawnrate() int { return int(n.typ.RespawnRate) }
```

### §1.7 — Tests

In `pkg/script/handlers_npc_test.go`, mirror the `TestNpcDelay*` and `TestObjDel*` family:

1. **`TestHandleNpcDel_CallsRemoveNpc`** — register a fake `mockWorld`-derivative `fakeWorldRemoveNpc` (mirrors `fakeWorldRemoveObj` at obj-test sites) whose `RemoveNpc` records `(npc, duration)` calls; seed `mockNpc{typeID: 5, respawnrate: 50}`; execute a one-op `OpNpcDel; OpReturn` script with `state.ActiveNpc` set; assert exactly one call recorded with `duration == 50`.
2. **`TestHandleNpcDel_PassesActiveNpcInstance`** — assert the recorded `npc` value is identity-equal (`==`) to `state.ActiveNpc`.
3. **`TestHandleNpcDel_NoActiveNpcErrors`** — empty `state.ActiveNpc` → `Execute` returns an error matching the `requireActiveNpc` shape (`"NPC_DEL: ..."`); no `RemoveNpc` call recorded.
4. **`TestHandleNpcDel_NilWorldErrors`** — `state.World == nil` → `Execute` returns `"NPC_DEL: no world surface"` error (DEVIATION-NAI-126-D1); no recorded call.
5. **`TestHandleNpcDel_ZeroRespawnrate`** — `mockNpc{respawnrate: 0}`; assert `duration == 0` propagates to `RemoveNpc` (pins the accessor; NPCs that legitimately never respawn pass through unchanged).

Compile-time interface assertion `var _ ActiveNpc = (*mockNpc)(nil)` already exists at `handlers_npc_test.go:181` (`TestNpcLookupInterfaceShape`); the `Respawnrate()` addition gets coverage there automatically.

No integration test against `Server.removeNpc` — that surface is independently tested in `npc_registry_test.go`. The handler's responsibility is the wiring.

## §2 — Bundle 2: paramtype `DefaultInt` sign-extension

### §2.1 — Divergence

`pkg/objtype/paramtype.go:111` stores `DefaultInt uint32`; siblings store `int32`:
- `pkg/objtype/enumtype.go:17` — `DefaultInt int32`
- `pkg/objtype/dbtabletype.go:33` — `DefaultInts [][]int32`

Three consumer sites cast `int(pt.DefaultInt)`:
- `pkg/script/handlers_config.go:51` — `s.PushInt(int(pt.DefaultInt))`
- `pkg/script/handlers_inv.go:256` — `total += int(pt.DefaultInt)`
- `modules/world/npc_hunt.go:297` — `total += int(pt.DefaultInt)`

With `uint32` storage, `int(pt.DefaultInt)` **zero-extends** — a content-set `DefaultInt = -1` (raw `0xFFFFFFFF` on the wire) reads as `4294967295` instead of `-1`. The `paramtype.go:183` comment `// this is -1 in js, default 0 here` foreshadows the divergence.

**Why not NAI-124 smoke contributor:** all `combat.rs2`-path ParamTypes have non-negative configured defaults, so this never bit the damage-magnitude smoke. But `param_default(p)` opcodes are content-callable for any param, so any future content surface that uses a `-1` default is broken silently today.

### §2.2 — Changes

| File:Line | Before | After |
|---|---|---|
| `pkg/objtype/paramtype.go:111` | `DefaultInt    uint32` | `DefaultInt    int32` |
| `pkg/objtype/paramtype.go:121` | `pt.DefaultInt = dat.G4()` | `pt.DefaultInt = int32(dat.G4())` |
| `pkg/objtype/paramtype.go:183` | `//DefaultInt: -1, // this is -1 in js, default 0 here` | Drop comment. The post-bundle field type (`int32`) and consumer sites (`int(pt.DefaultInt)` sign-extends correctly) make the original concern obsolete. |

**Consumer sites unchanged:** `int(pt.DefaultInt)` already sign-extends correctly once the field is `int32`. No belt-and-suspenders `int(int32(...))` cast needed.

### §2.3 — Tests

In `pkg/objtype/paramtype_test.go` — new file (verified absent at HEAD `a5be9b6`):

1. **`TestParamType_DecodeNegativeDefault`** — feed a 4-byte payload `0xFFFFFFFF` to `Decode(code=2, ...)`; assert `pt.DefaultInt == int32(-1)` and `int(pt.DefaultInt) == -1` (sign-extended).
2. **`TestParamType_DecodePositiveDefault`** — feed `0x00000064`; assert `pt.DefaultInt == int32(100)`.
3. **`TestParamType_DecodeMaxInt32`** — feed `0x7FFFFFFF`; assert `pt.DefaultInt == int32(2147483647)`.

Existing tests, if any, must remain GREEN. Plan author re-greps `pt\.DefaultInt\b` and `\.DefaultInt\b` at dispatch time to surface any consumer added since this brainstorm.

### §2.4 — Why TS-fidelity audit at the consumer sites

TS `ParamType.defaultInt` is a JS `number` (no unsigned/signed distinction; sign comes from the wire). All three consumers ultimately push to the script stack as a signed 32-bit int. The bug is goscape-only.

## §3 — Bundle 3: modernization style-cleanup

Pure-style cleanup of staticcheck/modernize warnings catalogued at NAI-124 close (`nai_followups.md:6313`). No behavioral change.

| File:Line | Lint | Fix |
|---|---|---|
| `pkg/script/state.go:380, 385, 403, 407` | S1001 (copy-loop) | `for i := range src { dst[i] = src[i] }` → `copy(dst, src)` (per-site review; some may be intentional element-wise transforms — leave those alone). |
| `pkg/script/runner.go:30, 33` | S1001 | Same. |
| `pkg/script/handlers_npc.go:923, 957` | minmax | `if a < b { a = b }` → `a = max(a, b)` (Go 1.21+ builtins; project is Go 1.26+). |
| `pkg/script/handlers_npc_test.go:2113` | rangeint | `for i := 0; i < N; i++` → `for i := range N`. |
| `pkg/script/handlers_player_test.go:146` | rangeint | Same. |

**Per-site verification gate (controller pre-flight):** before each Edit, the implementer must confirm the line still has the exact warning-shape via `go vet ./...` or visual inspection. Any S1001 site that is a deliberate element-wise transform (e.g., `dst[i] = transform(src[i])`) is **left alone** and not classified as a S1001 fix — `copy` only applies to plain copies.

**No new tests.** Existing test suite must pass GREEN unchanged. The `verify_implementer_claims` post-commit gate catches any regression.

## §4 — Cadence and execution

- **Cadence:** subagent-driven-development; three independent bundles dispatched as separate task sequences; one Sonnet code-reviewer pass at end (per `superpowers_code_reviewer_model`); user-launched smoke binds Bundle 1 PRIMARY only.
- **Bundle ordering for commits:** Bundle 1 first (smoke-binding), then Bundle 2, then Bundle 3 — keeps cascade attribution clean per `cascade_theory_smoke_binding`. Each bundle gets its own commit chain.
- **Controller pre-flight** (`controller_preflight`): before each implementer dispatch, re-grep+Read the plan's premise lines at HEAD (file paths, line numbers, struct fields, accessor signatures, dispatch alphabetic neighbors). Mock-recorder field-naming check per `mock_recorder_field_naming_check` for `mockNpc` extension.
- **Verify implementer claims** (`verify_implementer_claims`): fresh independent `go test ./... && go vet ./... && go build ./...` after each commit.
- **Post-commit `git status`** per `feedback_subagent_wt_path`.
- **Smoke handoff** per `smoke_test_server_handoff` — user launches the server; we ask. Smoke target: same `[proc,npc_death]` script that surfaced the cascade — fresh char + bronze dagger vs Tutorial Island giant rat; expected post-NAI-126: no `no handler for NPC_DEL` WARN, rat fully despawns + respawns at its `RespawnRate`-tick mark.
- **Close commit** per `close_commit_memory_trailer` — `chore(close)` with `Closes memory:` trailer enumerating any new memory entries.

## §5 — Risk register

| Risk | Likelihood | Mitigation |
|---|---|---|
| `*Npc.typ == nil` for some test-built NPCs → `Respawnrate()` panic | Low | `*Npc.NpcCategory()` already reads `n.typ.Category` unprotected; same invariant. Bundle 1 unit tests use `mockNpc.respawnrate` field directly — `*Npc` impl path is exercised only via integration in `npc_registry_test.go` and `npc_test.go`, which always go through `NewNpc(... typ)`. |
| `worldVarsView.RemoveNpc` type-assertion failure (script provides a non-`*Npc` ActiveNpc) | Very low | Adapter silently no-ops on type-assert miss, mirroring `RemoveObj` at `server_varp.go:131`. Real production path always passes `*Npc` (set by OPNPC routing / NPC_FIND family). |
| Bundle 2 `int32` change breaks a consumer | Very low — three consumers grepped at brainstorm; all use `int(pt.DefaultInt)` which is type-promoting | Plan author re-greps `pt\.DefaultInt\b` and `\.DefaultInt\b` at dispatch time per `controller_preflight`; surfaces any consumer added since this brainstorm. |
| Bundle 3 S1001 warning on a deliberate element-wise transform | Medium | Per-site verification gate; reviewer instructed to flag any site where `copy()` would change semantics. |

## §6 — Out-of-scope / carry-forward

- All NAI-119/117/115/111/121-residual carryovers unchanged.
- `NpcQueueRequest` `args[]+lastInt` collapse (NAI-123-D1) parked.
- Bundle 1 does **not** wire registry GC for despawned NPCs (the `TODO(NAI-19)` at `npc_registry.go:198`); dead-bool model remains. NAI-19 is the foundation gap, not NAI-126.
- DEVIATION-NAI-126-D1 (nil-World defensive guard) parked; retires when an upstream invariant proves `s.World` is non-nil for any executing script.

## §7 — Memory pattern application

- `cascade_theory_smoke_binding` — Bundle 1 PRIMARY closes on smoke-bind regardless of Bundle 2/3 status.
- `smoke_surfaces_adjacent_divergences` — NAI-125 close smoke surfaced NPC_DEL at >30 LOC; routed forward as Bundle 1 of NAI-126 (this spec).
- `controller_preflight` — pre-flight re-grep on every implementer dispatch.
- `verify_implementer_claims` — fresh `go test ./... && go vet ./... && go build ./...` post each commit.
- `superpowers_code_reviewer_model` — Sonnet, never Opus.
- `defensive_gate_doc_comment_label` — Bundle 1 nil-World guard tagged DEVIATION-NAI-126-D1 in handler doc-comment.
- `mock_recorder_field_naming_check` — Bundle 1 plan author confirms the `mockNpc` field name (`respawnrate int`) matches actual struct ergonomics before dispatch.
- `plan_grep_helper_patterns` — Bundle 1 uses existing `requireActiveNpc` and the `s.World == nil` defensive idiom rather than inlining a new gate.
- `superpowers_clear_between_spec_and_impl` — after spec approval, write plan via writing-plans, then emit resume prompt and stop; user `/clear` before implementing.
- `execution_mode_default` — dispatch via subagent-driven-development without offering menu.
- `close_commit_memory_trailer` — `Closes memory:` trailer on the close commit.
