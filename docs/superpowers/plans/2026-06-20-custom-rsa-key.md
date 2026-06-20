# Custom RSA Key Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an operator optionally supply a custom RSA private key (PEM file path in the world config) for login decryption instead of the hard-coded key in `pkg/io/protocol/rsakey.go`, plus a `goscape-cli rsa` verb to generate keypairs and inspect existing keys.

**Architecture:** Introduce a small `RSAKey{N,E,D}` type and a `DefaultRSAKey` holding today's built-in 512-bit values; thread the key explicitly through the login decode path (no global mutation). The world module loads a custom key from a configured PEM path at boot (validated in `Config.Validate`, resolved onto `Server.rsaKey`) and falls back to the default otherwise. A new CLI verb generates keypairs and prints the values needed to bake the matching public key into the Java client.

**Tech Stack:** Go (`crypto/rsa`, `crypto/x509`, `encoding/pem`, `math/big`), existing `pkg/io/packet` RS2 buffer, `flag`-based CLI verbs.

## Global Constraints

- Go command invocations MUST be prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- All commits MUST use `git commit --no-gpg-sign`.
- Faithful-translation policy: this feature restores TS fidelity (Engine-TS `World.ts:104` loads `data/config/private.pem`; the generator mirrors `tools/server/rsa.ts`). Do not change wire values or the default key.
- The world config is parsed with `yaml.UnmarshalStrict` — any new YAML key MUST have a matching struct field with the exact `yaml:"..."` tag, or boot crashes.
- Default behavior is unchanged: the built-in 512-bit key is used unless `world.rsa_private_key_path` is set. Do not auto-load any file.
- Wire constraint: the login RSA block length is a single byte (`packet.RSAEnc` → `P1(len)`), so a modulus above ~254 bytes (~2032-bit) overflows. The generator warns; it does not hard-fail.
- Critical operator fact (document, do not enforce): a custom server key only works if the Java client is rebuilt with the matching public key (`Client.java:1290-1291` `LOGIN_RSAN`/`LOGIN_RSAE`).

---

### Task 1: `RSAKey` type, `DefaultRSAKey`, and PEM loader

**Files:**
- Modify: `pkg/io/protocol/rsakey.go`
- Create: `pkg/io/protocol/rsakey_load.go`
- Test: `pkg/io/protocol/rsakey_load_test.go`

**Interfaces:**
- Produces:
  - `type RSAKey struct { Modulus, PublicExponent, PrivateExponent *big.Int }`
  - `var DefaultRSAKey *RSAKey` (built-in 512-bit key)
  - package vars `Modulus`, `PublicExponent`, `PrivateExponent` remain, now aliasing `DefaultRSAKey`'s fields
  - `func ParseRSAKeyPEM(pemBytes []byte) (*RSAKey, error)`
  - `func LoadRSAKeyPEM(path string) (*RSAKey, error)`

- [ ] **Step 1: Write the failing tests** (`pkg/io/protocol/rsakey_load_test.go`)

```go
package protocol

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestParseRSAKeyPEM_PKCS1RoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	got, err := ParseRSAKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("ParseRSAKeyPEM: %v", err)
	}
	if got.Modulus.Cmp(priv.N) != 0 {
		t.Error("modulus mismatch")
	}
	if got.PrivateExponent.Cmp(priv.D) != 0 {
		t.Error("private exponent mismatch")
	}
	if got.PublicExponent.Int64() != int64(priv.E) {
		t.Errorf("public exponent: got %d want %d", got.PublicExponent.Int64(), priv.E)
	}
}

func TestParseRSAKeyPEM_PKCS8RoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	got, err := ParseRSAKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("ParseRSAKeyPEM: %v", err)
	}
	if got.Modulus.Cmp(priv.N) != 0 || got.PrivateExponent.Cmp(priv.D) != 0 {
		t.Error("PKCS8 key material mismatch")
	}
}

func TestParseRSAKeyPEM_Errors(t *testing.T) {
	if _, err := ParseRSAKeyPEM([]byte("not a pem")); err == nil {
		t.Error("expected error for non-PEM input")
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 1024)
	pubDER, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if _, err := ParseRSAKeyPEM(pubPEM); err == nil {
		t.Error("expected error for public-key PEM (not a private key)")
	}
}

func TestDefaultRSAKey_GlobalsAlias(t *testing.T) {
	if Modulus != DefaultRSAKey.Modulus ||
		PublicExponent != DefaultRSAKey.PublicExponent ||
		PrivateExponent != DefaultRSAKey.PrivateExponent {
		t.Error("legacy package globals must alias DefaultRSAKey fields")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/ -run 'RSAKey|DefaultRSAKey' -v`
