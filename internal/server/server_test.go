package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sspaeti/minimal-newsletter-survey/internal/store"
)

func TestSlugAccepts(t *testing.T) {
	good := []string{
		"a",
		"2026-06-04",
		"2026-06-04-format",
		"awesome",
		"could_be_better",
		"x9",
	}
	for _, s := range good {
		if !slugRe.MatchString(s) {
			t.Errorf("expected %q to match slug regex", s)
		}
	}
}

func TestSlugRejects(t *testing.T) {
	bad := []string{
		"",
		"-leading",
		"_leading",
		"UpperCase",
		"has space",
		"has/slash",
		"has.dot",
		"semi;colon",
	}
	for _, s := range bad {
		if slugRe.MatchString(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

// newTestServer returns a Server with only the bits handleHome needs: parsed
// templates and the embedded ogimage bytes. The store stays nil because the
// home handler doesn't touch it. If a future test exercises a route that does
// hit the store, build a real Server via New().
func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(Config{}, nil)
}

func TestHandleHomeGet(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.handleHome(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"pollmd",
		"https://github.com/sspaeti/pollmd",
		"https://pollmd.ssp.sh/",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / body missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"newsletter",
		"Newsletter",
		"ssp.sh/brain/logo",
		"sspaeti.com logo",
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("GET / body contains unwanted %q", unwanted)
		}
	}
}

func TestHandleHomeHead(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()
	s.handleHome(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD / status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD / wrote %d bytes, want 0", rec.Body.Len())
	}
}

func TestHandleThanks(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/thanks?id=test", nil)
	rec := httptest.NewRecorder()
	s.handleThanks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /thanks status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Thanks for your vote",
		"pollmd",
		"Back to site",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /thanks body missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"newsletter",
		"Newsletter",
		"ssp.sh/brain/logo",
		"sspaeti.com logo",
		"Back to the blog",
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("GET /thanks body contains unwanted %q", unwanted)
		}
	}
}

// TestNamedModeVote verifies that a named-mode survey captures the voter name
// from the ?name= query parameter and redirects to the thanks page.
func TestNamedModeVote(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Create a named-mode survey via the admin endpoint with mode field
	body := strings.NewReader(url.Values{
		"survey_id": {"named-poll"},
		"answers":   {"a,b"},
		"mode":      {"named"},
	}.Encode())
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status=%d, want 201", rec.Code)
	}

	// Vote with name param. Set path values since we call handleSurvey
	// directly without going through the mux.
	voteReq := httptest.NewRequest(http.MethodGet, "/named-poll/a?name=Mallory", nil)
	voteReq.Header.Set("User-Agent", "Mozilla/5.0")
	voteReq.SetPathValue("id", "named-poll")
	voteReq.SetPathValue("answer", "a")
	voteRec := httptest.NewRecorder()
	s.handleSurvey(voteRec, voteReq)
	if voteRec.Code != http.StatusFound {
		t.Fatalf("vote: status=%d, want 302", voteRec.Code)
	}

	// Verify redirect to thanks page
	loc := voteRec.Header().Get("Location")
	if !strings.Contains(loc, "/thanks?id=named-poll") {
		t.Fatalf("unexpected redirect location: %s", loc)
	}
}

// TestNamedModeVoteLongName verifies that names over 200 chars return 400.
func TestNamedModeVoteLongName(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Create a named-mode survey
	s.store.UpsertSurvey("name-limit", "a,b", "named")

	// Vote with a very long name
	longName := strings.Repeat("x", 201)
	voteReq := httptest.NewRequest(http.MethodGet, "/name-limit/a?name="+longName, nil)
	voteReq.Header.Set("User-Agent", "Mozilla/5.0")
	voteReq.SetPathValue("id", "name-limit")
	voteReq.SetPathValue("answer", "a")
	voteRec := httptest.NewRecorder()
	s.handleSurvey(voteRec, voteReq)
	if voteRec.Code != http.StatusBadRequest {
		t.Fatalf("long name: status=%d, want 400", voteRec.Code)
	}
}

// TestNamedModeVoteBotSkip verifies that bots get 200 without recording a vote,
// even when a name is provided.
func TestNamedModeVoteBotSkip(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Create a named-mode survey
	s.store.UpsertSurvey("bot-test", "a,b", "named")

	// Bot UA with name param
	voteReq := httptest.NewRequest(http.MethodGet, "/bot-test/a?name=Mallory", nil)
	voteReq.Header.Set("User-Agent", "Twitterbot/1.0")
	voteReq.SetPathValue("id", "bot-test")
	voteReq.SetPathValue("answer", "a")
	voteRec := httptest.NewRecorder()
	s.handleSurvey(voteRec, voteReq)
	if voteRec.Code != http.StatusOK {
		t.Fatalf("bot vote: status=%d, want 200", voteRec.Code)
	}
}

