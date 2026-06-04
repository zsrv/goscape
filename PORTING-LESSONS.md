# Porting Lessons (TypeScript → Go, RuneScape server)

Cross-revision knowledge for porting a LostCityRS Engine-TS server revision
to Go. This is the durable, repo-owned distillation of what makes these ports
correct and what bites if you translate naively. Read it before starting a
new revision.

Companion files: `REFERENCES.md` (pinned upstream commits per revision), and
each revision branch's own `README.md` / `CLAUDE.md` / `PORTING.md` /
`docs/PORTING-CLOSED.md` (revision-specific conventions and the deviation
tracker).

---

## 1. Philosophy

**Faithful 1:1 translation is the default.** Every ported game-logic region
maps to an identifiable Engine-TS function, cited inline as
`// TS <File>.ts:<lines>`. goscape adapts the *infrastructure* (dskit service
lifecycle, module system, config layering) to Go idiom, but inside ported
game logic do **not** refactor opportunistically — behaviour bugs are found
by diff-checking the Go region against the cited TS source.

**Byte-faithful at the boundaries.** Wire protocol (ISAAC, RSA, opcodes),
codecs (bzip2, CRC, wordenc), collision, and the pack pipeline's cache output
are byte-for-byte against the reference. The pack pipeline is verified by
byte-diffing `script.dat` (and friends) against the cache the upstream
TS meta-repo packs.

**Deviations are tracked, never silent.** Each revision branch carries
`PORTING.md` (active backlog) + `docs/PORTING-CLOSED.md` (closed rows, parity
tables, audit history). Accepted in-code divergences carry a
`PORTING-EXCEPTION (<row-id>, <short>)` marker — `grep -rn "PORTING-EXCEPTION"
modules pkg cmd internal` lists every accepted exception (10 at the time of
the multi-revision conversion).

---

## 2. Porting workflow for a new revision

The Go port of revision N is (Go port of revision N-1) + (the translated TS
delta). That makes it a branch operation:

1. **Branch** `rev-N` from the nearest prior Go revision branch (e.g.
   `rev-225`).
2. **Diff the primary TS reference across the gap.** Look up the prior
   revision's pinned commit in `REFERENCES.md`, then
   `git -C Engine-TS diff <prev-commit>..<new-commit>`. That diff is your
   work list.
3. **Translate each TS change** into the corresponding Go region, applying
   the gotcha rules in §3. The Go branch diff (`git diff rev-(N-1) rev-N`)
   should correspond change-for-change to the TS diff — this is your audit.
4. **Record** the new reference commits in `REFERENCES.md` under `## rev-N` —
   including the Content and RuneScriptTS pins, which move together with the
   engine (the pack byte-parity baseline shifts with all three).
5. Each revision branch is self-contained (its own code, tooling, CI). Do
   **not** share code packages across revisions — independent faithful
   translations.
6. **Carry the tracker forward.** `PORTING.md` rows that still apply travel
   with the branch; close rows using the closure shapes defined in
   `PORTING.md` §Tracking conventions (FIXED / EXCEPTION-DOCUMENTED /
   NO-DIVERGENCE / NOT-A-GAP).

---

## 3. TS → Go translation gotchas

Each is a real bug class hit during the rev-225 port. Verify against the TS
source before "fixing" anything that merely looks wrong — and before
*preserving* anything that merely looks intentional.

### Identity & operand encoding

- **Pick ONE identity per subsystem.** Player identity exists as both `uid`
  and protocol `slot`; mixing them inside one subsystem caused repeated bugs
  (e.g. an obj-ownership check keyed by `uid` everywhere except one network
  handler that used `slot`). Convention: the network layer speaks `slot`,
  world/script logic speaks `uid`.
- **Script int-operand encoding is bit-packed.** The VARP/VARN secondary-
  player flag is **bit 16** of the int operand (`(intOperand>>16)&1`), not a
  0/1 selector. `.`-prefixed (secondary-player) script commands must use
  operand-aware accessors (`activePlayer()`), not `Self`, or writes silently
  hit the wrong player.

### Shared-handler / fork drift

- **When N opcodes share one TS handler, Go shares one implementation.**
  Forking per-opcode drifts (three move opcodes → one TS `MoveClickHandler`;
  the Go fork forgot modal-close in one copy). Parameterize the wire delta,
  not the behaviour.
- **When TS keeps logic in a shared base class (`PathingEntity`), a fix must
  land in BOTH Go forks** (Player and Npc) — Go has no inheritance, so the
  shared TS line exists twice.