Expected: FAIL — `undefined: RSAKey`, `undefined: ParseRSAKeyPEM`, `undefined: DefaultRSAKey`.

- [ ] **Step 3: Refactor `rsakey.go` to add `RSAKey` + `DefaultRSAKey`**

Replace the entire body of `pkg/io/protocol/rsakey.go` with:

```go
package protocol

import "math/big"

// RSAKey holds the RSA key material for the login handshake: modulus N,
// public exponent E, and private exponent D. Only N and D are used at
// runtime (login decryption via Packet.RSADec); E is used by the test-only
// client-side RSAEnc path.
type RSAKey struct {
	Modulus         *big.Int // N
	PublicExponent  *big.Int // E
	PrivateExponent *big.Int // D
}

// DefaultRSAKey is the built-in 512-bit key compiled into the binary. It is
// used for login decryption unless the world module is configured with a
// custom key (world.rsa_private_key_path). The matching public key is baked
// into the Java client (Client.java LOGIN_RSAN / LOGIN_RSAE).
var DefaultRSAKey *RSAKey

// Modulus, PublicExponent, and PrivateExponent alias DefaultRSAKey's fields
// for existing callers (e.g. the client-side RSAEnc in login/req). New code
// should thread an *RSAKey explicitly instead of reading these globals.
var (
	Modulus         *big.Int // N
	PublicExponent  *big.Int // E
	PrivateExponent *big.Int // D
)

func init() {
	modulus, ok := new(big.Int).SetString("0088c38748a58228f7261cdc340b5691d7d0975dee0ecdb717609e6bf971eb3fe723ef9d130e4686813739768ad9472eb46d8bfcc042c1a5fcb05e931f632eea5d", 16)
	if !ok {
		panic("bad modulus")
	}

	publicExponent, ok := new(big.Int).SetString("81f390b2cf8ca7039ee507975951d5a0b15a87bf8b3f99c966834118c50fd94d", 16)
	if !ok {
		panic("bad public exponent")
	}

	privateExponent, ok := new(big.Int).SetString("571fb062048b61721ebfcf1e877153241b70c3aa26edb0f9f06a1b2be07c4e45eaba4fc356ea806cbed298d38613590a53fde0383c3a411758516293240925e5", 16)
	if !ok {
		panic("bad private exponent")
	}

	DefaultRSAKey = &RSAKey{
		Modulus:         modulus,
		PublicExponent:  publicExponent,
		PrivateExponent: privateExponent,
	}
	Modulus = DefaultRSAKey.Modulus
	PublicExponent = DefaultRSAKey.PublicExponent
	PrivateExponent = DefaultRSAKey.PrivateExponent
}
```

- [ ] **Step 4: Create the loader** (`pkg/io/protocol/rsakey_load.go`)

```go
package protocol

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
)

// ParseRSAKeyPEM decodes a PEM-encoded RSA private key — PKCS#1
// ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE KEY") — into an RSAKey. It mirrors
// Engine-TS forge.pki.privateKeyFromPem(...) at World.ts:104.
func ParseRSAKeyPEM(pemBytes []byte) (*RSAKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("protocol: no PEM block found")
	}

	var priv *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		priv = k
	} else {
		k8, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			return nil, fmt.Errorf("protocol: parse RSA private key: %w", err8)
		}
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("protocol: PEM is not an RSA private key")
		}
		priv = rk
	}

	return &RSAKey{
		Modulus:         priv.N,
		PublicExponent:  big.NewInt(int64(priv.E)),
		PrivateExponent: priv.D,
	}, nil
}

// LoadRSAKeyPEM reads path and parses it via ParseRSAKeyPEM.
func LoadRSAKeyPEM(path string) (*RSAKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("protocol: read RSA private key: %w", err)
	}
	return ParseRSAKeyPEM(b)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/ -v`
