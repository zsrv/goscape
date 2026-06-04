# NAI-138 Stage 2 — Smoke handoff #2 (fix close gate)

**Date:** 2026-05-09
**Predecessor:** Bundle β.3 fix at `4aa892b` (test cleanup follow-up to `8fc06d5`).
**Plan:** `docs/superpowers/plans/2026-05-09-nai-138-stage-2-encoder-defect.md` "Smoke handoff #2".
**Binding context:** `docs/superpowers/handoffs/2026-05-09-nai-138-stage-2-binding.md`.

## What changed since smoke #1

`pkg/objtype/varptype.go` — `LoadVarpTypes` now also loads `data/pack/client/config` Jagfile, and `parseVarpTypes(server, clientJag)` decodes both streams per-id (mirroring `loctype.go`). The clientcode opcode (case 5) lives only in the client stream, so prior to this fix `varpTypes.RunID` always defaulted to 0 and `(*Player).RunVarpID()` returned 0.

Production-cache integration test `TestLoadVarpTypes_ProductionCacheRunIDIs173` confirms post-fix: `RunID == 173`, `Configs[173].ClientCode == 7`, `ConfigNames["option_run"] == 173`.

The G1/G2/G3 probes from Bundle β.1 are still wired (permanent diagnostics per `nodedebug_gateway_probe_pattern`). Smoke #2's filtered log will let us confirm the wire path now carries varp 173.

## What I need you to do

### 1. Build + run

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o /tmp/goscape-nai138-fix ./cmd/goscape
/tmp/goscape-nai138-fix --config.file config.yaml 2>&1 | tee /tmp/nai138-fix.log
```

### 2. Re-run the energy=0 path (Run A from smoke #1)

1. Log in to the Java client.
2. Confirm the run-mode button is ON.
3. Walk continuously until run-energy depletes to **0%**.
4. **Observe:** does the run-mode button visually de-toggle now (transitions to walking pose) when the energy hits 0?

### 3. Capture filtered log

```bash
grep -E "nai138\.(p_run|update_energy\.zero|write_varp)" /tmp/nai138-fix.log > /tmp/nai138-fix-filtered.log
```

### 4. Hand back

- Visual symptom: `de-toggles` (success) or `stays-on` (still broken).
- The filtered log so we can confirm `varp_id=173` and `payload_hex=00ad00` (or `00ad01` for the click→on transition).

## Decision tree (per spec §8 / plan)

| Outcome | Close action |
|---|---|
| Button visually de-toggles at energy=0 | **PRIMARY met → close NAI-138.** Memory routing per plan §"Memory routing at Stage 2 close". |
| Button stays stuck-on, click toggles still work | Cascade-blocker — re-open Bundle β.1 with sharper probes per spec §10 R2 (binding may have under-counted blockers). |
| New regression (click toggles broken, login fails, etc.) | Revert `8fc06d5`+`4aa892b`; open NAI-138 stretch with regression-pin test. |

## Stop point

Controller pauses here. Resume by pasting the filtered log + symptom note. On a clean PRIMARY-met outcome, I'll author the Stage 2 close commit + memory entries.
