package asset

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRs2CgiClientTemplate verifies the default branch (plugin != 1 OR
// !Debug) renders templates/client.html with the configured NodeID/Members
// and the query-supplied lowmem substituted into the JS Client(...) call.
// Mirrors web.ts:104-112.
func TestRs2CgiClientTemplate(t *testing.T) {
	a := &Asset{
		log: discardLogger(),
		cfg: Config{
			NodeID:  10,
			Members: true,
			Debug:   true,
			Port:    43594,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rs2.cgi?lowmem=0", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}

	body, _ := io.ReadAll(rr.Body)
	got := string(body)

	// Shape-parity assertions: identifying strings from view/client.ejs.
	for _, want := range []string{
		"<title>2004Scape Game</title>",
		"import { Client } from './client/client.js';",
		"new Client( 10 ,  0 ,  true )",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing %q\nbody=%s", want, got)
		}
	}
	// Must NOT contain the java applet template's identifying markup.
	if strings.Contains(got, "<applet") {
		t.Errorf("client template unexpectedly contained <applet>")
	}
}

// TestRs2CgiJavaTemplate verifies that Debug=true + plugin=1 selects
// templates/java.html and that nodeid/members/lowmem/portoff are substituted
// into the <applet> param tags. Mirrors web.ts:92-102.
func TestRs2CgiJavaTemplate(t *testing.T) {
	a := &Asset{
		log: discardLogger(),
		cfg: Config{
			NodeID:  10,
			Members: false,
			Debug:   true,
			Port:    43595, // portoff = 43595 - 43594 = 1
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rs2.cgi?plugin=1&lowmem=1", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html" {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}

	body, _ := io.ReadAll(rr.Body)
	got := string(body)

	// Shape-parity assertions: identifying strings from view/java.ejs.
	// html/template renders unquoted attrs with quoted values, so we check for
	// the value-substring rather than the exact tag form.
	for _, want := range []string{
		"archive=loader.jar",
		"code=loader.class",
		`name=portoff`,
		`name=nodeid`,
		`name=free`,
		`name=lowmem`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing %q\nbody=%s", want, got)
		}
	}
	// Pin substituted values appear (as substrings — html/template may quote
	// them when rendering into an unquoted-HTML-attr context).
	for _, want := range []string{"1", "10", "false"} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing substituted value %q", want)
		}
	}
	// Must NOT contain the client template's identifying markup.
	if strings.Contains(got, "import { Client }") {
		t.Errorf("java template unexpectedly contained client.js import")
	}
}

// TestRs2CgiPluginIgnoredWhenDebugOff pins the TS guard at web.ts:92: if
// NODE_DEBUG is false, plugin=1 still serves the client template. The Java
// applet is debug-only.
func TestRs2CgiPluginIgnoredWhenDebugOff(t *testing.T) {
	a := &Asset{
		log: discardLogger(),
		cfg: Config{
			NodeID:  10,
			Members: true,
			Debug:   false, // gate closed
			Port:    43594,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rs2.cgi?plugin=1&lowmem=0", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	got := string(body)

	if !strings.Contains(got, "import { Client }") {
		t.Errorf("expected client template, body did not contain Client import")
	}
	if strings.Contains(got, "<applet") {
		t.Errorf("Debug=false must not serve java applet template")
	}
}

// TestRs2CgiPortoffComputation pins web.ts:97: portoff = NodePort - 43594.
func TestRs2CgiPortoffComputation(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string // expected portoff substring
	}{
		{"default port emits 0", 43594, "name=portoff value=0"},
		{"offset port emits delta", 43600, "name=portoff value=6"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &Asset{
				log: discardLogger(),
				cfg: Config{NodeID: 10, Debug: true, Port: tc.port},
			}
			req := httptest.NewRequest(http.MethodGet, "/rs2.cgi?plugin=1", nil)
			rr := httptest.NewRecorder()
			a.RootHandler(rr, req)
			body, _ := io.ReadAll(rr.Body)
			if !strings.Contains(string(body), tc.want) {
				t.Errorf("body missing %q\nbody=%s", tc.want, body)
			}
		})
	}
}

// TestRs2CgiTryParseIntFallback pins tryParseInt's empty/invalid-string
// fallback to 0 (TS web.ts:89-90 via tryParseInt).
func TestRs2CgiTryParseIntFallback(t *testing.T) {
	a := &Asset{
		log: discardLogger(),
		cfg: Config{NodeID: 10, Members: true, Debug: true, Port: 43594},
	}

	// "lowmem=notanint" should fall back to 0 and render client template.
	req := httptest.NewRequest(http.MethodGet, "/rs2.cgi?lowmem=notanint", nil)
	rr := httptest.NewRecorder()
	a.RootHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), "new Client( 10 ,  0 ,  true )") {
		t.Errorf("expected lowmem fallback to 0; body=%s", body)
	}
}
