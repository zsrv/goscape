# NAI-62 — `targetSubject.com → typeId` dispatch override (player-side)

> **TS-faithfulness gate.** Port TS `Player.getOpTrigger` / `getApTrigger`
> (`Player.ts:993-997`, `1027-1031`) override semantics to all 8 player-side
> trigger-lookup sites, fix the OpPlayerU producer-side `useObj` drop, and
> canonicalise `SetInteraction com=0 → -1` to match TS `PathingEntity.ts:520`
> truthy. Single new helper, no new deviations. Two-bundle sub-spec.

## §1. Origin

Surfaced during NAI-61 brainstorm option-C exploration; carved out as the
NAI-62-candidate block in `nai_followups.md` → `## NAI-61 — CLOSED 2026-05-01`
(memory line 3042). NAI-61 itself was a pure ordering fix (3 line-moves);
this divergence is content-script keying — strictly out of NAI-61 scope.

## §2. The divergence (verified at HEAD `326d959`)

### §2.1 TS reference

`engine/entity/Player.ts:966-998` (`getOpTrigger`):

```ts
let typeId = -1;
let categoryId = -1;
if (this.target instanceof Npc || this.target instanceof Loc || this.target instanceof Obj) {
    let type: NpcType | LocType | ObjType | null = null;
    if (this.target instanceof Npc)  type = NpcType.get(this.target.type);
    else if (this.target instanceof Loc) type = LocType.get(this.target.type);
    else if (this.target instanceof Obj) type = ObjType.get(this.target.type);
    if (!type) return null;
    typeId = type.id;
    categoryId = type.category;
}
if (this.targetSubject.com !== -1) {
    typeId = this.targetSubject.com;
}
return ScriptProvider.getByTrigger(this.targetOp + 7, typeId, categoryId) ?? null;
```

`getApTrigger` (`Player.ts:1000-1032`) is identical save for the missing
`+ 7` and the absent default-op `-1` clear (it stops at line 1031). Both
override `typeId` only — `categoryId` is never re-keyed.

`engine/entity/PathingEntity.ts:520`:

```ts
this.targetSubject.com = com ? com : -1;
```

JS-truthy: `com=0` and `com=undefined` both store `-1`. Consequence: a
caller that passes `useObj=0` gets `targetSubject.com=-1`, which suppresses
the override in `getOpTrigger`. goscape today stores `com` as-is.

### §2.2 Goscape state at HEAD

`modules/world/interaction.go:56-87` (`(*Player).SetInteraction`):

```go
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
    p.target = target
    p.targetOp = op
    p.targetSubject.com = com   // ← stored literally; no com=0 → -1 truthy
    p.interactionKind = kind
    // …
}
```

`modules/world/handler_op_player.go:216` (`handleOpPlayerU`):

```go
p.SetInteraction(InteractionEngine, other, targetOpPlayerU, -1)
//                                                          ^^^^
// TS OpPlayerUHandler.ts:77 passes useObj here, not -1.
```

The 8 player-side trigger-lookup sites all dispatch with the entity's
type as `typeId`, ignoring `targetSubject.com`:

| File | Line | Helper | Current call |
|---|---|---|---|
| `interaction_trigger.go` | 76  | `fireOpTriggerNpc` | `GetByTrigger(trigger, npc.typeId, category)` |
| `interaction_trigger.go` | 146 | `fireOpTriggerLoc` | `GetByTrigger(trigger, loc.Type(), category)` |
| `interaction_trigger.go` | 317 | `fireApTriggerNpc` | `GetByTrigger(trigger, npc.typeId, category)` |
| `interaction_trigger.go` | 377 | `fireApTriggerLoc` | `GetByTrigger(trigger, loc.Type(), category)` |
| `interaction_trigger.go` | 478 | `fireOpTriggerObj` | `GetByTrigger(trigger, obj.Type, category)` |
| `interaction_trigger.go` | 535 | `fireApTriggerObj` | `GetByTrigger(trigger, obj.Type, category)` |
| `player_interaction_trigger.go` | 55 | `fireOpTriggerPlayer` | `GetByTrigger(trigger, -1, -1)` |
| `player_interaction_trigger.go` | 86 | `fireApTriggerPlayer` | `GetByTrigger(trigger, -1, -1)` |

