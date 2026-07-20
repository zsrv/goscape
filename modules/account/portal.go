package account

import (
	"log/slog"
	"net/http"
)

// portal is the SSR web application. Handlers hang off this struct and
// are registered in routes(); later tasks add templates, session
// middleware, and the page handlers in sibling files.
type portal struct {
	cfg    Config
	store  *Store
	mailer Mailer
	log    *slog.Logger
}

func newPortal(cfg Config, store *Store, mailer Mailer, log *slog.Logger) (*portal, error) {
	return &portal{cfg: cfg, store: store, mailer: mailer, log: log}, nil
}

func (p *portal) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
