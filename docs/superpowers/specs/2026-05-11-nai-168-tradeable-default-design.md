# NAI-168 — `ObjType.Tradeable` default port (compressed cadence)

**Status:** spec written 2026-05-11. Compressed cadence — single combined spec+plan doc, no separate plan file (per `compressed_cadence` memory).

**Tech stack:** Go 1.26+ (per `go_version` memory).

**Lineage:** Retires `nai_followups.md NAI-152-D-OBJTYPE-TRADEABLE-DEFAULT` (opened end-of-plan reviewer of NAI-152 B1, `c814d99..f6144f6`; pre-dates B1).

## 1. Goal

Port TS class-field default `tradeable = true` (`ObjType.ts:177`) into goscape's `NewObjType` constructor (`pkg/objtype/objtype.go:307`). One-line struct-literal change + 3 unit pins. PRIMARY: `(*ObjType).Tradeable` defaults to `true` in goscape, matching TS.

## 2. TS source of truth

`/home/owner/Code/github.com/LostCityRS/Engine-TS/src/cache/config/ObjType.ts`:

| Line | Code | Effect |
|---|---|---|
| 177 | `tradeable = true;` | **Class-field default** — the divergence target. |
| 211 | `this.tradeable = false;` | Decode `case 15` flips to false. Already mirrored at goscape `objtype.go:229`. |
| 297 | `this.tradeable = link.tradeable;` | Cert-template inherits from link. Already mirrored at goscape `objtype.go:186`. |
| 57 / 61 | `config.tradeable = false;` | Post-decode F2P-members fixup. Already mirrored at goscape `objtype.go:103/107`. |

Decode `case 200` setting `tradeable = true` exists in both TS and goscape (`objtype.go:293`); not affected.

## 3. Goscape state at HEAD `866cb30`

`pkg/objtype/objtype.go:307-336` — `NewObjType` struct literal omits `Tradeable`. Go zero-value `false` propagates to:

- `pkg/script/handlers_config.go:592` — `OC_TRADEABLE` script handler returns 0 to content for un-decoded items (TS-faithful: 1).
- `pkg/script/handlers_inv.go:1762` — INV_MOVEITEM receiver-routing gate: `!objType.Tradeable ? fromPlayer : toPlayer`. Un-decoded items wrongly route to `fromPlayer` (TS-faithful: `toPlayer`).
- `pkg/script/handlers_inv.go:1869` — `InvDropAll` SCOPE_PERM wealth-log: `tradeable = objType.Tradeable`. Recorded bit is wrong for un-decoded items.

All three read sites use the bit correctly; only the *initial default* is divergent. Porting the default is purely TS-fidelity restoration with no logic-side risks.

## 4. Production change (1 LOC)

Add `Tradeable: true,` to the `NewObjType` struct literal at `pkg/objtype/objtype.go:307`. Place adjacent to other class-field-style defaults (e.g., `Cost: 1`, `RespawnRate: 100`).

```go
// pkg/objtype/objtype.go (NewObjType)
return &ObjType{
    ConfigType: ConfigType{ID: id},
    // …existing fields…
    Cost:        1,
    // …existing fields…
    RespawnRate: 100, // defaults to 1 minute
    Tradeable:   true, // TS ObjType.ts:177 class-field default
    Op:          []string{"", "", "Take", "", ""},
    IOp:         []string{"", "", "", "", "Drop"},
    // …existing fields…
}
```

(Implementer: pick the position consistent with existing field grouping; the comment is the only required marker.)

## 5. Test plan (3 pins)

All in `pkg/objtype/objtype_test.go`. Sibling-style with existing `TestNewObjTypeOpDefaults` (line 31), `TestApplyPostDecodeFixupsDummyItemZeroPreservesTradeable` (line 178), and the existing decode-code unit tests.

### 5.1 `TestNewObjType_TradeableDefaultsTrue`

```go
func TestNewObjType_TradeableDefaultsTrue(t *testing.T) {
    ot := NewObjType(123)
    if !ot.Tradeable {
        t.Fatalf("NewObjType(123).Tradeable: got false, want true (TS ObjType.ts:177 class-field default)")
    }
}
```

Pins the default. Sibling of `TestNewObjTypeOpDefaults`.

### 5.2 `TestObjTypeDecode_Code15FlipsTradeableFalse`

