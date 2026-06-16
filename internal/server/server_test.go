package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
