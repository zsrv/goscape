# NAI-114 Stage 4 — OPHELDU reject-gate probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 1 entry log + 17 reject-site logs to `handleOpHeldU` (after reverting Stage-3 instrumentation) so that one user smoke run binds which early-return gate rejects tutorial firemaking OPHELDU events.

**Architecture:** Three commits — two `git revert` of Stage-3 transients (`f3e5846`, `f9cce99`), then one transient instrumentation commit on top. All Stage-4 instrumentation is inline in `modules/world/handler_opheld.go`; one small package-level helper (`snapshotInvListenerKeys`) keeps the listener-keys snapshot DRY across sites #9 and #12. No new tests; existing `handler_opheld_test.go` must remain green; this is throwaway debug code reverted at NAI-114 Stage 5 close.

**Tech Stack:** Go 1.26+, `log/slog` stdlib (already used elsewhere in `modules/world`), `sort` stdlib.

---

## File Structure

| File | Status | Responsibility |
|------|--------|----------------|
| `modules/world/handler_opheld.go` | Modify | Add imports `log/slog` + `sort`; add `snapshotInvListenerKeys` helper; add 1 entry log + 17 reject logs in `handleOpHeldU`. |
| `modules/world/debug_nai114.go` | Delete (via revert) | Removed by Task 2 revert of `f9cce99`. |
| `modules/world/server.go` | Modify (via revert) | Restored to pre-Stage-3 shape by Task 2 revert. |

---

## Pre-flight (controller, not implementer)

- [ ] **Verify HEAD is `60b33a5`** (Stage 4 spec commit) and working tree is clean (only `.claude/` and `test_typed_nil.go` should be untracked, no modified files).

```bash
git rev-parse HEAD
# Expected: 60b33a5...
git status --short
# Expected: only ?? .claude/ and ?? test_typed_nil.go
```

- [ ] **Verify revert sources exist:**

```bash
git cat-file -t f3e5846 && git cat-file -t f9cce99
# Expected: commit\ncommit
```

---

## Task 1: Revert Stage-3 inline DEBUGs (`f3e5846`)

**Files:**
- Modify (via revert): `modules/world/handler_opheld.go`

- [ ] **Step 1.1: Run the revert**

```bash
git revert --no-gpg-sign --no-edit f3e5846
```

Expected: clean revert, no conflicts. Commit subject auto-generated as `Revert "chore(debug): NAI-114 Stage 3 — opheldu trigger-lookup hit-trace"`.

- [ ] **Step 1.2: Verify the revert touched only handler_opheld.go**

```bash
git show HEAD --stat
```

Expected:
```
 modules/world/handler_opheld.go | 21 -------------------
 1 file changed, 21 deletions(-)
```

- [ ] **Step 1.3: Verify post-revert handler shape**

Read `modules/world/handler_opheld.go` lines 365-400. The `handleOpHeldU` function should end with:

```go
		// Members-only gate (TS OpHeldUHandler.ts:90-93).
		if (objType.Members || useObjType.Members) && !s.cfg.NodeMembers {
			p.MessageGame("To use this item please login to a members' server.")
			return nil
		}

		// 4-arm trigger fallback (TS OpHeldUHandler.ts:96-117); first hit wins.
		sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)

		if sf == nil {
			sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, useObjType.ConfigType.ID, -1)
			// Arm (b): UNCONDITIONAL swap whenever (a) misses, regardless of
			// whether (b)'s lookup succeeded (TS OpHeldUHandler.ts:101-102).
			p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
			p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
		}

		if sf == nil && objType.Category != -1 {
			sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
		}

		if sf == nil && useObjType.Category != -1 {
			sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
			// Arm (d): UNCONDITIONAL swap whenever (c) misses or is skipped,
			// regardless of whether (d)'s lookup succeeded (TS OpHeldUHandler.ts:115-116).
			p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
			p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
		}

		if sf == nil {
			p.MessageGame("Nothing interesting happens.")
			return nil
		}

		s.runScript(sf, p, nil, true, nil, nil)
		return nil
	}
```

(Exact line numbers may shift; the shape above is what matters — no `s.log.Debug` calls in the trigger-lookup region.)

- [ ] **Step 1.4: Verify build + tests still pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: build clean, all PASS.

