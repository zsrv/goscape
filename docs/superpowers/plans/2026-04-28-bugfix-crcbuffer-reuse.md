# Compressed Spec+Plan: CrcBuffer empty on second /crc request

**Date:** 2026-04-28  
**Scope:** ~50 LOC across 3 files + 1 new test file  
**Tech Stack:** Go 1.26, modules/asset, pkg/cache

---

## Root Cause

`handler.go:24` passes `cache.CrcBuffer` (a `*packet.Packet`) directly to `io.Copy`:

```go
io.Copy(w, cache.CrcBuffer)
```

`Packet` implements `io.Reader` via `buffer.go:Read()`. That method advances `Pos`
and, when the buffer is empty, calls `Reset()` — which sets `Data = Data[:0]`.
After request 1, `Data` has length 0 and all subsequent requests return 0 bytes.

**TS reference (`web.ts:71`):**
```typescript
return new Response(Buffer.from(CrcBuffer.data));
```
TS snapshots the raw `Uint8Array` per request — no stateful reader, no mutation.

---

## Fix

### T1 — `pkg/cache/crctable.go`

Add `CrcBytes []byte` variable; populate at end of `MakeCRCs()`; clear in `ResetCRCState()`.

```go
var (
    CrcBuffer   = packet.NewPacket(make([]byte, 0, 4*9))
    CrcBytes    []byte   // ← add
    CrcTable    []uint32
    CrcBuffer32 uint32
)
```

`ResetCRCState` addition (after `CrcBuffer = ...` line):
```go
CrcBytes = nil
```

End of `MakeCRCs()` (after `CrcBuffer32 = ...` line):
```go
CrcBytes = append([]byte(nil), CrcBuffer.Bytes()...)
```

### T2 — `modules/asset/handler.go`

Replace line 24 and remove the now-unused `"io"` import.

```go
// old:
io.Copy(w, cache.CrcBuffer)

// new:
w.Write(cache.CrcBytes)
```

Remove `"io"` from the import block.

### T3 — `pkg/cache/crctable_test.go`

Extend `TestResetCRCStateRestoresInitialState` — add after existing assertions:
```go
if CrcBytes != nil {
    t.Errorf("CrcBytes = %v, want nil", CrcBytes)
}
```

Add new test:
```go
// TestMakeCRCsPopulatesCrcBytes pins that MakeCRCs snapshots the CRC
// payload into CrcBytes so HTTP handlers can serve it without a stateful reader.
func TestMakeCRCsPopulatesCrcBytes(t *testing.T) {
    ResetCRCState()
    t.Cleanup(ResetCRCState)

    MakeCRCs() // missing files are silently skipped; at least P4(0) is written

    if CrcBytes == nil {
        t.Fatal("CrcBytes is nil after MakeCRCs")
    }
    if len(CrcBytes) < 4 {
        t.Errorf("CrcBytes len = %d, want >= 4", len(CrcBytes))
    }
    // Must not alias CrcBuffer.Data — mutation of one must not affect the other.
    if len(CrcBytes) > 0 && len(CrcBuffer.Data) > 0 &&
        &CrcBytes[0] == &CrcBuffer.Data[0] {
        t.Error("CrcBytes aliases CrcBuffer.Data; must be an independent copy")
    }
}
```

### T4 — `modules/asset/handler_test.go` (new file)

```go
package asset

import (
    "bytes"
    "io"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/zsrv/goscape/pkg/cache"
)

// TestRootHandlerCrcEndpointServesOnEveryRequest pins the fix for the
// CrcBuffer-as-stateful-reader bug: both the first and second /crc request
// must return the full CrcBytes payload, not an empty body.
func TestRootHandlerCrcEndpointServesOnEveryRequest(t *testing.T) {
    prev := cache.CrcBytes
    t.Cleanup(func() { cache.CrcBytes = prev })
    cache.CrcBytes = []byte{0x00, 0x00, 0x00, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}

    a := &Asset{log: discardLogger()}

    for i := range 2 {
        req := httptest.NewRequest(http.MethodGet, "/crc", nil)
        rr := httptest.NewRecorder()
        a.RootHandler(rr, req)

        if rr.Code != http.StatusOK {
            t.Fatalf("request %d: status = %d, want 200", i+1, rr.Code)
        }
        got, _ := io.ReadAll(rr.Body)
        if !bytes.Equal(got, cache.CrcBytes) {
            t.Fatalf("request %d: body = %v, want %v", i+1, got, cache.CrcBytes)
        }
    }
}
```

Add helper (same file):
```go
import "log/slog"

func discardLogger() *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}
```

---

## Verification

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/cache/... -run "TestResetCRCState|TestMakeCRCs" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/asset/... -run "TestRootHandlerCrc" -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/cache/... ./modules/asset/...
```

## Deviations

None — mirrors TS `Buffer.from(CrcBuffer.data)` snapshot behaviour exactly.
