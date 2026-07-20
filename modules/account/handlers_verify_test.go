package account

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func extractLink(t *testing.T, body, marker string) string {
	t.Helper()
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no %q link in mail body:\n%s", marker, body)
	}
	link := body[i:]
	if j := strings.IndexAny(link, "\r\n \t"); j >= 0 {
		link = link[:j]
	}
	return link
}

func TestVerifyEmailFlow(t *testing.T) {
	p, s := newTestPortal(t)
	mailer := p.mailer.(*recordingMailer)
	srv, client := portalClient(t, p)

	// Register → mail carries the verify link.
	postForm(t, client, srv.URL+"/register", url.Values{
		"email": {"v@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	link := extractLink(t, mailer.last(t).Body, "http://portal.test/verify-email?token=")
	local := strings.Replace(link, "http://portal.test", srv.URL, 1)

	// Following the link verifies the account.
	resp, err := client.Get(local)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("verify: %v %d", err, resp.StatusCode)
	}
	if !strings.Contains(readBody(t, resp), "verified") {
		t.Fatal("verify page must confirm")
	}
	acct, _ := s.AccountByEmail(t.Context(), "v@example.com")
	if !acct.EmailVerified {
		t.Fatal("account must be verified")
	}

	// Token is single-use.
	resp, _ = client.Get(local)
	if !strings.Contains(readBody(t, resp), "invalid or expired") {
		t.Fatal("second use must fail")
	}

	// Resend: log in (unverified users can), request resend, new link works.
	p2, s2 := newTestPortal(t)
	mailer2 := p2.mailer.(*recordingMailer)
	srv2, client2 := portalClient(t, p2)
	postForm(t, client2, srv2.URL+"/register", url.Values{
		"email": {"r@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	postForm(t, client2, srv2.URL+"/login", url.Values{
		"email": {"r@example.com"}, "password": {"hunter22!"},
	})
	var raw string
	u2, _ := url.Parse(srv2.URL)
	for _, c := range client2.Jar.Cookies(u2) {
		if c.Name == sessionCookieName {
			raw = c.Value
		}
	}
	before := len(mailer2.sent)
	resp = postForm(t, client2, srv2.URL+"/resend-verification", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound || len(mailer2.sent) != before+1 {
		t.Fatalf("resend: %d mails=%d", resp.StatusCode, len(mailer2.sent))
	}
	link2 := extractLink(t, mailer2.last(t).Body, "http://portal.test/verify-email?token=")
	resp, _ = client2.Get(strings.Replace(link2, "http://portal.test", srv2.URL, 1))
	if !strings.Contains(readBody(t, resp), "verified") {
		t.Fatal("resent link must verify")
	}
	acct2, _ := s2.AccountByEmail(t.Context(), "r@example.com")
	if !acct2.EmailVerified {
		t.Fatal("account 2 must be verified")
	}
	_ = s
}
