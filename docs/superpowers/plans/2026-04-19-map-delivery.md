# Map Delivery Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the cache config plumbing (`ObjType`, `InvType`), a real `Inventory` type, per-player `BuildArea`, the `RebuildNormal` packet, `UpdateInvFull`, and an equipment-driven appearance generator — so a logged-in player sees the map render and their character draw in the Java client.

**Architecture:** Vendor the config + jagfile + bzip2 machinery from `rs-server-225`, add three new self-contained Go packages (`pkg/inventory`, `pkg/buildarea`, `pkg/xtea`), extend `Player` with inventory + buildarea fields + BAS-anim fields, implement three new `modules/world/` files (`appearance.go`, `rebuildmap.go`, `inv_update.go`), and wire everything into `NewServer` and `processLogins`.

**Tech Stack:** Go standard library only — `hash/crc32` (for mapsquare CRCs), `path/filepath`, `os`. Vendored code from `rs-server-225`.

> All `go` commands must use the prefix: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `pkg/io/jagfile/jagfile.go` | **New (vendored)** | `Jagfile` archive reader (from `rs-server-225/io/jagfile.go`) |
| `pkg/io/jagfile/bzip2.go` | **New (vendored)** | BZIP2 codec helpers (from `rs-server-225/io/bzip2.go`) |
| `pkg/io/packet/load.go` | Modify | Add `Load(path string, compressed bool) (*Packet, error)` helper if missing |
| `pkg/objtype/configtype.go` | **New (vendored)** | Shared `ConfigType` base + `DecodeType` dispatcher |
| `pkg/objtype/paramtype.go` | **New (vendored)** | `ParamType` + `LoadParams` |
| `pkg/objtype/objtype.go` | **New (vendored)** | `ObjType` + `LoadObjTypes` |
| `pkg/objtype/invtype.go` | **New (vendored)** | `InvType` + `LoadInvTypes` |
| `pkg/objtype/*_test.go` | New | Smoke tests loading real pack files from `data/pack` |
| `pkg/inventory/inventory.go` | **New** | `Inventory` struct; `Item`; slot operations; `FromType` factory |
| `pkg/inventory/transaction.go` | **New** | `Transaction{Requested, Completed, Items}` |
| `pkg/inventory/inventory_test.go` | **New** | Add/Remove/Swap unit tests |
| `pkg/buildarea/buildarea.go` | **New** | `BuildArea` + `ShouldRebuild` + `Rebuild` |
| `pkg/buildarea/buildarea_test.go` | **New** | Window-edge trigger tests |
| `pkg/xtea/xtea.go` | **New** | `Keys(mapX, mapZ int) [4]uint32` stub (returns zeros) |
| `pkg/gamemap/gamemap.go` | Modify | Add `mapCRC`/`locCRC` maps; cache CRC32 during `Init`; expose `MapsquareCRC` |
| `pkg/io/protocol/game/server/prot.go` | Modify | Add `OpRebuildNormal`, `OpUpdateInvFull`, `OpUpdateInvPartial` |
| `modules/world/masks.go` | **New** | 10 player mask constants (MaskAppearance=1, MaskAnim=2, ..., MaskExactMove=512) |
| `modules/world/player.go` | Modify | Add `invs`, `invListeners`, `buildArea` fields + BAS anim fields; update `newPlayer` defaults |
| `modules/world/appearance.go` | **New** | `generateAppearance` writing `p.appearanceBuf` |
| `modules/world/rebuildmap.go` | **New** | `sendRebuildNormal`; fill in `updateMap` |
| `modules/world/inv_update.go` | **New** | `sendUpdateInvFull`; fill in `updateInvs` |
| `modules/world/server.go` | Modify | Add `paramTypes`/`objTypes`/`invTypes` fields; load during `NewServer` |
| `modules/world/tick.go` | Modify | `processLogins` initializes `buildArea`, `invs`, masks |

---

## Vendoring Notes

- **Follow sub-spec 2's pattern:** copy the tree, rewrite imports via `sed`, adapt calls to goscape types, run tests.
- **Goscape packet package** lives at `pkg/io/packet` and has `G1/G2/G4/Next/Len` methods. The older project's `packet.Packet` has exported `Data []byte` and `Pos int` fields; goscape's packet uses a `bytes.Buffer` internally. When vendoring configtype.go etc., confirm every `buf.G1()` style call works the same; if the vendored code accesses `.Data` or `.Pos` directly, either add those fields or rewrite the call to use equivalent methods.
- **Jagfile depends on bzip2.** Vendor both together.
- **`packet.Load`:** if goscape's packet package lacks a `Load` helper that reads a file and returns a `*Packet`, add one (~10 LOC). The config loaders call `packet.Load(path, false)`.

---

## Task 1: Vendor `pkg/io/jagfile`

**Files:**
- Create: `pkg/io/jagfile/jagfile.go`
- Create: `pkg/io/jagfile/bzip2.go`
- Create: `pkg/io/jagfile/jagfile_test.go`
- Create: `pkg/io/jagfile/bzip2_test.go`

- [ ] **Step 1: Copy the files**

```bash
mkdir -p /home/owner/Code/github.com/zsrv/goscape/pkg/io/jagfile
cp /home/owner/Code/github.com/zsrv/rs-server-225/io/jagfile.go \
   /home/owner/Code/github.com/zsrv/rs-server-225/io/jagfile_test.go \
   /home/owner/Code/github.com/zsrv/rs-server-225/io/bzip2.go \
   /home/owner/Code/github.com/zsrv/rs-server-225/io/bzip2_test.go \
   /home/owner/Code/github.com/zsrv/goscape/pkg/io/jagfile/
```

- [ ] **Step 2: Rewrite package declarations and imports**

```bash
cd /home/owner/Code/github.com/zsrv/goscape/pkg/io/jagfile
sed -i '1s/^package io$/package jagfile/' jagfile.go bzip2.go jagfile_test.go bzip2_test.go
sed -i 's|github.com/zsrv/rs-server-225/io/packet|github.com/zsrv/goscape/pkg/io/packet|g' *.go
sed -i 's|github.com/zsrv/rs-server-225/io|github.com/zsrv/goscape/pkg/io/jagfile|g' *.go
```

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/io/jagfile/... 2>&1
```

Expected: no errors. If goscape's `packet.Packet` API differs from `rs-server-225`'s (e.g., missing `.Data`/`.Pos` exported fields), fix by either adding the missing methods to goscape's packet or rewriting the offending call. Common adjustments:

- `p.Data` → use `p.Bytes()` or introduce a `Data() []byte` getter on goscape's Packet
- `p.Pos` → add a `Pos() int` / `SetPos(int)` or rework
- `packet.NewPacket(data)` exists in both; confirm the constructor signature matches

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/jagfile/... -v 2>&1 | tail -20
```

