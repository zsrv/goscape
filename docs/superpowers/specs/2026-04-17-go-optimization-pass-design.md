# Go Optimization Pass — Design Spec

**Date:** 2026-04-17  
**Source guide:** `$HOME/Code/github.com/astavonin/go-optimization-guide/docs`  
**Approach:** Option A — Packet I/O first, then networking; benchmarks alongside each change.

---

## Goals

1. Eliminate per-call heap allocations in the `Packet` read/write hot path.
2. Complete the two explicit `// TODO: sync.Pool` items in the connection handler.
3. Confirm struct field alignment is optimal via static analysis.
4. Add TCP keepalive detection for dead peers.

---

## Section 1 — Packet I/O hot-path: eliminate per-call allocations

**File:** `pkg/io/packet/packet.go`

### Problem

Every `G1`, `G2`, `G3`, `G4`, `IG2`, `IG4`, `G8` call allocates a temporary `[]byte` via `make`, routes through the `Packet.Read` method, then bit-shifts the result out of the temp slice. In a game loop these are called hundreds of times per tick per player. Each call = one heap allocation + one `copy()` + one GC root.

The writer methods `P1`–`P8` similarly do `p.Write([]byte{...})`, allocating a slice literal on every call.

`GData` loops byte-by-byte. `PJStr` writes one character at a time.

### Fix

Replace all reader methods with direct indexed access into `p.Data`:

```go
// Before
func (p *Packet) G2() uint16 {
    b := make([]byte, 2)
    _, err := p.Read(b)
    if err != nil { panic(err) }
    return uint16(b[0])<<8 | uint16(b[1])
}

// After
func (p *Packet) G2() uint16 {
    if p.Pos+2 > len(p.Data) {
        panic(io.EOF)
    }
    v := uint16(p.Data[p.Pos])<<8 | uint16(p.Data[p.Pos+1])
    p.Pos += 2
    return v
}
```

Apply the same pattern to: `G1`, `G1B`, `G3`, `G4`, `IG2`, `IG4`, `G8`, `GBool`, `GSmart`, `GSmartS`.

Replace writer methods to use `WriteByte` / `WriteString` / direct `tryGrowByReslice` instead of `p.Write([]byte{...})`:

```go
// Before
func (p *Packet) P1(value uint8) {
    _, err := p.Write([]byte{value})
    if err != nil { panic(err) }
}

// After
func (p *Packet) P1(value uint8) {
    if err := p.WriteByte(value); err != nil {
        panic(err)
    }
}
```

Apply `WriteByte` for `P1`. For multi-byte writers (`P2`, `IP2`, `P3`, `P4`, `IP4`, `P8`), use a single `tryGrowByReslice` call to grow the buffer once, then write bytes directly by index — avoiding both the slice literal allocation and multiple separate `WriteByte` calls:

