# Login DB Migrations Design

**Date:** 2026-04-17  
**Status:** Approved  
**Scope:** `modules/login/` — adding golang-migrate with embedded SQL files

---

## Context

The Go login server (`modules/login/`) already has a working `database/sql` + `modernc.org/sqlite` layer. The schema is only defined inline in `db_test.go` as a `testSchema` constant. There is no mechanism to create the schema in production — this design adds one.

The Go schema has intentionally diverged from the TypeScript Prisma schema (different column names, extra `node_id` column in `account_login`, redesigned `session` table). This is a fresh start; no migration path from the TS schema is required.

### DB-per-microservice split (confirmed)

| Service | Tables | Status |
|---|---|---|
| Login | `account`, `account_login`, `session`, `ipban` | Exists in Go |
| Friend | `friendlist`, `ignorelist`, `private_chat` | Future |
| Logger/Audit | `session_log`, `session_wealth`, `report`, `input_report`, `public_chat` | Future |

The friend server queries `account` by username in TS via shared DB. In the Go split it will call the login server's gRPC API to resolve usernames → account IDs instead.

---

## Approach: golang-migrate + embedded SQL

**Tool:** `github.com/golang-migrate/migrate/v4`  
**Source:** `golang-migrate/migrate/v4/source/iofs` (bridges `embed.FS`)  
**Driver:** `golang-migrate/migrate/v4/database/sqlite` (uses `modernc.org/sqlite`, CGO-free)

The `sqlite3` database adapter requires CGO and is explicitly excluded. The `sqlite` adapter works with the existing `modernc.org/sqlite` driver already in `go.mod`, keeping `CGO_ENABLED=0` builds intact.

---

## File Layout

```
modules/login/
  migrations/
    000001_init.up.sql   ← current Go schema
  db.go                  ← embed directive + migrateDB()
  db_test.go             ← testSchema removed; createTestDB uses openDB
```

Migration files follow `{version}_{title}.up.sql` naming. Only `.up.sql` files are included — `.down.sql` rollbacks are omitted. SQLite's limited ALTER TABLE support makes DDL rollbacks impractical, and a forward-only migration policy is correct for a game server.

---

## Migration Runner

`openDB` calls `migrateDB` after setting SQLite pragmas. On failure the DB is closed and an error is returned, preventing the service from starting with an uninitialized schema.

```go
//go:embed migrations/*.sql
var migrations embed.FS

func openDB(dsn string) (*sql.DB, error) {
    db, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, fmt.Errorf("open sqlite: %w", err)
    }
    if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
        db.Close()
        return nil, fmt.Errorf("sqlite pragma journal_mode: %w", err)
    }
    if _, err = db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
        db.Close()
        return nil, fmt.Errorf("sqlite pragma foreign_keys: %w", err)
    }
    if err = migrateDB(db); err != nil {
        db.Close()
        return nil, fmt.Errorf("migrate: %w", err)
    }
    return db, nil
}
```

`migrateDB` uses golang-migrate's `WithInstance` API to reuse the existing `*sql.DB` (no second connection opened):

```go
func migrateDB(db *sql.DB) error {
    src, err := iofs.New(migrations, "migrations")
    if err != nil {
        return fmt.Errorf("iofs source: %w", err)
    }
    drv, err := sqlitedriver.WithInstance(db, &sqlitedriver.Config{})
    if err != nil {
        return fmt.Errorf("sqlite driver: %w", err)
    }
    m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
    if err != nil {
        return fmt.Errorf("migrate instance: %w", err)
    }
    if err = m.Up(); errors.Is(err, migrate.ErrNoChange) {
        return nil
    }
    return err
}
```

golang-migrate owns a `schema_migrations` table (version int + dirty bool). If a migration fails mid-run, the table is marked dirty and the service refuses to start on the next run — the correct failure mode for a stateful game server.

---

## Initial Migration: `000001_init.up.sql`

Captures the current Go schema. Uses plain `CREATE TABLE` (not `IF NOT EXISTS`) because golang-migrate's version tracking makes idempotent guards unnecessary and they would mask bugs.

```sql
CREATE TABLE account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    registration_ip TEXT NOT NULL DEFAULT '',
    staff_mod_level INTEGER NOT NULL DEFAULT 0,
    members INTEGER NOT NULL DEFAULT 0,
    banned_until TEXT,
    muted_until TEXT,
    logout_time TEXT
);

CREATE TABLE account_login (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    node_id INTEGER NOT NULL DEFAULT 0,
    logged_in INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, profile)
);

CREATE TABLE session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL,
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time TEXT NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

CREATE TABLE ipban (
    ip TEXT NOT NULL PRIMARY KEY,
    added_by TEXT NOT NULL DEFAULT '',
    added_on TEXT NOT NULL DEFAULT ''
);
```

---

## Test Update

`testSchema` in `db_test.go` is deleted. `createTestDB` calls `openDB` directly with a unique in-memory DSN:

```go
func createTestDB(t *testing.T) *sql.DB {
    t.Helper()
    dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
    db, err := openDB(dsn)
    if err != nil {
        t.Fatalf("createTestDB: %v", err)
    }
    t.Cleanup(func() { db.Close() })
    return db
}
```

Tests now run against the real schema, making them a true regression check against schema changes.

---

## Dependencies to Add

```
github.com/golang-migrate/migrate/v4
```

Sub-packages used (no separate `go get` needed, resolved transitively):
- `github.com/golang-migrate/migrate/v4/source/iofs`
- `github.com/golang-migrate/migrate/v4/database/sqlite`
