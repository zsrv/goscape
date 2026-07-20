package account

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// authedPortal returns a portal + client already logged in as a fresh
// verified account, plus the raw session value for CSRF.
func authedPortal(t *testing.T) (*portal, *Store, *httptest.Server, *http.Client, int64, string) {
	t.Helper()
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	id, cookie := loginTestAccount(t, p, s, "a@example.com")
	if err := s.SetEmailVerified(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})
	return p, s, srv, client, id, cookie.Value
}

func TestDashboard(t *testing.T) {
	_, s, srv, client, id, _ := authedPortal(t)
	if _, err := s.CreateCharacter(t.Context(), id, "zezima", 5); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(srv.URL + "/dashboard")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard: %v %d", err, resp.StatusCode)
	}
	body := readBody(t, resp)
	for _, want := range []string{"zezima", "a@example.com", "not eligible"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestCharacterCreation_GateAndLimit(t *testing.T) {
	p, s, srv, client, id, raw := authedPortal(t)
	form := func(name string) url.Values {
		return url.Values{"name": {name}, "csrf": {csrfToken(raw)}}
	}

	// Not eligible: no link, not approved.
	resp := postForm(t, client, srv.URL+"/characters/new", form("zezima"))
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "not eligible") {
		t.Fatalf("ineligible create must re-render with error: %d", resp.StatusCode)
	}
	if chars, _ := s.CharactersByAccount(t.Context(), id); len(chars) != 0 {
		t.Fatal("no character may be created while ineligible")
	}

	// manually_approved unlocks it.
	if err := s.AddGroupMember(t.Context(), GroupManuallyApproved, id, 0); err != nil {
		t.Fatal(err)
	}
	resp = postForm(t, client, srv.URL+"/characters/new", form("Zezima"))
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("eligible create: %d", resp.StatusCode)
	}
	chars, _ := s.CharactersByAccount(t.Context(), id)
	if len(chars) != 1 || chars[0].Username != "zezima" {
		t.Fatalf("created: %+v", chars)
	}

	// Bad name re-renders.
	resp = postForm(t, client, srv.URL+"/characters/new", form("bad!name!!"))
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "may only contain") {
		t.Fatal("invalid name must re-render with the validation error")
	}

	// Duplicate name.
	resp = postForm(t, client, srv.URL+"/characters/new", form("zezima"))
	if !strings.Contains(readBody(t, resp), "already taken") {
		t.Fatal("dup name must be reported")
	}

	// Limit: cfg default is 5; drop it to 1 for the test portal.
	p.cfg.CharacterLimit = 1
	resp = postForm(t, client, srv.URL+"/characters/new", form("alt1"))
	if !strings.Contains(readBody(t, resp), "character limit") {
		t.Fatal("limit must be reported")
	}

	// Unverified email blocks creation even when approved.
	if _, err := s.db.ExecContext(t.Context(), s.db.Rebind(
		`UPDATE portal_account SET email_verified = 0 WHERE id = ?`), id); err != nil {
		t.Fatal(err)
	}
	resp = postForm(t, client, srv.URL+"/characters/new", form("alt2"))
	if !strings.Contains(readBody(t, resp), "verify your email") {
		t.Fatal("unverified must be blocked")
	}
}

func TestSettings_ChangePassword(t *testing.T) {
	p, s, srv, client, id, raw := authedPortal(t)
	resp := postForm(t, client, srv.URL+"/settings/password", url.Values{
		"csrf": {csrfToken(raw)}, "current": {"hunter22!"},
		"password": {"newpass33!"}, "password2": {"newpass33!"},
	})
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("change password: %d → %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	acct, _ := s.AccountByID(t.Context(), id)
	if ok, _ := VerifyPassword("newpass33!", acct.PasswordHash); !ok {
		t.Fatal("new password must verify")
	}
	if _, err := s.SessionAccount(t.Context(), HashToken(raw), p.cfg.Session); err == nil {
		t.Fatal("all sessions must be invalidated")
	}

	// Wrong current password is rejected.
	_, cookie2 := loginTestAccount(t, p, s, "b@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie2})
	resp = postForm(t, client, srv.URL+"/settings/password", url.Values{
		"csrf": {csrfToken(cookie2.Value)}, "current": {"wrong"},
		"password": {"newpass44!"}, "password2": {"newpass44!"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "current password") {
		t.Fatal("wrong current password must be rejected")
	}
}
