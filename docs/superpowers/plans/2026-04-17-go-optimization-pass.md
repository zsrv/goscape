# Go Optimization Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate per-call heap allocations in the Packet I/O hot path, complete two `sync.Pool` TODOs for per-connection buffers, and add TCP keepalive support — applying patterns from the Go Optimization Guide.

**Architecture:** Three independent layers worked in order: (1) `pkg/io/packet` — reader/writer methods rewritten to use direct indexed access instead of `make`+`Read`; (2) `modules/world` — read buffer and `client.in` Packet use `sync.Pool`; (3) `modules/world` — TCP keepalive added to `Config` and applied per connection.

**Tech Stack:** Go 1.26 standard library (`sync`, `io`, `net`, `testing`). No new dependencies.

---

## File Map

| File | What changes |
|------|-------------|
| `pkg/io/packet/packet.go` | Rewrite `G1`/`G2`/`G3`/`G4`/`IG2`/`IG4` readers; rewrite `P1`/`P2`/`IP2`/`P3`/`P4`/`IP4`/`P8` writers; fix `GData`, `PJStr`; replace `packetPool` with `poolForCapacity`; fix `Alloc`/`Release` |
| `pkg/io/packet/packet_test.go` | Add zero-alloc tests and benchmarks for all changed methods |
| `modules/world/client.go` | Add `readBuf64kPool`; change `newClient` to use `packet.Alloc(65536)` |
| `modules/world/server.go` | Use pooled read buffer; add `c.in.Release()` to defer; add keepalive calls |
| `modules/world/server_test.go` | Add `BenchmarkClientSetup` |
| `modules/world/config.go` | Add `TCPKeepAlivePeriod` field and flag |
| `config.yaml` | Add `tcp_keepalive_period: 30s` |

---

## Task 1: Add zero-alloc tests and benchmarks for Packet reader methods

**Files:**
- Modify: `pkg/io/packet/packet_test.go`

These tests assert 0 allocations per call. If the current compiler eliminates stack allocations for a given method, the test may already pass — that's fine. In either case the benchmarks measure ns/op improvement, which is the primary signal.

- [ ] **Step 1: Append the following to `pkg/io/packet/packet_test.go`**

```go
// --- Zero-alloc tests (readers) ---

func TestG1NoAlloc(t *testing.T) {
	p := &Packet{Data: []byte{0x42}}
	allocs := testing.AllocsPerRun(1000, func() {
		p.Pos = 0
		_ = p.G1()
	})
	if allocs > 0 {
		t.Fatalf("G1: want 0 allocs/op, got %.1f", allocs)
	}
}

func TestG2NoAlloc(t *testing.T) {
	p := &Packet{Data: []byte{0x01, 0x02}}
	allocs := testing.AllocsPerRun(1000, func() {
		p.Pos = 0
		_ = p.G2()
	})
	if allocs > 0 {
		t.Fatalf("G2: want 0 allocs/op, got %.1f", allocs)
	}
}

func TestG4NoAlloc(t *testing.T) {
	p := &Packet{Data: []byte{0x01, 0x02, 0x03, 0x04}}
	allocs := testing.AllocsPerRun(1000, func() {
		p.Pos = 0
		_ = p.G4()
	})
	if allocs > 0 {
		t.Fatalf("G4: want 0 allocs/op, got %.1f", allocs)
	}
}

func TestGDataNoAlloc(t *testing.T) {
	p := &Packet{Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	dest := make([]byte, 8)
	allocs := testing.AllocsPerRun(1000, func() {
		p.Pos = 0
		p.GData(dest, 8)
	})
	if allocs > 0 {
		t.Fatalf("GData: want 0 allocs/op, got %.1f", allocs)
	}
}

// --- Benchmarks (readers) ---

func BenchmarkG1(b *testing.B) {
	p := &Packet{Data: []byte{0x42}}
	b.ReportAllocs()
	for range b.N {
		p.Pos = 0
		_ = p.G1()
	}
}

func BenchmarkG2(b *testing.B) {
	p := &Packet{Data: []byte{0x01, 0x02}}
	b.ReportAllocs()
	for range b.N {
		p.Pos = 0
		_ = p.G2()
	}
}

func BenchmarkG4(b *testing.B) {
	p := &Packet{Data: []byte{0x01, 0x02, 0x03, 0x04}}
	b.ReportAllocs()
	for range b.N {
		p.Pos = 0
		_ = p.G4()
	}
}

func BenchmarkGData(b *testing.B) {
	p := &Packet{Data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	dest := make([]byte, 8)
	b.ReportAllocs()
	for range b.N {
		p.Pos = 0
		p.GData(dest, 8)
	}
}
```