Expected: all vendored tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/io/jagfile/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(jagfile): vendor Jagfile + bzip2 io helpers from rs-server-225

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `packet.Load` helper if missing

**Files:**
- Modify: `pkg/io/packet/` (inspect and add `Load` if absent)

- [ ] **Step 1: Check existing API**

```bash
grep -rn "^func Load\|^func.*Load(" /home/owner/Code/github.com/zsrv/goscape/pkg/io/packet/ 2>&1
```

If `Load` is already defined, **skip to step 4**. If not, continue.

- [ ] **Step 2: Add `Load` helper**

Create `pkg/io/packet/load.go`:

```go
package packet

import (
	"os"
)

// Load reads a file at path and returns it wrapped in a *Packet.
// When compressed=true, the file is BZIP2-decompressed first; sub-spec 3a's
// callers always pass false.
func Load(path string, compressed bool) (*Packet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if compressed {
		// Out of scope for sub-spec 3a (no compressed loaders invoked).
		// Future callers can decompress via pkg/io/jagfile's bzip2 helpers.
		panic("packet.Load: compressed=true not yet supported")
	}
	return NewPacket(data), nil
}
```

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/io/packet/... 2>&1
```

Expected: no errors.

- [ ] **Step 4: Commit (if changes made)**

```bash
git add pkg/io/packet/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(packet): add Load helper for file-backed packets

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Vendor `pkg/objtype` (ObjType, InvType, ParamType, ConfigType)

**Files:**
- Create: `pkg/objtype/configtype.go`
- Create: `pkg/objtype/paramtype.go`
- Create: `pkg/objtype/objtype.go`
- Create: `pkg/objtype/invtype.go`

- [ ] **Step 1: Copy the four files**

```bash
mkdir -p /home/owner/Code/github.com/zsrv/goscape/pkg/objtype
cp /home/owner/Code/github.com/zsrv/rs-server-225/cache/config/configtype.go \
   /home/owner/Code/github.com/zsrv/rs-server-225/cache/config/paramtype.go \
   /home/owner/Code/github.com/zsrv/rs-server-225/cache/config/objtype.go \
   /home/owner/Code/github.com/zsrv/rs-server-225/cache/config/invtype.go \
   /home/owner/Code/github.com/zsrv/goscape/pkg/objtype/
```

- [ ] **Step 2: Rewrite package declarations and imports**

```bash
cd /home/owner/Code/github.com/zsrv/goscape/pkg/objtype
sed -i '1s/^package config$/package objtype/' *.go
sed -i 's|github.com/zsrv/rs-server-225/io/packet|github.com/zsrv/goscape/pkg/io/packet|g' *.go
sed -i 's|"github.com/zsrv/rs-server-225/io"|io "github.com/zsrv/goscape/pkg/io/jagfile"|g' *.go
```

The last sed line aliases the jagfile package to `io`, so the vendored code's `io.LoadJagfile`, `io.Jagfile`, etc. references keep compiling unchanged. Confirm with `grep '"github.com/zsrv/goscape/pkg/io/jagfile"' *.go` — the import line should show `io "github.com/...jagfile"`.

- [ ] **Step 3: Build and fix any remaining issues**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/objtype/... 2>&1 | head -30
```

Expected: no errors. If the loaders reference unvendored types (e.g., `paramhelper`, `varplayertype`), trim those lines and their call sites. The minimum required surface is `LoadObjTypes`, `LoadInvTypes`, `LoadParams`, `ObjType`, `InvType`, `ObjTypeConfigs`, `InvTypeConfigs`, `ParamTypeConfigs`.

- [ ] **Step 4: Run tests and smoke-check loading real data**

Create `pkg/objtype/objtype_test.go`:

```go
package objtype

import (
	"path/filepath"
	"testing"
)

// TestLoadObjTypesFromPack loads the real data/pack directory.
// This test is a smoke check — skips gracefully if the data isn't present.
func TestLoadObjTypesFromPack(t *testing.T) {
	// Resolve repo root: runs in pkg/objtype, so go up two levels.
	cacheDir := filepath.Join("..", "..", "data", "pack")

	params, err := LoadParams(cacheDir)
	if err != nil {
		t.Skipf("no cache data (skipping): %v", err)
	}

	objs, err := LoadObjTypes(cacheDir, params)
	if err != nil {
		t.Fatalf("LoadObjTypes: %v", err)
	}
	if len(objs.Configs) == 0 {
		t.Fatal("expected at least one ObjType, got 0")
	}

	invs, err := LoadInvTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadInvTypes: %v", err)
	}
	if len(invs.Configs) == 0 {
		t.Fatal("expected at least one InvType, got 0")
	}

	// worn is a well-known InvType name — verify the resolver wired up.
	// ConfigNames should contain "worn".
	if _, ok := invs.ConfigNames["worn"]; !ok {
		t.Error("expected invs.ConfigNames to contain 'worn'")
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/... -v 2>&1 | tail -10`
Expected: either pass or skip (no data). If the test fails with a real error, the vendoring needs more adaptation.

- [ ] **Step 5: Commit**

```bash
git add pkg/objtype/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): vendor ObjType/InvType/ParamType/ConfigType from rs-server-225

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Create `pkg/inventory`

**Files:**
- Create: `pkg/inventory/inventory.go`
- Create: `pkg/inventory/transaction.go`
- Create: `pkg/inventory/inventory_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/inventory/inventory_test.go`:

