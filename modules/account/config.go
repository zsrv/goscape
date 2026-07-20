// modules/account/config.go
package account

import (
	"flag"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/util/log"
)

// Account status values (portal_account.status).
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// Seeded portal_group names (migration 000003).
const (
	GroupManuallyApproved = "manually_approved"
	GroupAdmin            = "admin"
)

// knownProviders is the closed set of third-party providers this build
// implements. gate.providers entries must come from it.
var knownProviders = []string{"discord"}

type Config struct {
	LogLevel          *log.Level `yaml:"log_level"` // optional per-module override; nil = inherit global
	Enable            bool       `yaml:"enable"`
	HTTPListenAddress string     `yaml:"http_listen_address"`
	HTTPListenPort    int        `yaml:"http_listen_port"`
	GRPCListenAddress string     `yaml:"grpc_listen_address"`
	GRPCListenPort    int        `yaml:"grpc_listen_port"`
	// PublicURL is the externally-reachable base URL of the portal, used
	// in email links and the OAuth redirect URI. No trailing slash.
	PublicURL      string `yaml:"public_url"`
	CharacterLimit int    `yaml:"character_limit"`
	// AdminToken guards the admin gRPC surface (bearer token in
	// metadata). Empty disables every admin RPC. YAML-only (secret).
	AdminToken string          `yaml:"admin_token"`
	Gate       GateConfig      `yaml:"gate"`
	Argon2     Argon2Config    `yaml:"argon2"`
	Session    SessionConfig   `yaml:"session"`
	SMTP       SMTPConfig      `yaml:"smtp"`
	Providers  ProvidersConfig `yaml:"providers"`
}

// GateConfig controls the character-creation gate. An account may create
// characters iff it is active, email-verified, under the character
// limit, AND (member of manually_approved OR holds a non-revoked
// identity whose provider is listed here). Empty = manual approval only.
type GateConfig struct {
	Providers []string `yaml:"providers"` // YAML-only (list)
}

// Argon2Config parameterizes argon2id password hashing (RFC 9106).
type Argon2Config struct {
	MemoryKiB   int `yaml:"memory_kib"`
	Time        int `yaml:"time"`
	Parallelism int `yaml:"parallelism"`
}

// SessionConfig bounds portal cookie sessions: a session expires
// IdleTTL after last use, and unconditionally AbsoluteTTL after login.
type SessionConfig struct {
	IdleTTL     time.Duration `yaml:"idle_ttl"`
	AbsoluteTTL time.Duration `yaml:"absolute_ttl"`
}

// SMTPConfig configures the mailer. net/smtp negotiates STARTTLS
// automatically when the server advertises it.
type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	From     string `yaml:"from"`
	Username string `yaml:"username"` // YAML-only (secret)
	Password string `yaml:"password"` // YAML-only (secret)
}

type ProvidersConfig struct {
	Discord DiscordConfig `yaml:"discord"`
}