---

## Task 2: Revert Stage-3 boot-time registry log (`f9cce99`)

**Files:**
- Delete (via revert): `modules/world/debug_nai114.go`
- Modify (via revert): `modules/world/server.go`

- [ ] **Step 2.1: Run the revert**

```bash
git revert --no-gpg-sign --no-edit f9cce99
```

Expected: clean revert (no conflict — `60b33a5` Stage 4 spec doesn't touch these files; Task 1 revert doesn't touch them either).

- [ ] **Step 2.2: Verify the revert removed `debug_nai114.go` and 3 lines from server.go**

```bash
git show HEAD --stat
```

Expected:
```
 modules/world/debug_nai114.go | 30 ------------------------------
 modules/world/server.go       |  3 ---
 2 files changed, 33 deletions(-)
```

- [ ] **Step 2.3: Confirm `debug_nai114.go` is gone**

```bash
ls modules/world/debug_nai114.go 2>&1
```

Expected: `ls: cannot access 'modules/world/debug_nai114.go': No such file or directory`

- [ ] **Step 2.4: Verify build + tests still pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: build clean, all PASS.

- [ ] **Step 2.5: Confirm `a999f1e` Provider.Names() accessor still present (KEEP — long-term API)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go doc github.com/zsrv/goscape/pkg/script Provider.Names
```

Expected: prints docstring for `func (p *Provider) Names() []string`.

---

## Task 3: Add Stage-4 reject-gate instrumentation

**Files:**
- Modify: `modules/world/handler_opheld.go`

This task is purely additive. No new tests are required — the instrumentation has no behavioral effect, so existing `handler_opheld_test.go` assertions remain valid as-is. The verification step is "build clean + existing tests still PASS".

- [ ] **Step 3.1: Read current `handleOpHeldU` (post-revert) and re-anchor line refs**

Read `modules/world/handler_opheld.go` from `func handleOpHeldU(p *Player, payload []byte) error {` through the members-only `return nil`. Confirm the 17 reject sites enumerated in the spec §4.3 table all exist as `return nil` statements in the same shape:

| Spec # | Pattern to find |
|---|---|
| 1 | `if p.client == nil \|\| p.client.server == nil { return nil }` |
| 2 | `if p.delayed && s.currentTick < p.delayedUntil { return nil }` |
| 3 | `if len(payload) < 12 { return nil }` |
| 4 | `if comId != useComId { return nil }` |
| 5 | `if com == nil \|\| !com.Usable { return nil }` |
| 6 | `if !p.IsComponentVisible(com) { return nil }` |
| 7 | `if useCom == nil \|\| !useCom.Usable { return nil }` |
| 8 | `if !p.IsComponentVisible(useCom) { return nil }` |
| 9 | `listener, ok := p.invListeners[comId]; if !ok { return nil }` |
| 10 | `inv := resolveListenerInv(s, listener); if inv == nil { return nil }` |
| 11 | `if !inv.HasAt(slot, obj) { p.moveClickRequest = false; p.ClearPendingAction(); return nil }` |
| 12 | `useListener, ok := p.invListeners[useComId]; if !ok { return nil }` |
| 13 | `useInv := resolveListenerInv(s, useListener); if useInv == nil { return nil }` |
| 14 | `if !useInv.HasAt(useSlot, useObj) { p.moveClickRequest = false; p.ClearPendingAction(); return nil }` |
| 15 | obj defensive check: `if s.objTypes == nil \|\| obj < 0 \|\| obj >= len(s.objTypes.Configs) \|\| s.objTypes.Configs[obj] == nil { return nil // goscape defensive; TS throws here }` |
| 16 | useObj defensive check: `if useObj < 0 \|\| useObj >= len(s.objTypes.Configs) \|\| s.objTypes.Configs[useObj] == nil { return nil // goscape defensive; TS throws here }` |
| 17 | `if (objType.Members \|\| useObjType.Members) && !s.cfg.NodeMembers { p.MessageGame(...); return nil }` |

If any pattern is missing, STOP and surface — the spec's hypothesis space assumed a specific shape.

- [ ] **Step 3.2: Add imports `log/slog` and `sort` to `modules/world/handler_opheld.go`**

The current import block (post-revert) is:

```go
import (
	"fmt"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

Replace with:

```go
import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 3.3: Add `snapshotInvListenerKeys` package-level helper at end of `modules/world/handler_opheld.go`**

Append this helper after the closing `}` of the file (i.e. after `handleOpHeldU`'s closing brace):

```go
// snapshotInvListenerKeys returns up to 16 sorted comId keys from
// p.invListeners. NAI-114 Stage 4 throwaway instrumentation; reverted
// at Stage 5 close.
func snapshotInvListenerKeys(p *Player) []int {
	keys := make([]int, 0, len(p.invListeners))
	for k := range p.invListeners {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	if len(keys) > 16 {
		keys = keys[:16]
	}
	return keys
}
```

- [ ] **Step 3.4: Add reject log at site #1 (client_nil) using `slog.Default()`**

`s.log` is unreachable in this branch (`p.client.server` may be nil). Use `slog.Default()`.

Find:

```go
	if p.client == nil || p.client.server == nil {
		return nil
	}
```

Replace with:

```go
	if p.client == nil || p.client.server == nil {
		slog.Default().Debug("opheldu reject", "gate", "client_nil",
			"client_nil", p.client == nil)
		return nil
	}
```

- [ ] **Step 3.5: Add reject log at site #2 (delayed)**

Find:

```go
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
```

Replace with:

```go
	if p.delayed && s.currentTick < p.delayedUntil {
		s.log.Debug("opheldu reject", "gate", "delayed",
			"currentTick", s.currentTick, "delayedUntil", p.delayedUntil)
		return nil
	}
```

- [ ] **Step 3.6: Add reject log at site #3 (short_payload)**

Find:

```go
	if len(payload) < 12 {
		return nil
	}
```

Replace with:

```go
	if len(payload) < 12 {
		s.log.Debug("opheldu reject", "gate", "short_payload",
			"payload_len", len(payload))
		return nil
	}
```

- [ ] **Step 3.7: Add the entry log immediately after the packet decode block**

Find:

```go
	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useComId := int(r.G2())

	if comId != useComId {
```

Replace with:

```go
	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useComId := int(r.G2())

	s.log.Debug("opheldu entry",
		"tick", s.currentTick,
		"obj", obj, "slot", slot, "comId", comId,
		"useObj", useObj, "useSlot", useSlot, "useComId", useComId,
		"delayed", p.delayed, "delayedUntil", p.delayedUntil)

	if comId != useComId {
```

- [ ] **Step 3.8: Add reject log at site #4 (comId_mismatch)**

Find:

```go
	if comId != useComId {
		return nil
	}
```

Replace with:

```go
	if comId != useComId {
		s.log.Debug("opheldu reject", "gate", "comId_mismatch",
			"comId", comId, "useComId", useComId)
		return nil
	}
```

- [ ] **Step 3.9: Add reject log at site #5 (com_nil_or_unusable)**

Find:

```go
	com := s.lookupComponent(comId)
	if com == nil || !com.Usable {
		return nil
	}
```

Replace with:

```go
	com := s.lookupComponent(comId)
	if com == nil || !com.Usable {
		s.log.Debug("opheldu reject", "gate", "com_nil_or_unusable",
			"com_nil", com == nil, "com_usable", com != nil && com.Usable)
		return nil
	}
```

- [ ] **Step 3.10: Add reject log at site #6 (com_invisible)**

Find:

```go
	if !p.IsComponentVisible(com) {
		return nil
	}
```

(There are two `IsComponentVisible(com)` calls in the file — the one for `handleOpHeldU` is the FIRST occurrence inside `handleOpHeldU`, immediately after the `com_nil_or_unusable` block. To disambiguate, pair with surrounding context.)

Replace with:

```go
	if !p.IsComponentVisible(com) {
		s.log.Debug("opheldu reject", "gate", "com_invisible")
		return nil
	}
```

- [ ] **Step 3.11: Add reject log at site #7 (useCom_nil_or_unusable)**

Find:

```go
	useCom := s.lookupComponent(useComId)
	if useCom == nil || !useCom.Usable {
		return nil
	}
```

Replace with:

```go
	useCom := s.lookupComponent(useComId)
	if useCom == nil || !useCom.Usable {
		s.log.Debug("opheldu reject", "gate", "useCom_nil_or_unusable",
			"useCom_nil", useCom == nil, "useCom_usable", useCom != nil && useCom.Usable)
		return nil
	}
```

- [ ] **Step 3.12: Add reject log at site #8 (useCom_invisible)**

Find (the SECOND `IsComponentVisible` call in `handleOpHeldU`, with `useCom`):

```go
	if !p.IsComponentVisible(useCom) {
		return nil
	}
```

Replace with:

```go
	if !p.IsComponentVisible(useCom) {
		s.log.Debug("opheldu reject", "gate", "useCom_invisible")
		return nil
	}
```

- [ ] **Step 3.13: Add reject log at site #9 (invListener_missing)**

Find:

```go
	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
```

Replace with:

```go
	listener, ok := p.invListeners[comId]
	if !ok {
		s.log.Debug("opheldu reject", "gate", "invListener_missing",
			"comId", comId,
			"listener_count", len(p.invListeners),
			"listener_keys", snapshotInvListenerKeys(p))
		return nil
	}
```

- [ ] **Step 3.14: Add reject log at site #10 (inv_unresolved)**

Find:

```go
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
```

Replace with:

```go
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		s.log.Debug("opheldu reject", "gate", "inv_unresolved",
			"listener_type", listener.Type, "listener_source", listener.Source)
		return nil
	}
```

- [ ] **Step 3.15: Add reject log at site #11 (inv_hasAt_failed) BEFORE the TS-cleanup**

Find:

```go
	if !inv.HasAt(slot, obj) {
		// TS OpHeldUHandler.ts:54-58 — extra cleanup on this specific reject.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}
```

Replace with:

```go
	if !inv.HasAt(slot, obj) {
		s.log.Debug("opheldu reject", "gate", "inv_hasAt_failed",
			"slot", slot, "obj", obj)
		// TS OpHeldUHandler.ts:54-58 — extra cleanup on this specific reject.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}
```

- [ ] **Step 3.16: Add reject log at site #12 (useInvListener_missing)**

Find:

```go
	useListener, ok := p.invListeners[useComId]
	if !ok {
		return nil
	}
```

Replace with:

```go
	useListener, ok := p.invListeners[useComId]
	if !ok {
		s.log.Debug("opheldu reject", "gate", "useInvListener_missing",
			"useComId", useComId,
			"listener_count", len(p.invListeners),
			"listener_keys", snapshotInvListenerKeys(p))
		return nil
	}
```

- [ ] **Step 3.17: Add reject log at site #13 (useInv_unresolved)**

Find:

```go
	useInv := resolveListenerInv(s, useListener)
	if useInv == nil {
		return nil
	}
```

Replace with:

```go
	useInv := resolveListenerInv(s, useListener)
	if useInv == nil {
		s.log.Debug("opheldu reject", "gate", "useInv_unresolved",
			"useListener_type", useListener.Type, "useListener_source", useListener.Source)
		return nil
	}
```

- [ ] **Step 3.18: Add reject log at site #14 (useInv_hasAt_failed) BEFORE the TS-cleanup**

Find:

```go
	if !useInv.HasAt(useSlot, useObj) {
		// TS OpHeldUHandler.ts:71-75.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}
```

Replace with:

```go
	if !useInv.HasAt(useSlot, useObj) {
		s.log.Debug("opheldu reject", "gate", "useInv_hasAt_failed",
			"useSlot", useSlot, "useObj", useObj)
		// TS OpHeldUHandler.ts:71-75.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}
```

- [ ] **Step 3.19: Add reject log at site #15 (objType_unregistered for obj)**

Find:

```go
	if s.objTypes == nil || obj < 0 || obj >= len(s.objTypes.Configs) || s.objTypes.Configs[obj] == nil {
		return nil // goscape defensive; TS throws here
	}
```

Replace with:

```go
	if s.objTypes == nil || obj < 0 || obj >= len(s.objTypes.Configs) || s.objTypes.Configs[obj] == nil {
		s.log.Debug("opheldu reject", "gate", "objType_unregistered",
			"which", "obj", "id", obj)
		return nil // goscape defensive; TS throws here
	}
```

- [ ] **Step 3.20: Add reject log at site #16 (objType_unregistered for useObj)**

Find:

```go
	if useObj < 0 || useObj >= len(s.objTypes.Configs) || s.objTypes.Configs[useObj] == nil {
		return nil // goscape defensive; TS throws here
	}
```

Replace with:

```go
	if useObj < 0 || useObj >= len(s.objTypes.Configs) || s.objTypes.Configs[useObj] == nil {
		s.log.Debug("opheldu reject", "gate", "objType_unregistered",
			"which", "useObj", "id", useObj)
		return nil // goscape defensive; TS throws here
	}
```

- [ ] **Step 3.21: Add reject log at site #17 (members_only)**

Find:

```go
	// Members-only gate (TS OpHeldUHandler.ts:90-93).
	if (objType.Members || useObjType.Members) && !s.cfg.NodeMembers {
		p.MessageGame("To use this item please login to a members' server.")
		return nil
	}
```

Replace with:

```go
	// Members-only gate (TS OpHeldUHandler.ts:90-93).
	if (objType.Members || useObjType.Members) && !s.cfg.NodeMembers {
		s.log.Debug("opheldu reject", "gate", "members_only",
			"obj_members", objType.Members,
			"useObj_members", useObjType.Members,
			"node_members", s.cfg.NodeMembers)
		p.MessageGame("To use this item please login to a members' server.")
		return nil
	}
```

- [ ] **Step 3.22: Build + run all tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: build clean (no unused-import errors — `slog` and `sort` are used by Steps 3.4 and 3.3 respectively), all PASS.

If build fails on `slog` or `sort` unused, double-check Steps 3.3 and 3.4 actually wrote the helper / the slog.Default() call.

If any test fails: STOP. The instrumentation is supposed to be additive. Any test failure means a literal-byte mismatch (typo) was introduced in the find-and-replace. Diff against HEAD~1 to locate.

- [ ] **Step 3.23: Verify the 18 instrumentation lines + 1 helper are all present**

```bash
grep -c 'opheldu reject' modules/world/handler_opheld.go
# Expected: 17

grep -c 'opheldu entry' modules/world/handler_opheld.go
# Expected: 1

grep -c 'snapshotInvListenerKeys' modules/world/handler_opheld.go
# Expected: 3 (1 definition + 2 call sites at #9, #12)

grep -c 'slog.Default()' modules/world/handler_opheld.go
# Expected: 1 (site #1)
```

If any count is wrong: locate the missing one against the spec §4.3 table.

- [ ] **Step 3.24: Commit**

```bash
git add modules/world/handler_opheld.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation

Adds 1 entry log + 17 inline DEBUG reject-site logs to handleOpHeldU
covering every early-return gate between L272-L368. Stage 3 smoke
proved the handler enters but exits via one of these gates upstream of
the trigger lookup; this pass binds which gate fires for tutorial
firemaking (obj=2511 logs, useObj=590 tinderbox, comId=3214) in a
single smoke iteration.

Site #1 (client_nil) uses slog.Default() since s.log is unreachable in
that branch. Sites #9 and #12 (invListener_missing) share a small
package-level helper snapshotInvListenerKeys that returns up to 16
sorted comId keys.

All instrumentation is additive — no behavior change. Existing
handler_opheld_test.go assertions remain green; no new tests added
(throwaway probe code). Reverted at NAI-114 Stage 5 close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3.25: Verify commit content matches stated scope**

```bash
git show HEAD --stat
```

Expected: `modules/world/handler_opheld.go | <about 30-40> +`. Single-file change, additions only.

```bash
git status --short
```

Expected: only `?? .claude/` and `?? test_typed_nil.go` (working tree clean otherwise).

Per `implementer_commit_content_verify` memory: confirm the diff is what we said it is, not some scope drift.

---

## Task 4: Smoke handoff to user

Stage 4 is diagnostic-only; no Claude-runnable smoke. The user launches goscape and walks Tutorial Island fire-making.

- [ ] **Step 4.1: Confirm three commits landed in expected order**

```bash
git log --oneline -5
```

Expected (top to bottom):
```
<sha> chore(debug): NAI-114 Stage 4 — opheldu reject-gate instrumentation
<sha> Revert "chore(debug): NAI-114 Stage 3 — boot-time opheldu script-registry log"
<sha> Revert "chore(debug): NAI-114 Stage 3 — opheldu trigger-lookup hit-trace"
<sha> docs(plan): NAI-114 Stage 4 — reject-gate probe plan
60b33a5 docs(spec): NAI-114 Stage 4 — reject-gate probe design
```

Per memory `smoke_test_server_handoff`, the smoke must be user-launched.

- [ ] **Step 4.2: Emit paste-ready smoke handoff prompt**

Surface this verbatim to the user:

> Stage 4 instrumentation landed. To smoke:
>
> 1. `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml`
> 2. Connect via Java client rev-225, log in as the same tutorial-firemaking-step character used in Stage 3.
> 3. At the firemaking step, use tinderbox on logs (3-5 attempts).
> 4. Stop the server and capture stdout for any line containing `"opheldu entry"` or `"opheldu reject"`.
>
> Expected: each OPHELDU packet (opcode 130, len=12) produces exactly one `opheldu entry` log followed by exactly one `opheldu reject` log with `gate=<name>`.
>
> Decision matrix (spec §5.1) routes Stage 5:
> - Most-likely: `invListener_missing`, `inv_hasAt_failed`, `delayed`
> - Less-likely: `com_invisible`, `useCom_invisible`, `inv_unresolved`, `members_only`, `objType_unregistered`
> - Rule-out: `client_nil`, `short_payload`, `comId_mismatch`, `com_nil_or_unusable`, `useCom_nil_or_unusable`, `useInvListener_missing`, `useInv_unresolved`, `useInv_hasAt_failed`
>
> Paste the captured `opheldu entry`/`opheldu reject` lines back to start the Stage 5 brainstorm.

---

## Self-Review

**1. Spec coverage:**

| Spec section | Plan task |
|---|---|
| §3 In-scope: revert `f3e5846` | Task 1 |
| §3 In-scope: revert `f9cce99` | Task 2 |
| §3 In-scope: 1 entry log + 17 reject logs in handleOpHeldU | Task 3 (Steps 3.4-3.21) |
| §4.1 Commit sequence | Task 1 (commit 3), Task 2 (commit 4), Task 3.24 (commit 5) |
| §4.2 Entry log site/shape | Step 3.7 |
| §4.3 17 reject-site table | Steps 3.4 (site #1), 3.5 (#2), 3.6 (#3), 3.8 (#4), 3.9 (#5), 3.10 (#6), 3.11 (#7), 3.12 (#8), 3.13 (#9), 3.14 (#10), 3.15 (#11), 3.16 (#12), 3.17 (#13), 3.18 (#14), 3.19 (#15), 3.20 (#16), 3.21 (#17) |
| §4.4 listener_keys cap=16 | Step 3.3 (helper); Steps 3.13/3.16 use it |
| §4.5 Build + test verification | Step 3.22 |
| §5 Smoke handoff | Task 4 |
| §6 R6 (post-revert line drift) | Step 3.1 (re-anchor by pattern, not line number) |
| §6 R7 (out-of-order revert) | Step 4.1 (verify commit order) |

All §3 in-scope items have tasks; all §4 deliverables are step-anchored.

**2. Placeholder scan:** none. Every step has either an exact command or an exact code block.

**3. Type/identifier consistency:**
- `snapshotInvListenerKeys(p *Player) []int` defined at Step 3.3, called at Steps 3.13 and 3.16. Same signature.
- Gate names match spec §4.3 table verbatim (`client_nil`, `delayed`, `short_payload`, `comId_mismatch`, `com_nil_or_unusable`, `com_invisible`, `useCom_nil_or_unusable`, `useCom_invisible`, `invListener_missing`, `inv_unresolved`, `inv_hasAt_failed`, `useInvListener_missing`, `useInv_unresolved`, `useInv_hasAt_failed`, `objType_unregistered` (×2), `members_only`).
- Field names (`comId`, `useComId`, `slot`, `useSlot`, `obj`, `useObj`, `tick`, `delayedUntil`, `currentTick`, `payload_len`, `com_nil`, `com_usable`, `useCom_nil`, `useCom_usable`, `listener_count`, `listener_keys`, `listener_type`, `listener_source`, `useListener_type`, `useListener_source`, `which`, `id`, `obj_members`, `useObj_members`, `node_members`) — all consistent across spec §4.3 and plan steps.