Expected: PASS (all four new tests + existing protocol tests).

- [ ] **Step 6: Commit**

```bash
git add pkg/io/protocol/rsakey.go pkg/io/protocol/rsakey_load.go pkg/io/protocol/rsakey_load_test.go
git commit --no-gpg-sign -m "feat(protocol): RSAKey type, DefaultRSAKey, and PEM loader

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Thread the key through login decode

**Files:**
- Modify: `pkg/io/protocol/login/req/req.go:125-149` (`UnmarshalRSA`, `UnmarshalBinary`)
- Modify: `modules/world/server.go:1203` (temporary: pass `protocol.DefaultRSAKey`)
- Test: `pkg/io/protocol/login/req/req_test.go` (add custom-key round-trip + import block)

**Interfaces:**
- Consumes: `protocol.RSAKey`, `protocol.DefaultRSAKey` (Task 1).
- Produces: `func (q *GameLogin) UnmarshalRSA(r *packet.Packet, key *protocol.RSAKey) error` — later tasks pass a per-server key here.

- [ ] **Step 1: Write the failing test** (append to `pkg/io/protocol/login/req/req_test.go`)

Add a helper and one test. The positive round-trip alone proves the key param is honored: the block is encrypted with a freshly generated key, so if `UnmarshalRSA` ignored its `key` argument and used the default, the magic-byte check would fail and the test would error.

```go
func newTestRSAKey(t *testing.T, bits int) *protocol.RSAKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return &protocol.RSAKey{
		Modulus:         k.N,
		PublicExponent:  big.NewInt(int64(k.E)),
		PrivateExponent: k.D,
	}
}

