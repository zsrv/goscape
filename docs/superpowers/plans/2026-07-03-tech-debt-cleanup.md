# Tech-Debt Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address the actionable, low-fidelity-risk tech debt surfaced by the 2026-07-03 three-agent debt audit, across all 5 rev branches (274 / 254 / 245.2 / 244 / 225).

**Architecture:** Implement and verify every item on the tip branch **rev-274** first (source of truth, Phase A). Then port each landed change to the other four rev branches (Phase B) using the established cross-rev methodology — per-file COPYABLE (`git checkout rev-274 -- <file>` when the target file is byte-identical to rev-274's pre-change state) vs. ADAPT (hand-apply when the branch has diverged), each gated by a compile-all + touched-package test run.

**Tech Stack:** Go 1.26 (`go 1.26` in go.mod). Server modules under `modules/`, shared libs under `pkg/`. Faithful Go port of TypeScript `LostCityRS/Engine-TS`.

## Global Constraints

- **Go invocations:** always prefix with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` (e.g. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`).
- **Commits:** always `git commit --no-gpg-sign`. End every commit message with the Co-Authored-By trailer per the harness convention.
- **Compile-all gate** (run after every code task): `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...` — must build clean.
- **Format gate** (run after every code task): `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg modules cmd` — must print nothing.
- **Race detector** is available on this box but needs `CGO_ENABLED=1`. Run `-race` only on touched packages that have concurrency (world module), not the whole tree.
- **Fidelity gate:** this project has a hard "true-to-TS behavior" contract. Every task here is behavior-preserving. If any step would change observable behavior, STOP and flag it — do not proceed.
- **Do NOT push.** Leave all branches local; the user pushes when ready. Do not touch `main` (codeless docs hub).
- **Branch/worktree map** (each rev branch has a dedicated worktree):
  - rev-274 → `~/Code/github.com/zsrv/goscape` (tip `c8fcba5b`)
  - rev-254 → `~/Code/github.com/zsrv/goscape-rev254` (tip `1f393f75`)
  - rev-245.2 → `~/Code/github.com/zsrv/goscape-rev245.2` (tip `73bcfdc3`)
  - rev-244 → `~/Code/github.com/zsrv/goscape-rev244` (tip `a07a44da`)
  - rev-225 → `~/Code/github.com/zsrv/goscape-rev225` (tip `253b17d4`)
- **DO NOT TOUCH (fidelity-locked, verified deliberate by the audit):** `queue_handler.go`/`longqueue_handler.go` (separate TS classes), `pathToTarget*` Player/Npc fork (R2 risk-register mitigation), cross-family stat-arg merge, WS-transport TODO (scoped unported feature — leave the `// TODO: WS support` marker), routefinder `panic`→`error` conversion (rsmod-faithful, already contained by `recoverPlayer`/`recoverNpc`).

---

## PHASE A — Implement & verify on rev-274

All Phase-A work happens in `~/Code/github.com/zsrv/goscape` on a single feature branch off `rev-274`, matching the established `fix/<name>-batch` pattern (precedent: `fix/followups-batch` merged `--no-ff` at `c0e482dc`).

### Task A0: Create the Phase-A feature branch

**Files:** none (git only).

- [ ] **Step 1: Branch off rev-274**

```bash
cd ~/Code/github.com/zsrv/goscape
git checkout rev-274
git checkout -b fix/tech-debt-cleanup
git status   # expect clean tracked tree (untracked ./goscape binary + .superpowers may be present; ignore)
```

- [ ] **Step 2: Baseline the suite is green before starting**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -20`
Expected: all packages `ok` or `no test files`. If anything fails pre-change, STOP and report — do not build on a red baseline.

---

### Task A1: `chan interface{}` → `chan struct{}` for the world `quit` signal channel

The `quit` channel carries no value — it is only `close(s.quit)` and received in `select`. `chan struct{}` is the idiomatic signal-channel type. Zero behavior change (`close`/receive semantics are identical). All sites (source + tests + the `ondemand.go` receiver) move together.

**Files:**
- Modify: `modules/world/server.go:63`, `modules/world/server.go:514`
- Modify: `modules/world/ondemand.go:388-390`
- Modify (test sites): `modules/world/interaction_test.go`, `modules/world/npc_interaction_test.go`, `modules/world/player_uid_test.go`, `modules/world/ondemand_test.go`, `modules/world/server_test.go`

**Interfaces:**
- Produces: `Server.quit` is now `chan struct{}`; `(*onDemand).run(stop <-chan struct{})`. No exported signature changes (`quit` is unexported; `run` is a method on an unexported type).

- [ ] **Step 1: Change the struct field and constructor**

`modules/world/server.go:63` — change:
```go
	quit        chan interface{}
```
to:
```go
	quit        chan struct{}
```

`modules/world/server.go:514` — change:
```go
		quit:          make(chan interface{}),
```
to:
```go
		quit:          make(chan struct{}),
