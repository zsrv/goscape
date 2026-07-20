package account

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type sentMail struct{ To, Subject, Body string }

// recordingMailer captures outbound mail for flow tests (Tasks 16-17
// pull verification/reset links out of Body).
type recordingMailer struct {
	mu   sync.Mutex
	sent []sentMail
}

func (m *recordingMailer) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to, subject, body})
	return nil
}

func (m *recordingMailer) last(t *testing.T) sentMail {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		t.Fatal("no mail sent")
	}
	return m.sent[len(m.sent)-1]
}

func newTestPortal(t *testing.T) (*portal, *Store) {
	t.Helper()
	s := openTestStore(t)
	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.PublicURL = "http://portal.test"
	cfg.Argon2 = testArgon2()
	p, err := newPortal(cfg, s, &recordingMailer{}, testLogger(t))
	if err != nil {
		t.Fatalf("newPortal: %v", err)
	}
	return p, s
}

func TestPortal_HomeRenders(t *testing.T) {
	p, _ := newTestPortal(t)
	srv := httptest.NewServer(p.routes())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	for _, want := range []string{"goscape", "/register", "/login"} {
		if !strings.Contains(body, want) {
			t.Errorf("home missing %q", want)
		}
	}

	// Static assets served from the embed.
	css, err := http.Get(srv.URL + "/static/style.css")
	if err != nil || css.StatusCode != http.StatusOK {
		t.Fatalf("style.css: %v %d", err, css.StatusCode)
	}
	css.Body.Close()

	// Unknown path is a 404, not the home page (the GET /{$} pattern).
	nf, _ := http.Get(srv.URL + "/no-such-page")
	nf.Body.Close()
	if nf.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path: %d", nf.StatusCode)
	}
}