```go
func TestObjTypeDecode_Code15FlipsTradeableFalse(t *testing.T) {
    ot := NewObjType(0)
    if !ot.Tradeable {
        t.Fatalf("precondition: NewObjType.Tradeable expected true")
    }
    if err := ot.Decode(15, packet2.NewPacket(nil)); err != nil {
        t.Fatalf("Decode(15): unexpected error: %v", err)
    }
    if ot.Tradeable {
        t.Fatalf("after Decode(15): Tradeable: got true, want false (TS ObjType.ts:211)")
    }
}
```

Pins regression-direction: opcode 15 still flips a true-default to false. Implementer confirms `packet2.NewPacket(nil)` matches the package import alias used in the existing `_test.go` (look at `TestObjTypeDecodeOpHiddenCoercedToEmpty` at line 10 for the canonical fixture pattern).

### 5.3 `TestObjTypeDecode_Code200KeepsTradeableTrue`

```go
func TestObjTypeDecode_Code200KeepsTradeableTrue(t *testing.T) {
    ot := NewObjType(0)
    if err := ot.Decode(200, packet2.NewPacket(nil)); err != nil {
        t.Fatalf("Decode(200): unexpected error: %v", err)
    }
    if !ot.Tradeable {
        t.Fatalf("after Decode(200): Tradeable: got false, want true (TS ObjType.ts case 200)")
    }
}
```

Pins idempotence: opcode 200 still re-asserts true.

## 6. Tests intentionally NOT included (with rationale)

| Skipped test | Rationale |
|---|---|
| Cert-template inheritance round-trip (link with `Tradeable=true/false` → cert via `toCertificate`) | `toCertificate` (`objtype.go:170`) line `ot.Tradeable = link.Tradeable` is unchanged by this sub-spec. The new default propagates implicitly. Not worth the fixture surface for a code path NAI-168 doesn't touch. |
| Read-site smoke (OC_TRADEABLE / INV_MOVEITEM / InvDropAll end-to-end) | Existing tests in `handlers_config_test.go` / `handlers_inv_test.go` explicitly stub `objType.Tradeable = true/false` for each branch; the new default doesn't change their behavior. Risk of false-pass through omitted setup is low because the existing tests use direct field assignment. |
| F2P-members post-decode fixup (`applyPostDecodeFixups`) | Already pinned by `TestApplyPostDecodeFixupsDummyItemZeroPreservesTradeable` (line 178) and friends. New default makes those tests stronger by removing the implicit assumption that pre-fixup Tradeable was already true via explicit setup. |

Per `helper_as_oracle_test_anti_pattern` and `audit_full_method_against_ts`: the 3 pins above are direct property assertions, not call-output equality through helpers. No oracle inversion.

## 7. Deviations expected

None. TS-faithful one-line port.

## 8. Tracker retirement

`nai_followups.md` lines 35-41 (`### NAI-152-D-OBJTYPE-TRADEABLE-DEFAULT — pending NAI-N+x`) struck through with `RETIRED 2026-05-11 by NAI-168` annotation. Close commit carries `Closes memory: nai_followups.md NAI-152-D-OBJTYPE-TRADEABLE-DEFAULT` trailer per `close_commit_memory_trailer`.

## 9. Risk register

| ID | Risk | Severity | Mitigation |
|---|---|---|---|
| R1 | Hidden read site somewhere outside `pkg/script` flips behavior on the new default. | Low | Pre-impl `rg -n "\.Tradeable" pkg/ modules/` against HEAD already enumerated 3 production read sites (handlers_config.go:592, handlers_inv.go:1762, handlers_inv.go:1869) plus 2 write sites (postdecode line 103/107) and the cert-template line 186. All TS-faithful. Re-grep at impl confirms. |
| R2 | Existing test stubs an `ObjType{}` literal (zero-value Tradeable=false) and relies on it. | Low | `rg -n "ObjType\{" pkg/ modules/` at impl-time enumerates such literals. Per `plan_enumerate_struct_literals`, audit each: if a test means "untradeable", it should explicitly set `Tradeable: false`. Implementer fixes any that break. (Expected: zero — production code paths use `s.Configs.ObjType(id)` which returns either a decoded or `nil` ObjType, not bare literals.) |
| R3 | `packet2.NewPacket(nil)` panics or rejects nil for the `Decode(15)` / `Decode(200)` paths. | Low | Codes 15 and 200 read no bytes from the packet (`case 15: ot.Tradeable = false`, `case 200: ot.Tradeable = true`). If `NewPacket(nil)` rejects, use `packet2.NewPacket([]byte{})` per the `TestObjTypeDecodeOpHiddenCoercedToEmpty` pattern at line 10. Implementer chooses based on existing fixture style. |

