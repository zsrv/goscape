# NAI-63 — `opcalled = true` ordering audit (12 Op*-handler sites)

> **TS-faithfulness gate.** Pure ordering fix across all 12 Op*-handler sites
> in `modules/world/handler_op{loc,obj,npc,_player}.go`. Each site currently
> writes `p.opcalled = true` BEFORE `p.SetInteraction(...)`; TS does the
> inverse (`setInteraction(...)` then `opcalled = true`) in every one of
> the 12 corresponding `Op*[T|U]?Handler.ts` files. No new deviations. No
> new APIs. Zero observable behaviour change today (no production READ of
> `opcalled` exists). Compressed-cadence sub-spec (combined spec+plan); see
> `compressed_cadence.md`.

## §1. Origin

Carved out at NAI-62 close as "Out-of-scope follow-up (surfaced during B1
review)" under `nai_followups.md` → `## NAI-62 — CLOSED 2026-05-01`:

> OpPlayerU handler sets `p.opcalled = true` BEFORE `SetInteraction`,
> while TS `OpPlayerUHandler.ts:79-80` does the inverse (`setInteraction`
> then `opcalled = true`). No current consumer reads `opcalled` inside
> `SetInteraction`, so behaviour is identical today, but the ordering
> divergence is unlabeled. Candidate for a future audit pass.

NAI-62 framed this as the OpPlayerU site only. A pre-spec grep against TS
shows the same divergence at all 12 Op*-handler sites — the audit pass
must cover the whole family, not the single carve-out site, per
`true_to_ts_gate.md` and `enumerate_all_sites.md`.

## §2. The divergence (verified at HEAD `a5d0282`)

TS source: every `Op*[T|U]?Handler.ts` calls `setInteraction(...)` THEN
`opcalled = true`, immediately before `return true`. Reference:

| TS file | Lines | Order |
|---|---|---|
| `OpLocHandler.ts` | 46-47 | `setInteraction(ENGINE, loc, trigger)` → `opcalled = true` |
| `OpLocTHandler.ts` | 49-50 | `setInteraction(ENGINE, loc, APLOCT, spellComId)` → `opcalled = true` |
| `OpLocUHandler.ts` | 79-80 | `setInteraction(ENGINE, loc, APLOCU)` → `opcalled = true` |
| `OpObjHandler.ts` | 46-47 | `setInteraction(ENGINE, obj, trigger)` → `opcalled = true` |
| `OpObjTHandler.ts` | 49-50 | `setInteraction(ENGINE, obj, APOBJT, spellComId)` → `opcalled = true` |
| `OpObjUHandler.ts` | 79-80 | `setInteraction(ENGINE, obj, APOBJU)` → `opcalled = true` |
| `OpNpcHandler.ts` | 48-49 | `setInteraction(ENGINE, npc, trigger)` → `opcalled = true` |
| `OpNpcTHandler.ts` | 51-52 | `setInteraction(ENGINE, npc, APNPCT, spellComId)` → `opcalled = true` |
| `OpNpcUHandler.ts` | 81-82 | `setInteraction(ENGINE, npc, APNPCU)` → `opcalled = true` |
| `OpPlayerHandler.ts` | 38-39 | `setInteraction(ENGINE, other, trigger)` → `opcalled = true` |
| `OpPlayerTHandler.ts` | 47-48 | `setInteraction(ENGINE, other, APPLAYERT, spellComId)` → `opcalled = true` |
| `OpPlayerUHandler.ts` | 77-78 | `setInteraction(ENGINE, other, APPLAYERU, useObj)` → `opcalled = true` |

Goscape source: every Op*-handler does the inverse (`opcalled = true` →
`SetInteraction(...)`):