func TestUnmarshalRSA_CustomKey(t *testing.T) {
	rk := newTestRSAKey(t, 1024) // Go's crypto/rsa rejects <1024-bit generation

	pt := packet.NewPacket(make([]byte, 0, 256))
	pt.P1(10) // RSA magic number
	for _, s := range []uint32{1, 2, 3, 4} {
		pt.P4(s)
	}
	pt.P4(0xDEADBEEF) // uid
	pt.PJStrLF("alice")
	pt.PJStrLF("hunter2")
	pt.RSAEnc(rk.Modulus, rk.PublicExponent)

	var q GameLogin
	if err := q.UnmarshalRSA(packet.NewPacket(pt.Bytes()), rk); err != nil {
		t.Fatalf("UnmarshalRSA with custom key: %v", err)
	}
	if q.Username != "alice" || q.Password != "hunter2" {
		t.Errorf("round-trip mismatch: user=%q pass=%q", q.Username, q.Password)
	}
	if q.UID != 0xDEADBEEF {
		t.Errorf("uid mismatch: %#x", q.UID)
	}
	if q.ISAACSeed != [4]uint32{1, 2, 3, 4} {
		t.Errorf("seed mismatch: %v", q.ISAACSeed)
	}
}
```

Also extend the existing import block at the top of `req_test.go` to:

```go
import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"math/big"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
)
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/login/req/ -run TestUnmarshalRSA_CustomKey -v`
Expected: FAIL — `too many arguments in call to q.UnmarshalRSA`.

- [ ] **Step 3: Change `UnmarshalRSA` and `UnmarshalBinary`** (`req.go`)

At `req.go:125`, change the signature and the `RSADec` call:

```go
func (q *GameLogin) UnmarshalRSA(r *packet.Packet, key *protocol.RSAKey) error {
	decrypted, err := r.RSADec(key.Modulus, key.PrivateExponent)
	if err != nil {
		return err
	}
```

At `req.go:148` (inside `UnmarshalBinary`), pass the default key:

```go
	return q.UnmarshalRSA(r, protocol.DefaultRSAKey)
```

- [ ] **Step 4: Keep the production call site compiling** (`modules/world/server.go:1203`)

Change:

```go
		if err := req.UnmarshalRSA(r); err != nil {
```

to (temporary — Task 3 swaps in the per-server key):

```go
		if err := req.UnmarshalRSA(r, protocol.DefaultRSAKey); err != nil {
```

`server.go` already imports `github.com/zsrv/goscape/pkg/io/protocol` (line 33); no import change needed.

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/io/protocol/login/req/ ./modules/world/ -count=1`
Expected: PASS (new custom-key test + existing default-key tests + world tests still build).

- [ ] **Step 6: Commit**

```bash
git add pkg/io/protocol/login/req/req.go pkg/io/protocol/login/req/req_test.go modules/world/server.go
git commit --no-gpg-sign -m "feat(login): thread RSA key into UnmarshalRSA

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: World config field + `Server.rsaKey` wiring

**Files:**
- Modify: `modules/world/config.go` (struct field, flag, `Validate`, import)
- Modify: `modules/world/server.go` (`Server` struct field, `NewServer` resolution, decode-site swap)
- Test: `modules/world/config_rsakey_test.go` (new)

**Interfaces:**
- Consumes: `protocol.LoadRSAKeyPEM`, `protocol.DefaultRSAKey`, `protocol.RSAKey` (Task 1); `req.UnmarshalRSA(r, key)` (Task 2).
- Produces: `Config.RSAPrivateKeyPath string` (`yaml:"rsa_private_key_path"`); `Server.rsaKey *protocol.RSAKey`.

- [ ] **Step 1: Write the failing test** (`modules/world/config_rsakey_test.go`)

```go
package world

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writeTestPrivatePEM(t *testing.T, bits int) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	path := filepath.Join(t.TempDir(), "private.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestConfigValidate_RSAKeyPath(t *testing.T) {
	// Valid key path validates clean.
	good := Config{Enable: true, TCPListenPort: 40000, CachePath: "x", RSAPrivateKeyPath: writeTestPrivatePEM(t, 1024)}
	if err := good.Validate(); err != nil {
		t.Errorf("valid key path: unexpected error: %v", err)
	}

	// Missing file fails validation.
	bad := Config{Enable: true, TCPListenPort: 40000, CachePath: "x", RSAPrivateKeyPath: "/no/such/key.pem"}
	if err := bad.Validate(); err == nil {
		t.Error("missing key path: expected validation error, got nil")
	}

	// Empty path is allowed (use built-in default).
	none := Config{Enable: true, TCPListenPort: 40000, CachePath: "x"}
	if err := none.Validate(); err != nil {
		t.Errorf("empty key path: unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConfigValidate_RSAKeyPath -v`
Expected: FAIL — `unknown field RSAPrivateKeyPath in struct literal`.

- [ ] **Step 3: Add the config field, flag, and validation** (`config.go`)

Add `protocol` to the import block:

```go
import (
	"flag"
	"fmt"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/io/protocol"
)
```

Add the struct field immediately after the `ContentPath` field (`config.go:21`):

```go
	// RSAPrivateKeyPath optionally points to a PEM-encoded RSA private key
	// (PKCS#1 or PKCS#8) used to decrypt the login block, replacing the
	// built-in default key in pkg/io/protocol/rsakey.go. Empty (default) uses
	// the built-in key. Mirrors Engine-TS World.ts:104 (data/config/private.pem).
	// The Java client must be rebuilt with the matching public key
	// (Client.java LOGIN_RSAN / LOGIN_RSAE) or every login fails.
	RSAPrivateKeyPath string `yaml:"rsa_private_key_path"`
```

Add the flag in `RegisterFlagsAndApplyDefaults`, after the `world.content-path` flag (`config.go:96`):

```go
	f.StringVar(&c.RSAPrivateKeyPath, "world.rsa-private-key-path", "", "Optional PEM RSA private key (PKCS#1/PKCS#8) for login decryption, replacing the built-in default key. The client must carry the matching public key. Empty uses the built-in key.")
```

Add validation inside the existing `if c.Enable {` block in `Validate` (after the `ContentWatch` check, before the block closes at `config.go:131`):

```go
		if c.RSAPrivateKeyPath != "" {
			if _, err := protocol.LoadRSAKeyPEM(c.RSAPrivateKeyPath); err != nil {
				return fmt.Errorf("world.rsa-private-key-path: %w", err)
			}
		}
```

- [ ] **Step 4: Run the config test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConfigValidate_RSAKeyPath -v`
Expected: PASS.

- [ ] **Step 5: Add `Server.rsaKey` and resolve it in `NewServer`** (`server.go`)

Add a field to the `Server` struct (after `cfg Config` at `server.go:73`):

```go
	// rsaKey is the RSA key used to decrypt the login block. Resolved in
	// NewServer from cfg.RSAPrivateKeyPath (custom) or protocol.DefaultRSAKey
	// (built-in). May be nil in test-only Server literals; the login decode
	// site falls back to DefaultRSAKey when nil.
	rsaKey *protocol.RSAKey
```

At the very top of `NewServer` (before `net.Listen`, `server.go:398`), resolve the key so a bad key fails before the socket opens:

```go
	rsaKey := protocol.DefaultRSAKey
	if cfg.RSAPrivateKeyPath != "" {
		k, err := protocol.LoadRSAKeyPEM(cfg.RSAPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load RSA private key: %w", err)
		}
		rsaKey = k
	}
```

After the `s := &Server{...}` literal, alongside the other `s.xxx = ...` assignments (e.g. near `server.go:436` `s.packFn = ...`), add:

```go
	s.rsaKey = rsaKey
```

- [ ] **Step 6: Swap the decode site to the per-server key** (`server.go:1203`)

Replace the temporary line from Task 2:

```go
		if err := req.UnmarshalRSA(r, protocol.DefaultRSAKey); err != nil {
```

with the nil-guarded per-server key (test Server literals leave `rsaKey` nil):

```go
		rsaKey := protocol.DefaultRSAKey
		if c.server != nil && c.server.rsaKey != nil {
			rsaKey = c.server.rsaKey
		}
		if err := req.UnmarshalRSA(r, rsaKey); err != nil {
```

- [ ] **Step 7: Run the world tests to verify everything passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS (config test + existing login/server tests using the default fallback).

- [ ] **Step 8: Commit**

```bash
git add modules/world/config.go modules/world/server.go modules/world/config_rsakey_test.go
git commit --no-gpg-sign -m "feat(world): optional rsa_private_key_path for login decryption

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: `goscape-cli rsa` verb (`gen` + `info`)

**Files:**
- Create: `cmd/goscape-cli/cmd_rsa.go`
- Modify: `cmd/goscape-cli/main.go` (register the verb)
- Test: `cmd/goscape-cli/cmd_rsa_test.go`

**Interfaces:**
- Consumes: `protocol.DefaultRSAKey`, `protocol.ParseRSAKeyPEM` (Task 1); `pemtoken.Token` (existing).
- Produces: `func runRSA(args []string, stdout, stderr io.Writer) int`.

- [ ] **Step 1: Write the failing tests** (`cmd/goscape-cli/cmd_rsa_test.go`)

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/protocol"
	"github.com/zsrv/goscape/pkg/util/pemtoken"
)

func TestRunRSAInfo_DefaultKey(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runRSA([]string{"info"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errb.String())
	}
	s := out.String()
	// Built-in default public key must match the Java client constants
	// (Client.java:1290-1291). Guards against accidental key changes.
	const wantN = "7162900525229798032761816791230527296329313291232324290237849263501208207972894053929065636522363163621000728841182238772712427862772219676577293600221789"
	const wantE = "58778699976184461502525193738213253649000149147835990136706041084440742975821"
	if !strings.Contains(s, wantN) {
		t.Errorf("default key N decimal not found in output:\n%s", s)
	}
	if !strings.Contains(s, wantE) {
		t.Errorf("default key E decimal not found in output:\n%s", s)
	}
}

func TestRunRSAGen_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	var genOut, genErr bytes.Buffer
	if code := runRSA([]string{"gen", "--bits", "1024", "--out-dir", dir}, &genOut, &genErr); code != 0 {
		t.Fatalf("gen exit %d, stderr=%s", code, genErr.String())
	}
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	pemBytes, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatalf("read private.pem: %v", err)
	}
	key, err := protocol.ParseRSAKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("ParseRSAKeyPEM: %v", err)
	}
	wantN := key.Modulus.String()
	if !strings.Contains(genOut.String(), wantN) {
		t.Errorf("gen output missing modulus N decimal")
	}

	var infoOut, infoErr bytes.Buffer
	if code := runRSA([]string{"info", privPath}, &infoOut, &infoErr); code != 0 {
		t.Fatalf("info exit %d, stderr=%s", code, infoErr.String())
	}
	if !strings.Contains(infoOut.String(), wantN) {
		t.Errorf("info output missing modulus N decimal")
	}

	// public.pem must be usable as the ondemand pub_pem (PKIX RSA public key).
	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read public.pem: %v", err)
	}
	if _, err := pemtoken.Token(pubBytes, "host"); err != nil {
		t.Errorf("public.pem not usable as ondemand pub_pem: %v", err)
	}
}

func TestRunRSA_UnknownSubVerb(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runRSA([]string{"bogus"}, &out, &errb); code != 2 {
		t.Errorf("expected exit 2 for unknown sub-verb, got %d", code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunRSA -v`
Expected: FAIL — `undefined: runRSA`.

- [ ] **Step 3: Create the verb** (`cmd/goscape-cli/cmd_rsa.go`)

```go
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/protocol"
)

// wireLengthCapBytes is the largest RSA modulus (in bytes) whose ciphertext
// still fits the login block's 1-byte length prefix once Java's BigInteger
// sign byte is accounted for. packet.RSAEnc writes P1(len(ciphertext)); a
// modulus above this overflows that byte. See packet.RSAEnc / packet.RSADec.
const wireLengthCapBytes = 254

// runRSA dispatches the `rsa` verb's sub-verbs: `gen` and `info`.
func runRSA(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: goscape-cli rsa <gen|info> [flags]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "gen":
		return runRSAGen(rest, stdout, stderr)
	case "info":
		return runRSAInfo(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "Usage: goscape-cli rsa <gen|info> [flags]")
		fmt.Fprintln(stdout, "  gen   Generate an RSA keypair (private.pem + public.pem).")
		fmt.Fprintln(stdout, "  info  Print N/E/D for a PEM key, or the built-in default key when no path is given.")
		return 0
	}
	fmt.Fprintf(stderr, "unknown rsa sub-verb: %q\n", sub)
	return 2
}

// runRSAGen generates an RSA keypair, writes private.pem (PKCS#1, forge/TS
// compatible) and public.pem (PKIX, ondemand pub_pem compatible), and prints
// the values needed to bake the matching public key into the Java client.
func runRSAGen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rsa gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bits := fs.Int("bits", 1024, "RSA key size in bits (TS default 1024).")
	outDir := fs.String("out-dir", ".", "Directory to write private.pem and public.pem.")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	priv, err := rsa.GenerateKey(rand.Reader, *bits)
	if err != nil {
		fmt.Fprintf(stderr, "rsa gen: generate key: %v\n", err)
		return 1
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		fmt.Fprintf(stderr, "rsa gen: marshal public key: %v\n", err)
		return 1
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	privPath := filepath.Join(*outDir, "private.pem")
	pubPath := filepath.Join(*outDir, "public.pem")
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		fmt.Fprintf(stderr, "rsa gen: write %s: %v\n", privPath, err)
		return 1
	}
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		fmt.Fprintf(stderr, "rsa gen: write %s: %v\n", pubPath, err)
		return 1
	}

	fmt.Fprintf(stdout, "wrote %s\nwrote %s\n\n", privPath, pubPath)
	printRSABakeValues(stdout, priv.N, big.NewInt(int64(priv.E)), priv.D)

	if (priv.N.BitLen()+7)/8 > wireLengthCapBytes {
		fmt.Fprintf(stderr, "\nwarning: a %d-bit modulus exceeds the login wire cap (~%d-bit); "+
			"the RSA login block length is a single byte and will overflow.\n",
			priv.N.BitLen(), wireLengthCapBytes*8)
	}
	return 0
}

// runRSAInfo prints the bake values for a PEM key. With no path it prints the
// built-in default key (pkg/io/protocol/rsakey.go).
func runRSAInfo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rsa info", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()

	if len(rest) == 0 {
		fmt.Fprintln(stdout, "Built-in default key (pkg/io/protocol/rsakey.go):")
		printRSABakeValues(stdout, protocol.DefaultRSAKey.Modulus,
			protocol.DefaultRSAKey.PublicExponent, protocol.DefaultRSAKey.PrivateExponent)
		return 0
	}

	path := rest[0]
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "rsa info: read %s: %v\n", path, err)
		return 1
	}
	n, e, d, err := parseAnyRSAPEM(b)
	if err != nil {
		fmt.Fprintf(stderr, "rsa info: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s:\n", path)
	printRSABakeValues(stdout, n, e, d)
	return 0
}

