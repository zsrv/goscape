package account

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeDiscord stands in for discord.com: token exchange + identify.
func fakeDiscord(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var lastCode string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
		lastCode = r.FormValue("code")
		if r.FormValue("grant_type") != "authorization_code" || r.FormValue("client_id") == "" {
			http.Error(w, "bad request", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-" + lastCode, "token_type": "Bearer"})
	})
	mux.HandleFunc("GET /api/users/@me", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok-") {
			http.Error(w, "unauthorized", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "D-42", "username": "alice"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastCode
}

func newDiscordTestPortal(t *testing.T) (*portal, *Store, *httptest.Server) {
	t.Helper()
	disc, _ := fakeDiscord(t)
	s := openTestStore(t)
	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.PublicURL = "http://portal.test"
	cfg.Argon2 = testArgon2()
	cfg.Providers.Discord = DiscordConfig{
		ClientID: "cid", ClientSecret: "sec",
		AuthURL:  disc.URL + "/oauth2/authorize",
		TokenURL: disc.URL + "/api/oauth2/token",
		APIBase:  disc.URL + "/api",
	}
	p, err := newPortal(cfg, s, &recordingMailer{}, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	return p, s, disc
}

func TestDiscordLinkFlow(t *testing.T) {
	p, s, _ := newDiscordTestPortal(t)
	srv, client := portalClient(t, p)
	id, cookie := loginTestAccount(t, p, s, "a@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})

	// Start: redirects to Discord with client_id + state; state cookie set.
	resp, err := client.Get(srv.URL + "/link/discord")
	if err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("link start: %v %d", err, resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("client_id") != "cid" || loc.Query().Get("state") == "" ||
		loc.Query().Get("scope") != "identify" {
		t.Fatalf("authorize url: %s", loc)
	}
	state := loc.Query().Get("state")

	// Callback with matching state links the identity.
	resp, err = client.Get(srv.URL + "/oauth/discord/callback?code=abc&state=" + url.QueryEscape(state))
	if err != nil || resp.StatusCode != http.StatusFound ||
		!strings.HasPrefix(resp.Header.Get("Location"), "/dashboard") {
		t.Fatalf("callback: %v %d %s", err, resp.StatusCode, resp.Header.Get("Location"))
	}
	ids, err := s.IdentitiesByAccount(t.Context(), id)
	if err != nil || len(ids) != 1 || ids[0].ProviderUserID != "D-42" || ids[0].ProviderUsername != "alice" {
		t.Fatalf("identity: %+v err=%v", ids, err)
	}

	// State mismatch is rejected and links nothing.
	p2, s2, _ := newDiscordTestPortal(t)
	srv2, client2 := portalClient(t, p2)
	id2, cookie2 := loginTestAccount(t, p2, s2, "b@example.com")
	u2, _ := url.Parse(srv2.URL)
	client2.Jar.SetCookies(u2, []*http.Cookie{cookie2})
	if _, err := client2.Get(srv2.URL + "/link/discord"); err != nil {
		t.Fatal(err)
	}
	resp, _ = client2.Get(srv2.URL + "/oauth/discord/callback?code=abc&state=forged")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forged state: %d", resp.StatusCode)
	}
	if ids, _ := s2.IdentitiesByAccount(t.Context(), id2); len(ids) != 0 {
		t.Fatal("forged state must not link")
	}
}

func TestDiscordLink_TakenIdentity(t *testing.T) {
	p, s, _ := newDiscordTestPortal(t)
	srv, client := portalClient(t, p)
	// D-42 already belongs (burned) to another account.
	other, _ := s.CreateAccount(t.Context(), "other@example.com", "x")
	if err := s.LinkIdentity(t.Context(), other, "discord", "D-42", "bob"); err != nil {
		t.Fatal(err)
	}
	_, cookie := loginTestAccount(t, p, s, "a@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})

	resp, _ := client.Get(srv.URL + "/link/discord")
	loc, _ := url.Parse(resp.Header.Get("Location"))
	resp, _ = client.Get(srv.URL + "/oauth/discord/callback?code=abc&state=" + url.QueryEscape(loc.Query().Get("state")))
	if resp.StatusCode != http.StatusFound ||
		!strings.Contains(resp.Header.Get("Location"), "already+linked") {
		t.Fatalf("taken identity: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestDiscordLink_NotConfigured(t *testing.T) {
	p, s := newTestPortal(t) // no discord credentials
	srv, client := portalClient(t, p)
	_, cookie := loginTestAccount(t, p, s, "a@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})
	resp, _ := client.Get(srv.URL + "/link/discord")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unconfigured provider: %d, want 404", resp.StatusCode)
	}
}