```go
func (p *Packet) P4(value uint32) {
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

`PBool` is already zero-alloc (uses `WriteByte` internally) — no change needed.

Replace `GData` with `copy`:

```go
func (p *Packet) GData(dest []byte, length int) {
    copy(dest, p.Data[p.Pos:p.Pos+length])
    p.Pos += length
}
```

Replace `PJStr` loop with bulk write:

```go
func (p *Packet) PJStr(str string, terminator byte) {
    p.WriteString(str)
    p.WriteByte(terminator)
}
```

### Benchmarks added

In `pkg/io/packet/packet_test.go`:

- `BenchmarkG1` — 0 allocs/op after fix
- `BenchmarkG2` — 0 allocs/op after fix
- `BenchmarkG4` — 0 allocs/op after fix
- `BenchmarkGData` — verify `copy` vs byte loop
- `BenchmarkPJStr` — verify bulk write vs char loop

---

## Section 2 — Complete `sync.Pool` TODOs

### 2a. Per-connection read buffer

**File:** `modules/world/server.go:189`

```go
buf := make([]byte, 64<<10) // TODO: sync.Pool, release after writing to c.in
```

Add to `modules/world/client.go` alongside existing bufio pools:

```go
var readBuf64kPool = sync.Pool{
    New: func() any { return make([]byte, 64<<10) },
}
```

In `handleTCPConn`: acquire before the read loop, release in the `defer` block.

### 2b. `client.in` incoming Packet

**File:** `modules/world/client.go:61`

```go
in: packet.NewPacket(make([]byte, 0, 64<<10)), // TODO: sync.Pool
```

Replace `packet.NewPacket(make([]byte, 0, 64<<10))` with `packet.Alloc(3)` (the 100K pool, smallest size ≥ `maxClientInBufSize=65535`).

In the `defer` inside `handleTCPConn`, add `c.in.Release()` to return the packet to the pool.

### Benchmarks added

In `modules/world/` test file:

- `BenchmarkClientSetup` — allocation count with and without pooling.

---

## Section 3 — Struct field alignment

**Tool:** `go vet` with `golang.org/x/tools/go/analysis/passes/fieldalignment`

Run `fieldalignment ./...` across the codebase. Based on static analysis of the key structs:

- `Packet` (`pkg/io/packet/buffer.go`): 48 bytes. `lastRead int8` at the end causes 7 bytes padding, but no reordering eliminates this — size is already optimal for this field set.
- `client` (`modules/world/client.go`): all fields are pointer-sized or larger, no gaps.
- `Isaac` (`pkg/io/isaac/isaac.go`): `int32` + `[256]uint32` arrays, both 4-byte aligned, no gaps.

Expected outcome: no field reordering required. This step establishes the `fieldalignment` check as a verified baseline. If the tool surfaces anything unexpected, those fields are reordered.

No benchmarks needed — field reordering has no observable behavioural effect.

---

## Section 4 — TCP keepalives

**File:** `modules/world/server.go` (`handleTCPConn`), `modules/world/config.go`

### Problem

Dead peers (client crash, network drop) are currently detected only when the next read deadline fires. TCP keepalives give the OS an independent, faster detection path with no application-layer cost.

### Fix

Extend the existing `SetNoDelay` block:

```go
if tcpConn, ok := conn.(*net.TCPConn); ok {
    if err := tcpConn.SetNoDelay(true); err != nil {
        s.log.Warn("failed to set TCP_NODELAY", "error", err)
    }
    if err := tcpConn.SetKeepAlive(true); err != nil {
        s.log.Warn("failed to set TCP keepalive", "error", err)
    }
    if err := tcpConn.SetKeepAlivePeriod(s.cfg.TCPKeepAlivePeriod); err != nil {
        s.log.Warn("failed to set TCP keepalive period", "error", err)
    }
}
```

Add to `Config`:

```go
TCPKeepAlivePeriod time.Duration `yaml:"tcp_keepalive_period"`
```

Default: `30s`. Tunable via `config.yaml`:

```yaml
world:
  tcp_keepalive_period: 30s
```

**Scope:** `SetKeepAlivePeriod` controls `TCP_KEEPIDLE` only. Probe interval (`TCP_KEEPINTVL`) and probe count (`TCP_KEEPCNT`) require raw syscalls and are deferred to a future profile-guided pass.

**Socket buffer sizing** is explicitly excluded — the guide warns against guessing; `bufio.Reader/Writer` at 64 KB already smooths bursts, and correct sizing requires load testing with real player traffic.

No benchmarks for this section — keepalive behaviour is verified by integration testing.

---

## Out of Scope

- Socket buffer sizing (`SO_RCVBUF`/`SO_SNDBUF`) — deferred pending load testing.
- `TCP_KEEPINTVL` / `TCP_KEEPCNT` tuning — deferred pending profiling.
- `SO_REUSEPORT` — deferred; single-process model is sufficient for current scale.
- `pprof` endpoint — useful follow-up but not part of this pass.

---

## File Change Summary

| File | Changes |
|------|---------|
| `pkg/io/packet/packet.go` | Rewrite `G1`–`G8`, `P1`–`P8`, `GData`, `PJStr` to zero-alloc |
| `pkg/io/packet/packet_test.go` | Add benchmarks for reader/writer methods |
| `modules/world/client.go` | Add `readBuf64kPool`; use `packet.Alloc(3)` for `client.in` |
| `modules/world/server.go` | Use pooled read buf; add keepalive calls; release `c.in` on close |
| `modules/world/config.go` | Add `TCPKeepAlivePeriod` field |
| `config.yaml` | Add `tcp_keepalive_period: 30s` |
