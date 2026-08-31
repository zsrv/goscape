# Security policy

## Reporting a vulnerability

Report privately through GitHub: **Security → Report a vulnerability** on this
repository. Please do not open a public issue for anything exploitable against a
running server.

Include the revision branch you are on (`rev-225` … `rev-274`), since the ports
share most of their code but not all of it.

## Supported branches

Each `rev-N` branch is a complete, self-contained port of one game revision, and
all of them are maintained. Fixes normally land on the newest branch (`rev-274`)
and are backported to the others where the code is shared. `main` carries
documentation only.

## Trust model

**This server assumes a trusted network between its own modules.** Only some of
its listeners are meant to face the internet. Getting this wrong is the most
likely way to be compromised, so it is written out here rather than left implied.

| Listener | Default port | Exposure |
|---|---|---|
| `world` TCP | 43594 | Public — this is the game |
| `ondemand` HTTP | 8080 | Public — serves the game cache |
| `hiscore` HTTP | 8082 | Public — read-only JSON, coarse rate limit |
| `account` portal HTTP | 8081 | Public when enabled — the player portal |
| `login` gRPC | 2004 | **Internal only** |
| `friends` gRPC | 2005 | **Internal only** |
| `account` gRPC | 2006 | **Internal only** |

`login` and `friends` have **no authentication and no transport security**. Any
client that can reach those ports can do everything a world node can do: read and
modify accounts, saves, and friends data. `login` also registers gRPC server
reflection, so its full schema is discoverable.

`account`'s gRPC surface authenticates its admin RPCs with a bearer token
(`account.admin_token`; when empty, every admin RPC is refused). `VerifyGameLogin`
is deliberately exempt, because the login module calls it on every game login —
so anyone who can reach port 2006 can test passwords against it directly.

Every module defaults to binding `127.0.0.1`. A single-host deployment running
`--target all` is therefore closed by default, and it stays that way as long as
you do not move those three gRPC listeners onto a public address. If you split
modules across hosts, put them on a private network or a VPN; there is no
in-protocol authentication to fall back on.

### Kubernetes

The Helm chart binds every module to `0.0.0.0` inside the pod, which is normal —
the pod boundary is the isolation. But `networkPolicy.enabled` defaults to
`false`, so on a cluster with no default-deny policy, any pod can reach the
login/friends gRPC ports. Set `networkPolicy.enabled=true` in `Management` mode,
and see the chart README for what the policy does and does not cover.

## Known weak defaults

- **The built-in login RSA key is public.** `pkg/io/protocol/rsakey.go` compiles
  in a 512-bit key whose private exponent is in this repository, because the
  matching public key is baked into the stock Java client. Anyone can decrypt the
  login block of a server using it. Generate your own with `goscape-cli rsa gen`,
  point `world.rsa_private_key_path` at it, and rebuild your client with the
  matching public key.
- **`login.auth_mode: local` hashes with bcrypt** and auto-registers accounts on
  first login by default (`login.auto_register`). The portal path
  (`auth_mode: account`) uses argon2id and does not auto-register.
- **`ondemand.debug_status_enabled`** exposes tick and player-count information
  when enabled. It defaults to off; leave it off on a public server.
- **`hiscore.trust_gateway_headers`** makes the module believe `X-Consumer-*`
  request headers. Enable it only behind a gateway that sets them, or callers can
  mint a fresh rate-limit bucket per request.

## Scope

Bugs in this server's own handling of untrusted input — the RS2 protocol, HTTP
endpoints, the portal, RuneScript execution — are in scope. Exposing an
internal-only listener to the internet, as described above, is a deployment
choice rather than a vulnerability in the code.