| Goscape site | Function | Lines (HEAD `a5d0282`) |
|---|---|---|
| `handler_oploc.go` | `handleOpLoc` | 89-90 |
| `handler_oploc.go` | `handleOpLocT` | 176-177 |
| `handler_oploc.go` | `handleOpLocU` | 293-294 |
| `handler_opobj.go` | `handleOpObj` | 77-78 |
| `handler_opobj.go` | `handleOpObjT` | 161-162 |
| `handler_opobj.go` | `handleOpObjU` | 275-276 |
| `handler_opnpc.go` | `handleOpNpc` | 76-77 |
| `handler_opnpc.go` | `handleOpNpcT` | 157-158 |
| `handler_opnpc.go` | `handleOpNpcU` | 263-264 |
| `handler_op_player.go` | `handleOpPlayer` | 55-56 |
| `handler_op_player.go` | `handleOpPlayerT` | 121-122 |
| `handler_op_player.go` | `handleOpPlayerU` | 218-219 |

Each site is two consecutive lines of the canonical shape:

```go
p.opcalled = true
p.SetInteraction(InteractionEngine, <target>, <trigger>, <com>)
```

(OpObjU and OpLocU additionally set `targetSubject.{typ,x,z,level}` after
`SetInteraction`; that block stays in place — only the `opcalled` /
`SetInteraction` pair swaps.)

**Practical delta today.** Zero. `opcalled` has zero production READ sites
(`grep -rn "opcalled" modules/world/ pkg/ | grep -v _test.go | grep -v "= true\|= false\|opcalled bool"` yields only the field declaration at `player.go:199` and the comment at `handler_opobj.go:20`). The ordering swap is observably equivalent for every script-dispatch path through the engine today.

**Why it matters.** The fidelity gap is a latent landmine: any future port
that adds an `opcalled`-reader inside `SetInteraction` (e.g., a
`targetSubject` snapshot that wants to know whether the click already
fired its op trigger) would silently observe the wrong value with
goscape's current ordering. Closing the gap now while the file context is
fresh from NAI-60/61/62 is cheaper than re-discovering it later.

## §3. The fix — twelve mechanical 2-line swaps

For each of the 12 sites, swap the two consecutive lines so that
`SetInteraction(...)` precedes `opcalled = true`. The canonical post-swap
shape is:

```go
p.SetInteraction(InteractionEngine, <target>, <trigger>, <com>)
p.opcalled = true
```

### §3.1 `handler_oploc.go`

Three sites (lines 89-90, 176-177, 293-294). Each is:

**Before:**
```go
p.opcalled = true
p.SetInteraction(InteractionEngine, loc, <trigger>, <com>)
```

**After:**
```go
p.SetInteraction(InteractionEngine, loc, <trigger>, <com>)
p.opcalled = true
```

For `handleOpLocU` (293-294), the trailing `p.targetSubject.{typ,x,z,level}`
assignments at 295-298 are unchanged.

### §3.2 `handler_opobj.go`

Three sites (lines 77-78, 161-162, 275-276). Same shape with `obj` in
place of `loc`. For `handleOpObjU` (275-276), trailing `p.targetSubject.*`
at 277-280 unchanged.

### §3.3 `handler_opnpc.go`

Three sites (lines 76-77, 157-158, 263-264). Same shape with `npc` in
place of `loc`. No trailing `targetSubject` assignments to worry about
(NPCs use a different snapshot path).

### §3.4 `handler_op_player.go`

Three sites (lines 55-56, 121-122, 218-219). Same shape with `other` in
place of `loc`.

## §4. Doc-comment refreshes

Most function-level "On success:" doc-comments today narrate
`ClearPendingAction → SetInteraction → ...` and omit `opcalled` entirely
— after the swap, those comments remain accurate (with `opcalled = true`
implicit-trailing). Only one comment explicitly mis-orders the pair:

**Mandatory (must edit):**

- `handler_opobj.go:20-21` (`handleOpObj` doc-comment) currently reads:
  ```go
  // On success: ClearPendingAction → opcalled=true →
  // SetInteraction(Engine, obj, op, -1) → targetSubject snapshot.
  ```
  Change to:
  ```go
  // On success: ClearPendingAction → SetInteraction(Engine, obj, op, -1)
  // → opcalled=true → targetSubject snapshot.
  ```

**Optional (audit-trail polish):** explicitly append `→ opcalled=true` to
each of the 11 other "On success:" blocks so the new ordering is
grep-discoverable across the whole handler family. Implementer may include
or omit at their discretion — the cost is trivial; the value is purely
documentary.