- [ ] **Step 2: Run the new tests and record baseline benchmark output**

```bash
go test ./pkg/io/packet/... -run 'TestG1NoAlloc|TestG2NoAlloc|TestG4NoAlloc|TestGDataNoAlloc' -v
go test ./pkg/io/packet/... -bench 'BenchmarkG1|BenchmarkG2|BenchmarkG4|BenchmarkGData' -benchmem
```

Note the `allocs/op` and `ns/op` values — these are the baseline to beat.

---

## Task 2: Fix Packet reader methods G1, G2, IG2, G3, G4, IG4

**Files:**
- Modify: `pkg/io/packet/packet.go`

Replace the `make([]byte, n)` + `p.Read(b)` pattern with direct indexed reads into `p.Data`. `G1B`, `G2S`, `GBool`, `GSmart`, `GSmartS`, and `G8` all call the methods being fixed and automatically benefit — no changes needed for them.

- [ ] **Step 3: Replace `G1` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) G1() uint8 {
	if p.Pos >= len(p.Data) {
		panic(io.EOF)
	}
	b := p.Data[p.Pos]
	p.Pos++
	return b
}
```

- [ ] **Step 4: Replace `G2` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) G2() uint16 {
	if p.Pos+2 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint16(p.Data[p.Pos])<<8 | uint16(p.Data[p.Pos+1])
	p.Pos += 2
	return v
}
```

- [ ] **Step 5: Replace `IG2` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) IG2() uint16 {
	if p.Pos+2 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint16(p.Data[p.Pos]) | uint16(p.Data[p.Pos+1])<<8
	p.Pos += 2
	return v
}
```

- [ ] **Step 6: Replace `G3` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) G3() uint32 {
	if p.Pos+3 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint32(p.Data[p.Pos])<<16 | uint32(p.Data[p.Pos+1])<<8 | uint32(p.Data[p.Pos+2])
	p.Pos += 3
	return v
}
```

- [ ] **Step 7: Replace `G4` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) G4() uint32 {
	if p.Pos+4 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint32(p.Data[p.Pos])<<24 | uint32(p.Data[p.Pos+1])<<16 |
		uint32(p.Data[p.Pos+2])<<8 | uint32(p.Data[p.Pos+3])
	p.Pos += 4
	return v
}
```

- [ ] **Step 8: Replace `IG4` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) IG4() uint32 {
	if p.Pos+4 > len(p.Data) {
		panic(io.EOF)
	}
	v := uint32(p.Data[p.Pos+3])<<24 | uint32(p.Data[p.Pos+2])<<16 |
		uint32(p.Data[p.Pos+1])<<8 | uint32(p.Data[p.Pos])
	p.Pos += 4
	return v
}
```

- [ ] **Step 9: Replace `GData` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) GData(dest []byte, length int) {
	copy(dest, p.Data[p.Pos:p.Pos+length])
	p.Pos += length
}
```

- [ ] **Step 10: Run all Packet tests and the reader benchmarks**

```bash
go test ./pkg/io/packet/... -v
go test ./pkg/io/packet/... -bench 'BenchmarkG1|BenchmarkG2|BenchmarkG4|BenchmarkGData' -benchmem
```

Expected: all existing tests pass; `allocs/op` drops to 0 for G1/G2/G4; `ns/op` is lower than baseline. The zero-alloc tests (`TestG1NoAlloc` etc.) must pass.

---

## Task 3: Add zero-alloc tests and benchmarks for Packet writer methods

**Files:**
- Modify: `pkg/io/packet/packet_test.go`

- [ ] **Step 11: Append the following to `pkg/io/packet/packet_test.go`**

```go
// --- Zero-alloc tests (writers) ---

func TestP1NoAlloc(t *testing.T) {
	p := NewPacket(make([]byte, 0, 64))
	allocs := testing.AllocsPerRun(1000, func() {
		p.Reset()
		p.P1(0x42)
	})
	if allocs > 0 {
		t.Fatalf("P1: want 0 allocs/op, got %.1f", allocs)
	}
}

