package main

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sspaeti/minimal-newsletter-survey/internal/server"
	"github.com/sspaeti/minimal-newsletter-survey/internal/store"
)

func newTestServer(t *testing.T) (*store.Store, *httptest.Server, string) {
	t.Helper()
	token := "test-token"
	st, err := store.Open(":memory:", "", "")
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := server.Config{
		SiteURL:    "http://localhost",
		AdminToken: token,
	}
	srv := server.New(cfg, st)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return st, ts, token
}

func TestPollctlListEmpty(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"list"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "No surveys found") {
		t.Errorf("expected 'No surveys found', got: %s", out.String())
	}
}

func TestPollctlCreate(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"create", "--answers", "a,b,c", "--mode", "named", "test-poll"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "test-poll") {
		t.Errorf("expected survey ID in output, got: %s", out.String())
	}
}

func TestPollctlCreateAndList(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	// Create
	var out1, errOut1 bytes.Buffer
	code := Run([]string{"create", "--answers", "yes,no", "cli-test"}, &out1, &errOut1, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("create: exit %d, stderr: %s", code, errOut1.String())
	}

	// List
	var out2, errOut2 bytes.Buffer
	code = Run([]string{"list"}, &out2, &errOut2, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("list: exit %d, stderr: %s", code, errOut2.String())
	}
	if !strings.Contains(out2.String(), "cli-test") {
		t.Errorf("expected cli-test in list, got: %s", out2.String())
	}
	// Should NOT show No surveys found
	if strings.Contains(out2.String(), "No surveys found") {
		t.Errorf("unexpected 'No surveys found' after creating a survey")
	}
}

func TestPollctlUnauthorized(t *testing.T) {
	_, srv, _ := newTestServer(t) // token not passed
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"list"}, &out, &errOut, strings.NewReader(""), srv.URL, "wrong-token")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "401") {
		t.Errorf("expected 401 error, got: %s", errOut.String())
	}
}

func TestPollctlResetWithConfirmation(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	// Create survey
	var createOut, createErr bytes.Buffer
	code := Run([]string{"create", "--answers", "a", "reset-test"}, &createOut, &createErr, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("create: exit %d", code)
	}

	// Reset with --yes flag
	var out, errOut bytes.Buffer
	code = Run([]string{"reset", "--yes", "reset-test"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("reset: exit %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "reset") {
		t.Errorf("expected 'reset' in output, got: %s", out.String())
	}
}

func TestPollctlDeleteWithConfirmation(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	// Create survey
	Run([]string{"create", "--answers", "a", "del-test"}, &bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""), srv.URL, token)

	// Delete with --yes
	var out, errOut bytes.Buffer
	code := Run([]string{"delete", "--yes", "del-test"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("delete: exit %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "deleted") {
		t.Errorf("expected 'deleted' in output, got: %s", out.String())
	}
}

func TestPollctlResultNoVotes(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	Run([]string{"create", "--answers", "a,b", "result-test"}, &bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""), srv.URL, token)

	var out, errOut bytes.Buffer
	code := Run([]string{"result", "result-test"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("result: exit %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "No votes") {
		t.Errorf("expected 'No votes', got: %s", out.String())
	}
}

func TestPollctlUnknownSubcommand(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"foobar"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "unknown subcommand") {
		t.Errorf("expected 'unknown subcommand', got: %s", errOut.String())
	}
}

func TestPollctlHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"help"}, &out, &errOut, strings.NewReader(""), "http://localhost", "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "pollctl") {
		t.Errorf("expected help text with 'pollctl', got: %s", out.String())
	}
}

func TestPollctlListJSON(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	// Create a survey first
	Run([]string{"create", "--answers", "x,y", "--mode", "anonymous", "json-test"}, &bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""), srv.URL, token)

	var out, errOut bytes.Buffer
	code := Run([]string{"list", "--json"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "json-test") {
		t.Errorf("expected JSON output with survey ID, got: %s", out.String())
	}
	if !strings.Contains(out.String(), `"survey_id"`) {
		t.Errorf("expected JSON keys, got: %s", out.String())
	}
}

func TestPollctlResultNotFound(t *testing.T) {
	_, srv, token := newTestServer(t)
	defer srv.Close()

	var out, errOut bytes.Buffer
	code := Run([]string{"result", "nonexistent"}, &out, &errOut, strings.NewReader(""), srv.URL, token)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "not found") {
		t.Errorf("expected 'not found', got: %s", errOut.String())
	}
}