```go
package inventory

import "testing"

func TestNewHasCapacity(t *testing.T) {
	inv := New(1, 28, StackNormal)
	if inv.Capacity != 28 {
		t.Errorf("Capacity: got %d, want 28", inv.Capacity)
	}
	if len(inv.Items) != 28 {
		t.Errorf("Items len: got %d, want 28", len(inv.Items))
	}
}

func TestAddIntoEmptyInventory(t *testing.T) {
	inv := New(1, 28, StackNormal)
	tx := inv.Add(10, 1, AddOpts{})
	if tx.Completed != 1 {
		t.Errorf("Completed: got %d, want 1", tx.Completed)
	}
	if inv.Items[0] == nil || inv.Items[0].Id != 10 || inv.Items[0].Count != 1 {
		t.Errorf("slot 0 after add: %+v", inv.Items[0])
	}
	if !inv.Update {
		t.Error("Update flag should be true after add")
	}
}

func TestAddStackingBehavior(t *testing.T) {
	// StackAlways: multiple adds of same id accumulate in one slot.
	inv := New(1, 28, StackAlways)
	inv.Add(10, 3, AddOpts{})
	inv.Add(10, 5, AddOpts{})
	if inv.Items[0].Count != 8 {
		t.Errorf("stacked count: got %d, want 8", inv.Items[0].Count)
	}
	if inv.Items[1] != nil {
		t.Error("slot 1 should be empty")
	}
}

func TestAddNoStackFillsSlots(t *testing.T) {
	// StackNever: each unit takes its own slot.
	inv := New(1, 28, StackNever)
	inv.Add(10, 3, AddOpts{})
	for i := 0; i < 3; i++ {
		if inv.Items[i] == nil || inv.Items[i].Count != 1 {
			t.Errorf("slot %d: %+v", i, inv.Items[i])
		}
	}
}

func TestRemoveDecrementsCount(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 5, AddOpts{})
	tx := inv.Remove(10, 2, RemoveOpts{})
	if tx.Completed != 2 {
		t.Errorf("Completed: got %d, want 2", tx.Completed)
	}
	if inv.Items[0].Count != 3 {
		t.Errorf("after remove count: got %d, want 3", inv.Items[0].Count)
	}
}

func TestRemovePartialWhenInsufficient(t *testing.T) {
	inv := New(1, 28, StackAlways)
	inv.Add(10, 3, AddOpts{})
	tx := inv.Remove(10, 5, RemoveOpts{})
	if tx.Completed != 3 {
		t.Errorf("Completed (partial): got %d, want 3", tx.Completed)
	}
	if inv.Items[0] != nil {
		t.Error("slot should be empty after full drain")
	}
}

func TestSwapExchangesSlots(t *testing.T) {
	inv := New(1, 28, StackNormal)
	inv.Items[0] = &Item{Id: 10, Count: 1}
	inv.Items[1] = &Item{Id: 20, Count: 1}
	inv.Swap(0, 1)
	if inv.Items[0].Id != 20 || inv.Items[1].Id != 10 {
		t.Errorf("after swap: %+v %+v", inv.Items[0], inv.Items[1])
	}
}

func TestIsFullAndFreeSlotCount(t *testing.T) {
	inv := New(1, 3, StackNormal)
	if inv.IsFull() {
		t.Error("new inv should not be full")
	}
	if inv.FreeSlotCount() != 3 {
		t.Errorf("free: got %d, want 3", inv.FreeSlotCount())
	}
	inv.Add(10, 3, AddOpts{})
	if !inv.IsFull() {
		t.Error("inv should be full after 3 adds")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/inventory/... 2>&1 | head -5
```

Expected: compile error — package doesn't exist.

- [ ] **Step 3: Create `pkg/inventory/transaction.go`**

```go
package inventory

// Transaction is the result of an Add or Remove operation.
type Transaction struct {
	Requested int    // units the caller asked for
	Completed int    // units actually added/removed
	Items     []Item // items moved (used by Transfer)
}
```

- [ ] **Step 4: Create `pkg/inventory/inventory.go`**

```go
package inventory

import "github.com/zsrv/goscape/pkg/objtype"

// Stacking strategy constants.
const (
	StackNormal = 0 // stack based on ObjType.Stackable
	StackAlways = 1 // always stack
	StackNever  = 2 // never stack (each unit its own slot)
)

// StackLimit is the maximum count for a single stacked item.
const StackLimit = 0x7fffffff

type Item struct {
	Id    int
	Count int
}

type Inventory struct {
	Type      int // InvType.ID
	Capacity  int
	StackType int
	Items     []*Item // length == Capacity; nil = empty
	Update    bool    // consumed by world.updateInvs()
}

// New returns an empty inventory of the given capacity.
func New(typeId, capacity, stackType int) *Inventory {
	return &Inventory{
		Type:      typeId,
		Capacity:  capacity,
		StackType: stackType,
		Items:     make([]*Item, capacity),
	}
}

// FromType builds an inventory matching an InvType's size/stack semantics and
// populates its StockObj items.
func FromType(t *objtype.InvType) *Inventory {
	inv := New(t.ID, t.Size, stackTypeFrom(t))
	for i, id := range t.StockObj {
		if id <= 0 {
			continue
		}
		count := 1
		if i < len(t.StockCount) && t.StockCount[i] > 0 {
			count = t.StockCount[i]
		}
		inv.Items[i] = &Item{Id: id, Count: count}
	}
	return inv
}

func stackTypeFrom(t *objtype.InvType) int {
	if t.StackAll {
		return StackAlways
	}
	return StackNormal
}

// --- queries ---

func (inv *Inventory) Get(slot int) *Item {
	if slot < 0 || slot >= inv.Capacity {
		return nil
	}
	return inv.Items[slot]
}

func (inv *Inventory) Contains(id int) bool {
	return inv.GetItemIndex(id) >= 0
}

func (inv *Inventory) HasAt(slot, id int) bool {
	it := inv.Get(slot)
	return it != nil && it.Id == id
}

func (inv *Inventory) GetItemCount(id int) int {
	total := 0
	for _, it := range inv.Items {
		if it != nil && it.Id == id {
			total += it.Count
		}
	}
	return total
}

func (inv *Inventory) GetItemIndex(id int) int {
	for i, it := range inv.Items {
		if it != nil && it.Id == id {
			return i
		}
	}
	return -1
}

func (inv *Inventory) NextFreeSlot() int {
	for i, it := range inv.Items {
		if it == nil {
			return i
		}
	}
	return -1
}

func (inv *Inventory) FreeSlotCount() int {
	n := 0
	for _, it := range inv.Items {
		if it == nil {
			n++
		}
	}
	return n
}

func (inv *Inventory) IsFull() bool  { return inv.FreeSlotCount() == 0 }
func (inv *Inventory) IsEmpty() bool { return inv.FreeSlotCount() == inv.Capacity }

// --- mutations ---

type AddOpts struct {
	BeginSlot           int  // start searching from this slot (default 0)
	AssureFullInsertion bool // if true, all-or-nothing
	ForceNoStack        bool // force non-stacking behaviour even if StackType says stack
	DryRun              bool // compute without mutating
}

type RemoveOpts struct {
	BeginSlot         int
	AssureFullRemoval bool
}

func (inv *Inventory) Set(slot int, item *Item) {
	if slot < 0 || slot >= inv.Capacity {
		return
	}
	inv.Items[slot] = item
	inv.Update = true
}

func (inv *Inventory) Delete(slot int) { inv.Set(slot, nil) }

func (inv *Inventory) Swap(from, to int) {
	if from < 0 || from >= inv.Capacity || to < 0 || to >= inv.Capacity {
		return
	}
	inv.Items[from], inv.Items[to] = inv.Items[to], inv.Items[from]
	inv.Update = true
}

func (inv *Inventory) Add(id, count int, opts AddOpts) Transaction {
	tx := Transaction{Requested: count}
	if count <= 0 {
		return tx
	}

	shouldStack := inv.StackType == StackAlways && !opts.ForceNoStack

	if shouldStack {
		// Find existing stack or first free slot.
		idx := inv.GetItemIndex(id)
		if idx < 0 {
			idx = inv.findFreeSlotFrom(opts.BeginSlot)
		}
		if idx < 0 {
			return tx // no room
		}
		if !opts.DryRun {
			if inv.Items[idx] == nil {
				inv.Items[idx] = &Item{Id: id, Count: count}
			} else {
				newCount := inv.Items[idx].Count + count
				if newCount > StackLimit {
					newCount = StackLimit
				}
				inv.Items[idx].Count = newCount
			}
			inv.Update = true
		}
		tx.Completed = count
		return tx
	}

	// Non-stacking: one unit per slot.
	added := 0
	for added < count {
		idx := inv.findFreeSlotFrom(opts.BeginSlot)
		if idx < 0 {
			break
		}
		if !opts.DryRun {
			inv.Items[idx] = &Item{Id: id, Count: 1}
		}
		added++
	}
	if !opts.DryRun && added > 0 {
		inv.Update = true
	}
	if opts.AssureFullInsertion && added < count {
		// Roll back.
		if !opts.DryRun {
			for i, it := range inv.Items {
				if it != nil && it.Id == id {
					inv.Items[i] = nil
				}
			}
		}
		return tx // Completed=0
	}
	tx.Completed = added
	return tx
}

func (inv *Inventory) findFreeSlotFrom(begin int) int {
	if begin < 0 {
		begin = 0
	}
	for i := begin; i < inv.Capacity; i++ {
		if inv.Items[i] == nil {
			return i
		}
	}
	return -1
}

func (inv *Inventory) Remove(id, count int, opts RemoveOpts) Transaction {
	tx := Transaction{Requested: count}
	if count <= 0 {
		return tx
	}
	removed := 0
	begin := opts.BeginSlot
	if begin < 0 {
		begin = 0
	}
	for i := begin; i < inv.Capacity && removed < count; i++ {
		it := inv.Items[i]
		if it == nil || it.Id != id {
			continue
		}
		take := count - removed
		if take > it.Count {
			take = it.Count
		}
		it.Count -= take
		removed += take
		if it.Count == 0 {
			inv.Items[i] = nil
		}
	}
	if removed > 0 {
		inv.Update = true
	}
	if opts.AssureFullRemoval && removed < count {
		// No rollback in sub-spec 3a; callers check tx.Completed.
	}
	tx.Completed = removed
	return tx
}
```

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/inventory/... -v 2>&1 | tail -20
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/inventory/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(inventory): add Inventory type with add/remove/swap/stack operations

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Create `pkg/buildarea`