```

- [ ] **Step 2: Change the ondemand receiver signature + its explanatory comment**

`modules/world/ondemand.go:388-390` — change:
```go
// stop is chan interface{} to match Server.quit used throughout the world
// server.
func (od *onDemand) run(stop <-chan interface{}) {
```
to:
```go
// stop is a receive-only signal channel matching Server.quit used throughout
// the world server.
func (od *onDemand) run(stop <-chan struct{}) {
```

- [ ] **Step 3: Find and fix every remaining `chan interface{}` / `make(chan interface{})` in the world test files**

Run: `grep -rn 'chan interface{}\|make(chan interface{})' modules/world/`
Expected residual sites to convert (per the audit): `interaction_test.go`, `npc_interaction_test.go`, `player_uid_test.go`, `ondemand_test.go`, `server_test.go`. For each hit that constructs or types the quit/stop channel, replace `interface{}` with `struct{}`. Re-run the grep until it prints **nothing** in `modules/world/`.

- [ ] **Step 4: Compile-all + format gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg modules cmd
```
Expected: build clean; gofmt prints nothing.

- [ ] **Step 5: Run world tests (with race, since this is concurrency plumbing)**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ 2>&1 | tail -20`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
git add modules/world/
git commit --no-gpg-sign -m "refactor(world): use chan struct{} for the quit signal channel"
```

---

### Task A2: Delete stale/dead TODOs in `modules/world/server.go`

The audit verified these against the current implementation. `:1576`/`:1578` describe work already done (save arrives via gRPC `c.savePayload` at `:1653`, applied in `tick.go:469` `LoadSave`; reconnecting is wired via `c.reconnecting` at `:1534`). `:1247` references a graceful-shutdown blog post for work already implemented (`s.quit` + admission gate + `closeLiveConns` + `tcpWg.Wait`). `:935` and `:1095` are resolvable questions the audit answered — replace the TODO with a confirming comment rather than deleting the code they annotate. Leave `:932 // TODO: WS support` untouched (scoped unported feature).

**Files:**
- Modify: `modules/world/server.go` (lines ~935, ~1095, ~1247, ~1576, ~1578)

- [ ] **Step 1: Delete the two dead login TODOs**

`modules/world/server.go` — delete these two comment lines (currently `:1576` and `:1578`, with the surrounding blank lines) so the block reads:
```go
		default:
			return c.sendLoginError(reply)
		}

		c.log.Info("login accepted", "safename", safeName, "reply", reply, "reconnecting", reconnecting)
		return c.sendLoginOK()
```
(Remove `// TODO: save var from msg` and `// TODO: save + reconnecting check` and their now-redundant blank lines.)

- [ ] **Step 2: Delete the resolved graceful-shutdown link TODO**

`modules/world/server.go:1247` — remove the line:
```go
		// TODO: https://eli.thegreenplace.net/2020/graceful-shutdown-of-a-tcp-server-in-go/
```
(and the trailing blank line if it leaves a double blank). The `Fix 6` comment immediately below stays.

- [ ] **Step 3: Resolve the `net.ErrClosed` TODO with a confirming comment**

`modules/world/server.go:935` — change:
```go
		if errors.Is(err, net.ErrClosed) { // TODO: verify if this is appropriate - does errclosed only happen when server closes the conn, not client?
```
to:
```go
		// net.ErrClosed from Accept only arises when Shutdown closes the
		// listener (never from a client-side close), so mapping it to nil is
		// the clean-shutdown path; the accept loop's <-s.quit guard corroborates.
		if errors.Is(err, net.ErrClosed) {
```

- [ ] **Step 4: Resolve the listener-close TODO with an ownership comment**

`modules/world/server.go:1095` — change:
```go
	defer s.tcpListener.Close() // TODO: put somewhere else? is this in the greenplace example?
```
to:
```go
	// Shutdown is the primary listener owner (nil-guarded, under the admission
	// gate); this defer additionally covers a serveTCP accept error that
	// returns before Shutdown runs. The resulting double-Close is harmless
	// (the second returns ErrClosed, which callers ignore).
	defer s.tcpListener.Close()
```

- [ ] **Step 5: Compile-all + format gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg modules cmd
```
Expected: build clean; gofmt prints nothing.

- [ ] **Step 6: Confirm no accidental behavior/text drift**

Run: `git diff --stat modules/world/server.go` and `grep -n 'TODO' modules/world/server.go`
Expected: only `// TODO: WS support` and `// TODO: return error?` (client.go, unrelated) remain in the world module's server-side TODOs; the five addressed ones are gone.

- [ ] **Step 7: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "docs(world): remove stale server.go TODOs; document ErrClosed + listener-close contracts"
```

---

### Task A3: Add a table-driven `LayerOf` test (TDD safety net)

`pkg/pathfinder/loc/LayerOf` is a 23-case `Shape → Layer` switch on hot paths (`pkg/entity/loc.go`, `pkg/gamemap/gamemap.go`, `pkg/script/handlers_loc.go`, `pkg/script/handlers_map.go`) with **no test**. Because it keys on `iota`-ordered `Shape` constants, inserting or reordering a `Shape` silently reclassifies every later shape (a wall becomes ground) with nothing to catch it. This test pins all 23 shape→layer pairs plus the panic default. Test-only — zero fidelity risk.

**Files:**
- Create: `pkg/pathfinder/loc/layer_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/pathfinder/loc/layer_test.go`:
```go
package loc

import "testing"

// TestLayerOf pins every Shape → Layer mapping. LayerOf keys on iota-ordered
// Shape constants, so inserting or reordering a Shape silently reclassifies
// every later shape; this table is the regression net for that class of bug.
func TestLayerOf(t *testing.T) {
	cases := []struct {
		shape Shape
		want  Layer
	}{
		{ShapeWallStraight, LayerWall},
		{ShapeWallDiagonalCorner, LayerWall},
		{ShapeWallL, LayerWall},
		{ShapeWallSquareCorner, LayerWall},
		{ShapeWallDecorStraightNoOffset, LayerWallDecor},
		{ShapeWallDecorStraightOffset, LayerWallDecor},
		{ShapeWallDecorDiagonalOffset, LayerWallDecor},
		{ShapeWallDecorDiagonalNoOffset, LayerWallDecor},
		{ShapeWallDecorDiagonalBoth, LayerWallDecor},
		{ShapeWallDiagonal, LayerGround},
		{ShapeCentrepieceStraight, LayerGround},
		{ShapeCentrepieceDiagonal, LayerGround},
		{ShapeRoofStraight, LayerGround},
		{ShapeRoofDiagonalWithRoofEdge, LayerGround},
		{ShapeRoofDiagonal, LayerGround},
		{ShapeRoofLConcave, LayerGround},
		{ShapeRoofLConvex, LayerGround},
		{ShapeRoofFlat, LayerGround},
		{ShapeRoofEdgeStraight, LayerGround},
		{ShapeRoofEdgeDiagonalCorner, LayerGround},
		{ShapeRoofEdgeL, LayerGround},
		{ShapeRoofEdgeSquareCorner, LayerGround},
		{ShapeGroundDecor, LayerGroundDecor},
	}
	if len(cases) != int(ShapeGroundDecor)+1 {
		t.Fatalf("LayerOf test table has %d cases but there are %d Shape constants; a Shape was added without updating this test", len(cases), int(ShapeGroundDecor)+1)
	}
	for _, c := range cases {
		if got := LayerOf(c.shape); got != c.want {
			t.Errorf("LayerOf(%d) = %d, want %d", c.shape, got, c.want)
		}
	}
}

func TestLayerOfPanicsOnUnknownShape(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("LayerOf did not panic on an out-of-range Shape")
		}
	}()
	LayerOf(ShapeGroundDecor + 1)
}
```

- [ ] **Step 2: Run to verify it passes against the current (correct) implementation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/loc/ -run TestLayerOf -v`
Expected: `PASS` for both `TestLayerOf` and `TestLayerOfPanicsOnUnknownShape`.

