package account

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
