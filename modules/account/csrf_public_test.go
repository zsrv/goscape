package account

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// SEC1 M-8: anonymous POSTs (/login, /register, /forgot-password,
// /reset-password) require the double-submit CSRF token; a cross-site
// form that cannot read the cookie is rejected with 403.
func TestPublicPOST_RejectsMissingOrWrongCSRF(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)

	for _, path := range []string{"/login", "/register", "/forgot-password", "/reset-password"} {
		resp, err := client.PostForm(srv.URL+path, url.Values{"email": {"x@example.com"}})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s without csrf: got %d, want 403", path, resp.StatusCode)
		}

		// GET the form first so the jar really holds a goscape_csrf
		// cookie: without it the forged POST would be rejected by the
		// `want == ""` arm and never reach the comparison this case is
		// about. With the cookie present, only the constant-time
		// mismatch can produce the 403.
		if _, err := client.Get(srv.URL + path); err != nil {
			t.Fatal(err)
		}
		if tok := csrfFor(t, client, srv.URL+path); tok == "" {
			t.Fatalf("%s: GET did not seed a csrf cookie", path)
		} else if tok == "forged" {
			t.Fatalf("%s: seeded token collides with the forged one", path)
		}
		resp, err = client.PostForm(srv.URL+path, url.Values{"email": {"x@example.com"}, "csrf": {"forged"}})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s forged csrf (real cookie present): got %d, want 403", path, resp.StatusCode)
		}
	}
}

// SEC1 final review: CSRF must never run before an authorisation
// decision. public() therefore does not check it at all — admin() checks
// after its group check (so an anonymous admin POST is 404, not the 403
// that would confirm the route exists) and authed() after its login
// check (so an anonymous POST /logout is the usual 302 to /login).
func TestCSRFOrdering_AuthorisationDecidesFirst(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)

	resp, err := client.PostForm(srv.URL+"/admin/accounts/1/status", url.Values{"status": {StatusDisabled}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("anonymous admin POST without csrf: got %d, want 404", resp.StatusCode)
	}

	resp, err = client.PostForm(srv.URL+"/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("anonymous POST /logout without csrf: got %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Fatalf("anonymous POST /logout redirected to %q, want /login", loc)
	}
}

// The rendered login form carries the anonymous token and the cookie
// that backs it, and a POST using both is accepted.
func TestPublicPOST_AcceptsDoubleSubmitToken(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)
	resp, _ := client.Get(srv.URL + "/login")
	body := readBody(t, resp)
	if !strings.Contains(body, `name="csrf"`) {
		t.Fatal("login form must embed csrf")
	}
	tok := csrfFor(t, client, srv.URL+"/login")
	resp = postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"nobody@example.com"}, "password": {"wrong"}, "csrf": {tok},
	})
	if resp.StatusCode != http.StatusOK { // wrong password renders the form again; not 403
		t.Fatalf("valid csrf: got %d", resp.StatusCode)
	}
}

// Logging in invalidates whatever session the browser already had
// (fixation / cross-site login defence) and issues a fresh one.
func TestLogin_RotatesExistingSession(t *testing.T) {
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	phc, _ := HashPassword("hunter22!", testArgon2())
	_, _ = s.CreateAccount(t.Context(), "a@example.com", phc)

	login := func() string {
		t.Helper()
		resp := postForm(t, client, srv.URL+"/login", url.Values{"email": {"a@example.com"}, "password": {"hunter22!"}})
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("login: %d", resp.StatusCode)
		}
		u, _ := url.Parse(srv.URL)
		for _, c := range client.Jar.Cookies(u) {
			if c.Name == sessionCookieName {
				return c.Value
			}
		}
		t.Fatal("no session cookie")
		return ""
	}
	first := login()
	second := login()
	if first == second {
		t.Fatal("second login must issue a new session token")
	}
	if _, err := s.SessionAccount(t.Context(), HashToken(first), p.cfg.Session); err == nil {
		t.Fatal("first session must be deleted on re-login")
	}
}
