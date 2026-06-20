# Custom RSA Key Support Design

**Date:** 2026-06-20
**Branch:** rev-274
**Scope:** Let an operator optionally supply a custom RSA private key (PEM file path in the world config) for login decryption instead of the hard-coded key in `pkg/io/protocol/rsakey.go`, plus a `goscape-cli rsa` verb to generate keypairs and introspect existing keys.

## Context

The RuneScape login handshake encrypts the sensitive login block (magic byte, ISAAC seeds, UID, username, password) with the server's RSA **public** key — baked into the Java client — and the server decrypts it with the matching **private** key.

- **goscape today** hard-codes the key as three package-level `*big.Int`s (`Modulus` N, `PublicExponent` E, `PrivateExponent` D) in `pkg/io/protocol/rsakey.go`, set in `init()`. The default is a **512-bit** key (64-byte modulus). Only the decrypt side runs at runtime: `req.GameLogin.UnmarshalRSA` → `packet.RSADec(Modulus, PrivateExponent)`, called from `modules/world/server.go:1203`. `RSAEnc`/`MarshalBinary` are test-only.
- **Engine-TS** (reference) loads the key from a PEM file: `World.ts:104` does `forge.pki.privateKeyFromPem(fs.readFileSync('data/config/private.pem'))` and uses it for `rsadec`. It ships a generator at `tools/server/rsa.ts` (`forge.pki.rsa.generateKeyPair(1024)` → `public.pem` + `private.pem`).
- **Client-Java** hard-codes the matching public key as decimal `BigInteger`s: `LOGIN_RSAN` (modulus) and `LOGIN_RSAE` (exponent) at `Client.java:1290-1291`, used by `rsaenc` at `Client.java:3578`.

This feature therefore **restores TS fidelity** (PEM-file loading + a generator mirroring `tools/server/rsa.ts`) rather than diverging from it; it does not conflict with the faithful-translation policy.

### Critical operator caveats

1. **The client must be rebuilt with the matching public key.** The server private key only decrypts blocks encrypted with its own public key. Swapping the server key without updating `Client.java:1290-1291` (`LOGIN_RSAN`/`LOGIN_RSAE`) makes every login produce garbage plaintext and fail. The `rsa` verb prints the values needed to do this.
2. **Wire-format size cap.** The login RSA block length is written as a single byte (`RSAEnc` → `P1(len(ciphertext))`; `RSADec` → `G1()`). A modulus larger than ~255 bytes (~2040 bits) overflows the length prefix. 1024-bit (the TS default) is the safe recommendation; the generator warns above the cap.
3. `RSADec` is key-size agnostic (`big.Int.SetBytes` + `Exp`); the existing 63/64/65-byte special cases are value-preserving no-ops for 512-bit blocks and need no change.

## Design decisions

- **Default behavior is unchanged:** the built-in 512-bit key remains the default. A custom key is strictly opt-in via config path. We do **not** auto-load `data/config/private.pem` (rejected: surprising implicit file dependency; the user asked for explicit opt-in).
- **World-only scope.** Only the world module's login decryption consumes a custom key. The OnDemand module keeps its independent `pub_pem` (operators sync it manually, as today). The modules do not import each other; no cross-module plumbing is added.
- **No global mutation.** The key is threaded explicitly through the decode path (chosen over mutating the `protocol` package globals at startup, which would be racy under parallel tests and couple every consumer).

## Files

### New

| File | Purpose |
|---|---|
| `pkg/io/protocol/rsakey_load.go` | `ParseRSAKeyPEM([]byte)` / `LoadRSAKeyPEM(path)` — decode PEM, parse PKCS#1 or PKCS#8 RSA private key into an `RSAKey` |
| `pkg/io/protocol/rsakey_load_test.go` | round-trip + error-path tests for the loader |
| `cmd/goscape-cli/cmd_rsa.go` | `goscape-cli rsa` verb (`gen` + `info` sub-verbs) |
| `cmd/goscape-cli/cmd_rsa_test.go` | tests for `rsa gen` / `rsa info` |

### Modified