func TestP4NoAlloc(t *testing.T) {
	p := NewPacket(make([]byte, 0, 64))
	allocs := testing.AllocsPerRun(1000, func() {
		p.Reset()
		p.P4(0xDEADBEEF)
	})
	if allocs > 0 {
		t.Fatalf("P4: want 0 allocs/op, got %.1f", allocs)
	}
}

func TestPJStrNoAlloc(t *testing.T) {
	p := NewPacket(make([]byte, 0, 64))
	allocs := testing.AllocsPerRun(1000, func() {
		p.Reset()
		p.PJStr("hello", 0)
	})
	if allocs > 0 {
		t.Fatalf("PJStr: want 0 allocs/op, got %.1f", allocs)
	}
}

// --- Benchmarks (writers) ---

func BenchmarkP1(b *testing.B) {
	p := NewPacket(make([]byte, 0, 64))
	b.ReportAllocs()
	for range b.N {
		p.Reset()
		p.P1(0x42)
	}
}

func BenchmarkP4(b *testing.B) {
	p := NewPacket(make([]byte, 0, 64))
	b.ReportAllocs()
	for range b.N {
		p.Reset()
		p.P4(0xDEADBEEF)
	}
}

func BenchmarkPJStr(b *testing.B) {
	p := NewPacket(make([]byte, 0, 64))
	b.ReportAllocs()
	for range b.N {
		p.Reset()
		p.PJStr("username", 0)
	}
}
```

- [ ] **Step 12: Record baseline writer benchmark output**

```bash
go test ./pkg/io/packet/... -run 'TestP1NoAlloc|TestP4NoAlloc|TestPJStrNoAlloc' -v
go test ./pkg/io/packet/... -bench 'BenchmarkP1|BenchmarkP4|BenchmarkPJStr' -benchmem
```

---

## Task 4: Fix Packet writer methods P1, P2, IP2, P3, P4, IP4, P8 and PJStr

**Files:**
- Modify: `pkg/io/packet/packet.go`

`P1` uses `WriteByte` (already zero-alloc for single bytes). `P2`–`P8` use `tryGrowByReslice` to grow the buffer once and write bytes directly by index, avoiding the `[]byte{...}` slice literal allocation in the current `p.Write([]byte{...})` calls. `PBool` already uses `WriteByte` — no change needed.

- [ ] **Step 13: Replace `P1` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) P1(value uint8) {
	if err := p.WriteByte(value); err != nil {
		panic(err)
	}
}
```

- [ ] **Step 14: Replace `P2` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) P2(value uint16) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(2)
	if !ok {
		i = p.grow(2)
	}
	p.Data[i] = uint8(value >> 8)
	p.Data[i+1] = uint8(value)
}
```

- [ ] **Step 15: Replace `IP2` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) IP2(value uint16) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(2)
	if !ok {
		i = p.grow(2)
	}
	p.Data[i] = uint8(value)
	p.Data[i+1] = uint8(value >> 8)
}
```

- [ ] **Step 16: Replace `P3` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) P3(value uint32) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(3)
	if !ok {
		i = p.grow(3)
	}
	p.Data[i] = uint8(value >> 16)
	p.Data[i+1] = uint8(value >> 8)
	p.Data[i+2] = uint8(value)
}
```

- [ ] **Step 17: Replace `P4` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) P4(value uint32) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(4)
	if !ok {
		i = p.grow(4)
	}
	p.Data[i] = uint8(value >> 24)
	p.Data[i+1] = uint8(value >> 16)
	p.Data[i+2] = uint8(value >> 8)
	p.Data[i+3] = uint8(value)
}
```

- [ ] **Step 18: Replace `IP4` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) IP4(value uint32) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(4)
	if !ok {
		i = p.grow(4)
	}
	p.Data[i] = uint8(value)
	p.Data[i+1] = uint8(value >> 8)
	p.Data[i+2] = uint8(value >> 16)
	p.Data[i+3] = uint8(value >> 24)
}
```

- [ ] **Step 19: Replace `P8` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) P8(value uint64) {
	p.lastRead = opInvalid
	i, ok := p.tryGrowByReslice(8)
	if !ok {
		i = p.grow(8)
	}
	p.Data[i] = uint8(value >> 56)
	p.Data[i+1] = uint8(value >> 48)
	p.Data[i+2] = uint8(value >> 40)
	p.Data[i+3] = uint8(value >> 32)
	p.Data[i+4] = uint8(value >> 24)
	p.Data[i+5] = uint8(value >> 16)
	p.Data[i+6] = uint8(value >> 8)
	p.Data[i+7] = uint8(value)
}
```

