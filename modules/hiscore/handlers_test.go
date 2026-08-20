package hiscore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// newTestAPI builds an api over a fresh DB with a frozen clock.
func newTestAPI(t *testing.T) (*api, *gamedb.DB) {
	t.Helper()
	db := createTestDB(t)
	cfg := defaultConfig(t)
	cfg.Enable = true

	a, err := newAPI(cfg, NewStore(db), noopLogger())
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	a.now = func() time.Time { return testClock }
	return a, db
}

func doGET(t *testing.T, a *api, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	a.register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSkillsEndpoint(t *testing.T) {
	a, _ := newTestAPI(t)
	rec := doGET(t, a, "/v1/skills")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("ETag missing")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=60") {
		t.Errorf("Cache-Control = %q, want max-age=60", cc)
	}

	var body struct {
		Skills []Skill `json:"skills"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Skills) != 19 {
		t.Fatalf("got %d skills, want 19", len(body.Skills))
	}
	if body.Skills[0].Name != "attack" || body.Skills[0].Type != 1 {
		t.Errorf("first skill = %+v, want {Type:1 Name:attack}", body.Skills[0])
	}
}

func TestConditionalRequest_304(t *testing.T) {
	a, _ := newTestAPI(t)
	first := doGET(t, a, "/v1/skills")
	etag := first.Header().Get("ETag")

	mux := http.NewServeMux()
	a.register(mux)
	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("304 carried a %d-byte body, want empty", rec.Body.Len())
	}
}

func TestETagIsStableAcrossIdenticalRequests(t *testing.T) {
	a, _ := newTestAPI(t)
	first := doGET(t, a, "/v1/skills").Header().Get("ETag")
	second := doGET(t, a, "/v1/skills").Header().Get("ETag")
	if first != second {
		t.Errorf("ETag changed between identical requests: %q vs %q", first, second)
	}
}

func TestUnknownRoute_404JSON(t *testing.T) {
	a, _ := newTestAPI(t)
	rec := doGET(t, a, "/v1/nonsense")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	if body.Error.Code != codeNotFound {
		t.Errorf("code = %q, want %q", body.Error.Code, codeNotFound)
	}
}

func TestBackstopLimiter_429(t *testing.T) {
	db := createTestDB(t)
	cfg := defaultConfig(t)
	cfg.BackstopRate = 1
	a, err := newAPI(cfg, NewStore(db), noopLogger())
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	a.now = func() time.Time { return testClock }

	if rec := doGET(t, a, "/v1/skills"); rec.Code != http.StatusOK {
		t.Fatalf("first request: status %d, want 200", rec.Code)
	}
	rec := doGET(t, a, "/v1/skills")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: status %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 missing Retry-After")
	}
}

func TestPlayerEndpoint(t *testing.T) {
	a, db := newTestAPI(t)

	acct := insertAccount(t, db, "zezima", 0, nil)
	// 13,034,431 whole XP is stored as 130,344,310 tenths.
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 1893, 130_344_310, testClock)
	insertHiscore(t, db, "hiscore", acct, "main", 1, 99, 130_344_310, testClock)

	rec := doGET(t, a, "/v1/players/Zezima")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	var body playerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Name != "Zezima" {
		t.Errorf("name = %q, want display form Zezima", body.Name)
	}
	if body.Profile != "main" {
		t.Errorf("profile = %q, want main", body.Profile)
	}
	if body.Overall == nil {
		t.Fatal("overall = nil, want the aggregate entry")
	}
	if *body.Overall.XP != 13_034_431 {
		t.Errorf("overall xp = %d, want 13034431 (whole XP, x10 divided out)", *body.Overall.XP)
	}
	if len(body.Skills) != 19 {
		t.Fatalf("got %d skill entries, want all 19 enabled stats", len(body.Skills))
	}

	var attack, defence *skillEntry
	for i := range body.Skills {
		switch body.Skills[i].Name {
		case "attack":
			attack = &body.Skills[i]
		case "defence":
			defence = &body.Skills[i]
		}
	}
	if attack == nil || defence == nil {
		t.Fatal("attack and defence entries must both be present")
	}
	if !attack.Ranked {
		t.Error("attack: ranked = false, want true")
	}
	if attack.XP == nil || *attack.XP != 13_034_431 {
		t.Errorf("attack xp = %v, want 13034431", attack.XP)
	}
	if defence.Ranked {
		t.Error("defence: ranked = true, want false — no row below level 15")
	}
	if defence.XP != nil || defence.Rank != nil || defence.Level != nil {
		t.Errorf("defence: got xp=%v rank=%v level=%v, want all null",
			defence.XP, defence.Rank, defence.Level)
	}
}

// Name normalization: base37 safe-name round trip means these all
// address the same account.
func TestPlayerEndpoint_NameNormalization(t *testing.T) {
	a, db := newTestAPI(t)
	acct := insertAccount(t, db, "ze_zima", 0, nil)
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 100, 1_000_000, testClock)

	// A raw unescaped space in the target ("ZE ZIMA") is not usable here:
	// httptest.NewRequest builds a literal "GET <target> HTTP/1.0" request
	// line and http.ReadRequest splits it on the first space, so an
	// in-path space panics with "malformed HTTP version" before the
	// request ever reaches the mux. Percent-encoded, as any real client
	// would send it, exercises the same case+space normalization safely.
	for _, name := range []string{"ze_zima", "Ze_Zima", "Ze%20zima", "ZE%20ZIMA"} {
		rec := doGET(t, a, "/v1/players/"+name)
		if rec.Code != http.StatusOK {
			t.Errorf("GET /v1/players/%s: status %d, want 200", name, rec.Code)
		}
	}
}

func TestPlayerEndpoint_NotFound(t *testing.T) {
	a, db := newTestAPI(t)
	future := testClock.Add(24 * time.Hour)
	insertAccount(t, db, "cheater", 0, &future)
	insertAccount(t, db, "modash", 2, nil)

	// Unknown, banned, and staff must all be indistinguishable 404s.
	for _, name := range []string{"nobody", "cheater", "modash"} {
		rec := doGET(t, a, "/v1/players/"+name)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /v1/players/%s: status %d, want 404", name, rec.Code)
		}
	}
}

func TestPlayerEndpoint_NeverExportedIs404(t *testing.T) {
	a, db := newTestAPI(t)
	insertAccount(t, db, "freshman", 0, nil)

	rec := doGET(t, a, "/v1/players/freshman")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an account with no exported rows", rec.Code)
	}
}

func TestPlayerEndpoint_LastModified(t *testing.T) {
	a, db := newTestAPI(t)
	acct := insertAccount(t, db, "zezima", 0, nil)
	newest := testClock.Add(-2 * time.Hour)
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 100, 1_000_000, testClock.Add(-5*time.Hour))
	insertHiscore(t, db, "hiscore", acct, "main", 1, 99, 900_000, newest)

	rec := doGET(t, a, "/v1/players/zezima")
	got := rec.Header().Get("Last-Modified")
	if want := newest.UTC().Format(http.TimeFormat); got != want {
		t.Errorf("Last-Modified = %q, want %q (newest row in the response)", got, want)
	}
}

// A dead database must produce an opaque 500, not a panic and not a
// leak of SQL or internal identifiers.
func TestPlayerEndpoint_DatabaseFailure(t *testing.T) {
	a, db := newTestAPI(t)
	insertAccount(t, db, "zezima", 0, nil)

	if err := db.Close(); err != nil {
		t.Fatalf("closing db: %v", err)
	}

	rec := doGET(t, a, "/v1/players/zezima")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	if body.Error.Code != codeInternal {
		t.Errorf("code = %q, want %q", body.Error.Code, codeInternal)
	}
	for _, leak := range []string{"SELECT", "hiscore", "account_id", "sql"} {
		if strings.Contains(body.Error.Message, leak) {
			t.Errorf("error message %q leaks internals (%q)", body.Error.Message, leak)
		}
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — errors must not be cached at the edge", cc)
	}
}
