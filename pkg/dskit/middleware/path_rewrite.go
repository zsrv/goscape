// Provenance-includes-location: https://github.com/weaveworks/common/blob/main/middleware/path_rewrite.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Weaveworks Ltd.

package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
)

// PathRewrite supports regex matching and replace on Request URIs
func PathRewrite(log *slog.Logger, regexp *regexp.Regexp, replacement string) Interface {
	return pathRewrite{
		log:         log,
		regexp:      regexp,
		replacement: replacement,
	}
}

type pathRewrite struct {
	log         *slog.Logger
	regexp      *regexp.Regexp
	replacement string
}

func (p pathRewrite) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RequestURI = p.regexp.ReplaceAllString(r.RequestURI, p.replacement)
		r.URL.RawPath = p.regexp.ReplaceAllString(r.URL.EscapedPath(), p.replacement)
		path, err := url.PathUnescape(r.URL.RawPath)
		if err != nil {
			p.log.Error("got invalid url-encoded path after applying path rewrite",
				"path", r.URL.RawPath,
				"regexp", p.regexp.String(),
				"replacement", p.replacement,
				"error", err,
			)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.URL.Path = path
		next.ServeHTTP(w, r)
	})
}

// PathReplace replaces [http.Request.Method] with the specified string.
func PathReplace(replacement string) Interface {
	return pathReplace(replacement)
}

type pathReplace string

func (p pathReplace) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = string(p)
		r.RequestURI = string(p)
		next.ServeHTTP(w, r)
	})
}