> Note: this is a *characterization* test — it passes on first write because it pins already-correct behavior. To confirm it actually guards, temporarily flip one `want` (e.g. `{ShapeWallL, LayerGround}`), re-run, see it FAIL, then revert.

- [ ] **Step 3: Confirm the guard fires under mutation, then revert**

Temporarily edit one row's `want`, run the test, confirm FAIL, revert the edit, re-run, confirm PASS.

- [ ] **Step 4: Format gate + commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/pathfinder/loc/
git add pkg/pathfinder/loc/layer_test.go
git commit --no-gpg-sign -m "test(pathfinder): pin LayerOf shape-to-layer mapping"
```

---

### Task A4: Deduplicate `pack_configs.go` `packAndSave*` head/tail boilerplate

The safest dup win (~90–150 LOC). The 19 `packAndSave*` funcs share a read/validate head (`ReadTypedConfigs` + `validatePackNamesAgainstCfgs`) and the 9 *transmitted* configs share a ~12-line save tail (`BuildVerify` + stderr CRC print + `server.Save` + two `clientJag.Write` + `return nil`). The TS-parity surface is the `packXConfigs` **encoders**, which this task does not touch — zero fidelity risk.

**Files:**
- Modify: `pkg/pack/pack_configs.go` (helpers + 19 `packAndSave*` funcs, lines ~593–1095)
- Test: existing `pkg/pack/*_test.go` + the pack byte-parity gate

**Interfaces:**
- Produces two new unexported helpers in package `pack`:
  - `readAndValidate(srcDir, ext string, props packProps, parse ParseFn, c *cache..., pf ..., transmitted bool) ([]TypedConfig, error)` — mirror the exact head currently inlined; keep the real parameter types from the existing `ReadTypedConfigs`/`validatePackNamesAgainstCfgs` signatures (read them before writing the helper).
  - `saveTransmittedConfig(name string, serverOut string, server *..., client *..., clientJag *..., crc int) error` — the shared tail.

- [ ] **Step 1: Read the current signatures before writing helpers**

Run:
```bash
grep -n 'func ReadTypedConfigs\|func validatePackNamesAgainstCfgs\|func BuildVerify\|func.*Save(' pkg/pack/pack_configs.go pkg/pack/*.go
```
Read `packAndSaveSeqConfigs` (~915) and `packAndSaveVarpConfigs` (~593) end-to-end as the two representative shapes (transmitted-with-tail vs. non-transmitted). Copy the exact types into the helper signatures — do not guess.

- [ ] **Step 2: Extract `saveTransmittedConfig` from the 9 transmitted funcs**

Introduce the tail helper capturing the identical `BuildVerify → stderr print → server.Save → clientJag.Write ×2 → return nil` sequence, parameterized by `name` and `crc`. Replace the inline tail in all 9 transmitted funcs (seq, loc, flo, spotanim, npc, obj, idk, varp, varbit) with a call. The per-func `// TS PackShared.ts:NNN` CRC comment is redundant with the CRC constant block at lines 52–72; you may drop it at each call site.

- [ ] **Step 3: Extract `readAndValidate` from all 19 funcs**

Introduce the head helper capturing `ReadTypedConfigs(...) + validatePackNamesAgainstCfgs(...)`, parameterized by ext / parse-fn / transmitted bool. Replace the inline head in all 19 funcs with a call.

- [ ] **Step 4: Compile-all + format gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg modules cmd
```
Expected: clean.

- [ ] **Step 5: Run pack tests + byte-parity gate**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pack/... 2>&1 | tail -30`
Expected: `ok` for all pack packages. If a byte-parity or CRC test exists and is cache-gated (skips without fixtures), note the skip — the refactor is byte-identical by construction, but if fixtures are present the parity test MUST still pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/pack/pack_configs.go
git commit --no-gpg-sign -m "refactor(pack): extract read/validate + transmitted-save helpers in pack_configs"
```

---

### Task A5: Split the `modules/world/server.go` god-file into cohesive same-package files

`server.go` is 2240 LOC mixing 6 concerns. Go does not care which file a method lives in, so this is a **pure cut/paste with no logic change**. Move method groups into new `modules/world/server_*.go` files. Login helpers currently split across `client.go` and `server.go` get co-located.

**Files:**
- Modify: `modules/world/server.go` (remove moved funcs)
- Create: `modules/world/server_login.go`, `modules/world/server_accept.go`, `modules/world/server_players.go`, `modules/world/server_lookup.go`

**Move manifest** (verify exact current line ranges with `grep -n 'func ' modules/world/server.go` before moving — line numbers drift as earlier tasks edit the file):
- `server_login.go` ← `handleData`, `handleLogin`, `callPlayerLoginRPC`, `loginResultToRS2`, `disconnectSessionLogEvent`
- `server_accept.go` ← `serveTCP`, `serveConn`, `handleTCPConn`, `trackConn`, `untrackConn`, `closeLiveConns`
- `server_players.go` ← `addPlayer`, `getTotalPlayers`, `isUsernameLoggingOut`, `scaleByPlayerCount`, `removePlayerInternal`, `sendPlayerLogoutWithRetry`, `removePlayerOnTick`, `removePlayerOnDisconnect`, `saveAllOnShutdown`, `waitForSaveFlush`, `autosavePlayers`
- `server_lookup.go` ← `retryBridgeRegistration`, `initLoginGate`, `worldStartupCall`, and the `Lookup*` / `ZonePlayers` / `TrackZone` group

Leave in `server.go`: the `Server` struct definition, `NewServer`, `Run`, `Shutdown`, and any package-level vars/consts. Each new file gets `package world` and only the imports its moved funcs need.

- [ ] **Step 1: Snapshot the function inventory**

Run: `grep -n '^func ' modules/world/server.go > /tmp/claude/server_funcs_before.txt && cat /tmp/claude/server_funcs_before.txt`
Use this to locate exact ranges. Move whole functions (signature through closing `}`) including their doc-comments.

- [ ] **Step 2: Create `server_accept.go` and move the accept-loop group**

Create the file with `package world`, move the 6 accept-group funcs (with doc-comments) out of `server.go`, and add the imports they reference (`net`, `errors`, etc.). Repeat the pattern for Steps 3–5.

- [ ] **Step 3: Create `server_login.go` and move the login group.**
- [ ] **Step 4: Create `server_players.go` and move the registry/removal/save group.**
- [ ] **Step 5: Create `server_lookup.go` and move the bridge/lookup group.**

- [ ] **Step 6: Compile-all + format + goimports-style gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
```
Expected: build + vet clean (vet catches unused imports left behind in `server.go`); gofmt prints nothing.

- [ ] **Step 7: Verify no function was lost or duplicated**

Run:
```bash
grep -h '^func ' modules/world/server.go modules/world/server_*.go | sort > /tmp/claude/server_funcs_after.txt
comm -3 <(sed 's/.*func //' /tmp/claude/server_funcs_before.txt | sort) <(sed 's/.*func //' /tmp/claude/server_funcs_after.txt | sort)
```
Expected: the second `comm` output shows only additions from the *pre-existing* non-moved funcs (should be empty diff on the moved set). Every moved func appears exactly once across the new files.

- [ ] **Step 8: Run world tests (with race)**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ 2>&1 | tail -20`
Expected: `ok`.

- [ ] **Step 9: Commit**

```bash
git add modules/world/server.go modules/world/server_login.go modules/world/server_accept.go modules/world/server_players.go modules/world/server_lookup.go
git commit --no-gpg-sign -m "refactor(world): split server.go into cohesive same-package files"
```

---

### Task A6: Simplify manual clamps to builtin `min`/`max`

Three goscape-authored helpers/expressions. Keep the named helpers (they document intent); simplify only their bodies. All inputs satisfy `lo <= hi`, so `min(max(v,lo),hi)` is exactly equivalent. The `math.Max` nesting change is also *more* faithful to the TS source `Math.max(a,b,c)`.

**Files:**
- Modify: `pkg/rsbuf/playerinfo.go:370-378`, `pkg/rsbuf/zone_encoders.go:38-46`, `modules/world/player_script.go:877`

- [ ] **Step 1: Simplify `clampInt`**

`pkg/rsbuf/playerinfo.go` — replace the body:
```go
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
```
with:
```go
func clampInt(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
```
(Keep the doc-comment above it.)

- [ ] **Step 2: Simplify `clampU16`**

`pkg/rsbuf/zone_encoders.go` — replace the body:
```go
func clampU16(n int) uint16 {
	if n < 0 {
		return 0
	}
	if n > 65535 {
		return 65535
	}
	return uint16(n)
}
```
with:
```go
func clampU16(n int) uint16 {
	return uint16(min(max(n, 0), 65535))
}
```

- [ ] **Step 3: Flatten the nested `math.Max`**

`modules/world/player_script.go:877` — change:
```go
	return int(math.Floor(base + math.Max(melee, math.Max(rangd, magic))))
```
to:
```go
	return int(math.Floor(base + max(melee, rangd, magic)))
```
Then check whether `math` is still used in the file: `grep -n 'math\.' modules/world/player_script.go`. If `math.Floor` (or another `math.` call) remains, keep the import; otherwise remove `"math"` from the import block.

- [ ] **Step 4: Compile-all + format gate**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg modules cmd
```
Expected: clean.

- [ ] **Step 5: Run touched-package tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/rsbuf/ ./modules/world/ 2>&1 | tail -10`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
git add pkg/rsbuf/playerinfo.go pkg/rsbuf/zone_encoders.go modules/world/player_script.go
git commit --no-gpg-sign -m "refactor: use builtin min/max for clamp helpers"
```

---

### Task A7: Consolidate the `fire{Op,Ap}Trigger{Npc,Loc,Obj}` families

**HIGHEST fidelity CARE of this plan.** Six near-identical funcs (~160–190 LOC) in `modules/world/interaction_trigger.go`, whose own doc-comments invite consolidation ("Mirrors fireOpTriggerLoc with three substitutions", interaction_trigger.go:669/:736). They all mirror the *single* polymorphic TS block `Player.tryInteract` — so this is not a deliberate fork. BUT there are documented per-type deviations (S6j-D2/D4), so the consolidation must preserve each type's exact gate/action. Do this **behind the existing interaction tests** and diff behavior carefully.

**Files:**
- Modify: `modules/world/interaction_trigger.go`
- Test: `modules/world/interaction_test.go`, `modules/world/npc_interaction_test.go`

**Interfaces:**
- Produces two package-private helpers (exact field types to be read from the current funcs before writing):
  - a descriptor struct carrying: lifecycle-gate closure (`func() bool` — `npc.dead` / `locStillValid` / `objStillValid`), the trigger-lookup fn, the category source, the `Active*`/`Pointers` setter, and the no-script action.
  - `fireOpTrigger(p *Player, srv *Server, d opTriggerDesc) bool` and `fireApTrigger(p *Player, srv *Server, d apTriggerDesc) bool`.

- [ ] **Step 1: Establish the behavioral baseline**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'Interact|Trigger|Op|Ap' -v 2>&1 | tail -40`
Record which tests exercise these paths. If coverage looks thin, ADD characterization tests for at least one Npc, one Loc, and one Obj op-trigger AND ap-trigger path *before* refactoring (each asserting: delayed gate, lifecycle gate, script-found exec, and no-script action). Commit the added tests separately first.

- [ ] **Step 2: Read all six funcs and tabulate the 4-axis variation**

Read `fireOpTriggerNpc` (52-108), `fireOpTriggerLoc` (126-195), `fireOpTriggerObj` (673-733), `fireApTriggerNpc` (318-401), `fireApTriggerLoc` (415-506), `fireApTriggerObj` (741-824). Build a table: for each, capture (lifecycle-gate expr, trigger fn, category source, active-field setter, no-script action). Confirm the ScriptState setup block and save/exec/restore tail are byte-identical across the family (audit says they are).

- [ ] **Step 3: Introduce `fireOpTrigger` + descriptor; migrate the 3 OP funcs one at a time**

Add the helper and the OP descriptor. Rewrite `fireOpTriggerNpc` to build its descriptor and delegate. Run the OP tests. Then `fireOpTriggerLoc`, run. Then `fireOpTriggerObj`, run. **One type per sub-step, test between each** — never migrate all three blind.

- [ ] **Step 4: Introduce `fireApTrigger` + descriptor; migrate the 3 AP funcs one at a time (same discipline).**

- [ ] **Step 5: Full world test + race**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ 2>&1 | tail -20`
Expected: `ok`. If any interaction test fails, the consolidation changed behavior — revert to the last green sub-step and re-diff that type's axis.

- [ ] **Step 6: Format gate + commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
git add modules/world/interaction_trigger.go modules/world/*_test.go
git commit --no-gpg-sign -m "refactor(world): consolidate fire{Op,Ap}Trigger families behind a descriptor"
```

> If Step 5 cannot be made green without behavior change, ABANDON this task (revert the branch to before Step 3), leave the six funcs as-is, and note it in the handoff. The LOC win is not worth a fidelity break.

---

### Task A8: Extract resolve-prologue helpers in `handlers_config.go` / `handlers_inv.go`

Preserves the 1:1 handler funcs exactly — only the repeated prologue/tail moves into helpers. `resolveInv` (handlers_inv.go:32) and `paramLookup` (handlers_config.go:17) are the existing precedents for this pattern.

**Files:**
- Modify: `pkg/script/handlers_config.go`, `pkg/script/handlers_inv.go`
- Test: existing `pkg/script/*_test.go`

**Interfaces:**
- Produces in package `script`:
  - `resolveLocType(s, op) (*loctype.LocType, error)`, `resolveNpcType(...)`, `resolveObjType(...)` — the 4-line `requireConfigs → PopInt → check*Type → Configs.*Type` prologue. (Use the real type names from the current handlers.)
  - `pushStringOrNull(s, v string)` and `pushNameOrDebugOrNull(s, name, debug string)` tails.
  - `resolveInvOrErr(s, typeID, op) (*inventory.Inventory, error)` folding the `resolveInv` + nil-check + `fmt.Errorf("%s: no inv for type %d", op, typeID)`.

- [ ] **Step 1: handlers_inv — add `resolveInvOrErr`, migrate the uniform nil-check sites**

Add the helper; replace the ~9 byte-identical `inv := resolveInv(...); if inv == nil { <2-line comment>; return fmt.Errorf(...) }` blocks with `inv, err := resolveInvOrErr(...); if err != nil { return err }`. **Skip** sites with an intervening `-1` short-circuit (e.g. `handleInvTotal:76`) — leave those as-is. Fold the repeated "Defensive: unreachable post-checkInvType" comment into the helper's doc-comment.

- [ ] **Step 2: handlers_inv — compile + test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ 2>&1 | tail -10`
Expected: `ok`.

- [ ] **Step 3: handlers_config — add per-type resolvers + push tails, migrate the uniform getters**

Add `resolveLocType/NpcType/ObjType` and `pushStringOrNull`/`pushNameOrDebugOrNull`; migrate the ~20 single-id getters (LC/NC/OC across 203–731). **Exclude** `handleNcOp` (429) and the `*Param` delegators (they pop a second int in a different order — different behavior).

- [ ] **Step 4: handlers_config — compile + test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ 2>&1 | tail -10`
Expected: `ok`. If any handler's error order changed (a test catches a different error first), you migrated a non-uniform site — revert that one.

- [ ] **Step 5: Compile-all + format gate + commit**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/script/
git add pkg/script/handlers_config.go pkg/script/handlers_inv.go
git commit --no-gpg-sign -m "refactor(script): extract config/inv resolve-prologue helpers"
```

---

### Task A9: Collapse small adapter boilerplate (inv-button factory + cheat helpers)

`handleInvButton1..5` are five byte-identical adapters differing only by a trailing int literal. The cheat-handler cluster has repeated count/int32 clamps and a "not logged in" lookup guard. All pure Go adapters with no "mirrors TS" comment — zero fidelity cost.

**Files:**
- Modify: `modules/world/handlers_game.go`

- [ ] **Step 1: Replace the five inv-button adapters with a factory**

`modules/world/handlers_game.go` — remove `handleInvButton1..5` (166-199) and register via a closure factory. Read the exact `gameHandler` type and the registration block (113-117) first, then:
```go
func invButtonHandler(n int) gameHandler {
	return func(/* same params as handleInvButton1 */) /* same return */ {
		return handleInvButton(/* same receiver/args */, n)
	}
}
```
Register `gameHandlers[OpcInvButtonN] = invButtonHandler(N)` for N=1..5 (use the real opcode constant names).

- [ ] **Step 2: Extract the cheat-handler helpers**

Extract `clampCount(sub, idx)` (from 882-891 / 1230-1239), `clampInt32(sub, idx)` (1057-1063 / 1136-1142), and `lookupOrReport(p, name) *Player` (the 5× `LookupPlayerByUsername` + "is not logged in" guard at 1010/1118/1197/1221/1338). Replace call sites. Read the exact surrounding code before extracting — arg indices differ between give/giveother, so `clampCount` takes the index as a parameter.

- [ ] **Step 3: Compile-all + format + test**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l modules/world/
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ 2>&1 | tail -10
```
Expected: clean; `ok`.