## 10. Cadence + commits

Per `compressed_cadence`: one combined spec+plan doc; single TDD commit pair.

| Step | Commit | Body |
|---|---|---|
| Spec | `docs(spec): NAI-168 — ObjType.Tradeable default port (NAI-152-D retirement)` | This file. |
| T1 RED | `test(objtype): NAI-168 T1 — Tradeable default pins (RED)` | Adds the 3 pins from §5; T1 confirms failure on the default test (gets `false`, wants `true`). |
| T2 GREEN | `feat(objtype): NAI-168 T2 — port TS Tradeable=true class-field default (GREEN)` | Adds `Tradeable: true,` to `NewObjType`. T1's failures resolve; T1.2 and T1.3 stay green (validate decode-flip + decode-idem). |
| Close | `chore(close): NAI-168 — ObjType.Tradeable default port (NAI-152-D retirement)` | Strikes through the tracker entry; trailer `Closes memory: nai_followups.md NAI-152-D-OBJTYPE-TRADEABLE-DEFAULT`. |

## 11. Verification protocol (per `verification_before_completion`)

Pre-T1: snapshot HEAD `866cb30` clean `go test ./pkg/objtype/...` baseline (`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/...`).

Expected RED-phase outcomes after T1 commit (NewObjType still defaults Tradeable=false):

| Pin | RED outcome | Notes |
|---|---|---|
| `TestNewObjType_TradeableDefaultsTrue` | **FAIL** | Canonical RED signal: `got false, want true`. |
| `TestObjTypeDecode_Code15FlipsTradeableFalse` | **FAIL on precondition** | The precondition guard `if !ot.Tradeable { t.Fatalf("precondition: expected true") }` fires at RED. By design — keeps T1.2 honest about its dependency on the new default. |
| `TestObjTypeDecode_Code200KeepsTradeableTrue` | **PASS** | Decode(200) sets Tradeable=true regardless of starting value. |

Post-T2 (after adding `Tradeable: true,` to `NewObjType`): all three pins PASS.

Verification: re-run `go test ./pkg/objtype/...` AND `go test ./...` (broader regression). Both clean.

Per `verify_implementer_claims`: controller verifies fresh `git show <SHA>` post-T1 and post-T2 to confirm content matches stated diff. No reliance on stale IDE diagnostics.

## 12. Pattern memories applied

- `compressed_cadence` — combined spec+plan doc; single TDD commit pair (well within ≤~15 LOC threshold for production change).
- `runescript_cadence` — preserved spec → impl → close phasing despite cadence compression.
- `controller_preflight` — pre-impl grep+Read pass against HEAD `866cb30` already enumerated all read/write sites + TS source; re-grep at impl-time before T2.
- `verify_implementer_claims` — fresh `go test` and `git show` at every commit boundary.
- `plan_enumerate_struct_literals` — R2 mitigation: enumerate `ObjType{}` literals before T2 to catch incidental zero-value-Tradeable dependents.
- `audit_full_method_against_ts` — R1 mitigation: TS source of truth audited line-by-line for class field, decode-case-15, decode-case-200, post-decode fixups (lines 57/61), and cert-template inheritance (line 297).
- `close_commit_memory_trailer` — close commit carries `Closes memory:` trailer for NAI-152-D retirement provenance.
- `dead_api_polish` (preventive) — sub-spec adds no new APIs; nothing to audit at close.

## 13. Out of scope

- WealthEvent observability tail (`NAI-162-D-WEALTHEVENT-IN-MEMORY-ONLY`): orthogonal subsystem; queued for NAI-N+x.
- NAI-121 combat residuals: investigation cadence; queued separately.
- Cert-template `toCertificate` audit: line 186 is unchanged; orthogonal.
- F2P-members post-decode fixup audit (`applyPostDecodeFixups`): already pinned by existing tests.

## 14. Smoke handoff

Post-close smoke is OPTIONAL — read-site behavior changes are local and deterministic, fully covered by unit pins. If user wants a Java-client smoke pass, expected observation: any item that hits a content path reading `oc_tradeable` (e.g., shop-tab eligibility, GE listing eligibility) on a config-loaded item should see no regression; un-decoded items (which shouldn't exist in a healthy cache) report tradeable=true to content. No new server-log warnings/panics expected.

If a smoke pass surfaces an adjacent unhandled-opcode issue ≤30 LOC per `smoke_surfaces_adjacent_divergences`, route into NAI-168 in-scope-stretch; else open NAI-169 separately.
