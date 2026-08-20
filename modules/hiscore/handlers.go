package hiscore

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/middleware"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

// Error codes. These are part of the public contract.
const (
	codeNotFound       = "not_found"
	codeInvalidRequest = "invalid_request"
	codeRateLimited    = "rate_limited"
	codeInternal       = "internal"
)

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// api holds the HTTP surface. now is injectable so tests can freeze the
// clock that drives the visibility filter and the limiter. It must
// always yield UTC: modernc.org/sqlite serializes time.Time via
// t.String() with no zone conversion, so a non-UTC clock would corrupt
// every comparison against stored (UTC) timestamps.
type api struct {
	cfg       Config
	store     *Store
	sourceIPs *middleware.SourceIPExtractor
	limiter   *backstop
	now       func() time.Time
	log       *slog.Logger
}

func newAPI(cfg Config, store *Store, log *slog.Logger) (*api, error) {
	// Always construct the extractor: with header and regex both blank
	// it uses dskit's built-in Forwarded / X-Real-IP / X-Forwarded-For
	// chain, which is the default and is what a gateway populates.
	// NewSourceIPs only errors when exactly one of the pair is set.
	sourceIPs, err := middleware.NewSourceIPs(
		cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex, cfg.Server.LogSourceIPsFull)
	if err != nil {
		return nil, fmt.Errorf("hiscore: source IP extractor: %w", err)
	}
	return &api{
		cfg:       cfg,
		store:     store,
		sourceIPs: sourceIPs,
		limiter:   newBackstop(cfg.BackstopRate),
		now:       func() time.Time { return time.Now().UTC() },
		log:       log,
	}, nil
}

// register wires the routes. Every route goes through guard, which
// applies the backstop limiter.
func (a *api) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/skills", a.guard(a.handleSkills))
	mux.HandleFunc("GET /v1/players/{name}", a.guard(a.handlePlayer))
	mux.HandleFunc("GET /v1/leaderboards/{skill}", a.guard(a.handleLeaderboard))
	// A catch-all so unmatched paths get the JSON error shape rather
	// than net/http's text 404.
	mux.HandleFunc("/", a.guard(func(w http.ResponseWriter, r *http.Request) {
		a.writeError(w, http.StatusNotFound, codeNotFound, "no such resource")
	}))
}

// identify resolves the caller for logging and limiter keying.
func (a *api) identify(r *http.Request) caller {
	c := consumerFromHeaders(r, a.cfg.TrustGatewayHeaders)
	c.IP = a.clientIP(r)
	return c
}

// clientIP prefers the configured proxy header (dskit's extractor,
// which is how the real client IP is recovered from behind a gateway)
// and falls back to the socket peer.
func (a *api) clientIP(r *http.Request) string {
	if a.sourceIPs != nil {
		if ip := a.sourceIPs.Get(r); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// guard applies the backstop limiter to a handler.
func (a *api) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := a.identify(r)
		if !a.limiter.allow(c.limiterKey(), a.now()) {
			w.Header().Set("Retry-After", "60")
			a.writeError(w, http.StatusTooManyRequests, codeRateLimited,
				"too many requests; retry after 60s")
			return
		}
		h(w, r)
	}
}

func (a *api) handleSkills(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, struct {
		Skills []Skill `json:"skills"`
	}{Skills: Skills()}, time.Time{})
}