- [ ] **Step 4: Commit**

```bash
git add modules/world/handlers_game.go
git commit --no-gpg-sign -m "refactor(world): factory for inv-button adapters; shared cheat-handler helpers"
```

---

### Task A10: Tier-3 cosmetic consistency (lowest value — do last, drop if churn outweighs benefit)

Three small consistency items. These are cosmetic; the audit itself flagged them as "largely churn." Keep this task tightly scoped.

**Files:**
- Modify: `pkg/pathfinder/routefinder/naiveroutefinder.go` (import + one call), `pkg/pack/compiler/ast/nai206_field_existence_test.go:16`, and the verified-safe `for range N` subset below.

- [ ] **Step 1: `math/rand` → `math/rand/v2` in the one straggler file**

`pkg/pathfinder/routefinder/naiveroutefinder.go:5` — change import `"math/rand"` → `"math/rand/v2"`; line ~126 `rand.Intn(...)` → `rand.IntN(...)`. (Every other native file already uses v2.) Verify: `grep -rn '"math/rand"' pkg modules cmd | grep -v '/v2'` prints nothing afterward (excluding port dirs).

- [ ] **Step 2: `interface{}` → `any` in the one native test**

`pkg/pack/compiler/ast/nai206_field_existence_test.go:16` — `instance interface{}` → `instance any`.