- **Two parallel compute paths can feed the same wire field.** Fix the path
  that actually serializes (the renderer read accessors, not the bridge-fed
  struct field) — the spawn-orientation bug was "fixed" twice on the wrong
  path first.

### Tick order & scheduling

- **Tick ORDER is behaviour.** TS interleaves movement between pre-step and
  post-step interaction passes; running "movement, then interactions" broke
  walk-up combat. Port the pipeline order, not just the pieces.
- **`Player.Save()` is tick-goroutine-only.** Off-tick paths (disconnect,
  shutdown) must route through the relay action queue so saves run on-tick;
  three separate save-loss paths came from violating this.
- **The protected-access invariant is load-bearing for content.** All ~1159
  `.rs2` content scripts consume items *after* a dialogue yield without
  checking `inv_del`'s return. That is safe ONLY because `runScript` refuses
  to start a protected script while another holds protected access (TS
  `Player.ts:2094`). Immediate-run handlers (the opheld family, if_button /
  inv_button) bypass `CanAccess()` and rely solely on that guard; script
  resumes bypass `runScript` entirely (the `force=true` path). Removing or
  weakening the guard re-opens live item-dupe exploits.

### Trust nothing that isn't the TS source

- **A test can pin a bug.** A "deviation" test that contradicts TS may be
  enshrining the bug it was written around — verify against TS before
  preserving the pinned contract, and update the test when TS disagrees.
- **"By design" / "handled elsewhere" comments lie.** A comment claiming a
  divergence is intentional (or covered by another path) is the prime
  suspect, not evidence. Re-verify against TS before trusting it.
- **Ask "what does TS do?" FIRST.** Before estimating any gap as
  implementation work, check the TS state: if TS itself has the feature
  stubbed or commented out (NpcMode QUEUE1..20), the correct Go closure is
  documentation, not implementation.
- **Mis-described audit rows are themselves misdirection.** Severity or
  category in a finding can be wrong; re-derive the divergence claim from
  first principles against current TS + Go before acting on it.

### Pack pipeline / byte parity

- **Fix determinism FIRST, then byte-diff.** A "non-deterministic output"
  complaint against an external baseline can hide a multi-bug stack: one
  map-iteration-order bug masked four separate parity bugs that only became
  visible once output was stable. Go map iteration is randomized — any
  ordered output derived from a map needs explicit ordering.
- **Mirror TS's data-driven dispatch.** Discriminating types via Go pointer
  equality (instead of the data-driven check TS does) is fragile and broke
  default-value emission.
- **`jagFileVersion=26`.** Do not bump to 27 unless the upstream meta-repo
  pins `@lostcityrs/runescript` past `750291c` (see `REFERENCES.md`).

### Process

- **Capture `go test`'s real exit code** — and stress-run any fix that
  changes RNG-dependent behaviour. A flaky test passing once by RNG luck
  masked a real regression.
- **Get a live client debug log EARLY** when headless repros keep passing —
  one tick-ordering bug was invisible to every synthetic repro and obvious in
  the first live log.

---

## 4. Comment & reference conventions

- **Cite TS by file and line:** `// TS World.ts:128-129` next to the ported
  region. The file/symbol is the durable anchor; line numbers drift — fix
  them when touching the code, don't invest in keeping them precise.
- **No per-comment revision tags.** The branch *is* the revision context, and
  `REFERENCES.md` pins the exact TS commit — together they make every bare
  `// TS:` comment unambiguous.
- **`PORTING-EXCEPTION (<row-id>, <short>)`** one-line markers (with a `See
  PORTING.md` cross-reference) index every accepted divergence in code. Keep
  them grep-discoverable; each keeps its row-id reference.
- **`PORTING.md` is updated as a side effect** of any work that touches a
  tracked region; closed rows move to `docs/PORTING-CLOSED.md`.

---

## 5. Verification

- **Gates:** `CGO_ENABLED=0 go build -trimpath ./...`, `go vet ./...`,
  `go test ./...`, and `go test -race` on touched packages (the race detector
  needs `CGO_ENABLED=1`, the default). The world tick loop is goroutine-heavy;
  race coverage matters.
- **Byte parity:** ISAAC, RSA, opcodes, bzip2, CRC, wordenc, and collision are
  byte-faithful and pinned by tests; pack output is byte-diffed against the
  upstream-packed cache when the pack pipeline changes.
- **Pin tests:** when a fix lands, pin the TS-correct contract with a test.
  When an existing test pinned the buggy contract, update the test — after
  verifying against TS (§3, "A test can pin a bug").
