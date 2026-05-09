# NAI-138 Stage 2 — Smoke handoff #1 (probe binding)

**Date:** 2026-05-09
**Predecessor:** Stage 2 Bundle β.1 closed at `e29f301` (G3 fixture polish). Probe commits: `5fbd62c` (G1), `c5001c4` (G2), `be71d8c` (G3), `e29f301` (G3 fixture cleanup).
**Plan:** `docs/superpowers/plans/2026-05-09-nai-138-stage-2-encoder-defect.md` Bundle β.1.SMOKE.
**Predecessor handoff:** `docs/superpowers/handoffs/2026-05-09-nai-138-stage-1-binding.md`.

## What's in place

Three NodeDebug-gated `s.log.Info` gateways are now wired across the run-varp emit pathway, each with a unique `nai138.` key prefix for grep:

| Probe | Key | Site | Captures |
|---|---|---|---|
| G1 | `nai138.p_run` | `pkg/script/handlers_player.go` `handlePRun` | script_name, script_pc, tick, value, varp_id, varp_pre |
| G2 | `nai138.update_energy.zero` | `modules/world/player_run.go` `(*Player).updateEnergy` (energy=0 branch) | tick, player_uid, varp_id, varp_pre, run_pre |
| G3 | `nai138.write_varp` | `modules/world/player_varp.go` `(*Player).writeVarp` | tick, player_uid, id, value, opcode, payload_hex, payload_len |

`world.node-debug` defaults to `true` (per `modules/world/config.go:76`), so no extra flag is required at run time.

---

## What I need you to do

### 1. Build and run the server

The dev sandbox cannot reach the host network (per `smoke_test_server_handoff` memory) — please run these from your shell:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o /tmp/goscape-nai138 ./cmd/goscape
/tmp/goscape-nai138 --config.file config.yaml 2>&1 | tee /tmp/nai138-probe.log
```

Leave the server running for both Run A and Run B.

### 2. Paired smoke runs

**Run A — energy=0 path (the failing case)**
1. Log in to the Java client (`/home/owner/Code/github.com/LostCityRS/Client-Java`).
2. Confirm the run-mode button is ON (toggle it on if needed; it should glow / show the "running" pose).
3. Walk continuously until the run-energy orb depletes to **0%**. Open ground works (e.g. somewhere outside Lumbridge); stop walking only after the orb hits 0.
4. **Observe:** does the run-mode button visually de-toggle (the pose changes back to the walking pose) or does it stay stuck in the run pose?
5. Note the symptom verbatim: `stays-on` or `de-toggles`. Also note any visible client behavior between the moment energy hit 0 and the next click (e.g. did the next walk attempt actually walk, or did it sprint?).

**Run B — click-toggle path (the working baseline)**
1. Either keep the same session (after Run A) or fresh-login (preferred — avoids residual state). Recover full energy if needed.
2. Click the run-mode button to toggle it from ON → OFF.
3. **Observe:** the button should de-toggle visually (it does in normal play). If it does NOT, that's a separate regression — please flag it.

### 3. Capture the filtered log

After both runs:

```bash
grep -E "nai138\.(p_run|update_energy\.zero|write_varp)" /tmp/nai138-probe.log > /tmp/nai138-probe-filtered.log
wc -l /tmp/nai138-probe-filtered.log
```

### 4. Hand back

Post the contents of `/tmp/nai138-probe-filtered.log` (or attach it) plus a one-line note like:

```
Run A: stays-on (button stuck in run pose at 0% energy)
Run B: de-toggles cleanly on click
```

---

## What I'll do with the output

The filtered log feeds Bundle β.2 — controller-only synthesis against the decision table (plan §"Bundle β.2 — Synthesis"):

| Energy=0 G2 record | G3 record(s) | Binding |
|---|---|---|
| present, `varp_pre = 0` | present | **Hypothesis A — Sequencing.** Server-side `varps[173]` was already 0 at energy=0; the redundant emit hits the client's `if (varps[var26] != var52)` short-circuit at `Client-Java/deob/client.java:9367`. |
| present, `varp_pre = 1` | present, `payload_hex = 00 AD 00` | **Hypothesis A.b — Client-side desync.** Same fix family; audit click-path. |
| present, `varp_pre = 1` | present, `payload_hex` divergent from `00 AD 00` | **Hypothesis C — Encoder-byte divergence.** Fix layer at `pkg/io/packet` P1/P2 or `(*Player).writeVarp`. |
| present, `varp_pre = 1` | present, hex correct, ordering relative to other tick-end packets differs from click path | **Hypothesis B — Tick timing.** Fix at `modules/world/tick.go` `processEnergy` ordering vs `processOut`. |
| G2 absent | — | Probe didn't fire. Re-run with `--world.node-debug` explicit. |

After binding, I'll author Bundle β.3 (TDD red→green→commit) at the bound layer, then Smoke handoff #2 verifies the fix.

---

## Stop point

Controller pauses here. Resume by pasting the filtered log + symptom note.