- [ ] **Step 3: Convert the verified-safe `for i := 0; i < N; i++` → `for i := range N`**

Only these (audit-verified: fixed integer bound, index used trivially, body does not mutate the counter or break early):
`modules/world/appearance.go:67,88`; `modules/world/handlers_game.go:910,1245`; `modules/world/login_resync.go:90`; `modules/world/player.go:778,1002`; `pkg/objtype/playerstat.go:102`.
For each, rewrite `for i := 0; i < N; i++ {` as `for i := range N {`. **Do NOT touch** the `pkg/pack/compiler/symbols*.go` `<=`-bound loops (deliberate TS-mirror, per `typeinfo.go:80`) or any algorithm-port file.

- [ ] **Step 4: Compile-all + format + broad test**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg modules cmd
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ ./pkg/objtype/ ./pkg/pathfinder/... 2>&1 | tail -10
```
Expected: clean; `ok`.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit --no-gpg-sign -m "chore: tier-3 idiom consistency (rand/v2, any, for-range)"
```

---

### Task A11: Phase-A full-suite gate + merge to rev-274

- [ ] **Step 1: Full suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -30`
Expected: all `ok` / `no test files`, no `FAIL`.

- [ ] **Step 2: Race on the world module**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ 2>&1 | tail -10`
Expected: `ok`.