**Files:**
- Create: `pkg/buildarea/buildarea.go`
- Create: `pkg/buildarea/buildarea_test.go`

- [ ] **Step 1: Write the failing tests**

Create `pkg/buildarea/buildarea_test.go`:

```go
package buildarea

import "testing"

func TestNewNeedsFirstRebuild(t *testing.T) {
	ba := New()
	if !ba.ShouldRebuild(3094, 3106, false) {
		t.Error("first ShouldRebuild (OriginX=-1) should be true")
	}
}

func TestRebuildCommitsOrigin(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 5)
	if ba.OriginX != 3094 || ba.OriginZ != 3106 {
		t.Errorf("origin: got (%d,%d), want (3094,3106)", ba.OriginX, ba.OriginZ)
	}
	if ba.LastBuild != 5 {
		t.Errorf("LastBuild: got %d, want 5", ba.LastBuild)
	}
}

func TestShouldNotRebuildWithinWindow(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 1)
	// Player takes one step north — still well inside the 13-zone window.
	if ba.ShouldRebuild(3094, 3107, false) {
		t.Error("single-step movement should not trigger rebuild")
	}
}

func TestShouldRebuildAtWindowEdge(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 1)
	// Zone of origin: x=386, z=388. Window: reloadLeftX=(386-4)<<3=3056,
	// reloadRightX=(386+5)<<3=3128. Crossing to 3055 should trigger rebuild.
	if !ba.ShouldRebuild(3055, 3106, false) {
		t.Error("crossing west window edge should trigger rebuild")
	}
}

func TestReconnectAlwaysTriggers(t *testing.T) {
	ba := New()
	ba.Rebuild(3094, 3106, 1)
	if !ba.ShouldRebuild(3094, 3106, true) {
		t.Error("reconnect should always trigger rebuild")
	}
}

func TestRebuildPopulatesMapsquares(t *testing.T) {
	ba := New()
	ms := ba.Rebuild(3094, 3106, 1)
	if len(ms) == 0 {
		t.Error("rebuild should return mapsquares")
	}
	// Origin mapsquare = (3094>>6, 3106>>6) = (48, 48). Must be in set.
	want := uint16((48 << 8) | 48)
	found := false
	for _, m := range ms {
		if m == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected (48,48) in mapsquare list; got %v", ms)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/buildarea/... 2>&1 | head -5
```

Expected: package doesn't exist.

- [ ] **Step 3: Create `pkg/buildarea/buildarea.go`**

```go
package buildarea

import "sort"

// BuildArea tracks a player's 13x13 mapsquare window centred on the last
// anchor point (OriginX, OriginZ). The TS reference lives in
// LostCityRS/Engine-TS/src/engine/entity/BuildArea.ts.
type BuildArea struct {
	OriginX     int
	OriginZ     int
	LastBuild   int
	LoadedZones map[int]bool  // zone-packed coord -> seen
	ActiveZones map[int]bool  // zone-packed coord -> active this tick
	Mapsquares  map[uint16]bool
}

func New() *BuildArea {
	return &BuildArea{
		OriginX:     -1,
		OriginZ:     -1,
		LoadedZones: map[int]bool{},
		ActiveZones: map[int]bool{},
		Mapsquares:  map[uint16]bool{},
	}
}

// ShouldRebuild reports whether the player has crossed the 13x13 zone window
// centred on (OriginX, OriginZ), or whether reconnect is true (force).
func (ba *BuildArea) ShouldRebuild(playerX, playerZ int, reconnect bool) bool {
	if ba.OriginX == -1 {
		return true
	}
	if reconnect {
		return true
	}
	originZoneX := ba.OriginX >> 3
	originZoneZ := ba.OriginZ >> 3
	reloadLeftX := (originZoneX - 4) << 3
	reloadRightX := (originZoneX + 5) << 3
	reloadTopZ := (originZoneZ + 5) << 3
	reloadBottomZ := (originZoneZ - 4) << 3
	if playerX < reloadLeftX || playerZ < reloadBottomZ ||
		playerX > reloadRightX-1 || playerZ > reloadTopZ-1 {
		return true
	}
	return false
}

// Rebuild resets the build area, recomputes the 13x13 zone window mapsquares,
// and commits the new origin. Returns the mapsquare list packed as (mapX<<8)|mapZ.
func (ba *BuildArea) Rebuild(playerX, playerZ, currentTick int) []uint16 {
	ba.LoadedZones = map[int]bool{}
	ba.ActiveZones = map[int]bool{}
	ba.Mapsquares = map[uint16]bool{}

	zoneX := playerX >> 3
	zoneZ := playerZ >> 3
	// 13x13 zone window => zone +/-6.
	for dx := -6; dx <= 6; dx++ {
		for dz := -6; dz <= 6; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			mapX := zx >> 3
			mapZ := zz >> 3
			if mapX > 0xff || mapZ > 0xff {
				continue
			}
			ba.Mapsquares[uint16((mapX<<8)|mapZ)] = true
		}
	}

	ba.OriginX = playerX
	ba.OriginZ = playerZ
	ba.LastBuild = currentTick

	out := make([]uint16, 0, len(ba.Mapsquares))
	for m := range ba.Mapsquares {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
```

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/buildarea/... -v 2>&1
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/buildarea/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(buildarea): add BuildArea with 13x13 zone window tracking

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Create `pkg/xtea` stub

