# NAI-28 Zone PathingEntity Subscription Port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close NAI-19-D1 deviation by porting the missing PathingEntity subscription primitive into goscape's existing `pkg/zone/` (Players/Npcs `DoublyLinkList[T]` lists with `EnterPlayer`/`LeavePlayer`/`EnterNpc`/`LeaveNpc` + ZoneGrid flag/unflag wiring), wire all 11 modules/world position-mutation sites, then migrate `huntNpcs` and `huntPlayers` from `pkg/grid` to `pkg/zone` subscription.

**Architecture:** Three bundles. Bundle 1 lands a generic intrusive `DoublyLinkList[T]` + `Element[T]` in a new `pkg/zone/list.go` plus `PlayerLike`/`NpcLike` interfaces + Zone subscription methods (no consumers; isolated unit tests). Bundle 2 wires 8 logical groups → 11 individual edits across `modules/world/{server.go, npc_registry.go, movement.go, npc_interaction.go, npc_ai.go, player_script.go, player.go, npc.go}` and retires the `NAI-19-D1` deviation tag. Bundle 3 migrates `huntNpcs` (`npc_hunt_entities.go:23-76`) and `huntPlayers` (`npc_hunt.go:108-180`) to use Zone subscription; `pkg/grid` write-side calls remain (full retirement deferred to future NAI sub-spec).

**Tech Stack:** Go 1.26+ (project uses `iter.Seq` from Go 1.23+ stdlib). Existing packages: `pkg/zone`, `pkg/grid`, `pkg/coordgrid`, `modules/world`. No new third-party dependencies.

**Predecessors:** NAI-27 closed at `c10cc7b`. Spec: `docs/superpowers/specs/2026-04-25-nai-28-zone-subscription-port-design.md` committed at `513830f`.

**Build/test commands** (per `CLAUDE.md`):
- Build: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
- Test all: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
- Test single package: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/...`
- Test single function: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/ -run TestDoublyLinkList_AddTailIncrementsSize`

**Commit discipline:** All commits use `git commit --no-gpg-sign`. Each commit body includes the standard `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>` trailer.

---

# Bundle 1 — `pkg/zone` Subscription Primitive

Bundle 1 delivers the `DoublyLinkList[T]` primitive and the `Zone` subscription API in isolation. All tests live in `pkg/zone/` and use direct stub structs implementing `PlayerLike`/`NpcLike` — no `modules/world` dependencies.

## Task 1.1: DoublyLinkList primitive (`pkg/zone/list.go`)

**Files:**
- Create: `pkg/zone/list.go`
- Create: `pkg/zone/list_test.go`

- [ ] **Step 1: Write failing tests in `pkg/zone/list_test.go`**

```go
package zone

import (
	"testing"
)

func TestDoublyLinkList_AddTailIncrementsSize(t *testing.T) {
	var l DoublyLinkList[int]
	if l.Size() != 0 {
		t.Errorf("empty list Size: got %d, want 0", l.Size())
	}
	l.AddTail(10)
	l.AddTail(20)
	l.AddTail(30)
	if l.Size() != 3 {
		t.Errorf("after 3 AddTail, Size: got %d, want 3", l.Size())
	}
}

func TestDoublyLinkList_AddTailReturnsElement(t *testing.T) {
	var l DoublyLinkList[int]
	e := l.AddTail(42)
	if e == nil {
		t.Fatal("AddTail returned nil Element")
	}
	if e.Value != 42 {
		t.Errorf("Element.Value: got %d, want 42", e.Value)
	}
}

func TestDoublyLinkList_UnlinkRemovesAndDecrements(t *testing.T) {
	var l DoublyLinkList[int]
	_ = l.AddTail(10)
	e2 := l.AddTail(20)
	_ = l.AddTail(30)
	e2.Unlink()
	if l.Size() != 2 {
		t.Errorf("after Unlink, Size: got %d, want 2", l.Size())
	}
	got := []int{}
	for v := range l.All(false) {
		got = append(got, v)
	}
	want := []int{10, 30}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("after Unlink, iteration: got %v, want %v", got, want)
	}
}

func TestDoublyLinkList_UnlinkIdempotent(t *testing.T) {
	var l DoublyLinkList[int]
	e := l.AddTail(99)
	e.Unlink()
	if l.Size() != 0 {
		t.Errorf("after first Unlink, Size: got %d, want 0", l.Size())
	}
	// Second call must not panic and must not decrement size.
	e.Unlink()
	if l.Size() != 0 {
		t.Errorf("after second Unlink, Size: got %d, want 0", l.Size())
	}
}

func TestDoublyLinkList_AllForwardOrderMatchesInsertion(t *testing.T) {
	var l DoublyLinkList[int]
	for _, v := range []int{1, 2, 3, 4, 5} {
		l.AddTail(v)
	}
	got := []int{}
	for v := range l.All(false) {
		got = append(got, v)
	}
	want := []int{1, 2, 3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All(false)[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestDoublyLinkList_AllReverseOrderMatchesInsertion(t *testing.T) {
	var l DoublyLinkList[int]
	for _, v := range []int{1, 2, 3, 4, 5} {
		l.AddTail(v)
	}
	got := []int{}
	for v := range l.All(true) {
		got = append(got, v)
	}
	want := []int{5, 4, 3, 2, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("All(true)[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestDoublyLinkList_EmptyAllYieldsNothing(t *testing.T) {
	var l DoublyLinkList[int]
	count := 0
	for range l.All(false) {
		count++
	}
	if count != 0 {
		t.Errorf("empty All count: got %d, want 0", count)
	}
}

func TestElement_UnlinkClearsListPointer(t *testing.T) {
	var l DoublyLinkList[int]
	e := l.AddTail(42)
	e.Unlink()
	if e.list != nil {
		t.Error("after Unlink, e.list should be nil for idempotency")
	}
}

func TestDoublyLinkList_UnlinkMiddleRelinksNeighbors(t *testing.T) {
	var l DoublyLinkList[int]
	e1 := l.AddTail(1)
	e2 := l.AddTail(2)
	e3 := l.AddTail(3)
	e2.Unlink()
	if e1.next != e3 {
		t.Error("after Unlink middle, e1.next should be e3")
	}
	if e3.prev != e1 {
		t.Error("after Unlink middle, e3.prev should be e1")
	}
}

func TestDoublyLinkList_UnlinkHeadUpdatesHead(t *testing.T) {
	var l DoublyLinkList[int]
	e1 := l.AddTail(1)
	e2 := l.AddTail(2)
	e1.Unlink()
	if l.head != e2 {
		t.Error("after Unlink head, list.head should be e2")
	}
}

func TestDoublyLinkList_UnlinkTailUpdatesTail(t *testing.T) {
	var l DoublyLinkList[int]
	e1 := l.AddTail(1)
	e2 := l.AddTail(2)
	e2.Unlink()
	if l.tail != e1 {
		t.Error("after Unlink tail, list.tail should be e1")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/ -run TestDoublyLinkList`
Expected: FAIL with "undefined: DoublyLinkList" / "undefined: Element"

- [ ] **Step 3: Implement `pkg/zone/list.go`**

```go
package zone

import "iter"

// Element is an intrusive doubly-linked-list node owning a Value of type T.
// Stored as *Element by callers so they can call Unlink() in O(1).
//
// Mirrors TS DoublyLinkable's role in Engine-TS's #/datastruct/DoublyLinkList,
// translated to Element-based composition (Go doesn't support TS's abstract-
// base inheritance shape; behavior is identical — same O(1) cost, same
// iteration order, same visible state).
type Element[T any] struct {
	next, prev *Element[T]
	list       *DoublyLinkList[T]
	Value      T
}

// Unlink removes e from its list. Idempotent — second call is a no-op
// (mirrors TS DoublyLinkable.unlink2 idempotency).
func (e *Element[T]) Unlink() {
	if e.list == nil {
		return
	}
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		e.list.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		e.list.tail = e.prev
	}
	e.list.size--
	e.next, e.prev, e.list = nil, nil, nil
}

// DoublyLinkList is an intrusive doubly-linked list. Zero value is a valid
// empty list. All operations are O(1) except All which is O(N).
//
// Mirrors TS DoublyLinkList<T> at Engine-TS/datastruct/DoublyLinkList.ts.
type DoublyLinkList[T any] struct {
	head, tail *Element[T]
	size       int
}

// AddTail appends v to the end of the list and returns the new Element.
// Caller stores the *Element to support O(1) Unlink.
func (l *DoublyLinkList[T]) AddTail(v T) *Element[T] {
	e := &Element[T]{Value: v, list: l, prev: l.tail}
	if l.tail != nil {
		l.tail.next = e
	} else {
		l.head = e
	}
	l.tail = e
	l.size++
	return e
}

// Size returns the number of elements in the list.
func (l *DoublyLinkList[T]) Size() int { return l.size }

// All returns an iterator over the list's values. reverse=false yields
// in insertion order; reverse=true yields in reverse insertion order.
//
// Mirrors TS DoublyLinkList.all(reverse?: boolean) generator.
func (l *DoublyLinkList[T]) All(reverse bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		if reverse {
			for n := l.tail; n != nil; n = n.prev {
				if !yield(n.Value) {
					return
				}
			}
			return
		}
		for n := l.head; n != nil; n = n.next {
			if !yield(n.Value) {
				return
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/ -run TestDoublyLinkList`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/ -run TestElement`
Expected: All 11 tests PASS.

Also run the existing pkg/zone tests to ensure no regressions:
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/...`
Expected: PASS (existing pre-Bundle-1 tests + new list tests all green)

- [ ] **Step 5: Commit**