- [ ] **Step 3: Merge the feature branch `--no-ff`**

```bash
git checkout rev-274
git merge --no-ff fix/tech-debt-cleanup -m "Merge fix/tech-debt-cleanup: 2026-07-03 tech-debt batch"
```

- [ ] **Step 4: Record the rev-274 merge SHA** for the Phase-B port reference:

```bash
git rev-parse HEAD   # note this SHA; Phase B ports FROM it
```

---

## PHASE B — Port to rev-254, rev-245.2, rev-244, rev-225

Per the cross-rev methodology, port the **same set of changes** to each other branch in its dedicated worktree. Each item is either COPYABLE (target file byte-identical to rev-274's pre-change state → `git checkout` the file) or ADAPT (branch has diverged → hand-apply). Order branches newest→oldest (254, 245.2, 244, 225) since drift grows with distance from 274.

> **Per-branch applicability is not assumed.** Some rev-274 code may not exist, or may differ, on older branches (e.g. server.go layout, pack_configs func set, the Shape list, the fire*Trigger deviations). Each task below starts by *checking* the target, then COPYABLE-or-ADAPT-or-SKIP.

### Task B-<branch>: repeat for branch ∈ {rev-254, rev-245.2, rev-244, rev-225}

Run this whole block once per branch, in that branch's worktree (`~/Code/github.com/zsrv/goscape-rev254`, `-rev245.2`, `-rev244`, `-rev225`).

- [ ] **Step 1: Branch + baseline**

```bash
cd <worktree>
git checkout <rev-branch>
git checkout -b fix/tech-debt-cleanup
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -20   # must be green before porting
```

- [ ] **Step 2: For each Phase-A task (A1–A10), decide COPYABLE vs ADAPT vs SKIP**

For each file touched in Phase A, from the branch worktree:
```bash
# Is the target file identical to rev-274's PRE-change state? Compare against the
# rev-274 merge-base of this branch (or the file at the parent of the A-task commit).
git diff rev-274~<N> -- <file>   # inspect drift; if the relevant region matches, COPYABLE
```
- **COPYABLE** (region byte-identical): `git checkout rev-274 -- <file>` — BUT only when the *entire* file is safe to take wholesale. For partial files (most of these), hand-apply the same edit (ADAPT).
- **ADAPT**: apply the same transformation to the branch's version of the code. Watch the memory gotchas: stale-LSP-during-port (re-read the file fresh; don't trust cached diagnostics), and per-branch config/type drift.
- **SKIP**: if the target code doesn't exist on this branch (e.g. a Shape absent, a handler not yet ported, server.go smaller), skip that item and record it. For A3 (LayerOf test), regenerate the test table from *this branch's* `shape.go` — the shape count may differ; the `len(cases) != int(ShapeGroundDecor)+1` guard will catch a mismatch.

  Task-specific notes:
  - **A1 (chan struct{})**: verify this branch's `quit` field is still `chan interface{}`; grep the branch's `modules/world/` for all sites.
  - **A4 (pack_configs)**: the `packAndSave*` set may differ per rev (fewer configs on older branches). Only extract helpers over the funcs that exist.
  - **A5 (server.go split)**: server.go may be smaller/differently-shaped on older branches. Move only the function groups that exist; create only the needed `server_*.go` files. If server.go is already modest (< ~1200 LOC) on a branch, SKIP the split for that branch and note it.
  - **A7 (fire*Trigger)**: the per-type deviations (S6j-D2/D4) may differ per rev. Re-tabulate the 4-axis variation from *this branch's* funcs; if they don't cleanly share a shape, SKIP consolidation on that branch.

