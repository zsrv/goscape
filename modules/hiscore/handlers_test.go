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
