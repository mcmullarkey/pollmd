package store

import (
	"database/sql"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestRecordVoteWithName verifies that RecordVote stores a voter_name
// when one is provided. This tests the named-voting mode.
func TestRecordVoteWithName(t *testing.T) {
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema with voter_name column (the new schema)
	if _, err := db.Exec(`CREATE TABLE votes (
		ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		survey_id VARCHAR NOT NULL,
		answer VARCHAR NOT NULL,
		voter VARCHAR NOT NULL,
		voter_name VARCHAR,
		PRIMARY KEY (survey_id, voter)
	)`); err != nil {
		t.Fatalf("create votes: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE surveys (
		survey_id VARCHAR PRIMARY KEY,
		allowed_answers VARCHAR NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		mode VARCHAR DEFAULT 'anonymous'
	)`); err != nil {
		t.Fatalf("create surveys: %v", err)
	}

	s := &Store{db: db}

	// Insert a named-mode survey
	if _, err := db.Exec(`INSERT INTO surveys (survey_id, allowed_answers, mode) VALUES ('s1', 'a,b', 'named')`); err != nil {
		t.Fatalf("insert survey: %v", err)
	}

	// RecordVote with a name
	if err := s.RecordVote("s1", "a", "voter1", "Mallory"); err != nil {
		t.Fatalf("RecordVote: %v", err)
	}

	// Verify name stored
	var name string
	if err := db.QueryRow(`SELECT voter_name FROM votes WHERE survey_id = 's1' AND voter = 'voter1'`).Scan(&name); err != nil {
		t.Fatalf("query voter_name: %v", err)
	}
	if name != "Mallory" {
		t.Fatalf("expected voter_name='Mallory', got %q", name)
	}
}

// TestRecordVoteEmptyName verifies that RecordVote stores an empty string
// for voter_name when no name is provided (anonymous-mode behavior).
func TestRecordVoteEmptyName(t *testing.T) {
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create schema with voter_name column
	if _, err := db.Exec(`CREATE TABLE votes (
		ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		survey_id VARCHAR NOT NULL,
		answer VARCHAR NOT NULL,
		voter VARCHAR NOT NULL,
		voter_name VARCHAR,
		PRIMARY KEY (survey_id, voter)
	)`); err != nil {
		t.Fatalf("create votes: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE surveys (
		survey_id VARCHAR PRIMARY KEY,
		allowed_answers VARCHAR NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		mode VARCHAR DEFAULT 'anonymous'
	)`); err != nil {
		t.Fatalf("create surveys: %v", err)
	}

	s := &Store{db: db}

	// Insert a survey (default mode = anonymous)
	if _, err := db.Exec(`INSERT INTO surveys (survey_id, allowed_answers) VALUES ('s1', 'a,b')`); err != nil {
		t.Fatalf("insert survey: %v", err)
	}

	// RecordVote with empty name (as handler would for anonymous mode)
	if err := s.RecordVote("s1", "a", "voter1", ""); err != nil {
		t.Fatalf("RecordVote: %v", err)
	}

	// Verify voter_name is empty string (not NULL)
	var name string
	if err := db.QueryRow(`SELECT voter_name FROM votes WHERE survey_id = 's1' AND voter = 'voter1'`).Scan(&name); err != nil {
		t.Fatalf("query voter_name: %v", err)
	}
	if name != "" {
		t.Fatalf("expected empty voter_name, got %q", name)
	}

	// Verify the vote was recorded despite empty name
	var answer string
	if err := db.QueryRow(`SELECT answer FROM votes WHERE survey_id = 's1' AND voter = 'voter1'`).Scan(&answer); err != nil {
		t.Fatalf("query answer: %v", err)
	}
	if answer != "a" {
		t.Fatalf("expected answer 'a', got %q", answer)
	}

	// Verify GetSurveyMode returns anonymous for an unregistered survey
	// (legacy behavior — AC 11: surveys without a mode row default to anonymous).
	mode, err := s.GetSurveyMode("nonexistent")
	if err != nil {
		t.Fatalf("GetSurveyMode: %v", err)
	}
	if mode != "anonymous" {
		t.Fatalf("expected 'anonymous' for missing survey, got %q", mode)
	}
}

func TestGetSurveyMode(t *testing.T) {
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create surveys table with mode column (new schema)
	if _, err := db.Exec(`CREATE TABLE surveys (
		survey_id VARCHAR PRIMARY KEY,
		allowed_answers VARCHAR NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		mode VARCHAR DEFAULT 'anonymous'
	)`); err != nil {
		t.Fatalf("create surveys: %v", err)
	}

	s := &Store{db: db}

	// Test 1: Named mode
	if _, err := db.Exec(`INSERT INTO surveys (survey_id, allowed_answers, mode) VALUES ('named-survey', 'a,b', 'named')`); err != nil {
		t.Fatalf("insert named: %v", err)
	}
	mode, err := s.GetSurveyMode("named-survey")
	if err != nil {
		t.Fatalf("GetSurveyMode(named-survey): %v", err)
	}
	if mode != "named" {
		t.Fatalf("expected 'named', got %q", mode)
	}

	// Test 2: Default mode (anonymous)
	if _, err := db.Exec(`INSERT INTO surveys (survey_id, allowed_answers) VALUES ('default-survey', 'a,b')`); err != nil {
		t.Fatalf("insert default: %v", err)
	}
	mode, err = s.GetSurveyMode("default-survey")
	if err != nil {
		t.Fatalf("GetSurveyMode(default-survey): %v", err)
	}
	if mode != "anonymous" {
		t.Fatalf("expected 'anonymous', got %q", mode)
	}

	// Test 3: Missing survey → anonymous
	mode, err = s.GetSurveyMode("nonexistent")
	if err != nil {
		t.Fatalf("GetSurveyMode(nonexistent): %v", err)
	}
	if mode != "anonymous" {
		t.Fatalf("expected 'anonymous' for missing survey, got %q", mode)
	}
}

// Sanity check the upsert statement against the bundled DuckDB version.
func TestUpsertCurrentTimestamp(t *testing.T) {
	db, err := sql.Open("duckdb", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE votes (
        ts TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        survey_id VARCHAR NOT NULL,
        answer VARCHAR NOT NULL,
        voter VARCHAR NOT NULL,
        PRIMARY KEY (survey_id, voter)
    )`); err != nil {
		t.Fatalf("create: %v", err)
	}

	// First insert
	if _, err := db.Exec(`
        INSERT INTO votes (survey_id, answer, voter) VALUES ('s1','good','v1')
        ON CONFLICT (survey_id, voter) DO UPDATE
        SET answer = excluded.answer, ts = now()
    `); err != nil {
		t.Fatalf("insert 1: %v", err)
	}

	// Second insert triggers update branch
	if _, err := db.Exec(`
        INSERT INTO votes (survey_id, answer, voter) VALUES ('s1','ok','v1')
        ON CONFLICT (survey_id, voter) DO UPDATE
        SET answer = excluded.answer, ts = now()
    `); err != nil {
		t.Fatalf("insert 2: %v", err)
	}
}
