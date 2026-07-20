package account

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// portalClient wraps httptest with a cookie jar and no-redirect policy.
func portalClient(t *testing.T, p *portal) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(p.routes())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return srv, &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func postForm(t *testing.T, c *http.Client, u string, form url.Values) *http.Response {
	t.Helper()
	resp, err := c.PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestRegisterFlow(t *testing.T) {
	p, s := newTestPortal(t)
	mailer := p.mailer.(*recordingMailer)
	srv, client := portalClient(t, p)

	// GET form renders.
	resp, _ := client.Get(srv.URL + "/register")
	if body := readBody(t, resp); !strings.Contains(body, `name="password2"`) {
		t.Fatalf("register form: %q", body)
	}

	// Successful registration creates the account and sends a
	// verification mail.
	resp = postForm(t, client, srv.URL+"/register", url.Values{
		"email": {"new@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	acct, err := s.AccountByEmail(t.Context(), "new@example.com")
	if err != nil || acct.EmailVerified {
		t.Fatalf("created account: %+v err=%v", acct, err)
	}
	mail := mailer.last(t)
	if mail.To != "new@example.com" || !strings.Contains(mail.Body, "http://portal.test/verify-email?token=") {
		t.Fatalf("verification mail: %+v", mail)
	}

	// Password mismatch / policy violations re-render with an error.
	for _, form := range []url.Values{
		{"email": {"x@example.com"}, "password": {"hunter22!"}, "password2": {"different1!"}},
		{"email": {"x@example.com"}, "password": {"short"}, "password2": {"short"}},
		{"email": {"not-an-email"}, "password": {"hunter22!"}, "password2": {"hunter22!"}},
	} {
		resp = postForm(t, client, srv.URL+"/register", form)
		if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "error") {
			t.Fatalf("bad form %v must re-render with error, got %d", form, resp.StatusCode)
		}
	}

	// Duplicate email surfaces a friendly error.
	resp = postForm(t, client, srv.URL+"/register", url.Values{
		"email": {"new@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	if !strings.Contains(readBody(t, resp), "already registered") {
		t.Fatal("duplicate email must be reported")
	}
}

func TestLoginLogoutFlow(t *testing.T) {
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	phc, _ := HashPassword("hunter22!", testArgon2())
	id, _ := s.CreateAccount(t.Context(), "a@example.com", phc)

	// Wrong password.
	resp := postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"a@example.com"}, "password": {"wrong"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "error") {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}

	// Correct password: session cookie set, redirect to /dashboard.
	resp = postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"a@example.com"}, "password": {"hunter22!"},
	})
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/dashboard" {
		t.Fatalf("login: %d → %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Session works: home shows the logged-in nav.
	resp, _ = client.Get(srv.URL + "/")
	if !strings.Contains(readBody(t, resp), "/dashboard") {
		t.Fatal("nav must show dashboard when logged in")
	}

	// Disabled accounts cannot log in.
	if err := s.SetAccountStatus(t.Context(), id, StatusDisabled); err != nil {
		t.Fatal(err)
	}
	resp = postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"a@example.com"}, "password": {"hunter22!"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "disabled") {
		t.Fatal("disabled login must be rejected")
	}
	_ = s.SetAccountStatus(t.Context(), id, StatusActive)

	// Logout: needs CSRF, clears the session.
	var raw string
	u, _ := url.Parse(srv.URL)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == sessionCookieName {
			raw = c.Value
		}
	}
	resp = postForm(t, client, srv.URL+"/logout", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	resp, _ = client.Get(srv.URL + "/")
	if strings.Contains(readBody(t, resp), "/dashboard") {
		t.Fatal("logout must drop the session")
	}
}

func TestLoginRateLimit(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)
	form := url.Values{"email": {"rl@example.com"}, "password": {"wrong"}}
	var last *http.Response
	for range 11 {
		last = postForm(t, client, srv.URL+"/login", form)
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: %d, want 429", last.StatusCode)
	}
}
