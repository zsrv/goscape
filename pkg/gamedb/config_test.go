package gamedb

import (
	"flag"
	"strings"
	"testing"
)

func defaultConfig() Config {
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	return c
}

func TestConfig_Defaults(t *testing.T) {
	c := defaultConfig()
	if c.Backend != BackendSQLite {
		t.Errorf("Backend: got %q, want %q", c.Backend, BackendSQLite)
	}
	if c.SQLite.DSN != "data/goscape.db" {
		t.Errorf("SQLite.DSN: got %q, want data/goscape.db", c.SQLite.DSN)
	}
	if c.Postgres.MaxOpenConns != 8 {
		t.Errorf("Postgres.MaxOpenConns: got %d, want 8", c.Postgres.MaxOpenConns)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate on defaults: %v", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string // "" = valid
	}{
		{"defaults valid", func(c *Config) {}, ""},
		{"unknown backend", func(c *Config) { c.Backend = "mysql" }, "database: backend"},
		{"empty sqlite dsn", func(c *Config) { c.SQLite.DSN = "" }, "database: sqlite.dsn"},
		{"postgres without dsn", func(c *Config) { c.Backend = BackendPostgres }, "postgres.dsn"},
		{"postgres with dsn valid", func(c *Config) {
			c.Backend = BackendPostgres
			c.Postgres.DSN = "postgres://u:p@localhost:5432/goscape?sslmode=disable"
		}, ""},
		{"postgres bad pool size", func(c *Config) {
			c.Backend = BackendPostgres
			c.Postgres.DSN = "postgres://u:p@localhost:5432/goscape"
			c.Postgres.MaxOpenConns = 0
		}, "max_open_conns"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := defaultConfig()
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: unexpected error %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate: got %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
