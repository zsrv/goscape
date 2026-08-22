package account

import (
	"flag"
	"net/http"
	"net/url"
)

// NewTestConfig returns a Config at flag defaults. Exported for
// cross-module integration tests (modules/login e2e); production code
// builds Config through the app's flag/YAML pipeline instead.
func NewTestConfig() Config {
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	return c
}

// CSRFTokenForTest derives the portal CSRF token for a raw session
// cookie value. Exported for cross-module integration tests only.
func CSRFTokenForTest(rawSessionToken string) string { return csrfToken(rawSessionToken) }

// AnonymousCSRFTokenForTest returns the double-submit CSRF token (SEC1
// M-8) that jar holds for u — the raw goscape_csrf cookie value the
// anonymous /login, /register, /forgot-password and /reset-password
// forms require back as the "csrf" field. GET a public form page first
// (e.g. /login) so the portal has a chance to mint and set the cookie;
// returns "" if it never got one. Exported for cross-module integration
// tests only.
func AnonymousCSRFTokenForTest(jar http.CookieJar, u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	for _, c := range jar.Cookies(parsed) {
		if c.Name == csrfCookieName {
			return c.Value
		}
	}
	return ""
}
