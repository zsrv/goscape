package account

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
)

//go:embed templates static
var assetsFS embed.FS

// portal is the SSR web application. Handlers hang off this struct and
// are registered in routes(); later tasks add session middleware and
// the remaining page handlers in sibling files.
type portal struct {
	cfg    Config
	store  *Store
	mailer Mailer
	log    *slog.Logger
	pages  map[string]*template.Template
	rl     *rateLimiter
}

type pageData struct {
	Account *PortalAccount // nil when unauthenticated
	CSRF    string         // per-session CSRF token, "" when no session
	Msg     string         // flash message from ?msg= query param
	Data    any            // page-specific payload
}

func newPortal(cfg Config, store *Store, mailer Mailer, log *slog.Logger) (*portal, error) {
	pageFiles, err := fs.Glob(assetsFS, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob pages: %w", err)
	}
	pages := make(map[string]*template.Template, len(pageFiles))
	for _, f := range pageFiles {
		t, err := template.ParseFS(assetsFS, "templates/base.html", f)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		pages[path.Base(f)] = t
	}
	return &portal{
		cfg: cfg, store: store, mailer: mailer, log: log,
		pages: pages,
		rl:    newRateLimiter(),
	}, nil
}

// render executes a page template inside base.html. Errors after the
// header is written are unrecoverable, so pages render to a buffer
// first.
func (p *portal) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	pd := pageData{Account: ctxAccount(r), Msg: r.URL.Query().Get("msg"), Data: data}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		pd.CSRF = csrfToken(c.Value)
	}
	tmpl, ok := p.pages[page]
	if !ok {
		p.log.Error("unknown page template", slog.String("page", page))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", pd); err != nil {
		p.log.Error("render failed", slog.String("page", page), slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func (p *portal) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.FileServerFS(assetsFS))
	mux.HandleFunc("GET /{$}", p.public(p.handleHome))
	// Tasks 14-20 register the remaining routes here.
	return mux
}

func (p *portal) handleHome(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "home.html", nil)
}
