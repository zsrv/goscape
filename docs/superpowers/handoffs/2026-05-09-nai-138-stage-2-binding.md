# NAI-138 Stage 2 — Bundle β.2 binding

**Date:** 2026-05-09
**Predecessor:** smoke handoff #1 at `72b9d33`; smoke output captured by user.
**Plan:** `docs/superpowers/plans/2026-05-09-nai-138-stage-2-encoder-defect.md` Bundle β.2.

## Smoke output (verbatim, abbreviated)

```
tick=53 nai138.write_varp id=18  value=1 opcode=150 payload_hex=001201
tick=53 nai138.write_varp id=166 value=2 opcode=150 payload_hex=00a602
tick=53 nai138.write_varp id=168 value=2 opcode=150 payload_hex=00a802
tick=53 nai138.write_varp id=169 value=2 opcode=150 payload_hex=00a902
tick=53 nai138.write_varp id=43  value=0 opcode=150 payload_hex=002b00
tick=283 nai138.p_run script_name=[proc,tutorial_step_enable_run] script_pc=6  value=0 varp_id=0 varp_pre=0
tick=287 nai138.p_run script_name=[if_button,controls:com_5]      script_pc=24 value=1 varp_id=0 varp_pre=0
tick=423 nai138.update_energy.zero player_uid=2232170497 varp_id=0 varp_pre=1 run_pre=1
tick=449 nai138.p_run script_name=[if_button,controls:com_4]      script_pc=8  value=0 varp_id=0 varp_pre=0
```

Visual: Run A stays-on (button stuck in run pose at 0% energy). Run B (click toggle) de-toggles cleanly.

## Binding — refinement of Hypothesis A (Sequencing)

**`varp_id` is `0` in every G1 / G2 record.** `(*Player).RunVarpID()` returns the zero default instead of the configured run varp id (which should be 173). The login burst at tick 53 emits varps 18/166/168/169/43 — but never 173. The click+energy=0 paths write to `varps[0]` server-side only; `writeVarp(0, …)` early-returns at the `cfg == nil || !cfg.Transmit` gate (varp 0 is a sentinel/null), so **no wire packet for varp 173 is ever sent**.

The client's run-mode button state is keyed off varp 173. Since the client never receives any varp 173 transition, the cs1 redrawSidebar trigger at varp 173 (per `cs1_re_eval_triggers` memory) never fires, and the button stays in whatever pose the client last saw.

This is a refinement of Hypothesis A (Sequencing) one layer deeper than the decision table anticipated: not "the right varp written at the wrong time" but "the wrong varp written entirely."

## Root cause — verified by re-grep + cache probe

Per the `audit_subagent_fabrication` protocol I independently verified the binding at HEAD before recommending the fix:

1. **`(*Player).RunVarpID()` reads `p.client.server.varpTypes.RunID`.**
   - `modules/world/player_script.go:357-361`. Verified.

2. **`varpTypes.RunID` is set by `parseVarpTypes` only when a config has `ClientCode == 7`.**
   - `pkg/objtype/varptype.go:88-90`: `if config.ClientCode == 7 { runID = id }`. Verified.

3. **The compiled cache `data/pack/server/varp.dat` does not surface `ClientCode == 7` for any varp.**
   - One-shot probe loaded `LoadVarpTypes("data/pack")` against the production cache: `count=295 RunID=0`. No varp has `ClientCode == 7`. `varp[173]` is named `option_run`, has `Transmit=true`, but `ClientCode=0`.
   - Source `.varp` (`Content/scripts/interface_controls/configs/player_controls.varp:5-8`) does declare `clientcode=7` for `option_run`. The compiled cache file has lost this opcode.

4. **TS engine reads `clientcode` from the CLIENT varp.dat (a Jagfile), not the server varp.dat.**
   - `Engine-TS/src/cache/config/VarPlayerType.ts:25-42` loads `${dir}/server/varp.dat` AND `${dir}/client/config` Jagfile. For each id, calls `decodeType(server)` then `decodeType(client)`. The `clientcode` opcode (case 5) is in the client stream.
   - **goscape's `LoadVarpTypes` only loads server/varp.dat.** Verified at `pkg/objtype/varptype.go:64-70`. The client jag is never opened, so the clientcode opcode is never decoded, so `ClientCode` stays at zero, so `RunID` stays at zero, so `(*Player).RunVarpID()` returns zero.

5. **The `data/pack/client/config` Jagfile exists** on disk; `pkg/io/jagfile.LoadJagfile` infrastructure exists.

6. **`pkg/objtype/loctype.go:204-247` already implements the TS-faithful two-stream pattern** for `LoadLocTypes` / `parseLocTypes`. This is the local precedent the varptype fix should mirror.

## Why the existing parser test passed

`pkg/objtype/varptype_test.go` `TestParseVarpTypes_DiscoversRunIDFromClientCode7` constructs a synthetic `varp.dat` with opcode 5 (clientcode) embedded in the SERVER stream. It passes today because `Decode()` accepts opcode 5 anywhere — but in production the SERVER stream never contains opcode 5; it lives in the CLIENT stream which is never read. Per `test_passes_for_wrong_reason` memory: the test exercised the decode arm, not the production loading path.

## Fix layer — Bundle β.3

Mirror `loctype.go`'s pattern in `varptype.go`:
- `LoadVarpTypes(dir)` also loads `client/config` Jagfile.
- `parseVarpTypes(server, clientJag)`: read `varp.dat` from clientJag, skip 2-byte count, call `DecodeType(client, config)` after `DecodeType(server, config)`.
- Existing tests update to the new two-arg signature; add a `buildClientJag` helper mirroring the loctype-test pattern.

This is a TS-fidelity catch-up — no deviation tag.

## Smoke evidence trail (for future reference)

`docs/superpowers/handoffs/2026-05-09-nai-138-stage-2-probe-smoke.md` (the handoff) plus the inline log capture above. Both will close out at Stage 2 completion.
