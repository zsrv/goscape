package asset

import (
	"bytes"
	"net/http"
	"strconv"
)

// rs2cgiClientData is the template payload for templates/client.html
// (port of view/client.ejs). Mirrors the EJS locals at web.ts:104-107.
type rs2cgiClientData struct {
	NodeID  int
	Lowmem  int
	Members bool
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
// DEVIATION: Go's html/template auto-escapes substitutions for the surrounding
// HTML/JS context. Numeric/boolean substitutions inside <script> are rendered
// with surrounding whitespace (e.g. ` 10 ` instead of `10`), which is byte-
// different from EJS' raw substitution but parses identically as JavaScript.
// Shape-parity tests assert against the escaped form.
func (a *Asset) Rs2CgiHandler(w http.ResponseWriter, r *http.Request) {
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
		data := rs2cgiClientData{
			NodeID:  a.cfg.NodeID,
			Lowmem:  lowmem,
			Members: a.cfg.Members,
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
// web.ts:89-90: empty or non-numeric input falls back to def.
func tryParseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
