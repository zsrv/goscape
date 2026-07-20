package account

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// loginTestAccount creates an account + a live session, returning the
// raw cookie value. Reused by later portal flow tests.
func loginTestAccount(t *testing.T, p *portal, s *Store, email string) (int64, *http.Cookie) {
	t.Helper()
	ctx := t.Context()
	phc, _ := HashPassword("hunter22!", testArgon2())
	id, err := s.CreateAccount(ctx, email, phc)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := NewRawToken()
	if err := s.CreateSession(ctx, id, HashToken(raw), "t", "t", p.cfg.Session); err != nil {
		t.Fatal(err)
	}
	return id, &http.Cookie{Name: sessionCookieName, Value: raw}
}

func TestMiddleware_AuthGuards(t *testing.T) {
	p, s := newTestPortal(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /open", p.public(func(w http.ResponseWriter, r *http.Request) {
		if a := ctxAccount(r); a != nil {
			w.Write([]byte("hello " + a.Email))
			return
		}
		w.Write([]byte("anonymous"))
	}))
	mux.HandleFunc("GET /private", p.authed(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret"))
	}))
	mux.HandleFunc("POST /private", p.authed(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("posted"))
	}))
	mux.HandleFunc("GET /adminz", p.admin(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("admin"))
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	get := func(path string, cookie *http.Cookie) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// Anonymous.
	if resp := get("/open", nil); readBody(t, resp) != "anonymous" {
		t.Fatal("public without session must pass through as anonymous")
	}
	if resp := get("/private", nil); resp.StatusCode != http.StatusFound ||
		!strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("authed without session: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp := get("/adminz", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("admin route must 404 for anonymous, got %d", resp.StatusCode)
	}

	// Authenticated.
	id, cookie := loginTestAccount(t, p, s, "a@example.com")
	if body := readBody(t, get("/open", cookie)); body != "hello a@example.com" {
		t.Fatalf("public with session: %q", body)
	}
	if resp := get("/private", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("authed with session: %d", resp.StatusCode)
	}
	if resp := get("/adminz", cookie); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("admin route must 404 for non-admin, got %d", resp.StatusCode)
	}

	// Admin.
	if err := s.AddGroupMember(t.Context(), GroupAdmin, id, 0); err != nil {
		t.Fatal(err)
	}
	if resp := get("/adminz", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin route for admin: %d", resp.StatusCode)
	}

	// CSRF: POST without token 403; with token passes.
	post := func(form url.Values) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/private", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if resp := post(url.Values{}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without csrf: %d", resp.StatusCode)
	}
	if resp := post(url.Values{"csrf": {csrfToken(cookie.Value)}}); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST with csrf: %d", resp.StatusCode)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter()
	for i := range 3 {
		if !rl.allow("k", 3, time.Minute) {
			t.Fatalf("attempt %d must be allowed", i)
		}
	}
	if rl.allow("k", 3, time.Minute) {
		t.Fatal("4th attempt must be denied")
	}
	if !rl.allow("other", 3, time.Minute) {
		t.Fatal("different key must be independent")
	}
}
