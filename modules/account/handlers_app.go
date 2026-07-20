package account

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const oauthStateCookie = "goscape_oauth_state"

func (p *portal) discordRedirectURI() string {
	return p.cfg.PublicURL + "/oauth/discord/callback"
}

// handleLinkDiscord starts the OAuth dance: random state bound to the
// browser via a short-lived cookie, then redirect to Discord.
func (p *portal) handleLinkDiscord(w http.ResponseWriter, r *http.Request) {
	if !p.disc.configured() {
		http.NotFound(w, r)
		return
	}
	state, err := NewRawToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: state, Path: "/oauth/",
		MaxAge: int((10 * time.Minute) / time.Second), HttpOnly: true,
		Secure: strings.HasPrefix(p.cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, p.disc.authorizeURL(state, p.discordRedirectURI()), http.StatusFound)
}

func (p *portal) handleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	if !p.disc.configured() {
		http.NotFound(w, r)
		return
	}
	acct := ctxAccount(r)
	c, err := r.Cookie(oauthStateCookie)
	if err != nil || c.Value == "" || r.URL.Query().Get("state") != c.Value {
		http.Error(w, "oauth state mismatch — restart the linking flow", http.StatusForbidden)
		return
	}
	// One-shot state: clear immediately.
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/oauth/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		// Player denied the authorization on Discord's side.
		http.Redirect(w, r, "/dashboard?msg=Discord+linking+was+cancelled", http.StatusFound)
		return
	}
	token, err := p.disc.exchangeCode(r.Context(), code, p.discordRedirectURI())
	if err != nil {
		p.log.Warn("discord exchange failed", slog.Any("err", err))
		http.Redirect(w, r, "/dashboard?msg=Discord+linking+failed+—+try+again", http.StatusFound)
		return
	}
	discordID, discordName, err := p.disc.identify(r.Context(), token)
	if err != nil {
		p.log.Warn("discord identify failed", slog.Any("err", err))
		http.Redirect(w, r, "/dashboard?msg=Discord+linking+failed+—+try+again", http.StatusFound)
		return
	}
	err = p.store.LinkIdentity(r.Context(), acct.ID, "discord", discordID, discordName)
	switch {
	case errors.Is(err, ErrIdentityTaken):
		http.Redirect(w, r, "/dashboard?msg=That+Discord+account+is+already+linked+to+a+different+account", http.StatusFound)
		return
	case errors.Is(err, ErrAlreadyLinked):
		http.Redirect(w, r, "/dashboard?msg=Your+account+already+has+a+linked+Discord", http.StatusFound)
		return
	case err != nil:
		p.log.Error("link identity failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.AppendAudit(r.Context(), acct.ID, "identity.link",
		fmt.Sprintf("account:%d", acct.ID), "discord:"+discordID); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	http.Redirect(w, r, "/dashboard?msg=Discord+linked", http.StatusFound)
}

type dashboardData struct {
	Characters        []Character
	Identities        []Identity
	Eligible          bool
	CharacterLimit    int
	DiscordConfigured bool
}

func (p *portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	chars, err := p.store.CharactersByAccount(r.Context(), acct.ID)
	if err != nil {
		p.log.Error("dashboard characters", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ids, err := p.store.IdentitiesByAccount(r.Context(), acct.ID)
	if err != nil {
		p.log.Error("dashboard identities", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	eligible, err := p.store.GateEligible(r.Context(), acct.ID, p.cfg.Gate.Providers)
	if err != nil {
		p.log.Error("dashboard gate", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.render(w, r, "dashboard.html", dashboardData{
		Characters:        chars,
		Identities:        ids,
		Eligible:          eligible && acct.EmailVerified,
		CharacterLimit:    p.cfg.CharacterLimit,
		DiscordConfigured: p.disc.configured(),
	})
}

func (p *portal) handleCharacterForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "character_new.html", nil)
}

// handleCharacterCreate is the gate choke point (spec: single place).
func (p *portal) handleCharacterCreate(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	fail := func(msg string) { p.render(w, r, "character_new.html", msg) }
	if !acct.EmailVerified {
		fail("verify your email address before creating characters")
		return
	}
	eligible, err := p.store.GateEligible(r.Context(), acct.ID, p.cfg.Gate.Providers)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !eligible {
		fail("your account is not eligible yet — link a Discord account or ask an admin for approval")
		return
	}
	name, err := NormalizeCharacterName(r.FormValue("name"))
	if err != nil {
		fail(err.Error())
		return
	}
	ch, err := p.store.CreateCharacter(r.Context(), acct.ID, name, p.cfg.CharacterLimit)
	switch {
	case errors.Is(err, ErrNameTaken):
		fail("that name is already taken")
		return
	case errors.Is(err, ErrCharacterLimit):
		fail(fmt.Sprintf("you've reached the character limit (%d)", p.cfg.CharacterLimit))
		return
	case err != nil:
		p.log.Error("create character", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.AppendAudit(r.Context(), acct.ID, "character.create",
		fmt.Sprintf("account:%d", acct.ID), "name="+ch.Username); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	http.Redirect(w, r, "/dashboard?msg="+url.QueryEscape("Character "+ch.Username+" created. Log into the game with that name and your account password."), http.StatusFound)
}

func (p *portal) handleSettingsForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "settings.html", nil)
}

func (p *portal) handleSettingsPassword(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	fail := func(msg string) { p.render(w, r, "settings.html", msg) }
	ok, err := VerifyPassword(r.FormValue("current"), acct.PasswordHash)
	if err != nil || !ok {
		fail("your current password is wrong")
		return
	}
	newPW := r.FormValue("password")
	if newPW != r.FormValue("password2") {
		fail("the new passwords don't match")
		return
	}
	if err := ValidPortalPassword(newPW); err != nil {
		fail(err.Error())
		return
	}
	phc, err := HashPassword(newPW, p.cfg.Argon2)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.SetPasswordHash(r.Context(), acct.ID, phc); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.DeleteAccountSessions(r.Context(), acct.ID); err != nil {
		p.log.Warn("session sweep failed", slog.Any("err", err))
	}
	if err := p.store.AppendAudit(r.Context(), acct.ID, "account.password_change",
		fmt.Sprintf("account:%d", acct.ID), ""); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	p.clearSessionCookie(w)
	http.Redirect(w, r, "/login?msg="+url.QueryEscape("Password changed - log in again (this is also your game password)"), http.StatusFound)
}
