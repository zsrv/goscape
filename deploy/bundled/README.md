# Bundled example config

The goscape game server is self-contained: the `login` and `friends` modules
persist to local SQLite, and the `ondemand` (HTTP) and `world` (TCP) modules are
plain network listeners. **No external services are required to run it.**

## Quick start

Requirements: Go ≥ 1.23.

```bash
CGO_ENABLED=0 go run -trimpath ./cmd/goscape \
  --config.file deploy/bundled/goscape.yaml          # start the server

bash deploy/bundled/scripts/fake-login.sh            # trigger one login (needs grpcurl)
```

`goscape.yaml` here is a minimal preset (ondemand / login / friends / world). Verify
a config without starting the server:

```bash
go run ./cmd/goscape --config.file deploy/bundled/goscape.yaml --config.verify
```

## Services & ports

| Port | Module | Notes |
|------|--------|-------|
| 8888 | ondemand | HTTP OnDemand server |
| 2004 | login  | gRPC login service |
| (world) | world | TCP game server (port from world config) |

## Data

- `dataplayers/login.db` — login SQLite store (created on first run)
- `dataplayers/players/` — player save files
- `data/friends.db` — friends SQLite store
