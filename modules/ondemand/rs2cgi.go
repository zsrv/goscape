package ondemand

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/util/pemtoken"
)

// rs2cgiClientData is the template payload for templates/client.html
// (port of view/client.ejs). Mirrors the EJS locals at web.ts:101-106.
// Token is the per-deployment public token (non-empty only when
// WsTokenProtection is true); the template conditionally emits a
// document.cookie setter mirroring view/client.ejs:322-324.
type rs2cgiClientData struct {
	NodeID  int
	Lowmem  int
	Members bool
	// Token is the per-deployment public token. Empty string means the gate is
	// off (WEB_SOCKET_TOKEN_PROTECTION false, default). Non-empty means the
	// template should emit the document.cookie setter. Mirrors
	// web.ts:105 + view/client.ejs:322-324.
	Token string
}

// rs2cgiJavaData is the template payload for templates/java.html
// (port of view/java.ejs). Mirrors the EJS locals at web.ts:93-97. Portoff is
// computed as NodePort - 43594, matching the TS source.
type rs2cgiJavaData struct {
	NodeID  int
	Lowmem  int
	Members bool
	Portoff int
}

// rs2cgiPortBase is the canonical RuneScape world port base. portoff is the
// configured world TCP port minus this constant, mirroring TS web.ts:97
// (`Environment.NODE_PORT - 43594`).
const rs2cgiPortBase = 43594

// Rs2CgiHandler serves the Java applet bootstrap page at /rs2.cgi.
//
// Ports web.ts:88-113. The query param `plugin=1` selects the Java applet
// template (java.html) but only when Debug is enabled — matching TS'
// `Environment.NODE_DEBUG && plugin === 1` gate at web.ts:92. All other
// requests render the JS/WebSocket client bootstrap (client.html). Invalid
// numeric query params silently fall back to 0, matching tryParseInt.
//
// When WsTokenProtection is true (mirrors WEB_SOCKET_TOKEN_PROTECTION,
// Environment.ts:21 default false), the public per-deployment token is
// computed from PubPEM via pkg/util/pemtoken and injected into the client
// template (mirrors web.ts:105 + view/client.ejs:322-324). A bad PEM
// returns 500.
//
// DEVIATION: Go's html/template auto-escapes substitutions for the surrounding
// HTML/JS context. Numeric/boolean substitutions inside <script> are rendered
// with surrounding whitespace (e.g. ` 10 ` instead of `10`), which is byte-
// different from EJS' raw substitution but parses identically as JavaScript.
// Shape-parity tests assert against the escaped form.
func (a *OnDemand) Rs2CgiHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	plugin := tryParseIntDefault(q.Get("plugin"), 0)
	lowmem := tryParseIntDefault(q.Get("lowmem"), 0)

	w.Header().Set("Content-Type", "text/html")

	// Render into a buffer first so a template execution error surfaces as a
	// 500 instead of a partially-written 200.
	var buf bytes.Buffer

	if a.cfg.Debug && plugin == 1 {
		data := rs2cgiJavaData{
			NodeID:  a.cfg.NodeID,
			Lowmem:  lowmem,
			Members: a.cfg.Members,
			Portoff: a.cfg.Port - rs2cgiPortBase,
		}
		if err := rs2cgiTemplates.ExecuteTemplate(&buf, "java.html", data); err != nil {
			a.log.Error("rs2.cgi: java template render failed", "err", err)
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
	} else {
		// Compute the per-deployment token when WsTokenProtection is on.
		// Mirrors web.ts:105: WEB_SOCKET_TOKEN_PROTECTION ? getPublicPerDeploymentToken() : ''
		var token string
		if a.cfg.WsTokenProtection {
			var err error
			// hostname is intentionally empty string: the TS
			// getPublicPerDeploymentToken() calls Token(pubPEM, hostname) where
			// hostname comes from Environment.NODE_HOSTNAME (default '').
			// We pass "" to match the default TS behaviour; operators that set a
			// hostname should supply it via YAML (future extension).
			token, err = pemtoken.Token(a.cfg.PubPEM, "")
			if err != nil {
				a.log.Error("rs2.cgi: per-deployment token computation failed", "err", err)
				http.Error(w, "token error", http.StatusInternalServerError)
				return
			}
		}
		data := rs2cgiClientData{
			NodeID:  a.cfg.NodeID,
			Lowmem:  lowmem,
			Members: a.cfg.Members,
			Token:   token,
		}
		if err := rs2cgiTemplates.ExecuteTemplate(&buf, "client.html", data); err != nil {
			a.log.Error("rs2.cgi: client template render failed", "err", err)
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
	}

	_, _ = w.Write(buf.Bytes())
}

// tryParseIntDefault mirrors TS' tryParseInt(value, default) helper used at
// web.ts:89-90, which delegates to JavaScript's parseInt(value) (no radix).
// Per TryParse.ts:20 + the ECMAScript parseInt grammar: skip leading
// whitespace, accept optional sign, treat a leading "0x"/"0X" as hex,
// otherwise parse the leading decimal digits and stop at the first
// non-digit. Any value with no parseable leading digits returns NaN in TS,
// which tryParseInt then maps to def.
//
// Diverges from Go's strconv.Atoi (which is strict and rejects trailing
// garbage / floats / hex / leading whitespace).
func tryParseIntDefault(s string, def int) int {
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	if s == "" {
		return def
	}
	sign := 1
	i := 0
	if s[0] == '+' {
		i = 1
	} else if s[0] == '-' {
		sign = -1
		i = 1
	}
	base := 10
	if len(s)-i >= 2 && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X') {
		base = 16
		i += 2
	}
	start := i
	if base == 10 {
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	} else {
		for i < len(s) && isHexDigit(s[i]) {
			i++
		}
	}
	if i == start {
		return def
	}
	n, err := strconv.ParseInt(s[start:i], base, 64)
	if err != nil {
		return def
	}
	return sign * int(n)
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