- [ ] **Step 20: Replace `PJStr` in `pkg/io/packet/packet.go`**

```go
func (p *Packet) PJStr(str string, terminator byte) {
	p.WriteString(str)
	if err := p.WriteByte(terminator); err != nil {
		panic(err)
	}
}
```

- [ ] **Step 21: Run all Packet tests and writer benchmarks**

```bash
go test ./pkg/io/packet/... -v
go test ./pkg/io/packet/... -bench 'BenchmarkP1|BenchmarkP4|BenchmarkPJStr' -benchmem
```

Expected: all existing tests pass; `allocs/op` is 0; `ns/op` is lower than baseline recorded in Step 12.

---

## Task 5: Commit Section 1 — Packet I/O hot-path

- [ ] **Step 22: Stage and commit**

```bash
git add pkg/io/packet/packet.go pkg/io/packet/packet_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
perf: eliminate per-call allocations in Packet I/O hot path

Replace make+Read pattern in G1/G2/G3/G4/IG2/IG4 with direct indexed
reads into p.Data. Replace p.Write([]byte{...}) in P1-P8 with WriteByte
and tryGrowByReslice. Replace GData byte loop with copy, PJStr char loop
with WriteString+WriteByte. All methods now 0 allocs/op.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Fix `Alloc` and `Release` in packet.go

**Files:**
- Modify: `pkg/io/packet/packet.go`
- Modify: `pkg/io/packet/packet_test.go`

The current `packetPool(typ int)` takes a type index (0–5) but `Release()` passes `p.Unused()` (a raw capacity like 65536), so released packets are never returned to pools. `Alloc(typ)`'s fallback also creates a packet with cap=`typ` (e.g., 3 bytes) instead of the intended size. Replace `packetPool` with `poolForCapacity` that takes an actual byte count.

- [ ] **Step 23: Write a failing test for `Alloc`/`Release` roundtrip**

Append to `pkg/io/packet/packet_test.go`:

```go
func TestAllocRelease(t *testing.T) {
	p := Alloc(65536)
	if cap(p.Data) < 65536 {
		t.Fatalf("Alloc(65536): want cap >= 65536, got %d", cap(p.Data))
	}
	if p.Len() != 0 {
		t.Fatalf("Alloc(65536): want Len 0, got %d", p.Len())
	}
	p.P4(0xDEADBEEF)
	p.Release()

	p2 := Alloc(65536)
	if p2.Len() != 0 {
		t.Fatalf("after Release+Alloc: want Len 0, got %d", p2.Len())
	}
	p2.Release()
}
```

- [ ] **Step 24: Run the test to see it fail**

```bash
go test ./pkg/io/packet/... -run TestAllocRelease -v
```

Expected: FAIL — `Alloc(65536)` returns a 3-byte packet (current fallback bug), test panics or fails cap check.

- [ ] **Step 25: Replace `packetPool` with `poolForCapacity` and fix `Alloc`/`Release`**

In `pkg/io/packet/packet.go`, replace the `packetPool` function and update `Alloc` and `Release`:

```go
// poolForCapacity returns the pool whose capacity tier covers size,
// or nil if size exceeds all tiers.
func poolForCapacity(size int) *sync.Pool {
	switch {
	case size <= 100:
		return &packet100Pool
	case size <= 5_000:
		return &packet5000Pool
	case size <= 30_000:
		return &packet30000Pool
	case size <= 100_000:
		return &packet100000Pool
	case size <= 500_000:
		return &packet500000Pool
	case size <= 2_000_000:
		return &packet2000000Pool
	}
	return nil
}

// Alloc returns a reset Packet from the pool tier that covers size.
// If the pool is empty or size exceeds all tiers, a new Packet is allocated.
func Alloc(size int) *Packet {
	pool := poolForCapacity(size)
	if pool != nil {
		if v := pool.Get(); v != nil {
			p := v.(*Packet)
			p.Reset()
			return p
		}
	}
	return NewPacket(make([]byte, 0, size))
}