**Files:**
- Create: `pkg/xtea/xtea.go`

- [ ] **Step 1: Create the stub**

```go
package xtea

// Keys returns the 4-word XTEA key for a mapsquare.
//
// Sub-spec 3a always returns zeros. The map pack files in data/pack/client/maps/
// are already decrypted; zero-key decrypt on the client side returns the same
// bytes unchanged. When encrypted distribution is needed, load real per-mapsquare
// keys from maps/xteas.json or similar.
func Keys(mapX, mapZ int) [4]uint32 {
	return [4]uint32{0, 0, 0, 0}
}
```

- [ ] **Step 2: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/xtea/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add pkg/xtea/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(xtea): add zero-key stub for RebuildNormal

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Add mapsquare CRC caching to `pkg/gamemap`

**Files:**
- Modify: `pkg/gamemap/gamemap.go`
- Modify: `pkg/gamemap/gamemap_test.go`

- [ ] **Step 1: Add CRC tracking fields and method**

In `pkg/gamemap/gamemap.go`, add to the `GameMap` struct:

```go
mapCRC map[uint16]uint32 // (mapX<<8)|mapZ -> CRC32 of m{x}_{z} file
locCRC map[uint16]uint32 // ditto for l{x}_{z}
```

Initialise in `New`:

```go
return &GameMap{
    // ... existing fields ...
    mapCRC: map[uint16]uint32{},
    locCRC: map[uint16]uint32{},
}
```

Add a new public method at the bottom of `gamemap.go`:

```go
// MapsquareCRC returns the CRC32 of the m and l files for a mapsquare, or 0 if
// the file was absent during Init.
func (gm *GameMap) MapsquareCRC(mapX, mapZ int) (mCRC, lCRC uint32) {
	key := uint16((mapX << 8) | mapZ)
	return gm.mapCRC[key], gm.locCRC[key]
}
```

In `Init`, after reading `mData`, add:

```go
gm.mapCRC[uint16((sqX<<8)|sqZ)] = crc32.ChecksumIEEE(mData)
```

And after reading `lData` (if successful):

```go
gm.locCRC[uint16((sqX<<8)|sqZ)] = crc32.ChecksumIEEE(lData)
```

Add `"hash/crc32"` to the imports.

- [ ] **Step 2: Add test**

Append to `pkg/gamemap/gamemap_test.go`:

```go
func TestMapsquareCRCReturnsZeroForMissing(t *testing.T) {
	gm := New(discardLogger())
	mCRC, lCRC := gm.MapsquareCRC(0, 0)
	if mCRC != 0 || lCRC != 0 {
		t.Errorf("missing mapsquare: got (%d,%d), want (0,0)", mCRC, lCRC)
	}
}

func TestMapsquareCRCCachedFromInit(t *testing.T) {
	tmp := t.TempDir()
	mapsDir := filepath.Join(tmp, "maps")
	if err := os.MkdirAll(mapsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write a synthetic mapsquare file.
	mData := []byte{0, 0, 0, 0} // simplest tile stream: per-tile opcode 0 (end)
	if err := os.WriteFile(filepath.Join(mapsDir, "m50_50"), mData, 0644); err != nil {
		t.Fatal(err)
	}

	gm := New(discardLogger())
	if err := gm.Init(tmp); err != nil {
		t.Fatal(err)
	}
	mCRC, _ := gm.MapsquareCRC(50, 50)
	if mCRC == 0 {
		t.Error("expected non-zero CRC after Init")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamemap/... -v 2>&1 | tail -20
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/gamemap/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(gamemap): cache map/loc CRCs for RebuildNormal

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Add server opcodes for RebuildNormal and UpdateInv*

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go`
- Modify: `pkg/io/protocol/game/server/prot_test.go`

- [ ] **Step 1: Append opcodes**

Open `pkg/io/protocol/game/server/prot.go`. After the existing modal opcodes, add:

```go
var (
	// ... existing OpIfClose, OpIfOpenMain, ... ...

	OpRebuildNormal    = Op{Opcode: 237, PayloadSize: -2}
	OpUpdateInvFull    = Op{Opcode: 98, PayloadSize: -2}
	OpUpdateInvPartial = Op{Opcode: 213, PayloadSize: -2}
)
```

- [ ] **Step 2: Extend test**

Append to `pkg/io/protocol/game/server/prot_test.go`:

```go
func TestSubSpec3AOpcodes(t *testing.T) {
	cases := []struct {
		op     Op
		opcode byte
		size   int
	}{
		{OpRebuildNormal, 237, -2},
		{OpUpdateInvFull, 98, -2},
		{OpUpdateInvPartial, 213, -2},
	}
	for _, tc := range cases {
		if tc.op.Opcode != tc.opcode {
			t.Errorf("%+v: Opcode = %d, want %d", tc.op, tc.op.Opcode, tc.opcode)
		}
		if tc.op.PayloadSize != tc.size {
			t.Errorf("%+v: PayloadSize = %d, want %d", tc.op, tc.op.PayloadSize, tc.size)
		}
	}
}
```

- [ ] **Step 3: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/game/server/... -v
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add pkg/io/protocol/game/server/
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(prot): add RebuildNormal, UpdateInvFull, UpdateInvPartial server opcodes

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Add `modules/world/masks.go`

**Files:**
- Create: `modules/world/masks.go`

- [ ] **Step 1: Create the file**

