package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Portal abuse limits (fixed windows, per process).
const (
	registerLimit  = 5
	registerWindow = time.Hour
	loginLimit     = 10
	loginWindow    = 5 * time.Minute
	mailLimit      = 3
	mailWindow     = time.Hour
)

func validEmail(email string) bool {
	email = NormalizeEmail(email)
	at := strings.IndexByte(email, '@')
	return len(email) <= 255 && at > 0 && at < len(email)-1 && !strings.ContainsAny(email, " \t\r\n")
}

// sendVerificationEmail mints a 24h verify_email token and mails the
// link. Callers decide whether a failure is fatal (registration keeps
// the account and lets the player resend).
func (p *portal) sendVerificationEmail(ctx context.Context, acct *PortalAccount) error {
	raw, err := NewRawToken()
	if err != nil {
		return err
	}
	if err := p.store.CreateToken(ctx, acct.ID, TokenPurposeVerifyEmail, HashToken(raw), 24*time.Hour); err != nil {
		return err
	}
	link := p.cfg.PublicURL + "/verify-email?token=" + raw
	body := fmt.Sprintf("Welcome to goscape!\r\n\r\nVerify your email address by opening:\r\n\r\n%s\r\n\r\nThe link expires in 24 hours. If you didn't register, ignore this mail.\r\n", link)
	return p.mailer.Send(acct.Email, "Verify your goscape account", body)
}

func (p *portal) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "register.html", nil)
}