// Release resets the Packet and returns it to the appropriate pool tier.
func (p *Packet) Release() {
	p.Reset()
	if pool := poolForCapacity(cap(p.Data)); pool != nil {
		pool.Put(p)
	}
}
```

Delete the old `packetPool` function entirely.

- [ ] **Step 26: Run the test to see it pass**

```bash
go test ./pkg/io/packet/... -run TestAllocRelease -v
go test ./pkg/io/packet/... -v
```

Expected: `TestAllocRelease` passes; all other packet tests still pass.

---

## Task 7: Pool the per-connection 64KB read buffer

> **Depends on Task 6.** `c.in.Release()` added in this task relies on the fixed `Release()` from Task 6 to actually return packets to the pool.

**Files:**
- Modify: `modules/world/client.go`
- Modify: `modules/world/server.go`

The `handleTCPConn` method allocates `make([]byte, 64<<10)` fresh for every connection. A `sync.Pool` reuses these across connections.

- [ ] **Step 27: Add `readBuf64kPool` to `modules/world/client.go`**

Append the following after the existing `bufioWriter64kPool` block (after line 130):

```go
var readBuf64kPool = sync.Pool{
	New: func() any { return make([]byte, 64<<10) },
}

func getReadBuf64k() []byte {
	return readBuf64kPool.Get().([]byte)
}

func putReadBuf64k(b []byte) {
	readBuf64kPool.Put(b)
}
```

- [ ] **Step 28: Replace the `make` call and add pool release in `modules/world/server.go`**

In `handleTCPConn`, the current code (around line 167) is:

```go
defer func() {
    if err := c.flushWrite(); err != nil {
        s.log.Warn("failed to flush on connection close", "error", err, "remote_addr", conn.RemoteAddr())
    }
    putBufioReader64k(c.bufr)
    putBufioWriter64k(c.bufw)
    conn.Close()
    s.log.Info("connection closed", "remote_addr", conn.RemoteAddr())
}()
```

And later (around line 189):

```go
buf := make([]byte, 64<<10) // TODO: sync.Pool, release after writing to c.in
```

Make two changes:

**Change 1** — add `c.in.Release()` and acquire buf from pool. Replace the defer block:

```go
defer func() {
    if err := c.flushWrite(); err != nil {
        s.log.Warn("failed to flush on connection close", "error", err, "remote_addr", conn.RemoteAddr())
    }
    c.in.Release()
    putBufioReader64k(c.bufr)
    putBufioWriter64k(c.bufw)
    conn.Close()
    s.log.Info("connection closed", "remote_addr", conn.RemoteAddr())
}()
```

**Change 2** — replace the `make` line:

```go
buf := getReadBuf64k()
defer putReadBuf64k(buf)
```

(Remove the `// TODO: sync.Pool` comment.)

- [ ] **Step 29: Run world tests**

```bash
go test ./modules/world/... -v
```

Expected: all tests pass (server_test.go should still compile and pass).

---

## Task 8: Pool `client.in` and add connection-setup benchmark

**Files:**
- Modify: `modules/world/client.go`
- Modify: `modules/world/server_test.go`

- [ ] **Step 30: Change `newClient` to use `packet.Alloc`**

In `modules/world/client.go`, `newClient` currently has:

```go
in: packet.NewPacket(make([]byte, 0, 64<<10)), // TODO: sync.Pool
```

Replace with:

```go
in: packet.Alloc(65536),
```

(Remove the `// TODO: sync.Pool` comment. `65536` is `64<<10` — the smallest pool tier that covers `maxClientInBufSize=65535` is the 100K pool.)

- [ ] **Step 31: Add `BenchmarkClientSetup` to `modules/world/server_test.go`**

Open `modules/world/server_test.go` and append (or add if the file is empty):

```go
package world

import (
	"log/slog"
	"testing"
	"time"
)

func BenchmarkClientSetup(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		c := newClient(nil, 30*time.Second, slog.Default())
		c.in.Release()
		putBufioReader64k(c.bufr)
		putBufioWriter64k(c.bufw)
	}
}
```

If `server_test.go` already has a `package world` declaration, omit the duplicate package line and import block — only append the benchmark function.

- [ ] **Step 32: Run all world tests and the new benchmark**

```bash
go test ./modules/world/... -v
go test ./modules/world/... -bench BenchmarkClientSetup -benchmem
```

Expected: all tests pass; benchmark shows fewer allocs/op than a naive `newClient` without pooling.