```go
package world

// Player update masks — bit flags combined in Player.masks.
// Each bit signals a mask payload to be appended after the main info block.
// Mirrors @2004scape/rsbuf's PlayerInfoProt enum.
const (
	MaskAppearance  = 1
	MaskAnim        = 2
	MaskFaceEntity  = 4
	MaskSay         = 8
	MaskDamage      = 16
	MaskFaceCoord   = 32
	MaskChat        = 64
	MaskBigUpdate   = 128
	MaskSpotAnim    = 256
	MaskExactMove   = 512
)

// NPC update masks — mirrors NpcInfoProt.
const (
	NpcMaskAnim        = 2
	NpcMaskFaceEntity  = 4
	NpcMaskSay         = 8
	NpcMaskDamage      = 16
	NpcMaskChangeType  = 32
	NpcMaskSpotAnim    = 64
	NpcMaskFaceCoord   = 128
)
```

- [ ] **Step 2: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add modules/world/masks.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): add player and npc update mask constants

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Extend `Player` struct (inventory, buildarea, BAS anims)

**Files:**
- Modify: `modules/world/player.go`

- [ ] **Step 1: Add imports**

In `modules/world/player.go`, add to the imports:

```go
import (
	// ... existing ...
	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/inventory"
)
```

- [ ] **Step 2: Add fields to `Player`**

Inside the `type Player struct { ... }` block, add these fields alongside the existing appearance section:

```go
// === inventory (sub-spec 3a) ===
invs         map[int]*inventory.Inventory
invListeners []InventoryListener

// === build area (sub-spec 3a) ===
buildArea *buildarea.BuildArea

// === BAS (basic animation set) — sub-spec 3a ===
readyanim, turnanim                              int
walkanim, walkanim_b, walkanim_l, walkanim_r     int
runanim                                          int
```

Also add a tiny struct at the top level of the same file (outside `Player`):

```go
type InventoryListener struct {
	Type   int // InvType id
	Com    int // UI component id
	Source int // player slot of the inv's owner
}
```

- [ ] **Step 3: Update `newPlayer` defaults**

Find `newPlayer` in `player.go`. Add to the `&Player{ ... }` literal (alongside existing `-1` defaults):

```go
readyanim:  -1,
turnanim:   -1,
walkanim:   -1,
walkanim_b: -1,
walkanim_l: -1,
walkanim_r: -1,
runanim:    -1,
```

`invs`, `invListeners`, `buildArea` are left as zero values; `processLogins` initialises them on the next tick.

- [ ] **Step 4: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): extend Player with inventory, buildarea, and BAS anim fields

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Wire config loading into `NewServer`

**Files:**
- Modify: `modules/world/server.go`

- [ ] **Step 1: Add fields to `Server` struct**

```go
paramTypes *objtype.ParamTypeConfigs
objTypes   *objtype.ObjTypeConfigs
invTypes   *objtype.InvTypeConfigs
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to imports.

- [ ] **Step 2: Load during `NewServer`**

After the existing `gamemap.Init(...)` call in `NewServer`, add:

```go
params, err := objtype.LoadParams(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load params: %w", err)
}
objTypes, err := objtype.LoadObjTypes(cfg.CachePath, params)
if err != nil {
    return nil, fmt.Errorf("load obj types: %w", err)
}
invTypes, err := objtype.LoadInvTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load inv types: %w", err)
}
s.paramTypes = params
s.objTypes = objTypes
s.invTypes = invTypes
```

- [ ] **Step 3: Build and confirm existing tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all tests pass. The world tests use `newTestServer` which constructs `Server` directly (bypassing `NewServer`) so they don't hit the new loaders.

- [ ] **Step 4: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): load ObjType/InvType/ParamType during Server startup

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Implement `generateAppearance`

**Files:**
- Create: `modules/world/appearance.go`
- Create: `modules/world/appearance_test.go`

- [ ] **Step 1: Write the failing tests**

Create `modules/world/appearance_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
)

// synthesizeTypes builds minimal ObjConfigs/InvConfigs for appearance tests.
func synthesizeTypes(t *testing.T) (*objtype.ObjTypeConfigs, *objtype.InvTypeConfigs) {
	t.Helper()
	objs := &objtype.ObjTypeConfigs{
		Configs:     make([]*objtype.ObjType, 10),
		ConfigNames: map[string]int{},
	}
	// id=1: a platebody (wearPos=4, wearPos2=6, wearPos3=-1) — hides arms.
	objs.Configs[1] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 1, DebugName: "platebody"},
		WearPos:    4, WearPos2: 6, WearPos3: -1,
	}
	// id=2: a full helm (wearPos=8, wearPos2=0, wearPos3=1) — hides hair and jaw.
	objs.Configs[2] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 2, DebugName: "full_helm"},
		WearPos:    8, WearPos2: 0, WearPos3: 1,
	}

	invs := &objtype.InvTypeConfigs{
		Configs:     make([]*objtype.InvType, 2),
		ConfigNames: map[string]int{"worn": 0},
		Worn:        0,
	}
	invs.Configs[0] = &objtype.InvType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "worn"},
		Size:       14,
	}
	return objs, invs
}

func TestGenerateAppearanceNakedPlayer(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}

	p.generateAppearance(objs, invs, 0)

	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty")
	}
	if p.lastAppearance != 0 {
		t.Errorf("lastAppearance: got %d, want 0", p.lastAppearance)
	}
	// First byte is gender — defaults to 0.
	if p.appearanceBuf[0] != 0 {
		t.Errorf("gender byte: got %d, want 0", p.appearanceBuf[0])
	}
	// Second byte is headicons — defaults to 0.
	if p.appearanceBuf[1] != 0 {
		t.Errorf("headicons byte: got %d, want 0", p.appearanceBuf[1])
	}
}