| File | Change |
|---|---|
| `pkg/io/protocol/rsakey.go` | Add `type RSAKey struct{...}` + `var DefaultRSAKey *RSAKey`; keep `Modulus`/`PublicExponent`/`PrivateExponent` package vars as aliases into `DefaultRSAKey` |
| `pkg/io/protocol/login/req/req.go` | `UnmarshalRSA(r, key *protocol.RSAKey)`; `UnmarshalBinary` calls it with `protocol.DefaultRSAKey` |
| `modules/world/config.go` | New `RSAPrivateKeyPath string` (`yaml:"rsa_private_key_path"` + flag `world.rsa-private-key-path`); load+validate in `Validate` |
| `modules/world/server.go` | `Server.rsaKey *protocol.RSAKey` resolved in `NewServer`; pass `c.server.rsaKey` at the `UnmarshalRSA` call site (~line 1203) |
| `cmd/goscape-cli/main.go` | Register the `rsa` verb in the `verbs` slice |
| `pkg/io/protocol/login/req/req_test.go` | Update `UnmarshalRSA` callers to pass `protocol.DefaultRSAKey` |

## Component design

### `RSAKey` type (`pkg/io/protocol/rsakey.go`)

```go
type RSAKey struct {
    Modulus         *big.Int // N
    PublicExponent  *big.Int // E
    PrivateExponent *big.Int // D
}

var DefaultRSAKey *RSAKey // the current built-in 512-bit key
```

`init()` builds `DefaultRSAKey` from the existing hex literals, then assigns the three legacy package vars (`Modulus`, `PublicExponent`, `PrivateExponent`) to its fields so existing references keep working with zero churn.

### Loader (`pkg/io/protocol/rsakey_load.go`)

```go
// ParseRSAKeyPEM decodes a PEM block and parses it as an RSA private key in
// either PKCS#1 ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE KEY") form.
func ParseRSAKeyPEM(pemBytes []byte) (*RSAKey, error)

// LoadRSAKeyPEM reads path and delegates to ParseRSAKeyPEM.
func LoadRSAKeyPEM(path string) (*RSAKey, error)
```

Internally uses `pem.Decode`, then tries `x509.ParsePKCS1PrivateKey`; on failure tries `x509.ParsePKCS8PrivateKey` and type-asserts `*rsa.PrivateKey`. Returns `&RSAKey{N, big.NewInt(int64(E)), D}` from the parsed key. Errors: no PEM block, unparseable, or not an RSA key.

### Login decode threading (`req.go`)

```go
func (q *GameLogin) UnmarshalRSA(r *packet.Packet, key *protocol.RSAKey) error {
    decrypted, err := r.RSADec(key.Modulus, key.PrivateExponent)
    ...
}

func (q *GameLogin) UnmarshalBinary(data []byte) error {
    ...
    return q.UnmarshalRSA(r, protocol.DefaultRSAKey)
}
```

`MarshalBinary` (test-only encrypt) keeps using `protocol.DefaultRSAKey`'s `Modulus`/`PublicExponent`.

### World config + wiring

- `Config.RSAPrivateKeyPath` — empty default. Flag: `world.rsa-private-key-path`, usage noting it overrides the built-in key and that the client must carry the matching public key.
- `Config.Validate`: if `RSAPrivateKeyPath != ""`, call `protocol.LoadRSAKeyPEM` and return any error (so `--config.verify` catches a missing/garbage key before boot). Optionally warn (log) if the loaded modulus exceeds the wire cap.
- `NewServer`: `s.rsaKey = protocol.DefaultRSAKey`; if `cfg.RSAPrivateKeyPath != ""`, `s.rsaKey = protocol.LoadRSAKeyPEM(...)` (error → fail construction). Store on `Server`.
- `server.go` decode site: `req.UnmarshalRSA(r, c.server.rsaKey)`.

### `goscape-cli rsa` verb (`cmd_rsa.go`)

Sub-verb dispatcher mirroring the existing `jag` verb.

