package ondemand

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
	a := &OnDemand{
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
	a := &OnDemand{
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
	a := &OnDemand{
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
			a := &OnDemand{
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

// TestTryParseIntDefault_JSparseIntSemantics pins TS tryParseInt's delegation
// to JS parseInt(value) (no radix). Per TryParse.ts:20: leading whitespace
// is skipped, optional sign accepted, "0x"/"0X" prefix → hex, otherwise
// leading decimal digits are consumed and the parse stops at the first
// non-digit. Closes entry-1 (rs2.cgi parseInt-vs-Atoi divergence).
func TestTryParseIntDefault_JSparseIntSemantics(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		// Audit-cited divergences (RED pre-fix; GREEN post-fix):
		{"1x", 1},     // trailing garbage → TS 1 (Atoi err → 0)
		{"10abc", 10}, // trailing garbage → TS 10 (Atoi err → 0)
		{"3.5", 3},    // float-looking → TS 3 (Atoi err → 0)
		{"0x10", 16},  // hex prefix → TS 16 (Atoi err → 0)
		{"  42", 42},  // leading whitespace → TS 42 (Atoi err → 0)
		{"\t-7", -7},  // whitespace + sign → TS -7
		{"+99", 99},   // explicit positive sign
		// Regression guards (already worked pre-fix):
		{"0", 0},
		{"42", 42},
		{"-1", -1},
		// Genuine NaN cases → def:
		{"", 0},
		{"abc", 0},
		{"   ", 0},
		{"+", 0},
		{"-x", 0},
		{"0x", 0}, // 0x with no following hex digit → NaN
	}
	for _, tc := range cases {
		got := tryParseIntDefault(tc.in, 0)
		if got != tc.want {
			t.Errorf("tryParseIntDefault(%q, 0) = %d, want %d (TS parseInt via TryParse.ts:20)", tc.in, got, tc.want)
		}
	}
}

// TestRs2CgiTryParseIntFallback pins tryParseInt's empty/invalid-string
// fallback to 0 (TS web.ts:89-90 via tryParseInt).
func TestRs2CgiTryParseIntFallback(t *testing.T) {
	a := &OnDemand{
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