// parseAnyRSAPEM decodes a PEM block as an RSA private key (PKCS#1 or PKCS#8)
// or an RSA public key (PKIX or PKCS#1). For a public key, d is nil.
func parseAnyRSAPEM(b []byte) (*big.Int, *big.Int, *big.Int, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, nil, nil, errors.New("no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k.N, big.NewInt(int64(k.E)), k.D, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, nil, errors.New("PEM is not an RSA key")
		}
		return rk.N, big.NewInt(int64(rk.E)), rk.D, nil
	}
	if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		pk, ok := k.(*rsa.PublicKey)
		if !ok {
			return nil, nil, nil, errors.New("PEM is not an RSA key")
		}
		return pk.N, big.NewInt(int64(pk.E)), nil, nil
	}
	if k, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return k.N, big.NewInt(int64(k.E)), nil, nil
	}
	return nil, nil, nil, errors.New("unrecognized PEM key format")
}

// printRSABakeValues prints N and E in decimal (Client.java LOGIN_RSAN /
// LOGIN_RSAE) and N/E/D in hex (pkg/io/protocol/rsakey.go). D is printed only
// when known (private key).
func printRSABakeValues(w io.Writer, n, e, d *big.Int) {
	fmt.Fprintf(w, "Modulus (N):\n")
	fmt.Fprintf(w, "  decimal (Client.java LOGIN_RSAN): %s\n", n.String())
	fmt.Fprintf(w, "  hex (rsakey.go):                  %s\n", n.Text(16))
	fmt.Fprintf(w, "Public exponent (E):\n")
	fmt.Fprintf(w, "  decimal (Client.java LOGIN_RSAE): %s\n", e.String())
	fmt.Fprintf(w, "  hex (rsakey.go):                  %s\n", e.Text(16))
	if d != nil {
		fmt.Fprintf(w, "Private exponent (D):\n")
		fmt.Fprintf(w, "  hex (rsakey.go): %s\n", d.Text(16))
	}
}
```

- [ ] **Step 4: Register the verb** (`cmd/goscape-cli/main.go`)

Add to the `verbs` slice (after the `unpack` entry):

```go
	{"rsa", runRSA, "Generate or inspect RSA login keys (gen | info)."},
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestRunRSA -v`
Expected: PASS (info default-key, gen round-trip, unknown sub-verb).

- [ ] **Step 6: Commit**

```bash
git add cmd/goscape-cli/cmd_rsa.go cmd/goscape-cli/cmd_rsa_test.go cmd/goscape-cli/main.go
git commit --no-gpg-sign -m "feat(cli): goscape-cli rsa verb (gen + info)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Document the option in the example config

