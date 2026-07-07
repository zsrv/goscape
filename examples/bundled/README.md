# Bundled example config

The goscape game server is self-contained: the `login` and `friends` modules
persist to local SQLite, and the `ondemand` (HTTP) and `world` (TCP) modules are
plain network listeners. **No external services are required to run it.**

## Quick start

Requirements: Go ≥ 1.26.

```bash
CGO_ENABLED=0 go run -trimpath ./cmd/goscape \
  --config.file examples/bundled/goscape.yaml          # start the server

bash examples/bundled/scripts/fake-login.sh            # trigger one login (needs grpcurl)
```

`goscape.yaml` here is a minimal preset (ondemand / login / friends / world). For the
full set of options — every key at its default, with descriptions — see
`examples/full-config-reference.yaml`. Verify a config without starting the server:

```bash
go run ./cmd/goscape --config.file examples/bundled/goscape.yaml --config.verify
```

## Services & ports

| Port | Module | Notes |
|------|--------|-------|
| 8080 | ondemand | HTTP OnDemand server |
| 2004 | login  | gRPC login service |
| 2005 | friends | gRPC friends service |
| 43594 | world | TCP game server |

## Data

- `data/goscape.db` — the central SQLite database (created on first run; login and friends are both clients of it)
- `data/players/` — player save files