// writeJSON encodes v, sets caching headers, and honours If-None-Match.
// lastMod is written as Last-Modified when non-zero; build-static
// responses pass the zero time and carry an ETag only.
func (a *api) writeJSON(w http.ResponseWriter, r *http.Request, v any, lastMod time.Time) {
	body, err := json.Marshal(v)
	if err != nil {
		a.log.Error("hiscore: encoding response", slog.Any("err", err))
		a.writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
		return
	}

	sum := sha256.Sum256(body)
	etag := `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(a.cfg.CacheMaxAge.Seconds())))
	if !lastMod.IsZero() {
		h.Set("Last-Modified", lastMod.UTC().Format(http.TimeFormat))
	}

	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		a.log.Debug("hiscore: writing response", slog.Any("err", err))
	}
}

func (a *api) writeError(w http.ResponseWriter, status int, code, msg string) {
	var env errorEnvelope
	env.Error.Code = code
	env.Error.Message = msg

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Errors are per-caller and must not be cached at the edge.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		a.log.Debug("hiscore: writing error response", slog.Any("err", err))
	}
}

// skillEntry is one row of a player card. Pointer fields are null in
// JSON when the player has no row for that stat — the write path
// exports a stat only at base level >= 15, so a card is deliberately
// sparse. Every enabled stat is still listed, so a consumer can render
// a fixed table without special-casing absence.
type skillEntry struct {
	Type      int        `json:"type"`
	Name      string     `json:"name"`
	Ranked    bool       `json:"ranked"`
	Rank      *int64     `json:"rank"`
	Level     *int       `json:"level"`
	XP        *int64     `json:"xp"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type playerResponse struct {
	Name    string       `json:"name"`
	Profile string       `json:"profile"`
	Overall *skillEntry  `json:"overall"`
	Skills  []skillEntry `json:"skills"`
}

// wholeXP converts the stored fixed-point tenths to whole XP. This is
// the ONLY place the x10 representation is divided out.
func wholeXP(valueX10 int64) int64 { return valueX10 / 10 }

func (a *api) handlePlayer(w http.ResponseWriter, r *http.Request) {
	profile := a.profileParam(r)
	safeName := jstring.ToSafeName(r.PathValue("name"))
	if safeName == "" {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest, "player name is required")
		return
	}

	now := a.now()
	acct, err := a.store.LookupAccountByName(r.Context(), safeName, now)
	if errors.Is(err, ErrNotFound) {
		a.writeError(w, http.StatusNotFound, codeNotFound, "player not found")
		return
	}
	if err != nil {
		a.internal(w, "lookup account", err)
		return
	}

	card, err := a.store.PlayerCard(r.Context(), profile, acct.ID, now)
	if err != nil {
		a.internal(w, "player card", err)
		return
	}
	// A visible account that has never been exported is reported the
	// same as an unknown one: there is no standing to show.
	if card.Overall == nil && len(card.Skills) == 0 {
		a.writeError(w, http.StatusNotFound, codeNotFound, "player not found")
		return
	}

	byType := make(map[int]Entry, len(card.Skills))
	var newest time.Time
	for _, e := range card.Skills {
		byType[e.Type] = e
		if e.UpdatedAt.After(newest) {
			newest = e.UpdatedAt
		}
	}

	resp := playerResponse{
		Name:    jstring.ToDisplayName(acct.Username),
		Profile: profile,
		Skills:  make([]skillEntry, 0, len(Skills())),
	}
	if card.Overall != nil {
		e := entryToJSON(0, SkillOverall, *card.Overall)
		resp.Overall = &e
		if card.Overall.UpdatedAt.After(newest) {
			newest = card.Overall.UpdatedAt
		}
	}
	for _, s := range Skills() {
		if e, ok := byType[s.Type]; ok {
			resp.Skills = append(resp.Skills, entryToJSON(s.Type, s.Name, e))
			continue
		}
		resp.Skills = append(resp.Skills, skillEntry{Type: s.Type, Name: s.Name, Ranked: false})
	}

	a.writeJSON(w, r, resp, newest)
}

func entryToJSON(typ int, name string, e Entry) skillEntry {
	rank, level, xp, at := e.Rank, e.Level, wholeXP(e.ValueX10), e.UpdatedAt
	return skillEntry{
		Type: typ, Name: name, Ranked: true,
		Rank: &rank, Level: &level, XP: &xp, UpdatedAt: &at,
	}
}

// profileParam returns the requested profile, defaulting to config.
func (a *api) profileParam(r *http.Request) string {
	if p := r.URL.Query().Get("profile"); p != "" {
		return p
	}
	return a.cfg.Profile
}