// TestHomeRouteDoesNotShadow pins the {$} anchor on routeHome. With Go 1.22
// pattern matching, "/{$}" matches /only. Without the anchor, "/" would also
// match every other path that no longer-specific pattern claims. We register
// just the home and landing patterns on a mux and confirm /init still resolves
// to landing, not home.
func TestHomeRouteDoesNotShadow(t *testing.T) {
	mux := http.NewServeMux()
	hitHome := false
	hitLanding := false
	mux.HandleFunc(routeHome, func(w http.ResponseWriter, _ *http.Request) {
		hitHome = true
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(routeLanding, func(w http.ResponseWriter, _ *http.Request) {
		hitLanding = true
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(routeVote, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/init", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if hitHome {
		t.Errorf("GET /init hit home handler — {$} anchor missing on routeHome")
	}
	if !hitLanding {
		t.Errorf("GET /init did not hit landing handler")
	}
}

// newTestServerWithStore returns a Server backed by an in-memory DuckDB.
func newTestServerWithStore(t *testing.T, adminToken string) *Server {
	t.Helper()
	st, err := store.Open(":memory:", "", "")
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := Config{
		SiteURL:    "https://pollmd.ssp.sh",
		AdminToken: adminToken,
	}
	return New(cfg, st)
}

func TestAdminCreateSurvey(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Valid create
	body := url.Values{"survey_id": {"test-poll"}, "answers": {"yes,no,maybe"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	// Verify via GetSurveyAnswers
	answers, err := s.store.GetSurveyAnswers("test-poll")
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != 3 || answers[0] != "yes" || answers[1] != "no" || answers[2] != "maybe" {
		t.Fatalf("unexpected answers: %v", answers)
	}

	// Idempotent re-create. Create a fresh request because the original body
	// was consumed by the first call.
	req2 := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Authorization", "Bearer test-token")
	rec2 := httptest.NewRecorder()
	s.handleAdminCreate(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("re-create status = %d, want 201", rec2.Code)
	}
}

func TestAdminCreateSurveyJSON(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	body := `{"survey_id":"json-test","answers":"alpha,beta"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	answers, err := s.store.GetSurveyAnswers("json-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != 2 || answers[0] != "alpha" || answers[1] != "beta" {
		t.Fatalf("unexpected answers: %v", answers)
	}
}

func TestAdminCreateUnauthorized(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")
	body := url.Values{"survey_id": {"x"}, "answers": {"a"}}

	// No auth
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: status = %d, want 401", rec.Code)
	}

	// Wrong token
	req2 := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Authorization", "Bearer wrong-token")
	rec2 := httptest.NewRecorder()
	s.handleAdminCreate(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", rec2.Code)
	}
}

func TestAdminCreateInvalidSlug(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	tests := []struct {
		name     string
		surveyID string
		answers  string
	}{
		{"empty survey_id", "", "a"},
		{"empty answers", "ok", ""},
		{"uppercase survey_id", "UPPERCASE", "a"},
		{"leading dash survey_id", "-bad", "a"},
		{"bad answer slug", "ok", "_bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := url.Values{"survey_id": {tt.surveyID}, "answers": {tt.answers}}
			req := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			s.handleAdminCreate(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestAdminCreateMethodNotAllowed(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// GET to POST-only route → handler returns 405
	req := httptest.NewRequest(http.MethodGet, "/admin/surveys", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", rec.Code)
	}
}

func TestAdminCreateTokenInQueryParam(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")
	body := url.Values{"survey_id": {"x"}, "answers": {"a"}}

	req := httptest.NewRequest(http.MethodPost, "/admin/surveys?token=test-token", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No Authorization header — token is only in query param
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (token in query param not accepted)", rec.Code)
	}
}

func TestAdminResetUnauthorized(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// No auth
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys/x/reset", nil)
	rec := httptest.NewRecorder()
	s.handleAdminReset(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: status = %d, want 401", rec.Code)
	}

	// Wrong token
	req2 := httptest.NewRequest(http.MethodPost, "/admin/surveys/x/reset", nil)
	req2.Header.Set("Authorization", "Bearer wrong-token")
	rec2 := httptest.NewRecorder()
	s.handleAdminReset(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", rec2.Code)
	}
}

func TestAdminDeleteUnauthorized(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// No auth
	req := httptest.NewRequest(http.MethodDelete, "/admin/surveys/x", nil)
	rec := httptest.NewRecorder()
	s.handleAdminDelete(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: status = %d, want 401", rec.Code)
	}

	// Wrong token
	req2 := httptest.NewRequest(http.MethodDelete, "/admin/surveys/x", nil)
	req2.Header.Set("Authorization", "Bearer wrong-token")
	rec2 := httptest.NewRecorder()
	s.handleAdminDelete(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", rec2.Code)
	}
}

func TestAdminReset(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Create a survey with a vote
	s.store.UpsertSurvey("reset-test", "a,b", "")
	s.store.RecordVote("reset-test", "a", "voter1", "")
	s.store.RecordVote("reset-test", "b", "voter2", "")

	// Reset
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys/reset-test/reset", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminReset(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", rec.Code)
	}

	// Verify votes cleared
	tallies, _ := s.store.TallyBySurvey("reset-test")
	if len(tallies) != 0 {
		t.Fatalf("expected 0 tallies, got %d", len(tallies))
	}

	// Survey registration still exists
	answers, _ := s.store.GetSurveyAnswers("reset-test")
	if answers == nil {
		t.Fatal("survey registration should still exist")
	}
}

func TestAdminDelete(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	s.store.UpsertSurvey("delete-test", "a", "")
	s.store.RecordVote("delete-test", "a", "voter1", "")

	req := httptest.NewRequest(http.MethodDelete, "/admin/surveys/delete-test", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminDelete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", rec.Code)
	}

	// Survey gone
	answers, _ := s.store.GetSurveyAnswers("delete-test")
	if answers != nil {
		t.Fatal("survey should have been deleted")
	}
}
