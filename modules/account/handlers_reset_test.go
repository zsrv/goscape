package account

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPasswordResetFlow(t *testing.T) {
	p, s := newTestPortal(t)
	mailer := p.mailer.(*recordingMailer)
	srv, client := portalClient(t, p)
	phc, _ := HashPassword("oldpass99!", testArgon2())
	id, _ := s.CreateAccount(t.Context(), "a@example.com", phc)

	// Live session that must die after the reset.
	raw, _ := NewRawToken()
	if err := s.CreateSession(t.Context(), id, HashToken(raw), "", "", p.cfg.Session); err != nil {
		t.Fatal(err)
	}

	// Enumeration-safe: unknown email gets the SAME response and no mail.
	before := len(mailer.sent)
	resp := postForm(t, client, srv.URL+"/forgot-password", url.Values{"email": {"ghost@example.com"}})
	unknownBody := readBody(t, resp)
	p.mailWG.Wait()
	if len(mailer.sent) != before {
		t.Fatal("unknown email must not send mail")
	}
	resp = postForm(t, client, srv.URL+"/forgot-password", url.Values{"email": {"a@example.com"}})
	if knownBody := readBody(t, resp); knownBody != unknownBody {
		t.Fatal("known/unknown email responses must be identical (enumeration)")
	}
	p.mailWG.Wait()
	if len(mailer.sent) != before+1 {
		t.Fatal("known email must send mail")
	}

	// Follow the link, set a new password.
	link := extractLink(t, mailer.last(t).Body, "http://portal.test/reset-password?token=")
	local := strings.Replace(link, "http://portal.test", srv.URL, 1)
	resp, _ = client.Get(local)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset form: %d", resp.StatusCode)
	}
	tokenParam := strings.SplitN(link, "token=", 2)[1]
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {tokenParam}, "password": {"newpass22!"}, "password2": {"newpass22!"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("reset submit: %d", resp.StatusCode)
	}

	// New password works, old doesn't, all sessions invalidated.
	acct, _ := s.AccountByID(t.Context(), id)
	if ok, _ := VerifyPassword("newpass22!", acct.PasswordHash); !ok {
		t.Fatal("new password must verify")
	}
	if ok, _ := VerifyPassword("oldpass99!", acct.PasswordHash); ok {
		t.Fatal("old password must not verify")
	}
	if _, err := s.SessionAccount(t.Context(), HashToken(raw), p.cfg.Session); err == nil {
		t.Fatal("existing sessions must be invalidated by reset")
	}

	// Token is single-use.
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {tokenParam}, "password": {"another11!"}, "password2": {"another11!"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "invalid or expired") {
		t.Fatal("token reuse must fail")
	}

	// A weak new password does NOT burn the token.
	resp = postForm(t, client, srv.URL+"/forgot-password", url.Values{"email": {"a@example.com"}})
	p.mailWG.Wait()
	link2 := extractLink(t, mailer.last(t).Body, "http://portal.test/reset-password?token=")
	token2 := strings.SplitN(link2, "token=", 2)[1]
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {token2}, "password": {"short"}, "password2": {"short"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("weak password: %d", resp.StatusCode)
	}
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {token2}, "password": {"goodpass33!"}, "password2": {"goodpass33!"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatal("token must survive a failed policy check")
	}
}