**Files:**
- Modify: `config.yaml`

**Interfaces:**
- Consumes: `world.rsa_private_key_path` (Task 3).

- [ ] **Step 1: Add a commented example under the `world:` section** (`config.yaml`)

After the `content_path:` line (`config.yaml:33`), add:

```yaml
  # Optional: PEM-encoded RSA private key (PKCS#1 or PKCS#8) used to decrypt
  # the login block, replacing the built-in default key. The Java client must
  # be rebuilt with the matching public key (Client.java LOGIN_RSAN /
  # LOGIN_RSAE). Generate a keypair and print the bake values with:
  #   goscape-cli rsa gen --bits 1024 --out-dir data/config
  # Leave unset to use the built-in default key.
  #rsa_private_key_path: data/config/private.pem
```

- [ ] **Step 2: Verify the example config still parses**

Build the daemon, then run config verification (the commented line must not break strict YAML parsing, and the rest of the file must remain valid):

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape" ./cmd/goscape && \
"$TMPDIR/goscape" --config.file config.yaml --config.verify
```
Expected: exits 0 with no parse/validation error (the daemon may print a verification-success message; it must not error on the new commented key).

- [ ] **Step 3: Commit**

```bash
git add config.yaml
git commit --no-gpg-sign -m "docs(config): example world.rsa_private_key_path

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Full integration verification

