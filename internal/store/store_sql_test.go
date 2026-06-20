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

func TestVoteNamesByAnswer(t *testing.T) {
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

	s := &Store{db: db}

	// Insert votes: 2 on "a" (Mallory, Bob), 1 on "b" (Alice), 1 on "a" (empty)
	votes := []struct {
		answer string
		voter  string
		name   string
	}{
		{"a", "v1", "Mallory"},
		{"a", "v2", "Bob"},
		{"b", "v3", "Alice"},
		{"a", "v4", ""},
	}
	for _, v := range votes {
		if err := s.RecordVote("s1", v.answer, v.voter, v.name); err != nil {
			t.Fatalf("RecordVote: %v", err)
		}
	}

	result, err := s.VoteNamesByAnswer("s1")
	if err != nil {
		t.Fatalf("VoteNamesByAnswer: %v", err)
	}

	// Check answer "a" has "Bob" and "Mallory" (alphabetical)
	aVoters := result["a"]
	if len(aVoters) != 2 {
		t.Fatalf("answer 'a': expected 2 voters, got %d: %v", len(aVoters), aVoters)
	}
	if aVoters[0] != "Bob" || aVoters[1] != "Mallory" {
		t.Fatalf("answer 'a': expected [Bob Mallory], got %v", aVoters)
	}

	// Check answer "b" has "Alice"
	bVoters := result["b"]
	if len(bVoters) != 1 || bVoters[0] != "Alice" {
		t.Fatalf("answer 'b': expected [Alice], got %v", bVoters)
	}

	// Check unknown survey returns empty map
	empty, err := s.VoteNamesByAnswer("nonexistent")
	if err != nil {
		t.Fatalf("VoteNamesByAnswer(nonexistent): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty map, got %v", empty)
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
