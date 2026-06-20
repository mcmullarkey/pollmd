package server

import (
	"encoding/json"
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

	// Verify name persisted in the DB
	names, err := s.store.VoteNames("named-poll")
	if err != nil {
		t.Fatalf("VoteNames: %v", err)
	}
	found := false
	for _, n := range names {
		if n == "Mallory" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected vote with name 'Mallory' in DB, got: %v", names)
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

// TestHandleResultNamedMode verifies voter names appear on the results page
// for named-mode surveys, with correct cross-answer isolation.
func TestHandleResultNamedMode(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Create named-mode survey
	s.store.UpsertSurvey("named-poll", "a,b", "named")

	// Record votes with names
	s.store.RecordVote("named-poll", "a", "voter1", "Mallory")
	s.store.RecordVote("named-poll", "a", "voter2", "Bob")
	s.store.RecordVote("named-poll", "b", "voter3", "Alice")

	req := httptest.NewRequest(http.MethodGet, "/result/named-poll", nil)
	req.SetPathValue("id", "named-poll")
	rec := httptest.NewRecorder()
	s.handleResult(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()

	// All names present
	for _, name := range []string{"Mallory", "Bob", "Alice"} {
		if !strings.Contains(body, name) {
			t.Errorf("expected name %q in body", name)
		}
	}

	// No "hashed anonymously" text (named mode)
	if strings.Contains(body, "hashed anonymously") {
		t.Error("named mode should not say 'hashed anonymously'")
	}

	// Voters class present
	if !strings.Contains(body, "voter-name") {
		t.Error("expected voter-name class in body")
	}
}

// TestHandleResultAnonymousMode verifies anonymous-mode results page
// shows no voter names.
func TestHandleResultAnonymousMode(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Create anonymous survey (default)
	s.store.UpsertSurvey("anon-poll", "a,b", "anonymous")

	// Record votes with names (should be ignored on display)
	s.store.RecordVote("anon-poll", "a", "voter1", "Mallory")
	s.store.RecordVote("anon-poll", "b", "voter2", "Bob")

	req := httptest.NewRequest(http.MethodGet, "/result/anon-poll", nil)
	req.SetPathValue("id", "anon-poll")
	rec := httptest.NewRecorder()
	s.handleResult(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()

	// No voter names
	if strings.Contains(body, "Mallory") || strings.Contains(body, "Bob") {
		t.Error("anonymous mode should not show voter names")
	}

	// "hashed anonymously" text present
	if !strings.Contains(body, "hashed anonymously") {
		t.Error("anonymous mode should say 'hashed anonymously'")
	}
}

// TestHandleResultNoVotes verifies the empty state.
func TestHandleResultNoVotes(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	s.store.UpsertSurvey("empty-poll", "a,b", "named")

	req := httptest.NewRequest(http.MethodGet, "/result/empty-poll", nil)
	req.SetPathValue("id", "empty-poll")
	rec := httptest.NewRecorder()
	s.handleResult(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "No votes recorded yet") {
		t.Error("expected empty state message")
	}
}

// TestHandleResultXSS verifies Go html/template auto-escaping.
func TestHandleResultXSS(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	s.store.UpsertSurvey("xss-poll", "a,b", "named")
	s.store.RecordVote("xss-poll", "a", "voter1", "<script>alert(1)</script>")

	req := httptest.NewRequest(http.MethodGet, "/result/xss-poll", nil)
	req.SetPathValue("id", "xss-poll")
	rec := httptest.NewRecorder()
	s.handleResult(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "<script>") && !strings.Contains(body, "&lt;") {
		t.Error("XSS: script tag not escaped. Body contains raw <script>")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected escaped script tag in body")
	}
}

// TestHandleResultHeadMethod verifies HEAD on results page.
func TestHandleResultHeadMethod(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")
	req := httptest.NewRequest(http.MethodHead, "/result/test", nil)
	rec := httptest.NewRecorder()
	s.handleResult(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", rec.Body.Len())
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

func TestAdminListSurveys(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	// Create two surveys, one with votes
	s.store.UpsertSurvey("poll-1", "a,b", "named")
	s.store.UpsertSurvey("poll-2", "c,d", "anonymous")
	s.store.RecordVote("poll-1", "a", "v1", "Alice")
	s.store.RecordVote("poll-1", "b", "v2", "Bob")

	req := httptest.NewRequest(http.MethodGet, "/admin/surveys", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var items []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	// poll-1 (most recent due to ORDER BY created_at DESC) should have vote_count=2
	if items[0]["survey_id"] == "poll-1" {
		vc := int(items[0]["vote_count"].(float64))
		if vc != 2 {
			t.Errorf("poll-1 vote_count = %d, want 2", vc)
		}
	}
}

func TestAdminListSurveysEmpty(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/admin/surveys", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var items []any
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected [] for empty store, got %d items", len(items))
	}
}

func TestAdminListSurveysUnauthorized(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")
	req := httptest.NewRequest(http.MethodGet, "/admin/surveys", nil)
	rec := httptest.NewRecorder()
	s.handleAdminList(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: status = %d, want 401", rec.Code)
	}
}

func TestAdminDetailSurvey(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	s.store.UpsertSurvey("detail-poll", "yes,no,maybe", "named")
	s.store.RecordVote("detail-poll", "yes", "v1", "Alice")
	s.store.RecordVote("detail-poll", "yes", "v2", "Bob")
	s.store.RecordVote("detail-poll", "no", "v3", "Mallory")

	req := httptest.NewRequest(http.MethodGet, "/admin/surveys/detail-poll", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var data map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&data); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if data["survey_id"] != "detail-poll" {
		t.Errorf("survey_id = %v, want detail-poll", data["survey_id"])
	}
	if data["mode"] != "named" {
		t.Errorf("mode = %v, want named", data["mode"])
	}

	// Check tallies exist
	tallies, ok := data["tallies"].([]any)
	if !ok {
		t.Fatal("tallies missing or not an array")
	}
	if len(tallies) != 2 { // 2 answers with votes
		t.Fatalf("expected 2 tallies, got %d", len(tallies))
	}

	// First tally should be "yes" with 2 clicks (most popular)
	first := tallies[0].(map[string]any)
	if first["answer"] != "yes" || int(first["clicks"].(float64)) != 2 {
		t.Errorf("first tally = %v, want {yes, 2}", first)
	}
}

func TestAdminDetailSurveyNotFound(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/admin/surveys/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminDetail(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminDetailSurveyInvalidSlug(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/admin/surveys/UPPERCASE", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminDetail(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminCreateReturnsJSON(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	body := `{"survey_id":"json-test","answers":"x,y,z","mode":"named"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["survey_id"] != "json-test" {
		t.Errorf("survey_id = %v, want json-test", resp["survey_id"])
	}
	if resp["mode"] != "named" {
		t.Errorf("mode = %v, want named", resp["mode"])
	}
}

func TestAdminCreateFormReturnsJSON(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	body := url.Values{"survey_id": {"form-test"}, "answers": {"a,b,c"}, "mode": {"anonymous"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["survey_id"] != "form-test" {
		t.Errorf("survey_id = %v, want form-test", resp["survey_id"])
	}
}

func TestAdminCreateReturnsJSONWithDefaultMode(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	body := `{"survey_id":"no-mode","answers":"a,b"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Default mode should be "anonymous"
	if resp["mode"] != "anonymous" {
		t.Errorf("default mode = %v, want anonymous", resp["mode"])
	}
}

func TestAdminResetNotFound(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	req := httptest.NewRequest(http.MethodPost, "/admin/surveys/nonexistent/reset", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminReset(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminDeleteNotFound(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	req := httptest.NewRequest(http.MethodDelete, "/admin/surveys/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminDelete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminResetReturnsJSON(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	s.store.UpsertSurvey("reset-json", "a,b", "")
	req := httptest.NewRequest(http.MethodPost, "/admin/surveys/reset-json/reset", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminReset(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}

func TestAdminDeleteReturnsJSON(t *testing.T) {
	s := newTestServerWithStore(t, "test-token")

	s.store.UpsertSurvey("del-json", "a,b", "")
	req := httptest.NewRequest(http.MethodDelete, "/admin/surveys/del-json", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	s.handleAdminDelete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}