```bash
git add pkg/zone/list.go pkg/zone/list_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(zone): NAI-28 Bundle 1 Task 1.1 — DoublyLinkList[T] + Element[T] primitive

Generic intrusive doubly-linked list in pkg/zone/list.go. Mirrors TS DoublyLinkList<T> from Engine-TS/datastruct/DoublyLinkList — same O(1) AddTail/Unlink semantics, same iteration order. Element-based composition (Go-idiom for TS's abstract-base inheritance) is externally invisible per true_to_ts_gate; no deviation tag needed.

11 unit tests cover size tracking, idempotent Unlink, forward/reverse iteration, head/tail/middle unlink relink invariants, and empty-list iteration safety.

No consumers in this commit. Zone subscription methods land in Task 1.3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 1.2: PlayerLike / NpcLike interfaces

**Files:**
- Modify: `pkg/zone/zone.go` (add interface declarations near top of file, after imports)

- [ ] **Step 1: Read current top-of-file structure**

Run: `head -30 pkg/zone/zone.go`
Expected: see `package zone` + import block + Zone struct declaration starting around line 16.

- [ ] **Step 2: Insert interfaces between import block and `Zone` struct doc-comment**

Edit `pkg/zone/zone.go` — add the following interfaces immediately after the closing `)` of the `import (` block (before the `// Zone is an 8×8 tile region...` doc-comment):

```go
// PlayerLike is the minimum surface Zone needs from a player-like entity.
// Defined here (rather than imported from modules/world) to avoid a cyclic
// import — modules/world imports pkg/zone, not the reverse.
//
// Mirrors TS Player's role inside Zone.enter / Zone.leave at Zone.ts:80-83
// (only IsValid + identity are needed; richer accessors stay in modules/world).
type PlayerLike interface {
	IsValid() bool
	Slot() int
}

// NpcLike is the minimum surface Zone needs from an npc-like entity.
// Same cyclic-import rationale as PlayerLike.
//
// Mirrors TS Npc's role inside Zone.enter / Zone.leave at Zone.ts:84-87.
type NpcLike interface {
	IsValid() bool
	Nid() int
}
```

- [ ] **Step 3: Verify compilation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./pkg/zone/...`
Expected: build succeeds (no consumers yet, but the interfaces must compile).

- [ ] **Step 4: Verify existing tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/...`
Expected: PASS (interfaces are unused; existing tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add pkg/zone/zone.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(zone): NAI-28 Bundle 1 Task 1.2 — PlayerLike + NpcLike interfaces

Define minimum interface surface for Zone PathingEntity subscription. Boundary lives in pkg/zone (not modules/world) to avoid the cyclic import that would otherwise prevent Zone from holding typed Player/Npc subscription lists. Mirrors TS Zone.ts:80-87 dispatch contract.

No consumers yet — Zone subscription methods land in Task 1.3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 1.3: Zone subscription methods + ZoneGrid wiring

**Files:**
- Modify: `pkg/zone/zone.go` (add `players`/`npcs` fields to `Zone` struct + 8 new methods)
- Modify: `pkg/zone/zone_test.go` (add Enter/Leave/Safe/Count tests using stub PlayerLike/NpcLike implementations)

- [ ] **Step 1: Write failing tests in `pkg/zone/zone_test.go`**

Append the following at the end of `pkg/zone/zone_test.go`:

```go
// stubPlayer implements PlayerLike for Zone subscription tests.
type stubPlayer struct {
	slot  int
	valid bool
}

func (p *stubPlayer) IsValid() bool { return p.valid }
func (p *stubPlayer) Slot() int     { return p.slot }

// stubNpc implements NpcLike for Zone subscription tests.
type stubNpc struct {
	nid   int
	valid bool
}

func (n *stubNpc) IsValid() bool { return n.valid }
func (n *stubNpc) Nid() int      { return n.nid }

func TestZoneEnterPlayerFlagsGridOnFirstEntry(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p := &stubPlayer{slot: 1, valid: true}
	z.EnterPlayer(p, g)
	if !g.IsFlagged(400, 400, 0) {
		t.Error("first EnterPlayer should flag the grid at (400,400)")
	}
}

func TestZoneEnterPlayerSecondPlayerDoesNotReFlag(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	// Manually unflag, then add a second player. If the second EnterPlayer
	// re-flags, that's incorrect — only the first should flag.
	g.Unflag(400, 400)
	z.EnterPlayer(&stubPlayer{slot: 2, valid: true}, g)
	if g.IsFlagged(400, 400, 0) {
		t.Error("second EnterPlayer should NOT re-flag a previously-unflagged grid")
	}
}

func TestZoneLeaveLastPlayerUnflagsGrid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p := &stubPlayer{slot: 1, valid: true}
	e := z.EnterPlayer(p, g)
	z.LeavePlayer(p, e, g)
	if g.IsFlagged(400, 400, 0) {
		t.Error("LeavePlayer of last player should unflag grid")
	}
}

func TestZoneLeavePlayerNonLastDoesNotUnflag(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p1 := &stubPlayer{slot: 1, valid: true}
	p2 := &stubPlayer{slot: 2, valid: true}
	e1 := z.EnterPlayer(p1, g)
	z.EnterPlayer(p2, g)
	z.LeavePlayer(p1, e1, g)
	if !g.IsFlagged(400, 400, 0) {
		t.Error("LeavePlayer when others remain should NOT unflag grid")
	}
}

func TestZoneEnterNpcDoesNotFlagGrid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	n := &stubNpc{nid: 1, valid: true}
	z.EnterNpc(n)
	// NPC enter must not touch the grid (TS Zone.enter only flags for Player).
	if g.IsFlagged(400, 400, 0) {
		t.Error("EnterNpc should NOT flag grid")
	}
}

func TestZoneLeaveNpcDoesNotUnflagGrid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	// Manually flag the grid (e.g., a player is in this zone).
	g.Flag(400, 400)
	n := &stubNpc{nid: 1, valid: true}
	e := z.EnterNpc(n)
	z.LeaveNpc(n, e)
	if !g.IsFlagged(400, 400, 0) {
		t.Error("LeaveNpc should NOT unflag the grid (only LeavePlayer does)")
	}
}

func TestZoneEnterIncrementsCount(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	z.EnterPlayer(&stubPlayer{slot: 2, valid: true}, g)
	z.EnterNpc(&stubNpc{nid: 1, valid: true})
	if z.PlayersCount() != 2 {
		t.Errorf("PlayersCount: got %d, want 2", z.PlayersCount())
	}
	if z.NpcsCount() != 1 {
		t.Errorf("NpcsCount: got %d, want 1", z.NpcsCount())
	}
}

func TestZoneLeaveDecrementsCount(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	p := &stubPlayer{slot: 1, valid: true}
	e := z.EnterPlayer(p, g)
	z.LeavePlayer(p, e, g)
	if z.PlayersCount() != 0 {
		t.Errorf("PlayersCount after Leave: got %d, want 0", z.PlayersCount())
	}
}

func TestZonePlayersSafeFiltersInvalid(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	z.EnterPlayer(&stubPlayer{slot: 2, valid: false}, g)
	z.EnterPlayer(&stubPlayer{slot: 3, valid: true}, g)
	got := []int{}
	for p := range z.PlayersSafe(false) {
		got = append(got, p.Slot())
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("PlayersSafe filter: got %v, want [1 3]", got)
	}
}

func TestZoneNpcsSafeFiltersInvalid(t *testing.T) {
	z := New(0, 0, 400, 400)
	z.EnterNpc(&stubNpc{nid: 1, valid: true})
	z.EnterNpc(&stubNpc{nid: 2, valid: false})
	z.EnterNpc(&stubNpc{nid: 3, valid: true})
	got := []int{}
	for n := range z.NpcsSafe(false) {
		got = append(got, n.Nid())
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("NpcsSafe filter: got %v, want [1 3]", got)
	}
}

func TestZoneResetPreservesSubscription(t *testing.T) {
	z := New(0, 0, 400, 400)
	g := NewZoneGrid()
	z.EnterPlayer(&stubPlayer{slot: 1, valid: true}, g)
	z.EnterNpc(&stubNpc{nid: 1, valid: true})
	z.Reset()
	// Zone.reset clears events/entityEvents/shared but NOT subscription
	// (mirrors TS Zone.reset at Zone.ts:197-201 which only clears the event-side state).
	if z.PlayersCount() != 1 {
		t.Errorf("Reset should preserve PlayersCount: got %d, want 1", z.PlayersCount())
	}
	if z.NpcsCount() != 1 {
		t.Errorf("Reset should preserve NpcsCount: got %d, want 1", z.NpcsCount())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/ -run TestZoneEnter`
Expected: FAIL with "z.EnterPlayer undefined" / "z.EnterNpc undefined" etc.

- [ ] **Step 3: Add `players`/`npcs` fields to `Zone` struct + new methods to `pkg/zone/zone.go`**

In `pkg/zone/zone.go`, modify the `Zone` struct (currently at lines 16-27) to add two new unexported fields. Find this block:

```go
type Zone struct {
	Index       int
	X, Z, Level int // X and Z in zone units (tile >> 3)

	Locs []*entity.Loc
	Objs []*entity.Obj

	events       []ZoneEvent
	entityEvents map[*entity.NonPathing][]int // entity pointer → indexes into events

	shared []byte
}
```

Replace with:

```go
type Zone struct {
	Index       int
	X, Z, Level int // X and Z in zone units (tile >> 3)

	Locs []*entity.Loc
	Objs []*entity.Obj

	events       []ZoneEvent
	entityEvents map[*entity.NonPathing][]int // entity pointer → indexes into events

	shared []byte

	// PathingEntity subscription lists. Per TS Zone.ts:47-48, players and
	// npcs are tracked separately. Reset does NOT clear these — subscription
	// persists across ticks until LeaveX is called explicitly.
	players DoublyLinkList[PlayerLike]
	npcs    DoublyLinkList[NpcLike]
}
```

Then append the following methods at the end of `pkg/zone/zone.go` (after the existing `MapProjAnim` method around line 342):

```go
// ---- PathingEntity subscription (NAI-28) ----

// EnterPlayer adds p to z.players and returns the *Element for caller storage.
// If z's player count transitions 0→1, grid.Flag(z.X, z.Z) fires.
//
// Mirrors TS Zone.enter Player branch at Zone.ts:80-83.
func (z *Zone) EnterPlayer(p PlayerLike, grid *ZoneGrid) *Element[PlayerLike] {
	wasEmpty := z.players.Size() == 0
	e := z.players.AddTail(p)
	if wasEmpty && grid != nil {
		grid.Flag(z.X, z.Z)
	}
	return e
}

// LeavePlayer removes the element from z.players. If z's player count
// transitions 1→0, grid.Unflag(z.X, z.Z) fires. Caller must null its
// stored *Element after this call.
//
// Mirrors TS Zone.leave Player branch at Zone.ts:90-96.
func (z *Zone) LeavePlayer(p PlayerLike, e *Element[PlayerLike], grid *ZoneGrid) {
	if e == nil {
		return
	}
	e.Unlink()
	if z.players.Size() == 0 && grid != nil {
		grid.Unflag(z.X, z.Z)
	}
}

// EnterNpc adds n to z.npcs and returns the *Element for caller storage.
// NPC entries do NOT touch the grid (only player entries do).
//
// Mirrors TS Zone.enter Npc branch at Zone.ts:84-87.
func (z *Zone) EnterNpc(n NpcLike) *Element[NpcLike] {
	return z.npcs.AddTail(n)
}

// LeaveNpc removes the element from z.npcs.
//
// Mirrors TS Zone.leave Npc branch at Zone.ts:97-99.
func (z *Zone) LeaveNpc(n NpcLike, e *Element[NpcLike]) {
	if e == nil {
		return
	}
	e.Unlink()
}

// PlayersSafe yields players that pass IsValid(). reverse=true iterates
// in reverse insertion order. Mirrors TS Zone.getAllPlayersSafe at Zone.ts:387-393.
func (z *Zone) PlayersSafe(reverse bool) iter.Seq[PlayerLike] {
	return func(yield func(PlayerLike) bool) {
		for p := range z.players.All(reverse) {
			if !p.IsValid() {
				continue
			}
			if !yield(p) {
				return
			}
		}
	}
}

// NpcsSafe yields npcs that pass IsValid(). Mirrors TS Zone.getAllNpcsSafe
// at Zone.ts:399-405.
func (z *Zone) NpcsSafe(reverse bool) iter.Seq[NpcLike] {
	return func(yield func(NpcLike) bool) {
		for n := range z.npcs.All(reverse) {
			if !n.IsValid() {
				continue
			}
			if !yield(n) {
				return
			}
		}
	}
}

// PlayersCount returns the number of players currently subscribed.
// Mirrors TS Zone.playersCount field at Zone.ts:51.
func (z *Zone) PlayersCount() int { return z.players.Size() }

// NpcsCount returns the number of npcs currently subscribed.
// Mirrors TS Zone.npcsCount field at Zone.ts:52.
func (z *Zone) NpcsCount() int { return z.npcs.Size() }
```

Also add the `iter` import to the existing import block at the top of `zone.go`. Find:

```go
import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

Replace with:

```go
import (
	"iter"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/zone/...`
Expected: All pre-existing tests + 11 new subscription tests PASS. Total ~28+ tests in the `pkg/zone` package.

Per `verify_implementer_claims` discipline, also run:
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Expected: build succeeds across all packages — no downstream package consumes the new APIs yet, so this primarily validates that the new pkg/zone code is well-formed.

- [ ] **Step 5: Commit**

```bash
git add pkg/zone/zone.go pkg/zone/zone_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(zone): NAI-28 Bundle 1 Task 1.3 — Zone PathingEntity subscription methods

Extend Zone with players/npcs DoublyLinkList[PlayerLike|NpcLike] fields and 8 methods (EnterPlayer/LeavePlayer/EnterNpc/LeaveNpc/PlayersSafe/NpcsSafe/PlayersCount/NpcsCount). Mirrors TS Zone.enter/leave/getAllXxxSafe/playersCount/npcsCount per Zone.ts:79-100, 387-405, 51-52.

ZoneGrid flag/unflag wired only on Player branch (TS Zone.enter at L83 flags; L94-96 unflags); NPC branch does NOT touch grid. Reset preserves subscription (TS Zone.reset at L197-201 only clears event-side state).

11 new tests validate first-flag-on-first-entry, no-reflag-on-second-entry, last-leave-unflag, npc-enter-leaves-grid-untouched, count tracking, IsValid filtering, and Reset preservation. No deviation tags introduced.

Bundle 1 closes here. Bundle 2 wires modules/world consumers.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Bundle 2 — modules/world Wire-Through (retires NAI-19-D1)

Bundle 2 wires the 8 logical groups (11 individual code edits) and retires the `NAI-19-D1` deviation tag. Each task in this bundle contains its full TDD cycle plus per-task acceptance criteria.

**Bundle 2 prerequisite — extend `newTestServer` to init zoneMap.** Without `s.zoneMap` initialised, `addPlayer`/`addNpc`/etc. will panic when they call `s.zoneMap.Get(...)`. This is the first task.

## Task 2.1: Extend `newTestServer` to init `s.zoneMap`

**Files:**
- Modify: `modules/world/server_test.go:309-317` (newTestServer body)

- [ ] **Step 1: Read current `newTestServer`**

Run: `sed -n '309,317p' modules/world/server_test.go`
Expected output:
```
func newTestServer(t *testing.T) *Server {
    t.Helper()
    s := &Server{
        quit:           make(chan interface{}),
        log:            discardLogger(),
        scriptProvider: defaultTestProvider(),
    }
    return s
}
```

- [ ] **Step 2: Verify import for pkg/zone exists in server_test.go**

Run: `head -30 modules/world/server_test.go`
Expected: import block. Verify `"github.com/zsrv/goscape/pkg/zone"` is present. If absent, plan-author note: add it in the same edit.

- [ ] **Step 3: Edit `newTestServer` to init zoneMap**

Replace the function body with:

```go
func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		quit:           make(chan interface{}),
		log:            discardLogger(),
		scriptProvider: defaultTestProvider(),
		zoneMap:        zone.NewZoneMap(),
	}
	return s
}
```

If the `zone` import is missing from the file, add it to the import block.

- [ ] **Step 4: Verify all tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: ALL existing tests still pass. Adding `zoneMap` is additive — no test should break. If any test fails, the failure is informative and indicates that the test was relying on `s.zoneMap == nil` (no such test should exist; if one does, the failure points to a latent invariant).

- [ ] **Step 5: Commit**

```bash
git add modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-28 Bundle 2 Task 2.1 — extend newTestServer to init zoneMap

Bundle 2's wire-through tasks call addPlayer/addNpc which will dereference s.zoneMap. Without this init, every Bundle 2 wire-through task would have to construct zoneMap inline. Centralise here.

Additive change — no test should be relying on zoneMap == nil. Existing test suite re-runs green at HEAD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.2: Add `zoneListElement` field to `Player` struct

**Files:**
- Modify: `modules/world/player.go:58-...` (Player struct definition)

- [ ] **Step 1: Read current Player struct top**

Run: `sed -n '58,80p' modules/world/player.go`
Expected: see `type Player struct {` followed by field declarations.

- [ ] **Step 2: Add field to Player struct**

In `modules/world/player.go`, add the following field at an appropriate location inside the `Player` struct (e.g., immediately after `lastStepX, lastStepZ` if visible at line 76, or grouped with other zone-related fields). Pattern:

```go
	// zoneListElement is the player's intrusive subscription element in
	// pkg/zone.Zone.players. Set by Zone.EnterPlayer; nilled after
	// Zone.LeavePlayer. Used to support O(1) Unlink on cross-zone movement.
	// Per NAI-28 Bundle 2.
	zoneListElement *zone.Element[zone.PlayerLike]
```

- [ ] **Step 3: Verify `pkg/zone` is already imported in `player.go`**

Run: `head -30 modules/world/player.go | grep zone`
Expected: `"github.com/zsrv/goscape/pkg/zone"` present (existing imports use it via `s.zoneMap`).

If the import is missing, add it to the import block.

- [ ] **Step 4: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Expected: PASS. Field is unread/unwritten; build is clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: ALL existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.2 — Player zoneListElement subscription field

Add unexported field tracking the player's pkg/zone.Zone.players subscription Element. Field is unread until subsequent tasks wire EnterPlayer/LeavePlayer; this commit just lands the storage shape.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.3: Add `zoneListElement` field to `Npc` struct

**Files:**
- Modify: `modules/world/npc.go:25-...` (Npc struct definition)

- [ ] **Step 1: Read current Npc struct**

Run: `sed -n '25,50p' modules/world/npc.go`
Expected: see `type Npc struct {` followed by field declarations.

- [ ] **Step 2: Add field to Npc struct**

In `modules/world/npc.go`, add the following field grouped with other zone-related fields (e.g., near `nid`, `level` or `x`, `z`):

```go
	// zoneListElement is the NPC's intrusive subscription element in
	// pkg/zone.Zone.npcs. Set by Zone.EnterNpc; nilled after Zone.LeaveNpc.
	// Per NAI-28 Bundle 2.
	zoneListElement *zone.Element[zone.NpcLike]
```

- [ ] **Step 3: Verify `pkg/zone` import in `npc.go`**

Run: `head -30 modules/world/npc.go | grep zone`
Expected: `"github.com/zsrv/goscape/pkg/zone"` present. If absent, add the import.

- [ ] **Step 4: Verify build + tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: ALL pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.3 — Npc zoneListElement subscription field

Add unexported field tracking the NPC's pkg/zone.Zone.npcs subscription Element. Field is unread until subsequent tasks wire EnterNpc/LeaveNpc.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.4: Wire `addPlayer` to call `EnterPlayer`

**Files:**
- Modify: `modules/world/server.go:599-613` (addPlayer)
- Test: `modules/world/server_test.go` (extend `TestAddPlayerAssignsSlot` or add a new test)

- [ ] **Step 1: Write failing test**

Append to `modules/world/server_test.go`:

```go
func TestAddPlayerEntersZoneAndFlagsGrid(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	// newPlayer's default coords are (0,0,0); set to a known zone for clarity.
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 1 {
		t.Errorf("after addPlayer, Zone.PlayersCount: got %d, want 1", z.PlayersCount())
	}
	if !s.zoneMap.Grid(0).IsFlagged(400, 400, 0) {
		t.Error("after addPlayer, ZoneGrid should be flagged at (400,400)")
	}
	if p.zoneListElement == nil {
		t.Error("addPlayer should populate p.zoneListElement")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestAddPlayerEntersZoneAndFlagsGrid`
Expected: FAIL — `Zone.PlayersCount()` is 0 (no wire-through yet).

- [ ] **Step 3: Wire `addPlayer`**

Modify `modules/world/server.go:599-613`. Find:

```go
func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	for i := 1; i < len(s.players); i++ {
		if s.players[i] == nil {
			p.slot = i
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			p.active = true
			return nil
		}
	}
	return errWorldFull
}
```

Replace with:

```go
func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	for i := 1; i < len(s.players); i++ {
		if s.players[i] == nil {
			p.slot = i
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			p.active = true
			if s.zoneMap != nil {
				z := s.zoneMap.Get(p.level, p.x, p.z)
				p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
			}
			return nil
		}
	}
	return errWorldFull
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestAddPlayerEntersZoneAndFlagsGrid`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: ALL existing tests pass + new test passes.

- [ ] **Step 5: Commit**

```bash
git add modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.4 — addPlayer enters zone and flags grid

Player login now subscribes to pkg/zone.Zone.players and flags the ZoneGrid (the latter only on first-player-enter, per Zone.EnterPlayer's internal logic). Mirrors TS World.addPlayer at World.ts:941. Defensive nil-guard on s.zoneMap preserves existing test scaffolding that doesn't init it (none should remain after Task 2.1, but the guard is cheap insurance).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.5: Wire `removePlayer` to call `LeavePlayer`

**Files:**
- Modify: `modules/world/server.go:643-659` (removePlayer)
- Test: `modules/world/server_test.go`

- [ ] **Step 1: Write failing tests**

Append:

```go
func TestRemovePlayerLeavesZoneAndUnflagsGrid(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.removePlayer(p)
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 0 {
		t.Errorf("after removePlayer, Zone.PlayersCount: got %d, want 0", z.PlayersCount())
	}
	if s.zoneMap.Grid(0).IsFlagged(400, 400, 0) {
		t.Error("after removePlayer (last player gone), grid should be unflagged")
	}
	if p.zoneListElement != nil {
		t.Error("removePlayer should null p.zoneListElement")
	}
}

func TestRemovePlayerDoubleCallIsNoop(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.removePlayer(p)
	// Second call must not panic.
	s.removePlayer(p)
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 0 {
		t.Errorf("PlayersCount after double removePlayer: got %d, want 0", z.PlayersCount())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestRemovePlayerLeavesZone`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestRemovePlayerDoubleCallIsNoop`
Expected: FAIL — `PlayersCount()` is 1 (no wire-through yet).

- [ ] **Step 3: Wire `removePlayer`**

Modify `modules/world/server.go:643-659`. Find:

```go
func (s *Server) removePlayer(p *Player) {
	p.active = false
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if p.slot < 1 || p.slot >= len(s.players) || s.players[p.slot] != p {
		return
	}
	s.players[p.slot] = nil

	for i, lp := range s.playerLoop {
		if lp == p {
			s.playerLoop = append(s.playerLoop[:i], s.playerLoop[i+1:]...)
			break
		}
	}
}
```

Replace with:

```go
func (s *Server) removePlayer(p *Player) {
	p.active = false
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if p.slot < 1 || p.slot >= len(s.players) || s.players[p.slot] != p {
		return
	}
	if s.zoneMap != nil && p.zoneListElement != nil {
		z := s.zoneMap.Get(p.level, p.x, p.z)
		z.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(p.level))
		p.zoneListElement = nil
	}
	s.players[p.slot] = nil

	for i, lp := range s.playerLoop {
		if lp == p {
			s.playerLoop = append(s.playerLoop[:i], s.playerLoop[i+1:]...)
			break
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestRemovePlayer`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.5 — removePlayer leaves zone and unflags grid

Player logout calls Zone.LeavePlayer; if it was the last player, the ZoneGrid unflags. Defensive guard on p.zoneListElement makes double-removePlayer a no-op (e.g., during reconnect race conditions). Mirrors TS World.removePlayer at World.ts:1598.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.6: Wire `addNpc` to call `EnterNpc` (retires NAI-19-D1 site #1)

**Files:**
- Modify: `modules/world/npc_registry.go:60-95` (addNpc body — replace DEVIATION NAI-19-D1 comment block)
- Test: `modules/world/npc_registry_test.go`

- [ ] **Step 1: Re-read current `addNpc` deviation block**

Run: `sed -n '60,75p' modules/world/npc_registry.go`
Expected output includes:
```
	n.dead = false
	// DEVIATION NAI-19-D1: zone.enter omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	if s.gamemap != nil {
```

- [ ] **Step 2: Write failing test**

Append to `modules/world/npc_registry_test.go` (or to an existing addNpc-focused test file). First check the file has a basic addNpc test. Add:

```go
func TestAddNpcEntersZone(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	z := s.zoneMap.Get(n.level, n.x, n.z)
	if z.NpcsCount() != 1 {
		t.Errorf("after addNpc, Zone.NpcsCount: got %d, want 1", z.NpcsCount())
	}
	if n.zoneListElement == nil {
		t.Error("addNpc should populate n.zoneListElement")
	}
	// Dual-pin per ts_asymmetry_dual_pin: NPC enter does NOT flag grid.
	if s.zoneMap.Grid(n.level).IsFlagged(n.x>>3, n.z>>3, 0) {
		t.Error("addNpc must NOT flag the grid (only player enter flags)")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestAddNpcEntersZone`
Expected: FAIL — `NpcsCount()` is 0.

- [ ] **Step 4: Wire `addNpc` and retire NAI-19-D1 inline comment**

Modify `modules/world/npc_registry.go`. Find the `addNpc` body around line 64-66:

```go
	n.dead = false
	// DEVIATION NAI-19-D1: zone.enter omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	if s.gamemap != nil {
```

Replace with:

```go
	n.dead = false
	// Zone enter — mirrors TS World.addNpc at World.ts:1268-1269.
	if s.zoneMap != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		n.zoneListElement = z.EnterNpc(n)
	}
	if s.gamemap != nil {
```

- [ ] **Step 5: Run tests to verify pass + retirement complete**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestAddNpcEntersZone`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: ALL pass.

Verify NAI-19-D1 inline comment retired at this site:
Run: `rg -n 'NAI-19-D1' modules/world/npc_registry.go`
Expected: Only the line at `:149` remains (removeNpc — retired in Task 2.7). The `:64` line should be GONE.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.6 — addNpc enters zone (retires NAI-19-D1 site #1)

addNpc now subscribes the NPC to pkg/zone.Zone.npcs. Mirrors TS World.addNpc at World.ts:1268-1269. The inline DEVIATION NAI-19-D1 comment at npc_registry.go:64 is retired; the equivalent comment at :149 (removeNpc) is retired in Task 2.7.

Dual-pin test confirms NPC enter does NOT touch ZoneGrid (TS Zone.enter only flags for Player branch).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.7: Wire `removeNpc` to call `LeaveNpc` (retires NAI-19-D1 site #2)

**Files:**
- Modify: `modules/world/npc_registry.go:148-169` (removeNpc body)
- Test: `modules/world/npc_registry_test.go`

- [ ] **Step 1: Write failing tests**

Append:

```go
func TestRemoveNpcLeavesZone(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	z := s.zoneMap.Get(n.level, n.x, n.z)
	s.removeNpc(n, -1)
	if z.NpcsCount() != 0 {
		t.Errorf("after removeNpc, Zone.NpcsCount: got %d, want 0", z.NpcsCount())
	}
	if n.zoneListElement != nil {
		t.Error("removeNpc should null n.zoneListElement")
	}
}

func TestNpcRevertTypeHeavyPathLeavesAndReentersZone(t *testing.T) {
	// revertType heavy path is s.removeNpc(n, -1) + s.addNpc(n, -1, false).
	// Subscription should round-trip to its pre-call state.
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	z := s.zoneMap.Get(n.level, n.x, n.z)
	s.removeNpc(n, -1)
	if z.NpcsCount() != 0 {
		t.Fatalf("after removeNpc, NpcsCount: got %d, want 0", z.NpcsCount())
	}
	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("addNpc respawn: %v", err)
	}
	if z.NpcsCount() != 1 {
		t.Errorf("after revertType heavy path, NpcsCount: got %d, want 1", z.NpcsCount())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestRemoveNpcLeavesZone`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNpcRevertTypeHeavyPathLeavesAndReentersZone`
Expected: FAIL.

- [ ] **Step 3: Wire `removeNpc` and retire NAI-19-D1 inline comment**

Modify `modules/world/npc_registry.go:148-169`. Find:

```go
func (s *Server) removeNpc(n *Npc, duration int) {
	// DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	n.dead = true
	if s.gamemap != nil {
```

Replace with:

```go
func (s *Server) removeNpc(n *Npc, duration int) {
	// Zone leave — mirrors TS World.removeNpc at World.ts:1297-1299.
	if s.zoneMap != nil && n.zoneListElement != nil {
		z := s.zoneMap.Get(n.level, n.x, n.z)
		z.LeaveNpc(n, n.zoneListElement)
		n.zoneListElement = nil
	}
	n.dead = true
	if s.gamemap != nil {
```

- [ ] **Step 4: Run tests + verify NAI-19-D1 retirement**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestRemoveNpcLeavesZone`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNpcRevertType`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: PASS.

Verify NAI-19-D1 retirement complete:
Run: `rg -n 'NAI-19-D1' modules/world/`
Expected: ZERO matches in `npc_registry.go`. The 2 matches in `modules/world/npc.go:259, 283` (doc-comment narration) remain — retired in Task 2.12.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_registry.go modules/world/npc_registry_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.7 — removeNpc leaves zone (retires NAI-19-D1 site #2)

removeNpc now calls Zone.LeaveNpc. Both NAI-19-D1 inline DEVIATION comments in npc_registry.go are retired (sites #1 and #2). Doc-comment narration in npc.go:259, 283 is retired in Task 2.12.

revertType heavy-path round-trip test pins the (removeNpc + addNpc) cycle through Zone subscription.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.8: Add `refreshPlayerZone` helper + wire `(*Player).stepOnce`

**Files:**
- Create: `modules/world/zone_refresh.go` (new file holding both refreshPlayerZone + refreshNpcZone helpers)
- Modify: `modules/world/movement.go:64-94` (Player.stepOnce — insert refreshPlayerZone call)
- Test: `modules/world/movement_test.go`

- [ ] **Step 1: Write failing tests**

Append to `modules/world/movement_test.go`:

```go
func TestPlayerStepCrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	// Start in zone (399, 400) at (3199, 3200).
	p.x, p.z, p.level = 3199, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Queue a step east into zone (400, 400).
	p.queueWaypoint(3200, 3200)
	dir, ok := p.stepOnce()
	if !ok {
		t.Fatalf("stepOnce ok: got false (dir=%d)", dir)
	}
	prevZ := s.zoneMap.Get(0, 3199, 3200)
	newZ := s.zoneMap.Get(0, 3200, 3200)
	if prevZ.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount: got %d, want 0", prevZ.PlayersCount())
	}
	if newZ.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount: got %d, want 1", newZ.PlayersCount())
	}
}

func TestPlayerStepIntraZoneNoSubscriptionChange(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	// Start at (3200, 3200) zone (400, 400). Step to (3201, 3201) — same zone.
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.queueWaypoint(3201, 3201)
	if _, ok := p.stepOnce(); !ok {
		t.Fatal("stepOnce ok: got false")
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 1 {
		t.Errorf("intra-zone step PlayersCount: got %d, want 1", z.PlayersCount())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerStepCrossZone`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerStepIntraZone`
Expected: First test FAILS (still in prev zone). Second test passes incidentally (no migration anyway), but include for symmetry — must keep passing post-Step-3.

- [ ] **Step 3: Create `modules/world/zone_refresh.go` and wire `(*Player).stepOnce`**

Create `modules/world/zone_refresh.go`:

```go
package world

// refreshPlayerZone moves the player's pkg/zone subscription if (prevX>>3,
// prevZ>>3, prevLevel) differs from (p.x>>3, p.z>>3, p.level). Called from
// (*Player).stepOnce, (*Player).Teleport, and (*Player).TeleJump after the
// position is mutated.
//
// Mirrors TS PathingEntity.refreshZone at PathingEntity.ts:182-183, applied
// at every per-step boundary check (TS dispatches via instanceof inside Zone;
// Go uses the typed PlayerLike branch).
//
// nil-guards: if p.client/server/zoneMap are unset (e.g., test fixtures that
// bypass the standard server setup), skip silently.
func refreshPlayerZone(p *Player, prevX, prevZ, prevLevel int) {
	if p.client == nil || p.client.server == nil || p.client.server.zoneMap == nil {
		return
	}
	if (prevX>>3) == (p.x>>3) && (prevZ>>3) == (p.z>>3) && prevLevel == p.level {
		return
	}
	s := p.client.server
	prevZone := s.zoneMap.Get(prevLevel, prevX, prevZ)
	newZone := s.zoneMap.Get(p.level, p.x, p.z)
	prevZone.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(prevLevel))
	p.zoneListElement = newZone.EnterPlayer(p, s.zoneMap.Grid(p.level))
}

// refreshNpcZone is the NPC-side analogue of refreshPlayerZone. Called from
// (*Npc).stepOnce and the 3 NPC teleport sites in npc_interaction.go +
// npc_ai.go.
//
// NPC enter/leave do NOT touch ZoneGrid (only player branch flags).
func refreshNpcZone(s *Server, n *Npc, prevX, prevZ, prevLevel int) {
	if s == nil || s.zoneMap == nil {
		return
	}
	if (prevX>>3) == (n.x>>3) && (prevZ>>3) == (n.z>>3) && prevLevel == n.level {
		return
	}
	prevZone := s.zoneMap.Get(prevLevel, prevX, prevZ)
	newZone := s.zoneMap.Get(n.level, n.x, n.z)
	prevZone.LeaveNpc(n, n.zoneListElement)
	n.zoneListElement = newZone.EnterNpc(n)
}
```

Modify `modules/world/movement.go:64-94`. Find:

```go
func (p *Player) stepOnce() (coordgrid.Direction, bool) {
	if p.waypointIndex < 0 {
		return -1, false
	}
	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	if dir == -1 {
		p.waypointIndex--
		return -1, false
	}

	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
		if !p.client.server.gamemap.CanTravel(p.level, p.x, p.z, dx, dz) {
			p.waypointIndex = -1
			return -1, false
		}
	}

	p.lastStepX = p.x
	p.lastStepZ = p.z
	p.x += dx
	p.z += dz
	p.stepsTaken++

	if p.x == dest.X && p.z == dest.Z {
		p.waypointIndex--
	}
	return dir, true
}
```

Replace with:

```go
func (p *Player) stepOnce() (coordgrid.Direction, bool) {
	if p.waypointIndex < 0 {
		return -1, false
	}
	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	if dir == -1 {
		p.waypointIndex--
		return -1, false
	}

	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
		if !p.client.server.gamemap.CanTravel(p.level, p.x, p.z, dx, dz) {
			p.waypointIndex = -1
			return -1, false
		}
	}

	p.lastStepX = p.x
	p.lastStepZ = p.z
	p.x += dx
	p.z += dz
	p.stepsTaken++

	// Per-step refreshZone — mirrors TS PathingEntity.ts:182-183.
	// Level cannot change in stepOnce (single-tile delta); pass p.level for both.
	refreshPlayerZone(p, p.lastStepX, p.lastStepZ, p.level)

	if p.x == dest.X && p.z == dest.Z {
		p.waypointIndex--
	}
	return dir, true
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerStep`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/zone_refresh.go modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.8 — Player.stepOnce per-step refreshZone

Add modules/world/zone_refresh.go holding refreshPlayerZone + refreshNpcZone helpers (the NPC variant is wired in Task 2.9). Wire Player.stepOnce to call refreshPlayerZone after position is mutated. Mirrors TS PathingEntity.ts:182-183.

Tests pin cross-zone subscription handoff and intra-zone no-op behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.9: Wire `(*Npc).stepOnce` with `refreshNpcZone`

**Files:**
- Modify: `modules/world/npc_interaction.go:314-337` (Npc.stepOnce — insert refreshNpcZone call)
- Test: `modules/world/npc_interaction_test.go`

- [ ] **Step 1: Write failing test**

Append to `modules/world/npc_interaction_test.go`:

```go
func TestNpcStepCrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	// Start at (3200, 3200) zone (400, 400). Place at boundary first, then step east.
	prevZone := s.zoneMap.Get(0, n.x, n.z)
	prevX, prevZ := n.x, n.z
	// Manually craft a step: mutate n.x, then simulate the per-step refresh
	// the same way stepOnce would (via direct call). Real test uses queueWaypoints +
	// stepOnce; for this commit we exercise the wire-through.
	n.waypoints[0] = (0 << 28) | ((n.x + 8) << 14) | n.z // east 8 tiles to next zone
	n.waypointIndex = 0
	if !nIsInZone(n, prevZone) {
		t.Fatal("setup: NPC not in expected starting zone")
	}
	_ = prevX
	_ = prevZ
	// Use a single stepOnce. Since stepOnce moves only 1 tile, we step until
	// crossing zone (8 tiles). Easier: place at zone-boundary and step once.
	n.x = 3199
	n.z = 3200
	// Re-subscribe to the boundary zone for accurate test setup.
	prevZone.LeaveNpc(n, n.zoneListElement)
	prevZone2 := s.zoneMap.Get(0, n.x, n.z)
	n.zoneListElement = prevZone2.EnterNpc(n)
	n.waypoints[0] = (0 << 28) | (3200 << 14) | 3200
	n.waypointIndex = 0
	ok, _ := n.stepOnce(s)
	if !ok {
		t.Fatal("stepOnce returned false")
	}
	if prevZone2.NpcsCount() != 0 {
		t.Errorf("prev zone NpcsCount: got %d, want 0", prevZone2.NpcsCount())
	}
	newZ := s.zoneMap.Get(0, 3200, 3200)
	if newZ.NpcsCount() != 1 {
		t.Errorf("new zone NpcsCount: got %d, want 1", newZ.NpcsCount())
	}
}

// nIsInZone returns true if n is subscribed to z (helper for tests above).
func nIsInZone(n *Npc, z *zone.Zone) bool {
	for cn := range z.NpcsSafe(false) {
		if cn.Nid() == n.Nid() {
			return true
		}
	}
	return false
}
```

Add the `pkg/zone` import to `npc_interaction_test.go` if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNpcStepCrossZoneRefreshSubscription`
Expected: FAIL — NpcsCount on new zone is 0 (no wire-through yet).

- [ ] **Step 3: Wire `(*Npc).stepOnce`**

Modify `modules/world/npc_interaction.go:314-337`. Find:

```go
func (n *Npc) stepOnce(s *Server) (bool, int) {
	if n.waypointIndex < 0 {
		return false, -1
	}
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	if dir == -1 {
		n.waypointIndex--
		return false, -1
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz) {
		n.waypointIndex = -1
		return false, -1
	}
	n.x += dx
	n.z += dz
	n.stepsTaken++
	if n.x == dest.X && n.z == dest.Z {
		n.waypointIndex--
	}
	return true, int(dir)
}
```

Replace with:

```go
func (n *Npc) stepOnce(s *Server) (bool, int) {
	if n.waypointIndex < 0 {
		return false, -1
	}
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	if dir == -1 {
		n.waypointIndex--
		return false, -1
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz) {
		n.waypointIndex = -1
		return false, -1
	}
	prevX, prevZ := n.x, n.z
	n.x += dx
	n.z += dz
	n.stepsTaken++
	// Per-step refreshZone — mirrors TS PathingEntity.ts:182-183.
	refreshNpcZone(s, n, prevX, prevZ, n.level)
	if n.x == dest.X && n.z == dest.Z {
		n.waypointIndex--
	}
	return true, int(dir)
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNpcStepCrossZone`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.9 — Npc.stepOnce per-step refreshZone

Wire NPC stepOnce to call refreshNpcZone after position mutation. Mirrors TS PathingEntity.ts:182-183 (shared between Player and Npc via base class; goscape uses separate typed methods per parallel_slice_convention).

Cross-zone test pins the prev→new subscription handoff for NPC movement.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.10: Wire `(*Player).Teleport` and `(*Player).TeleJump`

**Files:**
- Modify: `modules/world/player_script.go:211-229` (TeleJump + Teleport)
- Test: `modules/world/player_script_test.go`

- [ ] **Step 1: Write failing tests**

Append to `modules/world/player_script_test.go` (create if absent — check first with `ls modules/world/player_script_test.go`):

```go
func TestPlayerTeleportCrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.Teleport(4000, 4000, 0)
	newZone := s.zoneMap.Get(0, 4000, 4000)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount after Teleport: got %d, want 0", prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount after Teleport: got %d, want 1", newZone.PlayersCount())
	}
}

func TestPlayerTeleJumpCrossLevelRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	prevZone := s.zoneMap.Get(0, 3200, 3200)
	p.TeleJump(3200, 3200, 1) // same xy, level=0→1
	newZone := s.zoneMap.Get(1, 3200, 3200)
	if prevZone.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount after cross-level TeleJump: got %d, want 0", prevZone.PlayersCount())
	}
	if newZone.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount after cross-level TeleJump: got %d, want 1", newZone.PlayersCount())
	}
}

func TestPlayerTeleportSameZoneNoRefresh(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	prevElement := p.zoneListElement
	p.Teleport(3201, 3201, 0) // same zone (400, 400)
	if z.PlayersCount() != 1 {
		t.Errorf("same-zone Teleport PlayersCount: got %d, want 1", z.PlayersCount())
	}
	// Same-zone teleport should NOT re-subscribe (no leave/enter dance).
	if p.zoneListElement != prevElement {
		t.Error("same-zone Teleport should preserve zoneListElement (no leave/enter)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerTele`
Expected: First two FAIL (cross-zone/level Teleport doesn't refresh). Third may pass incidentally — re-verify post-Step-3.

- [ ] **Step 3: Wire `Teleport` and `TeleJump`**

Modify `modules/world/player_script.go`. Find:

```go
// TeleJump instantly teleports the player to (x, z, level) with no
// interpolation, clearing any pending walk. ResetMasks clears the one-
// shot tele/jump flags after emission.
func (p *Player) TeleJump(x, z, level int) {
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
	p.jump = true
}

// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump).
func (p *Player) Teleport(x, z, level int) {
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
}
```

Replace with:

```go
// TeleJump instantly teleports the player to (x, z, level) with no
// interpolation, clearing any pending walk. ResetMasks clears the one-
// shot tele/jump flags after emission.
func (p *Player) TeleJump(x, z, level int) {
	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
	p.jump = true
	refreshPlayerZone(p, prevX, prevZ, prevLevel)
}

// Teleport moves the player to (x, z, level) and flags the client for a
// smooth teleport transition (tele without jump).
func (p *Player) Teleport(x, z, level int) {
	prevX, prevZ, prevLevel := p.x, p.z, p.level
	p.x = x
	p.z = z
	p.level = level
	p.tele = true
	refreshPlayerZone(p, prevX, prevZ, prevLevel)
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestPlayerTele`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.10 — Player Teleport/TeleJump refreshZone

Both teleport methods now capture pre-mutation coords and call refreshPlayerZone post-mutation. Cross-zone, cross-level, and same-zone cases all pinned by tests. Same-zone Teleport preserves zoneListElement (early-return inside refreshPlayerZone).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.11: Wire 3 NPC teleport sites

**Files:**
- Modify: `modules/world/npc_interaction.go:95` (stuck-teleport-to-spawn)
- Modify: `modules/world/npc_interaction.go:122` (patrol-teleport-to-waypoint)
- Modify: `modules/world/npc_ai.go:35` (respawn-to-startCoord)
- Test: `modules/world/npc_interaction_test.go` + `modules/world/npc_ai_test.go`

- [ ] **Step 1: Read each NPC teleport site**

Run: `sed -n '90,130p' modules/world/npc_interaction.go`
Run: `sed -n '30,40p' modules/world/npc_ai.go`
Expected: see the 3 lines that mutate `n.x, n.z, n.level` in a single composite assignment.

- [ ] **Step 2: Write failing tests**

Append to `modules/world/npc_interaction_test.go`:

```go
func TestNpcStuckTeleportRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	// startX/startZ are 3200/3200 by default per newRegisteredNpc.
	// Move NPC to a different zone, then trigger stuck teleport (n.x, n.z, n.level = startX, startZ, startLevel).
	prevZone := s.zoneMap.Get(0, n.x, n.z)
	// Manually move NPC to (4000, 4000, 0) to set up the stuck-teleport scenario.
	n.x, n.z, n.level = 4000, 4000, 0
	awayZone := s.zoneMap.Get(0, 4000, 4000)
	prevZone.LeaveNpc(n, n.zoneListElement)
	n.zoneListElement = awayZone.EnterNpc(n)
	// Now invoke stuck-teleport via the NPC patrol/wander path. Set startX/Z
	// for clarity. The actual stuck-teleport site at npc_interaction.go:95
	// fires when the wander mode reaches its 30-tick stuck horizon — direct
	// invocation in tests is tricky; a synthetic test calls the helper
	// directly to exercise the wire-through.
	prevX, prevZ, prevLevel := n.x, n.z, n.level
	n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
	refreshNpcZone(s, n, prevX, prevZ, prevLevel)
	homeZone := s.zoneMap.Get(0, n.startX, n.startZ)
	if awayZone.NpcsCount() != 0 {
		t.Errorf("away zone NpcsCount after stuck-teleport: got %d, want 0", awayZone.NpcsCount())
	}
	if homeZone.NpcsCount() != 1 {
		t.Errorf("home zone NpcsCount after stuck-teleport: got %d, want 1", homeZone.NpcsCount())
	}
}
```

This test exercises `refreshNpcZone` directly. The wire-through itself is verified by build success — adding the `prevX, prevZ, prevLevel := n.x, n.z, n.level` capture lines + the `refreshNpcZone(s, n, prevX, prevZ, prevLevel)` call after each of the 3 sites.

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNpcStuckTeleport`
Expected: PASS already (calls refreshNpcZone directly), but the goal is to confirm the helper functions correctly. Re-verify after wire-through.

- [ ] **Step 4: Wire 3 NPC teleport sites**

Modify `modules/world/npc_interaction.go:95`. Find the line that reads:

```go
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
```

Replace with:

```go
			prevX, prevZ, prevLevel := n.x, n.z, n.level
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			refreshNpcZone(s, n, prevX, prevZ, prevLevel)
```

Modify `modules/world/npc_interaction.go:122`. Find:

```go
		n.x, n.z, n.level = dest.X, dest.Z, 0
```

Replace with:

```go
		prevX, prevZ, prevLevel := n.x, n.z, n.level
		n.x, n.z, n.level = dest.X, dest.Z, 0
		refreshNpcZone(s, n, prevX, prevZ, prevLevel)
```

Modify `modules/world/npc_ai.go:35`. Find:

```go
				n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
```

Replace with:

```go
				prevX, prevZ, prevLevel := n.x, n.z, n.level
				n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
				refreshNpcZone(s, n, prevX, prevZ, prevLevel)
```

For `npc_ai.go:35`, verify that `s` is in scope at that point (read 5 lines around the site). If not, the Server reference is reachable some other way — read function signature and adjust. Most likely path: the function takes `s *Server` parameter or has it as receiver field; if neither, the implementer adjusts to plumb `s` from the existing call path.

- [ ] **Step 5: Run tests + verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestNpcStuckTeleport`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: ALL pass.

Per `enumerate_all_sites` discipline, re-grep to confirm no other NPC position-mutation site exists outside `addNpc`:
Run: `rg -n 'n\.x = |n\.z = |n\.level = ' modules/world/*.go | grep -v _test | grep -v '+= '`
Expected: only the 4 sites enumerated in spec — addNpc (`npc_registry.go:61-62`), stuck-teleport (`npc_interaction.go:95`), patrol-teleport (`:122`), respawn-teleport (`npc_ai.go:35`). If a 5th site appears, surface to user before continuing — implementer flags with rationale.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_ai.go modules/world/npc_interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 2 Task 2.11 — 3 NPC teleport sites refreshZone

All 3 NPC teleport sites (stuck-teleport-to-spawn, patrol-teleport-to-waypoint, respawn-to-startCoord) now capture pre-mutation coords and call refreshNpcZone post-mutation. Mirrors TS PathingEntity.ts:182-183 applied via shared base class to NPC teleport callsites.

Per enumerate_all_sites discipline, re-grep confirms exactly 4 NPC position-mutation sites remain: addNpc + the 3 wired here.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 2.12: Retire 2 NAI-19-D1 doc-comment narration lines in `npc.go`

**Files:**
- Modify: `modules/world/npc.go:259, 283` (doc-comment narration around two methods)

- [ ] **Step 1: Read both narration sites**

Run: `sed -n '255,290p' modules/world/npc.go`
Expected output: contains the lines:
```
//     deviation against this structural form (NAI-19-D1: no zone state;
...
//     Goscape deviation NAI-19-D1 (no zone state) is documented at the
```

Both narration lines describe the (now-removed) deviation.

- [ ] **Step 2: Edit both lines**

For `modules/world/npc.go:259`, find and replace the narration line within its containing doc-comment. Locate the multi-line comment that contains:

```
//     deviation against this structural form (NAI-19-D1: no zone state;
```

Replace the parenthetical reference. The exact replacement depends on the surrounding sentence — for the line at `:259` and the line at `:283`, drop the `(NAI-19-D1: no zone state)` parenthetical from each, since the deviation is now retired. The remaining sentence describes the structural-form note without the deviation reference.

If the implementer prefers to keep TS-source-mapping context, replace `NAI-19-D1: no zone state` with `now ported as NAI-28: per-zone subscription` — this is a stylistic choice; primary goal is to retire the `NAI-19-D1` token.

- [ ] **Step 3: Verify NAI-19-D1 retirement complete**

Run: `rg -n 'NAI-19-D1' modules/`
Expected: ZERO matches anywhere in `modules/`.

Run: `rg -n 'NAI-19-D1' pkg/ docs/`
Expected: only references in `docs/superpowers/specs/` and `docs/superpowers/plans/` (historical record) — no matches in `pkg/`.

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-28 Bundle 2 Task 2.12 — retire NAI-19-D1 doc-comment narration

Drop the two NAI-19-D1 references in modules/world/npc.go doc-comments (around :259 and :283). The deviation tag is fully retired across modules/ — verified by zero rg matches. Historical references in docs/superpowers/specs/ and docs/superpowers/plans/ remain (intended for the audit trail).

Bundle 2 closes here. Bundle 3 migrates huntNpcs + huntPlayers to consume the new subscription primitive.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Bundle 3 — Consumer Migrations: huntNpcs + huntPlayers

Bundle 3 migrates the two `pkg/grid.Nearby*` consumers to use `pkg/zone.Zone.NpcsSafe`/`PlayersSafe`. `pkg/grid` write-side calls are NOT touched — full retirement deferred to a future NAI sub-spec.

## Task 3.1: Update test scaffolding to subscribe via Zone (extend helpers)

**Files:**
- Modify: `modules/world/npc_hunt_entities_test.go:34, 172` (test fixtures that call `s.grid.AddNpc` directly)
- Modify: any other test sites where NPCs are seeded into the spatial index without going through `addNpc`

- [ ] **Step 1: Locate non-addNpc subscription sites**

Run: `rg -n 's\.grid\.AddNpc|s\.grid\.Add\b' modules/world/*_test.go | head -10`
Expected: surfaces every test fixture that registers an NPC into `s.grid` without going through `addNpc`. Likely sites: `npc_hunt_entities_test.go:34, 172`, `player_npc_test.go:44`, `npc_event_queue_test.go:533`, `player_info_test.go:29` (this last one is for player slot).

- [ ] **Step 2: Decide migration shape per site**

For each site, the migration is one of two patterns:

**Pattern A — "use newRegisteredNpc"**: if the test site is a synthetic NPC fixture that doesn't need to bypass `addNpc`, replace the manual `s.grid.AddNpc(nid, x, z, level)` call with a `newRegisteredNpc(t, s, typ, true)` call. This route uses the production addNpc path which (post-Bundle 2) subscribes to both pkg/grid AND pkg/zone.

**Pattern B — "manually subscribe to zone"**: if the test site has a structural reason to bypass `addNpc` (e.g., manual NID assignment, test-specific lifecycle), append after the existing `s.grid.AddNpc(...)` call:
```go
z := s.zoneMap.Get(level, x, z)
n.zoneListElement = z.EnterNpc(n)
```

Plan-author note: walk each surfaced site, classify A or B based on the existing test's intent. For Bundle 3's purposes — "huntNpcs/huntPlayers must read from zone, not grid" — the test fixture must subscribe to zone for the existing tests to keep passing.

- [ ] **Step 3: Apply migrations**

For each surfaced site, apply the chosen pattern. The implementer reads each test, picks pattern A or B based on whether the test relies on bypassing `addNpc`'s side-effects.

Per `plan_helper_coverage` memory: confirm `newRegisteredNpc` (post-Task-2.1's newTestServer extension) now subscribes via Zone. Spot-check by running:
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestAddNpcEntersZone`
Expected: PASS.

- [ ] **Step 4: Run all tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/...`
Expected: ALL tests still pass. Goal: zero test breakage from this scaffolding update — Bundle 3's actual migration (Tasks 3.2 + 3.3) is what changes huntNpcs/huntPlayers behavior.

- [ ] **Step 5: Commit**

```bash
git add modules/world/*_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-28 Bundle 3 Task 3.1 — extend NPC test scaffolding to subscribe via Zone

Bundle 3 migrates huntNpcs + huntPlayers to read from pkg/zone subscription. For existing huntNpcs tests to keep passing post-migration, test fixtures that manually call s.grid.AddNpc must ALSO subscribe to pkg/zone. Most fixtures route through newRegisteredNpc which calls addNpc (post-Bundle-2 wires both); a few that bypass addNpc gain explicit EnterNpc calls.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.2: Migrate `huntNpcs` to Zone subscription

**Files:**
- Modify: `modules/world/npc_hunt_entities.go:23-76`
- Test: `modules/world/npc_hunt_entities_test.go`

- [ ] **Step 1: Write negative-pin test**

Append to `modules/world/npc_hunt_entities_test.go`:

```go
func TestHuntNpcsUsesZoneSubscriptionExclusive(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone, Category: -1}
	hunter := newRegisteredNpc(t, s, typ, true)
	hunter.huntRange = 5
	// Add a phantom NPC to s.grid only — bypass Zone subscription. Post-migration,
	// huntNpcs reads from Zone, so this NPC must NOT be returned.
	s.grid.AddNpc(99, hunter.x+1, hunter.z+1, hunter.level)
	hunt := &objtype.HuntType{CheckNpc: -1, CheckCategory: -1, CheckVis: objtype.HuntVisOff}
	got := hunter.huntNpcs(s, hunt)
	for _, e := range got {
		if other, ok := e.(*Npc); ok && other.nid == 99 {
			t.Error("huntNpcs returned grid-only NPC; should be Zone-exclusive")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestHuntNpcsUsesZoneSubscriptionExclusive`
Expected: FAIL — current huntNpcs reads from grid, so the phantom NPC at (3201, 3201) IS returned. (May not actually fail — the phantom isn't registered in `s.npcs`, so the lookup `s.npcs[99]` returns nil and the loop continues. In that case the test trivially passes; rewrite to ALSO add the entry to `s.npcs[99]` to force the issue. Implementer adapts.)

If the test trivially passes due to s.npcs[99] being nil, modify the test to also assign:
```go
s.npcs = make([]*Npc, 100)
s.npcs[99] = &Npc{nid: 99, x: hunter.x + 1, z: hunter.z + 1, level: hunter.level, typ: typ}
```
…before the grid.AddNpc call. Then re-run — expect FAIL.

- [ ] **Step 3: Migrate `huntNpcs`**

Modify `modules/world/npc_hunt_entities.go:23-76`. Find:

```go
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil || s.npcTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	nids := s.grid.NearbyNpcs(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, nid := range nids {
		if nid < 0 || nid >= len(s.npcs) {
			continue
		}
		other := s.npcs[nid]
		if other == nil {
			continue
		}
		if hunt.CheckNpc != -1 && other.typeId != hunt.CheckNpc {
			continue
		}
		// ... rest of filter chain
```

Replace the dispatch to use Zone subscription:

```go
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity {
	if s.zoneMap == nil || s.npcTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	var hunted []entity
	for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
		for nl := range zn.NpcsSafe(false) {
			other, ok := nl.(*Npc)
			if !ok {
				continue
			}
			if hunt.CheckNpc != -1 && other.typeId != hunt.CheckNpc {
				continue
			}
			if hunt.CheckCategory != -1 {
				if other.typeId < 0 || other.typeId >= len(s.npcTypes.Configs) {
					continue
				}
				ot := s.npcTypes.Configs[other.typeId]
				if ot == nil || ot.Category != hunt.CheckCategory {
					continue
				}
			}
			dx := other.x - n.x
			if dx < 0 {
				dx = -dx
			}
			dz := other.z - n.z
			if dz < 0 {
				dz = -dz
			}
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// CheckVis gate — TS ScriptIterators.ts:113-118.
			// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
			if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
					n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0) {
				continue
			}
			if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
					n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0) {
				continue
			}
			hunted = append(hunted, other)
		}
	}
	return hunted
}
```

Update the doc comment at the top of `huntNpcs` to reflect the new spatial backend (drop `s.grid.NearbyNpcs` reference; mention `pkg/zone.Zone.NpcsSafe` per NAI-28).

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestHuntNpcs`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: ALL existing huntNpcs tests pass + new exclusive-pin test passes.

Verify migration complete:
Run: `rg -n 's\.grid\.NearbyNpcs' modules/world/`
Expected: ZERO matches in production code. The comment-only reference at `npc_script_lookup.go:10` (`// future: route via s.grid.NearbyNpcs`) becomes stale — update or drop it (plan-author preference: drop; the comment is legacy advice).

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt_entities.go modules/world/npc_hunt_entities_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 3 Task 3.2 — migrate huntNpcs to Zone subscription

huntNpcs now reads from pkg/zone.Zone.NpcsSafe instead of pkg/grid.NearbyNpcs. Mirrors TS Npc.huntNpcs at Npc.ts:975-977 which iterates HuntIterator's NPC branch via Zone.getAllNpcsSafe.

Type-assertion to *Npc at iteration is the canonical pattern for the cyclic-import boundary (PlayerLike/NpcLike interfaces in pkg/zone). All existing huntNpcs tests pass with no test-side changes (behavioral parity); new exclusive-pin test confirms grid-only NPCs are NOT returned.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

## Task 3.3: Migrate `huntPlayers` to Zone subscription

**Files:**
- Modify: `modules/world/npc_hunt.go:108-180`
- Test: `modules/world/npc_hunt_test.go`

- [ ] **Step 1: Write negative-pin test**

Append to `modules/world/npc_hunt_test.go`:

```go
func TestHuntPlayersUsesZoneSubscriptionExclusive(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, VisLevel: 50}
	hunter := newRegisteredNpc(t, s, typ, true)
	hunter.huntRange = 5
	// Add a phantom player to s.grid only — bypass Zone subscription.
	s.grid.Add(99, hunter.x+1, hunter.z+1, hunter.level)
	if len(s.players) <= 99 {
		s.players = make([]*Player, 100)
	}
	c, _ := newTestClient(t)
	phantom := newPlayer(c)
	phantom.slot = 99
	phantom.x, phantom.z, phantom.level = hunter.x+1, hunter.z+1, hunter.level
	phantom.combatLevel = 50
	s.players[99] = phantom
	hunt := &objtype.HuntType{CheckNpc: -1, CheckVis: objtype.HuntVisOff, CheckNotTooStrong: objtype.HuntCheckNotTooStrongOff}
	got := hunter.huntPlayers(s, hunt)
	for _, e := range got {
		if pl, ok := e.(*Player); ok && pl.slot == 99 {
			t.Error("huntPlayers returned grid-only player; should be Zone-exclusive")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestHuntPlayersUsesZoneSubscriptionExclusive`
Expected: FAIL — current huntPlayers reads from grid; phantom IS returned.

- [ ] **Step 3: Migrate `huntPlayers`**

Modify `modules/world/npc_hunt.go:108-180`. Find:

```go
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil {
		return nil
	}
	// TS HuntIterator zone-radius formula at ScriptIterators.ts:57:
	// radius = (1 + distance/8) | 0.
	zoneRadius := 1 + n.huntRange/8
	slots := s.grid.NearbyPlayers(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, slot := range slots {
		if slot < 0 || slot >= len(s.players) {
			continue
		}
		p := s.players[slot]
		if p == nil {
			continue
		}
		if p.level != n.level {
			continue
		}
		// ... rest of filter chain
```

Replace with (preserving all 5 filter branches verbatim):

```go
func (n *Npc) huntPlayers(s *Server, hunt *objtype.HuntType) []entity {
	if s.zoneMap == nil {
		return nil
	}
	// TS HuntIterator zone-radius formula at ScriptIterators.ts:57:
	// radius = (1 + distance/8) | 0.
	zoneRadius := 1 + n.huntRange/8
	var hunted []entity
	for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
		for pl := range zn.PlayersSafe(false) {
			p, ok := pl.(*Player)
			if !ok {
				continue
			}
			// Level filter is redundant — NearbyZones is already level-filtered —
			// but kept for defensive symmetry with TS huntPlayers; harmless.
			if p.level != n.level {
				continue
			}
			dx := p.x - n.x
			if dx < 0 {
				dx = -dx
			}
			dz := p.z - n.z
			if dz < 0 {
				dz = -dz
			}
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// checkNotBusy (TS:931-933): skip players whose state cannot
			// accept a hunt interaction (delayed or main/chat modal open).
			if hunt.CheckNotBusy && p.Busy() {
				continue
			}
			// checkAfk (TS:935-937): filter players who've gone AFK
			// (1000-tick same-zone threshold).
			if hunt.CheckAfk && p.IsZonesAfk() {
				continue
			}
			// CheckVis gate — TS ScriptIterators.ts:88-94.
			// FIDELITY: TS huntPlayers swaps src/dest vs other three variants —
			// player-as-source (p.x, p.z) → NPC-as-dest (n.x, n.z). Preserve
			// the asymmetry verbatim. See NAI-12 spec § Architecture.
			// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
			if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
					n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
				continue
			}
			if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
					n.level, p.x, p.z, n.x, n.z, 1, 1, 1, 0) {
				continue
			}
			// checkNotTooStrong (TS:939-941): skip players whose combatLevel
			// is more than 2x the NPC's vislevel when they are OUTSIDE the
			// wilderness (the wilderness disables this protection). Filter
			// only applies when CheckNotTooStrong is OutsideWilderness;
			// Off → filter skipped.
			if hunt.CheckNotTooStrong == objtype.HuntCheckNotTooStrongOutsideWilderness &&
				!p.IsInWilderness() &&
				p.combatLevel > n.typ.VisLevel*2 {
				continue
			}
			hunted = append(hunted, p)
		}
	}
	return hunted
}
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestHuntPlayers`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: ALL existing huntPlayers tests pass + new exclusive-pin test passes.

Verify migration complete:
Run: `rg -n 's\.grid\.NearbyPlayers|s\.grid\.NearbyNpcs' modules/world/`
Expected: ZERO matches in non-test production code.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_hunt.go modules/world/npc_hunt_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-28 Bundle 3 Task 3.3 — migrate huntPlayers to Zone subscription

huntPlayers now reads from pkg/zone.Zone.PlayersSafe instead of pkg/grid.NearbyPlayers. Mirrors TS Npc.huntPlayers HuntIterator player branch which iterates Zone.getAllPlayersSafe.

All existing tests pass with no behavioral changes; new exclusive-pin test confirms grid-only players are NOT returned. Both Bundle 3 read-side migrations complete; pkg/grid write-side calls remain in tick.go (full retirement deferred — tracked under "From NAI-28" entry at close).

Bundle 3 closes here; polish + close commit follow.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Polish + NAI-28 Close

## Task P.1: Polish commit (after Bundle 3 close, before NAI-28 close)

**Files:** Variable based on review feedback.

- [ ] **Step 1: Run final reviewers per `runescript_cadence`**

Per `runescript_cadence` memory: dispatch Stage 1 (spec compliance) + Stage 2 (code quality) reviewers across all three bundles + a final whole-impl review. Reviewer findings (if any minor) absorb into a single polish commit.

Verify zero deviation-tag references:
Run: `rg -n 'NAI-19-D1' modules/ pkg/`
Expected: ZERO matches.

Verify deviation count consistency in commit bodies:
Run: `git log --oneline --all | head -25`
Expected: each Bundle close commit references "14 → 13" or equivalent.

- [ ] **Step 2: Apply polish**

If reviewers flag minors (doc-comment polish, helper-name normalisation, dead-code removal), apply in one polish commit. If no minors, skip Task P.1 and proceed to Task P.2.

- [ ] **Step 3: Commit (if needed)**

```bash
git add <files>
git commit --no-gpg-sign -m "polish(world,zone): NAI-28 close polish — final review minors

<concise narrative of polish items>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

## Task P.2: NAI-28 close commit

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark NAI-19-D1 entry Resolved + add From-NAI-28 entry)

- [ ] **Step 1: Read current NAI-19-D1 entry in `nai_followups.md`**

Use the `Read` tool (NOT Bash, per `memory_write_sandbox_quirk` memory): read the section under `## From NAI-19 (2026-04-24)` covering NAI-19-D1.

- [ ] **Step 2: Update NAI-19-D1 entry to "Resolved"**

Insert a "Resolved 2026-04-25 (NAI-28, commits ...)" header at the top of the entry's body, preserving the original deferral narrative below per the established convention (see e.g. NAI-20 entry at `:1129`).

- [ ] **Step 3: Add new "From NAI-28" entry tracking pkg/grid retirement**

Append a new section at the end of `nai_followups.md`:

```markdown
## From NAI-28 (2026-04-25)

### Deferred: pkg/grid full retirement

NAI-28 Bundle 3 migrated the read-side consumers (huntNpcs, huntPlayers) from `s.grid.NearbyNpcs` / `s.grid.NearbyPlayers` to pkg/zone subscription. Write-side calls remain at:
- `modules/world/tick.go:320-322` (player movement)
- `modules/world/tick.go:356-357` (NPC movement)
- `modules/world/server.go:238` (NPC seeding)
- Plus test scaffolding in `*_test.go` files

Once a future sub-spec migrates these write-side calls to use Zone subscription state directly (or removes the redundant updates entirely), `pkg/grid/` is deletable.

**Remediation when picked up:** verify zero remaining `s.grid.` consumers via `rg -n 's\.grid\.' modules/`, then `git rm -r pkg/grid/`. The pkg/zone read-side already proves Zone iteration is sufficient for spatial queries; remaining work is mechanical.

### NAI-28 close — Zone PathingEntity subscription primitive port

[Brief 3-bundle close summary. Reference commits: Bundle 1 close `<hash>`, Bundle 2 close `<hash>`, Bundle 3 close `<hash>`, polish `<hash if any>`, close `<this commit>`. Net deviation count: 14 → 13.]
```

- [ ] **Step 4: Add NAI-28 memory entry pointer in MEMORY.md**

Per `post_task_handoff` memory: any new memory entries added at NAI-28 close get a one-line pointer in `MEMORY.md`. Pre-flagged candidates from spec § NAI-28 close:
- `parallel_spatial_index_migration_pattern`
- `interface_at_cyclic_import_boundary`
- `framing_drift_in_followup_tracker`

Implementer evaluates each at close and saves only those that surfaced a real lesson (per `dead_api_polish` memory analog: don't save speculative memory entries).

- [ ] **Step 5: Commit**

```bash
git add <memory files>
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world,zone): NAI-28 closed — Zone PathingEntity subscription port + huntNpcs/huntPlayers consumer migration

3 bundles + polish + close. Deviation count 14 → 13 (NAI-19-D1 retired, no new tags introduced).

Bundle 1 (commit <hash>): pkg/zone DoublyLinkList[T] primitive + Zone subscription methods, 11 list tests + 11 zone-subscription tests.
Bundle 2 (commit <hash>): 11 wire-through edits across modules/world covering addPlayer/removePlayer (server.go), addNpc/removeNpc (npc_registry.go — retires NAI-19-D1), Player.stepOnce/Npc.stepOnce per-step refreshZone, Player Teleport+TeleJump cross-zone refresh, 3 NPC teleport sites, and final NAI-19-D1 doc-comment retirement in npc.go.
Bundle 3 (commit <hash>): huntNpcs migrated to Zone.NpcsSafe (npc_hunt_entities.go), huntPlayers migrated to Zone.PlayersSafe (npc_hunt.go); both with Zone-exclusive negative-pin tests proving no fallback to pkg/grid.

pkg/grid write-side calls remain (tick.go:320-322, 356-357; server.go:238). Full pkg/grid retirement deferred to future NAI sub-spec; tracked under new "From NAI-28" entry in nai_followups.md.

Memory entries added at NAI-28 close: <enumerate as actually saved>.

Closes memory: nai_followups.md (NAI-19-D1 retirement)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

# Plan Self-Review Checklist

After completing all tasks, the implementer/controller verifies:

- [ ] `rg -n 'NAI-19-D1' modules/ pkg/` returns ZERO matches (deviation tag fully retired)
- [ ] `rg -n 's\.grid\.NearbyPlayers|s\.grid\.NearbyNpcs' modules/world/` returns only test-fixture matches (production migrated)
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...` passes (race detector clean)
- [ ] Bundle 1 close commit subject starts with `feat(zone): NAI-28 Bundle 1`
- [ ] Bundle 2 close commit subject starts with `feat(world): NAI-28 Bundle 2` or `docs(world): NAI-28 Bundle 2 Task 2.12`
- [ ] Bundle 3 close commit subject starts with `feat(world): NAI-28 Bundle 3 Task 3.3`
- [ ] Close commit subject starts with `chore(world,zone): NAI-28 closed`
- [ ] All commit bodies end with the standard `Co-Authored-By` trailer
- [ ] Memory entries added at close mention NAI-28 in their body
- [ ] `nai_followups.md` MEMORY.md index entries are line-bounded (≤150 char per memory discipline)
- [ ] Net deviation count consistent across all bundle commit bodies (14 → 13)

If any check fails, the implementer raises before declaring NAI-28 complete.
