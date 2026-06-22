# RESUME: rev-244 port — Bundle 3 (engine core)

Self-contained resume prompt. Written 2026-06-04 after Bundle 2 shipped.

## Where you are

Multi-revision Go port of the LostCityRS Engine-TS server: `main` = codeless
docs hub; **`rev-244` = the active 225→244 porting branch**. Work on
`rev-244` only. The work list is the cross-pin diff
`git -C $HOME/Code/github.com/LostCityRS/Engine-TS diff e1dea19f..9aadcec4`
(local checkout sits AT the 244 pin `9aadcec4`). Pins:
`git show main:REFERENCES.md` §rev-244.

## Read these first (in order)

1. `git show main:PORTING-LESSONS.md` — porting philosophy, §3 gotchas,
   §4 citations, §5 gates.
2. `docs/superpowers/specs/2026-06-03-rev244-port-design.md` — umbrella:
   7 bundles, definition of done.
3. `PORTING.md` §"rev-244 Bundle audit trail" — B1 + B2 correspondence
   tables and decision rows; the B5-early worker-eval subsection.
4. `docs/superpowers/specs/2026-06-04-rev244-worker-multiworld-eval.md` —
   B5-early eval; its §5/§6 flags belong to B3's scope.

## State: B1 + B2 SHIPPED

B1 (12 commits `8fcb734e..d82274fb`): io/cache primitives. B2 (28 commits
`b1cb81d4..2b32d051`): full opcode renumber (client `Opc*` constants +
registration), size-changed packets, REBUILD_GETMAPS/DATA_* removed, six
handler families re-shaped to 244, rsbuf damage2 + pulled-forward entity
feed. Suite + build + vet + `-race` all green 2026-06-04.

**Open windows:** map-delivery (DATA_* gone; OnDemand lands in B3 — the
post-B2+B3 client smoke closes it); midi-id (`PORTING-EXCEPTION
(rev244-b2-midi-window)` — `midiIDByName` stub in
`modules/world/midi_encoders.go` returns −1 until B3's MidiPack); B1 format
window (expires B6 repack; skipping tests listed in PORTING.md §B1).

## Next: Bundle 3 — engine core

Surface (umbrella): `World.ts` (262/232), entity family (`Player.ts`,
`Npc.ts`, `NetworkPlayer.ts`, `EntityList.ts`, `PathingEntity.ts`), new
engine `OnDemand.ts` (+123), `InputTrackingBlob.ts`; plus the
CrcTable/PreloadedPacks rewiring deferred from B1 (OnDemand-coupled).

**B3 brief flags (collected during B2):**

1. **MUST NOT double-apply** the pulled-forward damage2 hunks
   (PathingEntity.ts:92-96,606-610 / Player.ts:1870-1890 / Npc.ts:475-494 —
   shipped `2afa543c`). B3 owns any wholesale `damageAmt`→`hitmarkDamage`
   rename and the World.ts:1041-1042/1086-1087 compute-feed hunks (already
   wired at tick.go).
2. World-side login rate limiting removed upstream
   (`NODE_RATELIMIT_ADDRESS_LOGIN`/`NODE_RATELIMIT_DEVICE_LOGIN` gone) —
   B3 removes `modules/world/login_ratelimit*.go` + config; the replacement
   (login-server 3-in-5s + hop timer) is B5; carry a tracker row so the
   protection gap is explicit.
3. Login handshake re-shape (World.ts:2101-2162 at the pin): connect-time
   seed send is GONE; seed goes in the opcode-14 reply (8 zero bytes +
   status 0 + 8-byte seed); opcodes 16/18 are 1-byte-length framed with a
   plaintext revision byte → reply 6 on mismatch. TcpServer routes
   `client.state != 0` → `OnDemand.onClientData`.
4. MidiPack name→id registry + the PlaySong/PlayJingle producers
   (Player.ts:1919-1933) close the midi window.
5. `buildArea.clear` (`modules/world/build_area.go:59`) is pre-existing
   caller-less code mirroring TS BuildArea.clear — B3's login flow wires it.
6. The worker-eval doc §6 lists the web.ts/OnDemand-cache/per-deployment-token
   surface; §5 the rate-limit note.

**Acceptance gate after B3:** a 244 client (Client-Java `01f16088`) logs in
and plays — the end-to-end smoke deferred since B2.

## Process (B1+B2-proven; repeat it)

Brainstorm → spec (commit) → plan (writing-plans; bite-sized TDD; exact TS
extraction commands as contracts) → subagent-driven execution: implementer
(sonnet) → TS-parity spec reviewer → quality reviewer per substantive task;
controller-direct for leaf tasks; full-suite gate + PORTING.md
correspondence-audit at bundle end; final whole-bundle integration review.

**Bake into every implementer prompt (recurring B2 defects):**
- Every `// TS <File>.ts:<lines>` citation verified against a `| cat -n`
  numbered listing BEFORE writing (drifted repeatedly until mandated).
- Every reject-path test must seed earlier-gate prerequisites so the gate
  under test is the discriminating condition (four families hit this).
- Final-review "missing X" findings can be false positives — verify
  directly before fixing.

## Mechanics & gotchas

- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix.
  Build: `CGO_ENABLED=0 go build -trimpath ./...`. Race: CGO_ENABLED=1.
  Every commit: `--no-gpg-sign`.
- modules/world full suite ~2.5 min — not hung.
- Sandbox `git status` shows phantom `??` dotfiles (device-node masks) —
  never stage; never `git add -A`. Warn every subagent.
- Post-TDD stale LSP diagnostics are normal; trust real build/test runs.
- TS citations cite the 244 pin; adopt 244 names on renames; deviations get
  PORTING.md rows; accepted divergences get `PORTING-EXCEPTION (<id>, …)`.
