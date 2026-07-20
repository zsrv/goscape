package account

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"sync"
)

//go:embed templates static
var assetsFS embed.FS

// portal is the SSR web application. Handlers hang off this struct and
// are registered in routes(); later tasks add session middleware and
// the remaining page handlers in sibling files.
type portal struct {
	cfg      Config
	store    *Store
	mailer   Mailer
	log      *slog.Logger
	pages    map[string]*template.Template
	rl       *rateLimiter
	dummyPHC string         // anti-enumeration timing pad; see handleLogin
	mailWG   sync.WaitGroup // tracks fire-and-forget outbound-mail goroutines so tests and shutdown can wait for them
	disc     *discordClient
}

type pageData struct {
	Account *PortalAccount // nil when unauthenticated
	CSRF    string         // per-session CSRF token, "" when no session
	Msg     string         // flash message from ?msg= query param
	Data    any            // page-specific payload
}

func newPortal(cfg Config, store *Store, mailer Mailer, log *slog.Logger) (*portal, error) {
	pageFiles, err := fs.Glob(assetsFS, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob pages: %w", err)
	}
	pages := make(map[string]*template.Template, len(pageFiles))
	for _, f := range pageFiles {
		t, err := template.ParseFS(assetsFS, "templates/base.html", f)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		pages[path.Base(f)] = t
	}
	// Precompute a dummy password hash once, using the same Argon2 params
	// as real account hashes, so the unknown-email login path can burn an
	// equivalent amount of CPU time to the wrong-password path (anti
	// account-enumeration timing pad; see handleLogin).
	dummy, err := HashPassword("goscape-dummy-timing-pad", cfg.Argon2)
	if err != nil {
		return nil, fmt.Errorf("dummy hash: %w", err)
	}
	return &portal{
		cfg: cfg, store: store, mailer: mailer, log: log,
		pages:    pages,
		rl:       newRateLimiter(),
		dummyPHC: dummy,
		disc:     newDiscordClient(cfg.Providers.Discord),
	}, nil
}

// render executes a page template inside base.html. Errors after the
// header is written are unrecoverable, so pages render to a buffer
// first.
func (p *portal) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	pd := pageData{Account: ctxAccount(r), Msg: r.URL.Query().Get("msg"), Data: data}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		pd.CSRF = csrfToken(c.Value)
	}
	tmpl, ok := p.pages[page]
	if !ok {
		p.log.Error("unknown page template", slog.String("page", page))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", pd); err != nil {
		p.log.Error("render failed", slog.String("page", page), slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func (p *portal) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.FileServerFS(assetsFS))
	mux.HandleFunc("GET /{$}", p.public(p.handleHome))
	mux.HandleFunc("GET /register", p.public(p.handleRegisterForm))
	mux.HandleFunc("POST /register", p.public(p.handleRegister))
	mux.HandleFunc("GET /login", p.public(p.handleLoginForm))
	mux.HandleFunc("POST /login", p.public(p.handleLogin))
	mux.HandleFunc("POST /logout", p.authed(p.handleLogout))
	mux.HandleFunc("GET /verify-email", p.public(p.handleVerifyEmail))
	mux.HandleFunc("POST /resend-verification", p.authed(p.handleResendVerification))
	mux.HandleFunc("GET /forgot-password", p.public(p.handleForgotForm))
	mux.HandleFunc("POST /forgot-password", p.public(p.handleForgot))
	mux.HandleFunc("GET /reset-password", p.public(p.handleResetForm))
	mux.HandleFunc("POST /reset-password", p.public(p.handleReset))
	mux.HandleFunc("GET /link/discord", p.authed(p.handleLinkDiscord))
	mux.HandleFunc("GET /oauth/discord/callback", p.authed(p.handleDiscordCallback))
	mux.HandleFunc("GET /dashboard", p.authed(p.handleDashboard))
	mux.HandleFunc("GET /characters/new", p.authed(p.handleCharacterForm))
	mux.HandleFunc("POST /characters/new", p.authed(p.handleCharacterCreate))
	mux.HandleFunc("GET /settings/password", p.authed(p.handleSettingsForm))
	mux.HandleFunc("POST /settings/password", p.authed(p.handleSettingsPassword))
	mux.HandleFunc("GET /admin", p.admin(p.handleAdminSearch))
	mux.HandleFunc("GET /admin/accounts/{id}", p.admin(p.handleAdminAccount))
	mux.HandleFunc("GET /admin/audit", p.admin(p.handleAdminAudit))
	mux.HandleFunc("POST /admin/accounts/{id}/group", p.admin(p.adminAction("group.set",
		func(r *http.Request, target *PortalAccount) (string, error) {
			group := r.FormValue("group")
			if group != GroupManuallyApproved && group != GroupAdmin {
				return "", fmt.Errorf("unknown group %q", group)
			}
			member := r.FormValue("member") == "on"
			admin := ctxAccount(r)
			if member {
				return group + "=true", p.store.AddGroupMember(r.Context(), group, target.ID, admin.ID)
			}
			return group + "=false", p.store.RemoveGroupMember(r.Context(), group, target.ID)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/status", p.admin(p.adminAction("account.status",
		func(r *http.Request, target *PortalAccount) (string, error) {
			status := r.FormValue("status")
			if err := p.store.SetAccountStatus(r.Context(), target.ID, status); err != nil {
				return "", err
			}
			if status == StatusDisabled {
				return status, p.store.DeleteAccountSessions(r.Context(), target.ID)
			}
			return status, nil
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/unlink", p.admin(p.adminAction("identity.unlink",
		func(r *http.Request, target *PortalAccount) (string, error) {
			provider := r.FormValue("provider")
			return provider, p.store.RevokeIdentity(r.Context(), target.ID, provider)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/release", p.admin(p.adminAction("identity.release",
		func(r *http.Request, target *PortalAccount) (string, error) {
			provider, uid := r.FormValue("provider"), r.FormValue("provider_user_id")
			return provider + ":" + uid, p.store.ReleaseIdentity(r.Context(), provider, uid)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/resend-verification", p.admin(p.adminAction("account.resend_verification",
		func(r *http.Request, target *PortalAccount) (string, error) {
			return "", p.sendVerificationEmail(r.Context(), target)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/send-reset", p.admin(p.adminAction("account.reset_password",
		func(r *http.Request, target *PortalAccount) (string, error) {
			return "reset link mailed", p.sendResetEmail(r.Context(), target)
		})))
	return mux
}

func (p *portal) handleHome(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "home.html", nil)
}