**Files:** none (verification only)

- [ ] **Step 1: Format, vet, and build both binaries**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache gofmt -l pkg/io/protocol modules/world cmd/goscape-cli
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/io/protocol/... ./modules/world/... ./cmd/goscape-cli/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./cmd/goscape ./cmd/goscape-cli
```
Expected: `gofmt -l` prints nothing (no unformatted files); `go vet` clean; both builds succeed.

- [ ] **Step 2: Run the full test suite with the race detector on touched packages**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./pkg/io/protocol/... ./pkg/io/protocol/login/req/... ./modules/world/ ./cmd/goscape-cli/
```
Expected: all PASS.

- [ ] **Step 3: End-to-end custom-key smoke via `--config.verify`**

Generate a key, point a throwaway config at it, and confirm the world module validates it (proves config → `LoadRSAKeyPEM` → `Validate` wiring end to end):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape-cli" ./cmd/goscape-cli && \
"$TMPDIR/goscape-cli" rsa gen --bits 1024 --out-dir "$TMPDIR" && \
printf 'target: world\nworld:\n  enable: true\n  cache_path: ./data/pack\n  rsa_private_key_path: %s/private.pem\n' "$TMPDIR" > "$TMPDIR/rsa-test.yaml" && \
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath -o "$TMPDIR/goscape" ./cmd/goscape && \
"$TMPDIR/goscape" --config.file "$TMPDIR/rsa-test.yaml" --config.verify=true
```
Expected: `rsa gen` prints the bake values and writes both PEMs; `--config.verify=true` exits 0 (note: `--config.verify` is a bool flag and must use the `=true` form; bare exits 2). Then confirm a bogus path fails:

```bash
printf 'target: world\nworld:\n  enable: true\n  cache_path: ./data/pack\n  rsa_private_key_path: /no/such/key.pem\n' > "$TMPDIR/rsa-bad.yaml" && \
"$TMPDIR/goscape" --config.file "$TMPDIR/rsa-bad.yaml" --config.verify=true; echo "exit=$?"
```
Expected: non-zero exit with a `world.rsa-private-key-path: ...` error.

- [ ] **Step 4: Final commit (if any formatting changes were made)**

```bash
git add -A
git commit --no-gpg-sign -m "chore: gofmt/vet cleanup for custom RSA key feature

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>" || echo "nothing to commit"
```

---

## Self-Review

**Spec coverage:**
- RSAKey type + DefaultRSAKey + PEM loader (PKCS#1/#8) → Task 1 ✓
- `UnmarshalRSA(r, key)` threading; `UnmarshalBinary` defaults → Task 2 ✓
- World `rsa_private_key_path` (YAML + flag), `Validate` fail-fast, `Server.rsaKey`, decode-site swap → Task 3 ✓
- `goscape-cli rsa gen` + `rsa info` (default-key + file), bake values decimal+hex, ondemand-compatible public.pem, wire-cap warning → Task 4 ✓
- Operator docs / config example → Task 5 ✓ (in-code doc comments in Tasks 3–4)
- Default-key guard test vs Client.java constants → Task 4 `TestRunRSAInfo_DefaultKey` ✓
- Loader round-trip + error cases → Task 1 ✓
- End-to-end custom-key decode → Task 2 `TestUnmarshalRSA_CustomKey` ✓
- Config validation good/bad path → Task 3 ✓
- Out-of-scope items (ondemand unify, backport, hot-reload) → intentionally excluded ✓

**Placeholder scan:** none — every code/test step contains complete code; every command has expected output.

**Type consistency:** `RSAKey{Modulus,PublicExponent,PrivateExponent}` used identically across Tasks 1–4; `ParseRSAKeyPEM`/`LoadRSAKeyPEM` signatures stable; `runRSA(args, stdout, stderr) int` matches the `verbHandler` type registered in `main.go`; `UnmarshalRSA(r, key *protocol.RSAKey)` consistent between Task 2 (definition) and Task 3 (call site).
