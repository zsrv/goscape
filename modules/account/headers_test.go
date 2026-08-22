package account

import (
	"net/http"
	"strings"
	"testing"
)

// SEC1 M-8: every response carries the defensive headers; HSTS only when
// the public URL is https (cookies are Secure under the same rule).
func TestSecureHeaders(t *testing.T) {
	p, _ := newTestPortal(t) // PublicURL is http://portal.test
	srv, client := portalClient(t, p)
	resp, err := client.Get(srv.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	h := resp.Header
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("%s: got %q, want %q", k, got, v)
		}
	}
	csp := h.Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "form-action 'self'", "frame-ancestors 'none'", "base-uri 'self'"} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing %q: %q", directive, csp)
		}
	}
	if h.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be sent for an http public_url")
	}

	p.cfg.PublicURL = "https://portal.test"
	resp, _ = client.Get(srv.URL + "/login")
	if !strings.HasPrefix(resp.Header.Get("Strict-Transport-Security"), "max-age=") {
		t.Error("HSTS must be sent for an https public_url")
	}
}

// Oversized form bodies are refused instead of being buffered.
func TestBodyLimit(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)
	big := strings.Repeat("a", 70<<10)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/login", strings.NewReader("email="+big))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized body: got %d", resp.StatusCode)
	}
}
