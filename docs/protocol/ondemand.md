# OnDemand

"OnDemand" is how the client fetches cache data — the archives, maps, and loose
files it needs to run. goscape exposes this over **two** surfaces that share one
on-disk cache:

1. an **HTTP server** (the `ondemand` module) serving archive and bootstrap
   routes, and
2. a **TCP request channel** reached through the world server's login-state
   opcode `15`.

This page documents both at wire revision {{ revision }}, from
`modules/ondemand/` and `modules/world/ondemand.go`, plus the cache layout
(`pkg/io/filestream/`) they read from.

## HTTP surface

The HTTP OnDemand server is a `pkg/dskit/server` HTTP mux. Routes are registered
in `cmd/goscape/app/modules.go` (the module wiring) and dispatched by
`modules/ondemand/handler.go`. It listens on `ondemand.http-listen-address`
(default `127.0.0.1`).

### The `GET /` catch-all

A single `GET /` handler owns the root and internally dispatches by path. When
the WebSocket bridge is enabled *and* a world connection handler is wired,
`WebSocketHandler` takes `GET /`; otherwise `RootHandler` does. `WebSocketHandler`
upgrades only requests carrying `Upgrade: websocket` (subject to an origin
allowlist) and bridges the binary frames into the world server's TCP connection
handler — a browser client speaks the same RS2 framing over the WebSocket as a
native client does over TCP. Any non-upgrade `GET /` request falls through to
`RootHandler`, so the static dispatch below is reached either way.

`RootHandler` checks these in order:

| Path (prefix) | Serves | Content-Type |
|---|---|---|
| `/crc` | The archive CRC table (see below) | `application/octet-stream` |
| `/title` | Archive 0, file 1 | `application/octet-stream` |
| `/config` | Archive 0, file 2 | `application/octet-stream` |
| `/interface` | Archive 0, file 3 | `application/octet-stream` |
| `/media` | Archive 0, file 4 | `application/octet-stream` |
| `/versionlist` | Archive 0, file 5 | `application/octet-stream` |
| `/textures` | Archive 0, file 6 | `application/octet-stream` |
| `/wordenc` | Archive 0, file 7 | `application/octet-stream` |
| `/sounds` | Archive 0, file 8 | `application/octet-stream` |
| `/maps/{name}` | A per-zone map/loc file from `data/pack/client/maps/{name}` | `application/octet-stream` |
| `/rs2.cgi` | The client bootstrap HTML page (see below) | `text/html` |
| *(fallback)* | A static file from the configured `public_dir`, if any | mapped by extension |
| *(no match)* | `404 Not Found` | — |

Notes:

- The eight archive routes each serve `cache.Read(0, N)` from the module's
  read-only cache; a missing file returns **404**.
- `/maps/{name}` only accepts names matching `^[ml]\d+_\d+$` (the `m{x}_{z}` /
  `l{x}_{z}` cache-key convention); anything else is 404, which also blocks path
  traversal out of the maps directory.
- The `/rs2.cgi` handler renders a JS/WebSocket client bootstrap by default, or
  the Java-applet bootstrap when `plugin=1` **and** debug mode is on. It fills in
  node id, low-memory, members, and (for the applet) a port offset of
  `world port − 43594`.
- On rev-{{ revision }} the older `/ondemand.zip` and `/build` static routes were
  dropped upstream and are **not** served.

### `/crc`

`/crc` returns the serialized archive CRC table — at rev-{{ revision }} a fixed
**40-byte** blob: nine 4-byte big-endian CRC-32 slots (archive-0 files) followed
by a 4-byte big-endian rolling-hash trailer (`hash = 1234; for i in 0..9: hash =
(hash<<1) + crc[i]`). This is the same table the login handshake validates the
client's nine archive checksums against (see
[login](login.md#cleartext-header)) — the client fetches it to know whether its
cached archives are current. It is built by `cache.MakeCRCs`
(`pkg/cache/crctable.go`).

### Health routes

Two operational routes are registered alongside the OnDemand handlers
(`modules/ondemand/health.go`):

| Path | Method | Serves |
|---|---|---|
| `/healthz` | GET | Readiness probe: `200` when ticking, `503` during a slow cold boot |
| `/debug/status` | GET | JSON snapshot (tick age, current tick, players online, tick ms) |

These are goscape operational endpoints, not part of the RS2 client protocol.

## TCP OnDemand channel (op 15)

A native client can also stream cache files over the TCP game socket. In the
world server's login state, opcode `15` (opcode byte only, no payload)
transitions the connection to the OnDemand state and replies with 8 zero bytes
(`server_login.go`). From then on the connection's bytes are routed to the
OnDemand request pump (`modules/world/ondemand.go`).

Requests are **4-byte frames** (`onClientData`):

| Byte(s) | Field | Encoding |
|---|---|---|
| 0 | archive | `G1` |
| 1–2 | file | `G2`, big-endian |
| 3 | priority | `G1` |

A frame with `archive` outside `[0, 3]` or `priority` outside `[0, 2]` closes the
connection. The server enqueues each valid request per-client and a pump
goroutine serves the requested cache file back, prioritised and round-robined
across clients. Partial trailing frames (fewer than 4 bytes) are retained for the
next read.

## Cache layout

Both surfaces read the same RS2 client cache via `pkg/io/filestream` — the
classic dat/idx store rooted at `ondemand.cache-path` (default `./data/pack`):

- **`main_file_cache.dat`** — the data file, addressed in **520-byte sectors**
  (an 8-byte sector header plus up to 512 payload bytes).
- **`main_file_cache.idx0` … `main_file_cache.idx4`** — five index files; each
  6-byte entry is `size:3` + `firstSector:3`, both big-endian.

Archive index `0` is the JAG "config" archive that holds the eight files the
HTTP routes expose (`title`=1, `config`=2, `interface`=3, `media`=4,
`versionlist`=5, `textures`=6, `wordenc`=7, `sounds`=8). Archive index `0`
file `0` is conventionally absent, so its CRC slot is zero.

The `/maps/` HTTP route is separate from the dat/idx store: it serves loose
per-zone files from `data/pack/client/maps/`.

### What the client fetches, and how

The route/frame names above map directly to the client's cache needs:

- Over **HTTP**, a JS/WebSocket client fetches `/crc` to check freshness, then
  the archive-0 routes (`/title`, `/config`, …) and `/maps/{name}` for map/loc
  data.
- Over the **TCP OnDemand channel** (op 15), a native client streams cache files
  by `(archive, file)` with a priority, which is how it pulls in on-demand assets
  (models, animations, and other content) after the initial connection.

(These client-side behaviours are described from the server routes that serve
them and the porting comments in the handlers, which cite the upstream
`web.ts` / `OnDemandThread.ts` sources; the authoritative wire facts here are the
routes and frame layout the Go code implements.)