- [ ] **Step 33: Commit Section 2 — sync.Pool completions**

```bash
git add pkg/io/packet/packet.go pkg/io/packet/packet_test.go \
        modules/world/client.go modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
perf: complete sync.Pool TODOs for per-connection buffers

Fix Alloc/Release to use poolForCapacity (capacity-based lookup) instead
of broken type-index lookup. Pool the 64KB read buffer and client.in
Packet per connection. Add BenchmarkClientSetup.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Run fieldalignment analysis

**Files:** No changes expected.

The Go Optimization Guide recommends verifying struct field order with the `fieldalignment` tool. This task confirms layouts are optimal or identifies any fields to reorder.

- [ ] **Step 34: Install and run the fieldalignment tool**

```bash
go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
fieldalignment ./...
```

- [ ] **Step 35: Evaluate output**

If the tool reports no structs to reorder, proceed to Task 10.

If the tool suggests changes (e.g., for `Packet`, `client`, or `Isaac`), reorder the reported fields from largest to smallest alignment, run `go test ./...` to confirm no breakage, then commit:

```bash
git add <changed files>
git commit --no-gpg-sign -m "perf: reorder struct fields for optimal alignment

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 10: Add TCP keepalive to Config and handleTCPConn

**Files:**
- Modify: `modules/world/config.go`
- Modify: `modules/world/server.go`
- Modify: `config.yaml`

TCP keepalives detect dead peers (client crash, network drop) without waiting for the application-level read timeout to fire.

- [ ] **Step 36: Add `TCPKeepAlivePeriod` to `Config` in `modules/world/config.go`**

Add the field to the `Config` struct after the existing timeout fields (after `TCPServerIdleTimeout`):

```go
TCPKeepAlivePeriod time.Duration `yaml:"tcp_keepalive_period"`
```

Add the flag registration to `RegisterFlagsAndApplyDefaults` after the existing timeout flag registrations:

```go
f.DurationVar(&c.TCPKeepAlivePeriod, "world.tcp-keepalive-period", 30*time.Second,
    "TCP keepalive idle period before first probe; set to 0 to disable")
```

- [ ] **Step 37: Apply keepalive in `handleTCPConn` in `modules/world/server.go`**

Extend the existing `if tcpConn, ok := conn.(*net.TCPConn)` block (currently only sets `SetNoDelay`):

```go
if tcpConn, ok := conn.(*net.TCPConn); ok {
    if err := tcpConn.SetNoDelay(true); err != nil {
        s.log.Warn("failed to set TCP_NODELAY", "error", err)
    }
    if s.cfg.TCPKeepAlivePeriod > 0 {
        if err := tcpConn.SetKeepAlive(true); err != nil {
            s.log.Warn("failed to enable TCP keepalive", "error", err)
        } else if err := tcpConn.SetKeepAlivePeriod(s.cfg.TCPKeepAlivePeriod); err != nil {
            s.log.Warn("failed to set TCP keepalive period", "error", err)
        }
    }
}
```

- [ ] **Step 38: Add the default to `config.yaml`**

Under the `world:` section, add:

```yaml
world:
  enable: true
  tcp_keepalive_period: 30s
```

- [ ] **Step 39: Run all tests to confirm nothing broken**

```bash
go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 40: Commit Section 4 — TCP keepalive**

```bash
git add modules/world/config.go modules/world/server.go config.yaml
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat: add TCP keepalive support to world server

Add TCPKeepAlivePeriod to Config (default 30s, flag world.tcp-keepalive-period).
Enable SetKeepAlive + SetKeepAlivePeriod per connection alongside existing
TCP_NODELAY. Set to 0 to disable. Update config.yaml default.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Summary

| Task | Files | Outcome |
|------|-------|---------|
| 1–5  | `pkg/io/packet/packet.go`, `packet_test.go` | G1–G4/P1–P8/GData/PJStr: 0 allocs/op; lower ns/op |
| 6    | `pkg/io/packet/packet.go`, `packet_test.go` | `Alloc`/`Release` work correctly for all capacity tiers |
| 7–8  | `modules/world/client.go`, `server.go`, `server_test.go` | Per-connection buffers reused via pool |
| 9    | (analysis only) | Struct layouts confirmed optimal |
| 10   | `modules/world/config.go`, `server.go`, `config.yaml` | Keepalives detect dead peers, tunable via config |