func (p *portal) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !p.rl.allow("register:"+clientIP(r), registerLimit, registerWindow) {
		http.Error(w, "too many registrations from this address — try again later", http.StatusTooManyRequests)
		return
	}
	fail := func(msg string) { p.render(w, r, "register.html", msg) } // .Data doubles as the error line (class="error")
	email := r.FormValue("email")
	password := r.FormValue("password")
	if !validEmail(email) {
		fail("error: that email address doesn't look valid")
		return
	}
	if password != r.FormValue("password2") {
		fail("error: the passwords don't match")
		return
	}
	if err := ValidPortalPassword(password); err != nil {
		fail("error: " + err.Error())
		return
	}
	phc, err := HashPassword(password, p.cfg.Argon2)
	if err != nil {
		p.log.Error("hash failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	id, err := p.store.CreateAccount(r.Context(), email, phc)
	if errors.Is(err, ErrEmailTaken) {
		fail("error: that email is already registered — try logging in or resetting your password")
		return
	}
	if err != nil {
		p.log.Error("create account failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.AppendAudit(r.Context(), 0, "account.register", fmt.Sprintf("account:%d", id), "ip="+clientIP(r)); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	acct, err := p.store.AccountByID(r.Context(), id)
	if err == nil {
		if err := p.sendVerificationEmail(r.Context(), acct); err != nil {
			p.log.Warn("verification mail failed", slog.Any("err", err))
			http.Redirect(w, r, "/login?msg=Account+created,+but+the+verification+email+failed+to+send.+Log+in+and+use+Resend.", http.StatusFound)
			return
		}
	} else {
		p.log.Warn("post-register account lookup failed; verification mail not sent", slog.Any("err", err))
		http.Redirect(w, r, "/login?msg=Account+created,+but+the+verification+email+failed+to+send.+Log+in+and+use+Resend.", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login?msg=Account+created.+Check+your+email+for+a+verification+link.", http.StatusFound)
}

func (p *portal) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "login.html", nil)
}

func (p *portal) handleLogin(w http.ResponseWriter, r *http.Request) {
	email := NormalizeEmail(r.FormValue("email"))
	if !p.rl.allow("login:"+clientIP(r)+":"+email, loginLimit, loginWindow) {
		http.Error(w, "too many login attempts — try again later", http.StatusTooManyRequests)
		return
	}
	fail := func(msg string) { p.render(w, r, "login.html", msg) }
	acct, err := p.store.AccountByEmail(r.Context(), email)
	if errors.Is(err, ErrNotFound) {
		// Anti-enumeration timing pad: burn the same argon2id cost that
		// the wrong-password path below pays via VerifyPassword, so an
		// unknown email can't be distinguished from a wrong password by
		// response latency.
		_, _ = VerifyPassword(r.FormValue("password"), p.dummyPHC)
		fail("error: unknown email or wrong password")
		return
	}
	if err != nil {
		p.log.Error("login lookup failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ok, err := VerifyPassword(r.FormValue("password"), acct.PasswordHash)
	if err != nil || !ok {
		fail("error: unknown email or wrong password")
		return
	}
	if acct.Status != StatusActive {
		fail("error: this account is disabled — contact an admin")
		return
	}
	raw, err := NewRawToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.CreateSession(r.Context(), acct.ID, HashToken(raw), clientIP(r), r.UserAgent(), p.cfg.Session); err != nil {
		p.log.Error("create session failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.setSessionCookie(w, raw)
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (p *portal) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = p.store.DeleteSession(r.Context(), HashToken(c.Value))
	}
	p.clearSessionCookie(w)
	http.Redirect(w, r, "/?msg=Logged+out", http.StatusFound)
}

func (p *portal) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("token")
	if raw == "" {
		p.render(w, r, "message.html", "That verification link is invalid or expired.")
		return
	}
	accountID, err := p.store.ConsumeToken(r.Context(), TokenPurposeVerifyEmail, HashToken(raw))
	if errors.Is(err, ErrNotFound) {
		p.render(w, r, "message.html", "That verification link is invalid or expired.")
		return
	}
	if err != nil {
		p.log.Error("verify token failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.SetEmailVerified(r.Context(), accountID); err != nil {
		p.log.Error("set verified failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.render(w, r, "message.html", "Your email is verified. You can now create characters (once your account passes the gate).")
}

func (p *portal) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	if acct.EmailVerified {
		http.Redirect(w, r, "/dashboard?msg=Your+email+is+already+verified", http.StatusFound)
		return
	}
	if !p.rl.allow("mail:"+acct.Email, mailLimit, mailWindow) {
		http.Error(w, "too many emails requested — try again later", http.StatusTooManyRequests)
		return
	}
	if err := p.sendVerificationEmail(r.Context(), acct); err != nil {
		p.log.Warn("resend failed", slog.Any("err", err))
		http.Redirect(w, r, "/dashboard?msg=Sending+failed+—+try+again+later", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/dashboard?msg=Verification+email+sent", http.StatusFound)
}

type resetPageData struct {
	Token string
	Error string
}

// sendResetEmail mints a 1h reset_password token and mails the link.
// Shared by the self-service forgot flow and the admin "send reset"
// action (Task 20).
func (p *portal) sendResetEmail(ctx context.Context, acct *PortalAccount) error {
	raw, err := NewRawToken()
	if err != nil {
		return err
	}
	if err := p.store.CreateToken(ctx, acct.ID, TokenPurposeResetPassword, HashToken(raw), time.Hour); err != nil {
		return err
	}
	link := p.cfg.PublicURL + "/reset-password?token=" + raw
	body := fmt.Sprintf("A password reset was requested for your goscape account.\r\n\r\nSet a new password here:\r\n\r\n%s\r\n\r\nThe link expires in 1 hour and can be used once. If you didn't request this, ignore this mail.\r\n", link)
	return p.mailer.Send(acct.Email, "Reset your goscape password", body)
}

func (p *portal) handleForgotForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "forgot.html", nil)
}

func (p *portal) handleForgot(w http.ResponseWriter, r *http.Request) {
	if !p.rl.allow("mail:"+clientIP(r), mailLimit, mailWindow) {
		http.Error(w, "too many emails requested — try again later", http.StatusTooManyRequests)
		return
	}
	// Enumeration safety: identical response whether or not the account
	// exists, and the token mint + SMTP send happen OFF the request
	// path — a synchronous network send only on the known-email branch
	// would be a timing oracle (same attack class the login dummyPHC
	// pad closes). The goroutine uses its own context: the request's
	// dies as soon as we render below.
	if acct, err := p.store.AccountByEmail(r.Context(), r.FormValue("email")); err == nil {
		p.mailWG.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := p.sendResetEmail(ctx, acct); err != nil {
				p.log.Warn("reset mail failed", slog.Any("err", err))
			}
		})
	}
	p.render(w, r, "message.html", "If that email has an account, a reset link is on its way.")
}

func (p *portal) handleResetForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "reset.html", resetPageData{Token: r.URL.Query().Get("token")})
}

func (p *portal) handleReset(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	password := r.FormValue("password")
	fail := func(msg string) {
		p.render(w, r, "reset.html", resetPageData{Token: token, Error: msg})
	}
	// Validate the new password BEFORE consuming the single-use token,
	// so a policy failure doesn't burn the link.
	if password != r.FormValue("password2") {
		fail("the passwords don't match")
		return
	}
	if err := ValidPortalPassword(password); err != nil {
		fail(err.Error())
		return
	}
	accountID, err := p.store.ConsumeToken(r.Context(), TokenPurposeResetPassword, HashToken(token))
	if errors.Is(err, ErrNotFound) {
		p.render(w, r, "message.html", "That reset link is invalid or expired.")
		return
	}
	if err != nil {
		p.log.Error("reset consume failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	phc, err := HashPassword(password, p.cfg.Argon2)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.SetPasswordHash(r.Context(), accountID, phc); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.DeleteAccountSessions(r.Context(), accountID); err != nil {
		p.log.Warn("session sweep failed", slog.Any("err", err))
	}
	if err := p.store.AppendAudit(r.Context(), 0, "account.password_reset", fmt.Sprintf("account:%d", accountID), "self-service"); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	http.Redirect(w, r, "/login?msg=Password+changed.+Log+in+with+your+new+password.", http.StatusFound)
}