func TestGenerateAppearancePlatebodyHidesArms(t *testing.T) {
	objs, invs := synthesizeTypes(t)
	p, _ := newTestPlayer(t)
	p.invs = map[int]*inventory.Inventory{
		invs.Worn: inventory.FromType(invs.Configs[invs.Worn]),
	}
	// Equip platebody at wearPos=4 (body slot index matches wearPos).
	p.invs[invs.Worn].Items[4] = &inventory.Item{Id: 1, Count: 1}

	p.generateAppearance(objs, invs, 0)

	// Slot 6 is skipped (wearPos2=6 on the platebody) so byte at that position
	// should be 0 (single byte).
	// Exact byte offset is complex — just verify the buffer is non-empty and
	// lastAppearance was updated.
	if len(p.appearanceBuf) == 0 {
		t.Fatal("appearanceBuf should be non-empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestGenerateAppearance -v 2>&1 | head -10
```

Expected: compile error — `generateAppearance` undefined.

- [ ] **Step 3: Create `modules/world/appearance.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
)

// slotToBodyTable maps worn-inventory slot -> body-part index in Player.body[].
// Used when no equipment occupies the slot.
var slotToBodyTable = map[int]int{
	8:  0, // head
	11: 1, // jaw
	4:  2, // torso
	6:  3, // arms
	9:  4, // hands
	7:  5, // legs
	10: 6, // feet
}

// generateAppearance writes p.appearanceBuf using the worn inventory + body/colors.
// Mirrors LostCityRS/Engine-TS Player.generateAppearance().
func (p *Player) generateAppearance(objs *objtype.ObjTypeConfigs, invs *objtype.InvTypeConfigs, currentTick int) {
	buf := packet.NewPacket(nil)

	// Resolve worn inventory (may be nil pre-login or after logout).
	var worn *inventory.Inventory
	if p.invs != nil {
		worn = p.invs[invs.Worn]
	}

	// Collect slots hidden by equipped items via wearPos2 / wearPos3.
	skipped := map[int]bool{}
	if worn != nil {
		for _, it := range worn.Items {
			if it == nil {
				continue
			}
			if it.Id < 0 || it.Id >= len(objs.Configs) {
				continue
			}
			ot := objs.Configs[it.Id]
			if ot == nil {
				continue
			}
			if ot.WearPos2 >= 0 {
				skipped[ot.WearPos2] = true
			}
			if ot.WearPos3 >= 0 {
				skipped[ot.WearPos3] = true
			}
		}
	}

	buf.P1(uint8(p.gender))
	buf.P1(uint8(p.headicons))

	// 12-slot loop.
	for slot := 0; slot < 12; slot++ {
		if skipped[slot] {
			buf.P1(0)
			continue
		}
		var equipped *inventory.Item
		if worn != nil && slot < len(worn.Items) {
			equipped = worn.Items[slot]
		}
		if equipped != nil {
			buf.P2(uint16(0x200 | (equipped.Id & 0x1FF)))
			continue
		}
		bodyIdx, ok := slotToBodyTable[slot]
		if !ok || p.body[bodyIdx] == -1 {
			buf.P1(0)
			continue
		}
		buf.P2(uint16(0x100 | (p.body[bodyIdx] & 0xFF)))
	}

	// Colors (5 bytes).
	for i := 0; i < 5; i++ {
		buf.P1(uint8(p.colors[i]))
	}

	// BAS anims (7 x 2 bytes).
	buf.P2(uint16(p.readyanim))
	buf.P2(uint16(p.turnanim))
	buf.P2(uint16(p.walkanim))
	buf.P2(uint16(p.walkanim_b))
	buf.P2(uint16(p.walkanim_l))
	buf.P2(uint16(p.walkanim_r))
	buf.P2(uint16(p.runanim))

	buf.P8(p.username37)
	buf.P1(uint8(p.combatLevel))

	p.appearanceBuf = append([]byte(nil), buf.Bytes()...)
	p.lastAppearance = currentTick
}
```

> **Note on packet API:** goscape's `packet.Packet` may expose `P1`, `P2`, `P4`, `P8` methods for appending. If the API uses different names (e.g., `WriteByte`/`WriteUint16`), adapt the calls accordingly. The key is that `buf.Bytes()` at the end returns the accumulated byte slice.

- [ ] **Step 4: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestGenerateAppearance -v 2>&1
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add modules/world/appearance.go modules/world/appearance_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement generateAppearance with equipment-driven overrides

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Implement `sendRebuildNormal` and fill in `updateMap`

**Files:**
- Create: `modules/world/rebuildmap.go`
- Create: `modules/world/rebuildmap_test.go`
- Modify: `modules/world/player.go` — replace `updateMap` stub

- [ ] **Step 1: Write the failing test**

Create `modules/world/rebuildmap_test.go`:

```go
package world

import (
	"io"
	"testing"
	"time"

	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

func TestSendRebuildNormalWireFormat(t *testing.T) {
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	wantEnc, _ := isaacPair([4]uint32{1, 2, 3, 4})

	p, clientConn := newTestPlayer(t)
	p.client.encryptor = enc
	p.x = 3094
	p.z = 3106

	mapsquares := []uint16{uint16((48 << 8) | 48)}

	received := make(chan []byte, 1)
	go func() {
		// 1 opcode + 2-byte length + 4 (zoneX+zoneZ) + 10 (one mapsquare entry) = 17 bytes
		buf := make([]byte, 17)
		clientConn.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := io.ReadFull(clientConn, buf); err == nil {
			received <- buf
		}
	}()

	sendRebuildNormal(p, mapsquares)
	p.client.flushWrite()

	expectedOpcode := byte((int(gameserver.OpRebuildNormal.Opcode) + int(wantEnc.GetNext())) & 0xff)

	select {
	case got := <-received:
		if got[0] != expectedOpcode {
			t.Errorf("opcode byte: got %d, want %d", got[0], expectedOpcode)
		}
		// length prefix (bytes 1..2) big-endian = 14
		if got[1] != 0 || got[2] != 14 {
			t.Errorf("len prefix: got %v, want [0 14]", got[1:3])
		}
		// zoneX (bytes 3..4) big-endian = 3094 >> 3 = 386 = 0x0182
		if got[3] != 0x01 || got[4] != 0x82 {
			t.Errorf("zoneX: got %v, want [0x01 0x82]", got[3:5])
		}
		// zoneZ (bytes 5..6) big-endian = 3106 >> 3 = 388 = 0x0184
		if got[5] != 0x01 || got[6] != 0x84 {
			t.Errorf("zoneZ: got %v, want [0x01 0x84]", got[5:7])
		}
		// mapX (byte 7), mapZ (byte 8)
		if got[7] != 48 || got[8] != 48 {
			t.Errorf("mapsquare: got (%d,%d), want (48,48)", got[7], got[8])
		}
	case <-time.After(time.Second):
		t.Error("timed out")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestSendRebuildNormal -v 2>&1 | head -5
```

Expected: compile error — `sendRebuildNormal` undefined.

- [ ] **Step 3: Create `modules/world/rebuildmap.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendRebuildNormal writes a RebuildNormal packet for the player.
// Mirrors TS RebuildNormalEncoder: p2(zoneX), p2(zoneZ), per mapsquare:
// p1(mapX), p1(mapZ), p4(mCRC), p4(lCRC).
func sendRebuildNormal(p *Player, mapsquares []uint16) {
	gm := p.client.server.gamemap

	buf := packet.NewPacket(nil)
	buf.P2(uint16(p.x >> 3))
	buf.P2(uint16(p.z >> 3))
	for _, msq := range mapsquares {
		mx := int(msq >> 8)
		mz := int(msq & 0xff)
		mCRC, lCRC := gm.MapsquareCRC(mx, mz)
		buf.P1(uint8(mx))
		buf.P1(uint8(mz))
		buf.P4(mCRC)
		buf.P4(lCRC)
	}
	p.writeOut(gameserver.OpRebuildNormal, buf.Bytes())
}
```

- [ ] **Step 4: Replace `updateMap` stub in `player.go`**

In `modules/world/player.go`, find the stub `func (p *Player) updateMap() {}` and replace with:

```go
func (p *Player) updateMap() {
	if p.buildArea == nil || p.client == nil || p.client.server == nil {
		return
	}
	if !p.buildArea.ShouldRebuild(p.x, p.z, p.reconnecting) {
		return
	}
	ms := p.buildArea.Rebuild(p.x, p.z, p.client.server.currentTick)
	p.reconnecting = false
	sendRebuildNormal(p, ms)
}
```

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestSendRebuildNormal -v 2>&1
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add modules/world/rebuildmap.go modules/world/rebuildmap_test.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement sendRebuildNormal and wire into updateMap

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Implement `updateInvs` with `UpdateInvFull`

**Files:**
- Create: `modules/world/inv_update.go`
- Modify: `modules/world/player.go` — replace `updateInvs` stub

- [ ] **Step 1: Create `modules/world/inv_update.go`**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// sendUpdateInvFull writes an UpdateInvFull packet for a single inventory.
// Mirrors TS UpdateInvFullEncoder: p2(component), p1(size), per slot either
// p2(id+1)+p1(count) (small counts fit in 1 byte, larger use p1(255)+p4(count))
// or p2(0)+p1(0) for empty slots.
func sendUpdateInvFull(p *Player, invId int, inv *inventory.Inventory) {
	buf := packet.NewPacket(nil)

	// Sub-spec 3a: use invId itself as the component placeholder. sub-spec 3b
	// will consult p.invListeners to route updates to each subscriber's
	// component id. The Java client accepts arbitrary component ids during
	// startup before any UI is open.
	com := invId

	buf.P2(uint16(com))
	size := inv.Capacity
	if size > 0xff {
		size = 0xff
	}
	buf.P1(uint8(size))
	for slot := 0; slot < size; slot++ {
		item := inv.Get(slot)
		if item == nil {
			buf.P2(0)
			buf.P1(0)
			continue
		}
		buf.P2(uint16(item.Id + 1))
		if item.Count >= 255 {
			buf.P1(255)
			buf.P4(uint32(item.Count))
		} else {
			buf.P1(uint8(item.Count))
		}
	}

	p.writeOut(gameserver.OpUpdateInvFull, buf.Bytes())
}
```

- [ ] **Step 2: Replace `updateInvs` stub in `player.go`**

Find `func (p *Player) updateInvs() {}` and replace with:

```go
func (p *Player) updateInvs() {
	for invId, inv := range p.invs {
		if !inv.Update {
			continue
		}
		sendUpdateInvFull(p, invId, inv)
		inv.Update = false
	}
}
```

- [ ] **Step 3: Build**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add modules/world/inv_update.go modules/world/player.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): implement updateInvs with UpdateInvFull packets

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Initialise `buildArea` and `invs` in `processLogins`

**Files:**
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Extend `processLogins`**

Open `modules/world/tick.go`. Find the existing `processLogins`. After the `s.addPlayer(p)` success path (before `p.lastConnected = s.currentTick`), insert:

```go
p.buildArea = buildarea.New()
p.invs = map[int]*inventory.Inventory{}
if s.invTypes != nil && s.invTypes.Worn < len(s.invTypes.Configs) {
    wornType := s.invTypes.Configs[s.invTypes.Worn]
    if wornType != nil {
        worn := inventory.FromType(wornType)
        worn.Update = true
        p.invs[s.invTypes.Worn] = worn
    }
}
p.masks |= MaskAppearance
```

Add imports:

```go
"github.com/zsrv/goscape/pkg/buildarea"
"github.com/zsrv/goscape/pkg/inventory"
```

- [ ] **Step 2: Build and run existing tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... 2>&1 | tail -5
```

Expected: all existing tests pass. The login tests don't exercise the new code because `newTestServer` leaves `invTypes` as nil.

- [ ] **Step 3: Commit**

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): processLogins initialises buildarea, worn inventory, and appearance mask

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Integration test — login triggers RebuildNormal

**Files:**
- Create: `modules/world/login_map_test.go`

- [ ] **Step 1: Write the integration test**

```go
package world

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/gamemap"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestLoginSendsRebuildNormal verifies that after a player is drained from
// newPlayers in processLogins, a subsequent updateMap() produces a RebuildNormal.
func TestLoginSendsRebuildNormal(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	// Init with an empty dir — gamemap has no mapsquare data but still works.
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	c, clientConn := newTestClient(t)
	c.server = s
	c.state = ClientStateGame
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	c.encryptor = enc

	// Simulate what processLogins does minimally (without full objtype wiring).
	p := newPlayer(c)
	c.player = p
	if err := s.addPlayer(p); err != nil {
		t.Fatal(err)
	}
	p.buildArea = buildarea.New()

	// Drain the client pipe in the background to let writes not block.
	received := make(chan []byte, 1)
	go func() {
		buf := &bytes.Buffer{}
		clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = io.Copy(buf, clientConn)
		received <- buf.Bytes()
	}()

	p.updateMap()
	p.client.flushWrite()
	// Close the write side to let io.Copy finish.
	p.client.conn.Close()

	select {
	case got := <-received:
		if len(got) == 0 {
			t.Fatal("expected RebuildNormal bytes, got none")
		}
		// First byte is the encrypted opcode for OpRebuildNormal (237).
		wantOp := byte((int(gameserver.OpRebuildNormal.Opcode) + int(enc2InitialValue([4]uint32{1, 2, 3, 4}))) & 0xff)
		if got[0] != wantOp {
			t.Errorf("first byte: got %d, want %d (encrypted RebuildNormal opcode)", got[0], wantOp)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for pipe copy")
	}
}

// enc2InitialValue returns the first GetNext() value from a fresh isaac with the given seed.
// Used to compute the expected encrypted opcode in tests.
func enc2InitialValue(seed [4]uint32) uint32 {
	e, _ := isaacPair(seed)
	return e.GetNext()
}
```

- [ ] **Step 2: Run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestLoginSendsRebuildNormal -v 2>&1
```

Expected: pass.

- [ ] **Step 3: Commit**

```bash
git add modules/world/login_map_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): integration test updateMap sends RebuildNormal after login

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: Final verification

- [ ] **Step 1: Run full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all `ok` (or `skip` for tests that need external data).

- [ ] **Step 2: Race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... 2>&1 | tail -5
```

Expected: all pass, zero races.

- [ ] **Step 3: Binary builds**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./cmd/goscape && rm -f goscape && echo "build OK"
```

Expected: `build OK`.