// internal logs the real cause and returns an opaque 500. Callers never
// see SQL, table names, or account ids.
func (a *api) internal(w http.ResponseWriter, what string, err error) {
	a.log.Error("hiscore: "+what, slog.Any("err", err))
	a.writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
}

type boardEntry struct {
	Rank      int64     `json:"rank"`
	Name      string    `json:"name"`
	Level     int       `json:"level"`
	XP        int64     `json:"xp"`
	UpdatedAt time.Time `json:"updated_at"`
}

type leaderboardResponse struct {
	Skill   string       `json:"skill"`
	Profile string       `json:"profile"`
	Entries []boardEntry `json:"entries"`
	// NextCursor is empty when the page reached the end of the board.
	// Bulk readers should follow it rather than incrementing offset:
	// OFFSET is O(offset), so an offset walk of the whole board is
	// quadratic while a cursor walk is linear.
	NextCursor string `json:"next_cursor"`
}

func (a *api) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	skill, ok := SkillByName(r.PathValue("skill"))
	if !ok {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest, "unknown skill")
		return
	}

	q := r.URL.Query()
	profile := a.profileParam(r)

	limit, err := a.intParam(q, "limit", a.cfg.DefaultLimit)
	if err != nil || limit < 1 || limit > a.cfg.MaxLimit {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
			fmt.Sprintf("limit must be an integer in [1, %d]", a.cfg.MaxLimit))
		return
	}

	rawCursor := q.Get("cursor")
	hasOffset := q.Has("offset")
	if rawCursor != "" && hasOffset {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
			"offset and cursor are mutually exclusive")
		return
	}

	now := a.now()
	var rows []Row

	if rawCursor != "" {
		cur, cerr := DecodeCursor(rawCursor)
		if cerr != nil {
			a.writeError(w, http.StatusBadRequest, codeInvalidRequest, "malformed cursor")
			return
		}
		rows, err = a.store.LeaderboardByCursor(r.Context(), profile, skill.Type, cur, limit, now)
	} else {
		offset, oerr := a.intParam(q, "offset", 0)
		if oerr != nil || offset < 0 {
			a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
				"offset must be a non-negative integer")
			return
		}
		if offset+limit > a.cfg.LeaderboardMaxRank {
			a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
				fmt.Sprintf("offset+limit must not exceed %d; use cursor paging for deep reads",
					a.cfg.LeaderboardMaxRank))
			return
		}
		rows, err = a.store.LeaderboardByOffset(r.Context(), profile, skill.Type, offset, limit, now)
	}
	if err != nil {
		a.internal(w, "leaderboard", err)
		return
	}

	resp := leaderboardResponse{
		Skill:   skill.Name,
		Profile: profile,
		Entries: make([]boardEntry, 0, len(rows)),
	}
	var newest time.Time
	for _, row := range rows {
		resp.Entries = append(resp.Entries, boardEntry{
			Rank:      row.Rank,
			Name:      jstring.ToDisplayName(row.Username),
			Level:     row.Level,
			XP:        wholeXP(row.ValueX10),
			UpdatedAt: row.UpdatedAt,
		})
		if row.UpdatedAt.After(newest) {
			newest = row.UpdatedAt
		}
	}

	// A short page means the board is exhausted; only hand out a cursor
	// when there is plausibly more to read.
	if len(rows) == limit {
		last := rows[len(rows)-1]
		resp.NextCursor = Cursor{
			ValueX10:  last.ValueX10,
			UpdatedAt: last.UpdatedAt,
			AccountID: last.AccountID,
			Rank:      last.Rank + 1,
		}.Encode()
	}

	a.writeJSON(w, r, resp, newest)
}

// intParam parses an optional integer query parameter. A present but
// empty value is treated as absent.
func (a *api) intParam(q url.Values, name string, def int) (int, error) {
	raw := q.Get(name)
	if raw == "" {
		return def, nil
	}
	return strconv.Atoi(raw)
}