## §5. Tests

**No new tests.** Existing tests assert post-state `p.opcalled == true`
after each handler returns successfully — that assertion is unchanged by
the swap. A "field-set ordering" test would require a `SetInteraction`
test seam that captures `opcalled` at call time, which adds fixture
machinery for a divergence with no current consumer. Per
`true_to_ts_gate.md` discipline, the production swap + grep-discoverable
TS reference (this spec) is the audit trail.

The verification gate is `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go
test ./...` green at HEAD post-swap.

## §6. Cadence

**Compressed** per `compressed_cadence.md`: combined spec+plan in this
single document; one feature commit (12 swaps + 1 mandatory doc-comment
edit + optional 11 doc-comment polish edits); one close commit. No formal
whole-impl reviewer dispatch (mechanical change, zero behaviour delta,
single-file-family scope).

LOC budget: ~24 production-line moves + 1 mandatory comment edit
(2-3 LOC). Optional polish adds ~11 short comment-trailer edits (~11 LOC).
Total: 24-38 LOC.

This is borderline against the `compressed_cadence.md` ≤~15 LOC heuristic
but every line is a `git diff -U0 | wc -l`-trivial move; there is no
test-design surface, no API surface, and no review surface beyond visual
diff inspection. Compressed remains the right call. NAI-56 (`0259d56`,
~12 LOC + 4 doc-comment refreshes) and NAI-52 (`6fdde4e`, ~13 LOC + helper
extraction) are the precedents; NAI-61 (`75d3f4b`) was a similar-shape
ordering audit but ran full-cadence — the choice there was discretionary,
not forced.

## §7. Tasks

### T1 — Apply 12 swaps + mandatory doc-comment edit + verify

- For each of the 12 sites enumerated in §2, swap the two consecutive
  lines so `SetInteraction(...)` precedes `opcalled = true`.
- Edit `handler_opobj.go:20-21` per §4 (mandatory).
- Optionally apply the 11 polish doc-comment trailers per §4.
- Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -40`
- Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- Pre-commit grep: `grep -nB1 -A1 "p.opcalled = true\|p\.SetInteraction" modules/world/handler_op{loc,obj,npc,_player}.go`
  — every `p.opcalled = true` line should be IMMEDIATELY PRECEDED by a
  `p.SetInteraction(...)` line, not followed by one. Zero hits of the
  inverse pattern.
- Commit with message body referencing the 12 sites + TS line citations
  per §2 (use a HEREDOC and `--no-gpg-sign`).

### T2 — Close commit

- Update `nai_followups.md`:
  - Mark the "Out-of-scope follow-up (surfaced during B1 review)" bullet
    under `## NAI-62 — CLOSED 2026-05-01` as resolved-by-NAI-63 with
    commit reference.
  - Add a new `## NAI-63 — CLOSED 2026-05-01` section (rollup body
    matching the NAI-56 / NAI-52 compressed-cadence template: scope,
    cadence, close commit, follow-ups closed, deviations opened/closed,
    deviation tally unchanged).
- Commit with `Closes memory:` trailer per `close_commit_memory_trailer.md`
  if any memory entries are seeded (none expected — see §8).

## §8. Memory deltas

None expected. The audit doesn't surface a new gotcha pattern; it's a
direct application of `true_to_ts_gate.md` + `enumerate_all_sites.md`
(both already in MEMORY.md). The only NAI-63-specific note worth
preserving is the broadened scope vs the NAI-62 carve-out (carve-out
named OpPlayerU only; audit covered all 12) — captured in §1 above and in
the close commit body, not in a new memory entry.

## §9. Out-of-scope

- Non-Op*-handler `opcalled = true` writers — the grep at §2 confirms
  there are no others in production code.
- Adding an `opcalled`-reader inside `SetInteraction` or any consumer —
  that's a separate port whose owner is whichever future sub-spec needs
  the value.
- Audit of any other `Player.*` field with similar
  set-before-call-site-that-might-read-it ordering — out of scope for
  NAI-63; would need its own brainstorm if a candidate is identified.
