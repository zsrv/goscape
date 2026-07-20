package account

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func adminPortal(t *testing.T) (*portal, *Store, *httptest.Server, *http.Client, string) {
	t.Helper()
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	adminID, cookie := loginTestAccount(t, p, s, "admin@example.com")
	if err := s.AddGroupMember(t.Context(), GroupAdmin, adminID, 0); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})
	return p, s, srv, client, cookie.Value
}

func TestAdminPages(t *testing.T) {
	_, s, srv, client, raw := adminPortal(t)
	targetID := seedVerifiedAccountWithCharacter(t, s, "player@example.com", "zezima")
	if err := s.LinkIdentity(t.Context(), targetID, "discord", "D9", "bob"); err != nil {
		t.Fatal(err)
	}
	detail := fmt.Sprintf("%s/admin/accounts/%d", srv.URL, targetID)
	csrf := csrfToken(raw)

	// Search finds the player.
	resp, err := client.Get(srv.URL + "/admin?q=player@")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("search: %v %d", err, resp.StatusCode)
	}
	if body := readBody(t, resp); !strings.Contains(body, "player@example.com") {
		t.Fatalf("search results: %q", body)
	}

	// Detail shows identities + characters.
	resp, _ = client.Get(detail)
	body := readBody(t, resp)
	for _, want := range []string{"zezima", "discord", "D9", "manually_approved"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %q", want)
		}
	}

	// Approve → gate satisfied.
	resp = postForm(t, client, detail+"/group", url.Values{
		"csrf": {csrf}, "group": {GroupManuallyApproved}, "member": {"on"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("approve: %d", resp.StatusCode)
	}
	if ok, _ := s.IsGroupMember(t.Context(), GroupManuallyApproved, targetID); !ok {
		t.Fatal("approve must add group")
	}

	// Disable → status flips and sessions die.
	resp = postForm(t, client, detail+"/status", url.Values{"csrf": {csrf}, "status": {StatusDisabled}})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("disable: %d", resp.StatusCode)
	}
	acct, _ := s.AccountByID(t.Context(), targetID)
	if acct.Status != StatusDisabled {
		t.Fatal("disable must persist")
	}

	// Unlink burns; release frees.
	resp = postForm(t, client, detail+"/unlink", url.Values{"csrf": {csrf}, "provider": {"discord"}})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("unlink: %d", resp.StatusCode)
	}
	ids, _ := s.IdentitiesByAccount(t.Context(), targetID)
	if len(ids) != 1 || !ids[0].RevokedAt.Valid {
		t.Fatal("unlink must revoke")
	}
	resp = postForm(t, client, detail+"/release", url.Values{
		"csrf": {csrf}, "provider": {"discord"}, "provider_user_id": {"D9"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("release: %d", resp.StatusCode)
	}
	if ids, _ := s.IdentitiesByAccount(t.Context(), targetID); len(ids) != 0 {
		t.Fatal("release must delete")
	}

	// Audit page shows the admin actions with the admin as actor.
	resp, _ = client.Get(srv.URL + "/admin/audit")
	body = readBody(t, resp)
	for _, want := range []string{"group.set", "account.status", "identity.unlink", "identity.release"} {
		if !strings.Contains(body, want) {
			t.Errorf("audit missing %q", want)
		}
	}
}

func TestAdminMailActions(t *testing.T) {
	p, s, srv, client, raw := adminPortal(t)
	mailer := p.mailer.(*recordingMailer)
	targetID, _ := s.CreateAccount(t.Context(), "player@example.com", "x")
	detail := fmt.Sprintf("%s/admin/accounts/%d", srv.URL, targetID)

	resp := postForm(t, client, detail+"/resend-verification", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound || !strings.Contains(mailer.last(t).Body, "/verify-email?token=") {
		t.Fatalf("resend-verification: %d", resp.StatusCode)
	}
	resp = postForm(t, client, detail+"/send-reset", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound || !strings.Contains(mailer.last(t).Body, "/reset-password?token=") {
		t.Fatalf("send-reset: %d", resp.StatusCode)
	}
}