NPC-actor sites (`npc_interaction_trigger.go`) are out of scope — TS
`Npc.ts` has no analogue override; this is a `Player`-class method only.

### §2.3 Behavioural impact

Post-fix dispatch keys (assuming the override's `targetSubject.com != -1`):

| Click family | Pre-fix lookup | Post-fix lookup |
|---|---|---|
| OpLocT (`spellCom=K`)  | `(OPLOCT, locId, locCategory)`  | `(OPLOCT, K, locCategory)` |
| OpNpcT (`spellCom=K`)  | `(OPNPCT, npcId, npcCategory)`  | `(OPNPCT, K, npcCategory)` |
| OpObjT (`spellCom=K`)  | `(OPOBJT, objId, objCategory)`  | `(OPOBJT, K, objCategory)` |
| OpPlayerT (`spellCom=K`) | `(OPPLAYERT, -1, -1)`          | `(OPPLAYERT, K, -1)` |
| OpPlayerU (`useObj=I` after producer fix) | `(OPPLAYERU, -1, -1)` | `(OPPLAYERU, I, -1)` |
| OpLoc1..5 / OpNpc1..5 / OpObj1..5 / OpLocU / OpNpcU / OpObjU | unchanged | unchanged (`com=-1` → helper returns default) |
| AP variants of the above | parallel to Op variants above | parallel to Op variants above |

This is a real behavioural change: spell-on-X scripts in the LostCityRS
data pack are keyed by spellCom, and item-on-player scripts are keyed by
`useObj`. Today no player-side dispatch site reaches the spellCom-keyed
or useObj-keyed entries because the override is absent.

## §3. Design

### §3.1 Storage canonicalisation

`modules/world/interaction.go::SetInteraction`, line 59. Replace:

```go
p.targetSubject.com = com
```

with:

```go
// TS PathingEntity.ts:520 truthy: com=0 → -1. Lookup-side checks use
// !=  -1, so canonicalising at storage means a single sentinel reaches
// resolveTriggerTypeId.
if com == 0 {
    p.targetSubject.com = -1
} else {
    p.targetSubject.com = com
}
```

Update the `targetSubject.com` doc-comment on `interaction.go::SetInteraction`
(currently lines 47-48) and the field-level doc on `player.go:104-110` to
note the canonicalisation and that the field also carries `useObj` for
OpPlayerU post-NAI-62.

### §3.2 Producer-side fix

`modules/world/handler_op_player.go::handleOpPlayerU`, line 216:

```go
p.SetInteraction(InteractionEngine, other, targetOpPlayerU, useObj)
```

(was `-1`). Combined with §3.1, when `useObj=0` arrives from the wire the
canonicalisation in `SetInteraction` flips to `-1`, matching TS exactly.

Update `handleOpPlayerU`'s "On success" trailer (handler_op_player.go:141-143)
to read: `"… → SetInteraction(Engine, other, targetOpPlayerU, useObj)
(useObj threaded for trigger-lookup override per TS Player.ts:993-995;
canonicalised to -1 when useObj=0 per PathingEntity.ts:520)."`

### §3.3 Consumer override helper

New helper in `modules/world/interaction_trigger.go` (place adjacent to the
existing `apLocTriggerForOp` / `apNpcTriggerForOp` / `apObjTriggerForOp`
cluster around line 175-217):

```go
// resolveTriggerTypeId mirrors the typeId override in TS Player.getOpTrigger
// (Player.ts:993-995) and Player.getApTrigger (Player.ts:1027-1029):
// when targetSubject.com is set (≠ -1), it overrides the entity's typeId
// for trigger lookup. categoryId is NEVER overridden — the override flips
// only the type slot. Used by every player-side fire helper to thread
// spellCom (T-handlers) and useObj (OpPlayerU) into script-key resolution.
//
// Storage convention: SetInteraction canonicalises com=0 → -1 (matching
// TS PathingEntity.ts:520 truthy), so the != -1 check here behaves
// identically to TS !== -1 even at the com=0 boundary.
func resolveTriggerTypeId(p *Player, defaultTypeId int) int {
    if p.targetSubject.com != -1 {
        return p.targetSubject.com
    }
    return defaultTypeId
}
```

### §3.4 Consumer fan-out — 8 callsites

For each of the 8 sites, change the `GetByTrigger` call to thread the
default typeId through `resolveTriggerTypeId(p, …)`:

```go
// interaction_trigger.go:76 (fireOpTriggerNpc)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, npc.typeId), category)

// interaction_trigger.go:146 (fireOpTriggerLoc)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, loc.Type()), category)

// interaction_trigger.go:317 (fireApTriggerNpc)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, npc.typeId), category)

// interaction_trigger.go:377 (fireApTriggerLoc)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, loc.Type()), category)

// interaction_trigger.go:478 (fireOpTriggerObj)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, obj.Type), category)

// interaction_trigger.go:535 (fireApTriggerObj)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, obj.Type), category)

// player_interaction_trigger.go:55 (fireOpTriggerPlayer)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, -1), -1)

// player_interaction_trigger.go:86 (fireApTriggerPlayer)
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, -1), -1)
```

For the Player-target sites the default typeId is `-1` (Player has no
NpcType/LocType/ObjType counterpart in TS); the helper returns `-1` when
`com == -1` so behaviour is unchanged for OpPlayer1..4 clicks where the
producer never sets com. For OpPlayerT (already producing spellCom) and
OpPlayerU (after §3.2), the helper returns the override.

`categoryId` stays at the call's existing source — never overridden. Match
TS exactly.

### §3.5 Doc-comment trailers

Each of the 6 fire-helpers in `interaction_trigger.go` and 2 in
`player_interaction_trigger.go` gets a one-line note above the
`GetByTrigger` call:

> `// Reads p.targetSubject.com per TS Player.getOpTrigger:993-995 (or`
> `// :1027-1029 for AP) via resolveTriggerTypeId — spellCom / useObj`
> `// override defaultTypeId when set.`

(Implementer adapts wording; one explicit reference to TS line range per
helper is sufficient.)

## §4. Tests

Per option-C strategy: helper unit test + per-site override test (8) +
two producer-side tests = 11 new tests. Existing `com=-1` regression pins
remain untouched and continue to cover the no-override branch.

### §4.1 Helper unit test

`modules/world/interaction_trigger_test.go::TestResolveTriggerTypeId`:

```go
func TestResolveTriggerTypeId(t *testing.T) {
    p := &Player{}
    p.targetSubject.com = -1
    if got := resolveTriggerTypeId(p, 42); got != 42 {
        t.Errorf("com=-1: got %d, want 42 (default)", got)
    }
    p.targetSubject.com = 7777
    if got := resolveTriggerTypeId(p, 42); got != 7777 {
        t.Errorf("com=7777: got %d, want 7777 (override)", got)
    }
    // Boundary: com=-1 still returns default even when default is also -1
    p.targetSubject.com = -1
    if got := resolveTriggerTypeId(p, -1); got != -1 {
        t.Errorf("com=-1 default=-1: got %d, want -1", got)
    }
}
```

### §4.2 SetInteraction com=0 canonicalisation

`modules/world/interaction_test.go::TestSetInteractionComZeroCanonicalisation`:

```go
func TestSetInteractionComZeroCanonicalisation(t *testing.T) {
    p, _ := newTestPlayer(t)
    p.targetSubject.com = 999 // stale prior value

    fake := fakeEntity{x: 100, z: 100, level: 0}
    p.SetInteraction(InteractionEngine, fake, 1, 0)
    if p.targetSubject.com != -1 {
        t.Errorf("com=0 canonicalisation: got %d, want -1 (TS PathingEntity.ts:520)", p.targetSubject.com)
    }
    // Sanity: positive com is preserved
    p.SetInteraction(InteractionEngine, fake, 1, 12345)
    if p.targetSubject.com != 12345 {
        t.Errorf("positive com: got %d, want 12345", p.targetSubject.com)
    }
    // Sanity: -1 sentinel is preserved
    p.SetInteraction(InteractionEngine, fake, 1, -1)
    if p.targetSubject.com != -1 {
        t.Errorf("-1 sentinel: got %d, want -1", p.targetSubject.com)
    }
}
```

Pattern matches existing `interaction_test.go::TestSetInteractionStoresComField`
(line 414) and `TestSetInteractionPassesMinusOneForNonComOps` (line 431):
both use `newTestPlayer(t)` + `fakeEntity{...}` (defined at line 445).
`fakeEntity` falls through the `default` arm of `SetInteraction`'s
target-type switch (interaction.go:79-86) — no faceEntity write — which
keeps the test focused on the com-store path.

### §4.3 OpPlayerU producer test

Update **existing** `TestHandleOpPlayerU_HappyPath` at
`handler_op_player_test.go:303-337` (currently asserts `com==-1`):

```go
// CHANGE assertion at lines 335-337 from:
if clicker.targetSubject.com != -1 { … }
// to:
if clicker.targetSubject.com != useObj {
    t.Errorf("targetSubject.com: got %d, want %d (useObj — NAI-62 producer fix)", clicker.targetSubject.com, useObj)
}
```

And update the test's doc-comment at lines 303-305 from `targetSubject.com
= -1 (useCom discarded)` to `targetSubject.com = useObj (NAI-62: useObj
threaded through SetInteraction for trigger-lookup override per TS
OpPlayerUHandler.ts:77 + Player.ts:993-995)`.

ADD a new test:
`handler_op_player_test.go::TestHandleOpPlayerUUseObjZeroCanonicalisation`
— invokes `handleOpPlayerU` with `useObj=0` and asserts post-call
`clicker.targetSubject.com == -1` (proves §3.1 canonicalisation flows
through the producer change).

Plan-author note: `useObj=0` may collide with empty-slot semantics in
`HasAt` validation. Implementer should pick a slot+inv setup where
`useObj=0` is a valid item entry (the inventory `Item.Id == 0` is a
legitimate item per `pkg/inventory`); if `HasAt(slot, 0)` rejects, the
test must seed `Item{Id: 0, Count: 1}` explicitly. If the existing
inventory code rejects `Id=0` as a sentinel, fall back to seeding
`useObj=1` and a separate test demonstrating `SetInteraction(com=0)`
canonicalisation directly (covered by §4.2 already).

### §4.4 Per-site override tests (8)

Six in `modules/world/interaction_trigger_test.go`, two in
`modules/world/player_interaction_trigger_test.go`. Pattern:

```go
func TestFireOpTriggerNpcOverridesTypeIdFromTargetSubjectCom(t *testing.T) {
    s, p := setupOpInteractionFixture(t)  // existing helper from interaction_trigger_test.go
    npc := spawnNpcAt(t, s, /* type, x, z */)
    p.target = npc
    p.targetOp = 1                        // OPNPC1 path
    p.targetSubject.com = 7777            // override
    placePlayerAdjacentTo(t, p, npc)      // operable distance

    // Register TWO scripts:
    //  - keyed at (OPNPC1, npcId, _)   ← would fire pre-fix
    //  - keyed at (OPNPC1, 7777, _)    ← fires post-fix
    defaultScript := registerScript(t, s, script.TriggerOpNpc1, npc.typeId, /*category*/ -1, "default")
    overrideScript := registerScript(t, s, script.TriggerOpNpc1, 7777, /*category*/ -1, "override")
    _ = defaultScript

    fireOpTriggerNpc(p, s, npc)

    // Assert which script ran. The simplest pin: the script-completion
    // hook attaches a marker in scriptState — fixtures already in
    // interaction_trigger_test.go do this via a sentinel script-id read
    // through a recorder. Implementer follows existing pattern; if no
    // recorder exists, attach one to the fixture.
    if got := s.lastFiredScript; got != overrideScript.ID {
        t.Errorf("script: got %d, want %d (override should win)", got, overrideScript.ID)
    }
}
```

Plan-author note: the script-completion recorder pattern needs grep
verification at plan-write time. Existing tests in
`interaction_trigger_test.go` (e.g. `TestFireOpTriggerNpc*`) likely use
either (a) a no-op script that increments a counter via a side-effect
opcode, (b) a script-completion callback registered via the test helper,
or (c) a marker field on `mockScriptCompletionRecorder`. **Plan B2 must
grep `interaction_trigger_test.go` for the existing dispatch-pin pattern
before plan-author writes the per-site assertion shape**, and codify the
exact pattern (or pre-write a new pattern if none exists). This blocks
B2 plan-author dispatch.

The 8 sites and their override-script keys:

| Site | Trigger | Default key | Override key | File |
|---|---|---|---|---|
| `fireOpTriggerNpc`     | `TriggerOpNpc1`     | `npc.typeId` | `7777` | interaction_trigger_test.go |
| `fireOpTriggerLoc`     | `TriggerOpLoc1`     | `loc.Type()` | `7778` | interaction_trigger_test.go |
| `fireApTriggerNpc`     | `TriggerApNpc1`     | `npc.typeId` | `7779` | interaction_trigger_test.go |
| `fireApTriggerLoc`     | `TriggerApLoc1`     | `loc.Type()` | `7780` | interaction_trigger_test.go |
| `fireOpTriggerObj`     | `TriggerOpObj1`     | `obj.Type`   | `7781` | interaction_trigger_test.go |
| `fireApTriggerObj`     | `TriggerApObj1`     | `obj.Type`   | `7782` | interaction_trigger_test.go |
| `fireOpTriggerPlayer`  | `TriggerOpPlayer1`  | `-1`         | `7783` | player_interaction_trigger_test.go |
| `fireApTriggerPlayer`  | `TriggerApPlayer1`  | `-1`         | `7784` | player_interaction_trigger_test.go |

Each test uses the slot-1 op variant (avoiding T/U sentinel paths to keep
the assertion focused on the override mechanism). Distinct override keys
per test prevent registry collisions.

### §4.5 Existing regression coverage (preserved free)

- `npc_test.go:175-176` — `targetSubject.com == -1` default for NPC-actor
  paths (out of NAI-62 scope; remains green).
- `interaction_test.go:412-440` — existing `SetInteraction` com-write
  pins. Note: `interaction_test.go:438` asserts `com == -1` after a
  call that passes `com=-1`. With §3.1's canonicalisation, this still
  passes because `com=-1` is preserved (only `com=0` flips). Verify
  green post-§3.1.
- `handler_op_player_test.go:70` (Op1-5 happy-path), `:191` (OpT happy),
  `:335` (OpU happy — **MUST UPDATE** per §4.3).
- `handler_opnpc_test.go:257, 375`, `handler_oploc_test.go:312, 473`,
  `handler_opobj_test.go:256, 370` — T/U producer pins (`com == spellCom`
  for T, `com == -1` for U). Unchanged: T-handlers continue to set
  `com=spellCom`; U-handlers (Loc/Npc/Obj) continue to pass `-1`. Green
  post-fix.

## §5. Bundles

### §5.1 Bundle 1 — Foundation

**Scope:**
- `interaction.go::SetInteraction` `com=0 → -1` canonicalisation (§3.1).
- `interaction.go` doc + `player.go:104-110` field doc updates (§3.1).
- `interaction_trigger.go::resolveTriggerTypeId` helper definition (§3.3).
  Helper is added but NO callsites are wired yet — B1 lands the helper
  unused.
- `handler_op_player.go::handleOpPlayerU` producer fix (§3.2) + doc trailer.
- New tests: §4.1 (helper unit, 1 test), §4.2 (com=0 canonicalisation,
  1 test), §4.3 (OpPlayerU happy update + new useObj=0 test, 2 changes).

**Net:** 4 production edits (~20 LOC), 4 test changes (~80 LOC), no
observable trigger-dispatch behaviour change (helper is unwired; producer
fix only affects `targetSubject.com` storage and downstream `TargetSubjectCom()`
script-opcode reads, which existing scripts may or may not consult — no
production scripts read it for player targets today).

**TDD flow:** RED on each new test (B1 should compile but new tests fail
without §3.1/§3.2); GREEN after applying §3.1 + §3.2 + helper. The helper
unit test (§4.1) is GREEN immediately after the helper definition lands.

**Commit:** single commit per bundle convention.

```
feat(world): NAI-62 B1 — targetSubject.com canonicalisation, resolveTriggerTypeId helper, OpPlayerU producer fix

Closes the com=0 → -1 truthy boundary (TS PathingEntity.ts:520) by
canonicalising SetInteraction's com store; introduces the
resolveTriggerTypeId helper that B2 will wire into all 8 player-side
trigger-lookup sites; fixes the OpPlayerU producer-side useObj drop
(TS OpPlayerUHandler.ts:77).

Helper is added but unwired — no observable trigger-dispatch change
in this commit. B2 lands the consumer fan-out.

Tests: TestResolveTriggerTypeId (helper unit), TestSetInteractionComZeroCanonicalisation,
TestHandleOpPlayerUUseObjZeroCanonicalisation; TestHandleOpPlayerU_HappyPath
updated to assert targetSubject.com == useObj.

Refs: TS Player.ts:993-997, 1027-1031; PathingEntity.ts:520;
OpPlayerUHandler.ts:77.
```

### §5.2 Bundle 2 — Consumer fan-out

**Dispatched against B1 at HEAD.** Pre-flight per `controller_preflight.md`
must re-grep:
1. `resolveTriggerTypeId` exists in `interaction_trigger.go` post-B1.
2. The 8 callsite line numbers from §2.2 still match (B1 didn't shift them).
3. `interaction_trigger_test.go` script-completion recorder pattern
   (per §4.4 plan-author note) — codify the exact pattern in the B2 plan
   before dispatch.

**Scope:**
- 8 callsite edits per §3.4.
- 8 doc-comment trailers per §3.5.
- 8 new per-site override tests per §4.4.

**Net:** 8 production edits (8 LOC), 8 doc additions (~16 LOC), 8 tests
(~250 LOC).

**TDD flow:** RED on each per-site override test (the script-keyed-by-
override-com is registered but unreachable until the callsite edit
lands); GREEN after §3.4 edit. One callsite at a time per
`enumerate_all_sites.md` discipline.

**Per-site enumeration discipline.** Per `enumerate_all_sites.md`, plan
author must explicitly enumerate the 8 sites in a checklist; controller
re-greps post-commit:

```bash
# Should list exactly 8 hits, all using resolveTriggerTypeId:
rg -n "GetByTrigger\(trigger," modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
```

Any site that still passes a raw `npc.typeId` / `loc.Type()` / `obj.Type`
/ `-1` instead of `resolveTriggerTypeId(p, …)` is a missed site — fail
the bundle and re-dispatch.

**Commit:** single commit.

```
feat(world): NAI-62 B2 — wire resolveTriggerTypeId into all 8 player-side trigger lookups

Threads p.targetSubject.com through to script dispatch at the 6 fire
helpers in interaction_trigger.go (Op/Ap × Npc/Loc/Obj) and the 2 in
player_interaction_trigger.go (Op/Ap × Player), matching TS
Player.getOpTrigger:993-997 / getApTrigger:1027-1031.

Behavioural change: spell-on-X clicks (OpLocT/OpNpcT/OpObjT/OpPlayerT)
now key trigger lookup by spellCom; OpPlayerU clicks key by useObj.
Op1-5 / Ap1-5 / U-handler clicks unchanged (com=-1).

Tests: 8 new per-site override tests, one per fire helper.

Refs: TS Player.ts:993-997, 1027-1031.
```

### §5.3 Close commit

After B2 merges, single chore commit appends `## NAI-62 — CLOSED YYYY-MM-DD`
to `nai_followups.md` per `close_commit_memory_trailer.md`:

```
chore(close): NAI-62 — targetSubject.com → typeId dispatch override

Two-bundle sub-spec:
- B1: SetInteraction com=0 canonicalisation, resolveTriggerTypeId helper, OpPlayerU producer fix
- B2: 8-site consumer fan-out

All player-side trigger-lookup sites now match TS Player.getOpTrigger /
getApTrigger override semantics.

Closes memory: nai_followups.md → "## NAI-61 — CLOSED 2026-05-01" →
"NAI-62 candidate" block.
```

## §6. Verification (per bundle)

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Per `verify_implementer_claims.md`, controller re-runs each command
post-commit before merging — implementer-side green is necessary but
not sufficient.

## §7. Out of scope

1. **NPC-actor trigger-lookup sites** in `npc_interaction_trigger.go`. TS
   `Npc.ts` has no analogue override (`getOpTrigger` is a `Player`-only
   method); NPC-side dispatch keys on `target.Type` / `target.typeId`
   directly. No divergence to fix.
2. **`getInteractionTrigger` consolidation.** TS `Player.getOpTrigger` and
   `getApTrigger` are near-duplicates. goscape's 8 fire helpers are not.
   Consolidating into a single dispatcher is a separate refactor; out of
   scope for the fidelity port.
3. **`p_target_subject` script opcode** or related script-opcode
   producers/consumers. `TargetSubjectCom()` already exists
   (`player_script.go:785`) and its return value is now `useObj` for
   OpPlayerU-anchored interactions post-§3.2. If any script opcode reads
   it, behaviour shifts naturally — but no opcode plumbing changes here.
4. **OpPlayerT producer**: already passes `spellCom` via `SetInteraction`;
   no producer change needed.
5. **Validation that any specific data-pack script is now reachable.**
   The behavioural change is a key-lookup correction; whether the
   LostCityRS data pack contains scripts at the new keys is a runtime
   property of the script registry. If existing tests pin
   `default-typeId-keyed scripts run for spell clicks`, those tests pin
   the bug and must be updated as part of B2. Implementer surveys
   for these via `rg "TriggerOp(Loc|Npc|Obj|Player)T" modules/world/*_test.go`
   before B2 dispatch.

## §8. Tracker delta

- **Retire:** the `## NAI-61 — CLOSED 2026-05-01` → `### NAI-62 candidate`
  block in `nai_followups.md` (memory line 3042) is closed by this
  sub-spec. The close commit (§5.3) replaces it with a `## NAI-62 — CLOSED
  YYYY-MM-DD` block summarising the two bundles.
- **Add:** none. No new follow-ups expected unless B2 tests surface a
  data-pack or registry assumption that wasn't documented at HEAD.

## §9. Definition of done

- [ ] B1 production edits per §3.1, §3.2, §3.3 (helper definition only).
- [ ] B1 tests per §4.1, §4.2, §4.3 (3 new + 1 updated existing).
- [ ] B1 commit per §5.1 shape, all `modules/world/...` tests green,
      `go vet` clean, `go build ./...` succeeds.
- [ ] B2 controller pre-flight re-greps per §5.2 (3 checks).
- [ ] B2 production edits per §3.4, §3.5 (8 callsites + 8 doc trailers).
- [ ] B2 tests per §4.4 (8 per-site override tests).
- [ ] B2 commit per §5.2 shape, post-commit `rg "GetByTrigger\(trigger,"`
      shows exactly 8 hits all routing through `resolveTriggerTypeId`.
- [ ] All `modules/world/...` tests green, `go vet` clean, `go build ./...`
      succeeds.
- [ ] Close commit per §5.3, `nai_followups.md` updated.