**`rsa gen [--bits N] [--out-dir DIR]`**
- `--bits` default `1024` (TS default); `--out-dir` default `.`.
- `rsa.GenerateKey(rand.Reader, bits)`; write `<out-dir>/private.pem` (PKCS#1, `x509.MarshalPKCS1PrivateKey` → "RSA PRIVATE KEY"; forge/TS-readable) and `<out-dir>/public.pem` (PKIX, `x509.MarshalPKIXPublicKey` → "PUBLIC KEY"; matches `pemtoken`/ondemand `pub_pem`).
- Print the bake-values block (shared helper).
- If `bits/8 > 255` (or modulus byte-len > 254 to leave room for the Java sign byte), print a warning to stderr about the 1-byte length prefix.

**`rsa info [path]`**
- With a path: read+decode the PEM. Private key → print N, E, D; public key (PKIX "PUBLIC KEY") → print N, E. Accept both so an operator can introspect either half.
- With no path: print `protocol.DefaultRSAKey` (the built-in 512-bit values).

**Shared bake-values helper** prints, for a given key:
- `Modulus (N)` in decimal (→ `Client.java` `LOGIN_RSAN`) and hex (→ `rsakey.go`)
- `Public exponent (E)` in decimal (→ `Client.java` `LOGIN_RSAE`) and hex
- `Private exponent (D)` in hex (→ `rsakey.go`), only when known

So `rsa gen` and `rsa info <generated private.pem>` produce identical bake output for the same key.

## Data flow

```
config.yaml: world.rsa_private_key_path: "data/config/private.pem"
        │
        ▼
Config.Validate ── LoadRSAKeyPEM (fail-fast on --config.verify / boot)
        │
        ▼
NewServer ── s.rsaKey = LoadRSAKeyPEM(path)  (else DefaultRSAKey)
        │
        ▼
server.go login case ── req.UnmarshalRSA(r, s.rsaKey) ── RSADec(N, D)
```

Generator path (offline, operator tooling):
```
goscape-cli rsa gen --bits 1024 --out-dir data/config
   ├─ writes private.pem (server: rsa_private_key_path)
   ├─ writes public.pem  (ondemand: pub_pem)
   └─ prints N,E decimal  → paste into Client.java LOGIN_RSAN/LOGIN_RSAE, rebuild client
```

## Error handling

| Condition | Action |
|---|---|
| `rsa_private_key_path` set but file missing/unreadable | `Config.Validate` / `NewServer` returns error → boot aborts (or `--config.verify` fails) |
| PEM undecodable / not an RSA private key | same — descriptive error from `ParseRSAKeyPEM` |
| Loaded modulus exceeds 1-byte wire cap | log a warning; do not hard-fail (operator may have a matching client) |
| `rsa gen` write failure | print to stderr, exit 1 |
| `rsa info` bad/empty PEM | print to stderr, exit 1 |
| Runtime decrypt failure (wrong key vs client) | unchanged: `UnmarshalRSA` error → `sendLoginError(OpClientOutOfDate)` |

## Testing

1. **Loader round-trip** (`rsakey_load_test.go`): generate a key with `crypto/rsa`, marshal to PKCS#1 and PKCS#8 PEM, parse each via `ParseRSAKeyPEM`, assert N/E/D match. Error cases: empty bytes, non-PEM, EC key, public-only PEM.
2. **End-to-end decrypt with a custom key** (`req_test.go`): build a login packet encrypted with a freshly generated key's public half (`RSAEnc` with that key's N/E), decode with `UnmarshalRSA(r, customKey)`, assert username/password/seeds round-trip. Confirms a non-default key works through the real decode path.
3. **Default still works**: existing `UnmarshalBinary` tests pass unchanged (default key path).
4. **`rsa info` of the built-in default**: assert the printed decimal N/E equal the known `Client.java` `LOGIN_RSAN`/`LOGIN_RSAE` constants — guards against accidental key changes.
5. **`rsa gen` round-trip**: run `gen` into a temp dir, then `rsa info` the written `private.pem`, assert identical bake output; assert `public.pem` parses via `pemtoken.Token` (ondemand compatibility).
6. **Config validation**: `Config.Validate` with a bogus path returns an error; with a valid generated PEM returns nil.

## Out of scope

- Unifying the OnDemand `pub_pem` with the world key (kept decoupled per design decision).
- Backporting to the other rev branches (rev-225/244/245.2/254) — trivial to replicate later if desired; this change lands on rev-274 only.
- Key rotation / hot reload — the key is read once at boot, matching TS (`const priv = ...` at module load).