// DiscordConfig holds the OAuth2 app credentials. The URL overrides
// exist for tests (point at an httptest server); empty means the
// official Discord endpoints (see oauth.go).
type DiscordConfig struct {
	ClientID     string `yaml:"client_id"`     // YAML-only (secret-adjacent)
	ClientSecret string `yaml:"client_secret"` // YAML-only (secret)
	AuthURL      string `yaml:"auth_url,omitempty"`
	TokenURL     string `yaml:"token_url,omitempty"`
	APIBase      string `yaml:"api_base,omitempty"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.BoolVar(&c.Enable, "account.enable", false, "Whether to run the account module.")
	f.StringVar(&c.HTTPListenAddress, "account.http-listen-address", "127.0.0.1", "Portal HTTP listen address.")
	f.IntVar(&c.HTTPListenPort, "account.http-listen-port", 8081, "Portal HTTP listen port.")
	f.StringVar(&c.GRPCListenAddress, "account.grpc-listen-address", "127.0.0.1", "AccountService gRPC listen address.")
	f.IntVar(&c.GRPCListenPort, "account.grpc-listen-port", 2005, "AccountService gRPC listen port.")
	f.StringVar(&c.PublicURL, "account.public-url", "", "Externally reachable portal base URL (email links, OAuth redirect). No trailing slash.")
	f.IntVar(&c.CharacterLimit, "account.character-limit", 5, "Maximum characters per portal account.")
	f.IntVar(&c.Argon2.MemoryKiB, "account.argon2-memory-kib", 65536, "argon2id memory cost in KiB.")
	f.IntVar(&c.Argon2.Time, "account.argon2-time", 2, "argon2id time cost (passes).")
	f.IntVar(&c.Argon2.Parallelism, "account.argon2-parallelism", 1, "argon2id parallelism (lanes).")
	f.DurationVar(&c.Session.IdleTTL, "account.session-idle-ttl", 168*time.Hour, "Portal session idle expiry.")
	f.DurationVar(&c.Session.AbsoluteTTL, "account.session-absolute-ttl", 720*time.Hour, "Portal session absolute expiry.")
	f.StringVar(&c.SMTP.Host, "account.smtp-host", "", "SMTP relay host for verification/reset email. Empty disables outbound mail.")
	f.IntVar(&c.SMTP.Port, "account.smtp-port", 587, "SMTP relay port.")
	f.StringVar(&c.SMTP.From, "account.smtp-from", "", "From address for portal email.")

	// YAML-only (no flags): admin_token, gate.providers, smtp
	// credentials, providers.discord.* — lists and secrets follow the
	// ondemand.pub_pem precedent.
	c.Gate.Providers = []string{"discord"}
}

// Validate enforces runtime invariants; errors self-prefix "account: "
// (consumed unwrapped by cmd/goscape/app Config.Validate). Disabled
// module short-circuits.
func (c *Config) Validate() error {
	if !c.Enable {
		return nil
	}
	if c.HTTPListenPort < 1 || c.HTTPListenPort > 65535 {
		return fmt.Errorf("account: http_listen_port must be in [1, 65535], got %d", c.HTTPListenPort)
	}
	if c.GRPCListenPort < 1 || c.GRPCListenPort > 65535 {
		return fmt.Errorf("account: grpc_listen_port must be in [1, 65535], got %d", c.GRPCListenPort)
	}
	if c.PublicURL == "" {
		return fmt.Errorf("account: public_url must be non-empty when account.enable=true")
	}
	if strings.HasSuffix(c.PublicURL, "/") {
		return fmt.Errorf("account: public_url must not end with a trailing slash, got %q", c.PublicURL)
	}
	if c.CharacterLimit < 1 {
		return fmt.Errorf("account: character_limit must be >= 1, got %d", c.CharacterLimit)
	}
	if c.Argon2.MemoryKiB < 8*1024 {
		return fmt.Errorf("account: argon2.memory_kib must be >= 8192, got %d", c.Argon2.MemoryKiB)
	}
	if c.Argon2.Time < 1 {
		return fmt.Errorf("account: argon2.time must be >= 1, got %d", c.Argon2.Time)
	}
	if c.Argon2.Parallelism < 1 {
		return fmt.Errorf("account: argon2.parallelism must be >= 1, got %d", c.Argon2.Parallelism)
	}
	if c.Session.IdleTTL <= 0 || c.Session.AbsoluteTTL <= 0 {
		return fmt.Errorf("account: session TTLs must be > 0, got idle=%v absolute=%v", c.Session.IdleTTL, c.Session.AbsoluteTTL)
	}
	if c.Session.IdleTTL > c.Session.AbsoluteTTL {
		return fmt.Errorf("account: session idle_ttl (%v) must be <= absolute_ttl (%v)", c.Session.IdleTTL, c.Session.AbsoluteTTL)
	}
	for _, p := range c.Gate.Providers {
		if !slices.Contains(knownProviders, p) {
			return fmt.Errorf("account: gate.providers entry %q is not a known provider %v", p, knownProviders)
		}
	}
	return nil
}