- [ ] **Step 3: Compile-all + format + full suite (per branch)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg modules cmd
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -30
CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ 2>&1 | tail -10
```
Expected: clean; all `ok`.

- [ ] **Step 4: Commit per-item (mirror Phase-A commit messages) and merge `--no-ff`**

Commit each item with the same message as its Phase-A counterpart (skipped items simply have no commit). Then:
```bash
git checkout <rev-branch>
git merge --no-ff fix/tech-debt-cleanup -m "Merge fix/tech-debt-cleanup: 2026-07-03 tech-debt batch"
git rev-parse HEAD   # record per-branch tip SHA
```

- [ ] **Step 5: Record what was SKIPPED on this branch** (for the handoff memory).

---

## PHASE C — Documentation & handoff

### Task C1: Record the batch in PORTING.md and update memory

- [ ] **Step 1: Append a batch entry to `docs/PORTING.md`** on rev-274 (in the "Recent audit history" tail), listing: the audit date (2026-07-03), the tasks landed, per-branch tip SHAs, and any items SKIPPED per branch with the reason.

- [ ] **Step 2: Commit the doc** on rev-274 (`docs(porting): record 2026-07-03 tech-debt cleanup batch`), then port the same doc note to the other branches' PORTING.md if they carry per-branch trails (match existing convention).

- [ ] **Step 3: Update the arch-review memory file** (`.../memory/arch_review_2026_07_02_findings.md` or a new `tech_debt_cleanup_2026_07_03.md`) with: the batch scope, per-branch tip SHAs, SKIPPED items, and any new gotcha discovered during the port. Add a one-line pointer to `MEMORY.md`.

- [ ] **Step 4: Report to the user** — summary of what landed on each branch, what was skipped and why, confirmation nothing was pushed, and that `main` was untouched.

---

## Self-Review

**Spec coverage** — every audit finding is mapped to a task:
- Tier 1: #1→A1, #2→A2, #3→A4, #4→A5, #5→A3. ✓
- Tier 2: min/max→A6, fire*Trigger→A7, resolve-prologue→A8, small adapters→A9. ✓
- Tier 3: for-range / rand-v2 / any→A10. ✓
- Cross-branch (all 5)→Phase B. Docs/memory→Phase C. ✓
- Explicitly-excluded fidelity-locked items are listed in Global Constraints and NOT given tasks. ✓

**Placeholder scan** — mechanical items (A1, A2, A3, A6, A10) carry full before/after code. Structural items (A5, A7) carry function-move/axis manifests plus a mandate to read exact current code before editing (line numbers drift as earlier tasks touch the same files) and per-step test gates — this is deliberate, not a placeholder, because pasting 2000 lines of unchanged code verbatim would be noise and the code already exists to read.

**Type consistency** — helper names are fixed across the plan: `readAndValidate`/`saveTransmittedConfig` (A4), `fireOpTrigger`/`fireApTrigger` + descriptor (A7), `resolveLocType`/`resolveNpcType`/`resolveObjType`/`pushStringOrNull`/`pushNameOrDebugOrNull`/`resolveInvOrErr` (A8), `invButtonHandler`/`clampCount`/`clampInt32`/`lookupOrReport` (A9). Exact parameter/return types are to be read from current code before writing each helper (called out in the relevant steps) because the surrounding signatures (`gameHandler`, `ParseFn`, cache/jag types) are branch-specific.

**Fidelity risk ordering** — safest tasks first (A1–A6), the one high-care refactor (A7) is isolated with an explicit ABANDON path, and Tier-3 churn (A10) is last and droppable.
