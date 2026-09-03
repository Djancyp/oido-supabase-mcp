package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatRows(t *testing.T) {
	// Row array → table with deterministic (sorted) columns.
	out, err := formatRows(json.RawMessage(`[{"name":"a","id":1},{"name":"b","id":2}]`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "id | name") {
		t.Fatalf("columns not sorted: %q", out)
	}
	if !strings.Contains(out, "1 | a") || !strings.Contains(out, "Total rows: 2") {
		t.Fatalf("bad rows: %q", out)
	}

	// NULL handling.
	out, _ = formatRows(json.RawMessage(`[{"x":null}]`))
	if !strings.Contains(out, "NULL") {
		t.Fatalf("null not rendered: %q", out)
	}

	// Empty result.
	out, _ = formatRows(json.RawMessage(`[]`))
	if !strings.Contains(out, "0 rows") {
		t.Fatalf("empty not handled: %q", out)
	}

	// Non-array (status object) → passthrough.
	out, _ = formatRows(json.RawMessage(`{"status":"ok"}`))
	if !strings.Contains(out, "status") {
		t.Fatalf("status passthrough failed: %q", out)
	}
}

func TestQuoteLiteral(t *testing.T) {
	if got := quoteLiteral("it's"); got != "'it''s'" {
		t.Fatalf("quoteLiteral: %q", got)
	}
}

// The read_only flag sent to exec_sql is the write barrier, so it must be on
// whenever no write operation is enabled.
func TestWritesEnabledDrivesReadOnly(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SQLFunctionSettings)
		want   bool
	}{
		{"defaults are read-only", func(*SQLFunctionSettings) {}, false},
		{"select alone is read-only", func(s *SQLFunctionSettings) { s.AllowSelect = true }, false},
		{"insert enables writes", func(s *SQLFunctionSettings) { s.AllowInsert = true }, true},
		{"delete enables writes", func(s *SQLFunctionSettings) { s.AllowDelete = true }, true},
		{"drop enables writes", func(s *SQLFunctionSettings) { s.AllowDrop = true }, true},
		{"truncate enables writes", func(s *SQLFunctionSettings) { s.AllowTruncate = true }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultSQLFunctionSettings()
			tt.mutate(s)
			if got := s.writesEnabled(); got != tt.want {
				t.Errorf("writesEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrepareQuery(t *testing.T) {
	s := DefaultSQLFunctionSettings()
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"bare select gets a cap", "SELECT * FROM t", 10, "SELECT * FROM t LIMIT 10"},
		{"trailing semicolon is trimmed first", "SELECT * FROM t;", 10, "SELECT * FROM t LIMIT 10"},
		{"trailing semicolon and space", "SELECT * FROM t ;  ", 10, "SELECT * FROM t LIMIT 10"},
		{"existing cap is left alone", "SELECT * FROM t LIMIT 5", 10, "SELECT * FROM t LIMIT 5"},
		{"existing cap with offset", "SELECT * FROM t LIMIT 5 OFFSET 2", 10, "SELECT * FROM t LIMIT 5 OFFSET 2"},
		{"zero limit falls back to 100", "SELECT * FROM t", 0, "SELECT * FROM t LIMIT 100"},
		{"subquery LIMIT still caps the outer", "SELECT * FROM (SELECT a FROM u LIMIT 3) x", 10, "SELECT * FROM (SELECT a FROM u LIMIT 3) x LIMIT 10"},
		{"non-select untouched", "INSERT INTO t VALUES (1);", 10, "INSERT INTO t VALUES (1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prepareQuery(tt.in, tt.limit, s); got != tt.want {
				t.Errorf("prepareQuery(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRejectMultiStatement(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"single statement", "SELECT 1", false},
		{"single statement with trailing semicolon", "SELECT 1;", false},
		{"single statement, trailing semicolon and space", "SELECT 1 ;  ", false},
		{"smuggled SET before a write", "SET LOCAL transaction_read_only = off; DROP TABLE t", true},
		{"two selects", "SELECT 1; SELECT 2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectMultiStatement(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("rejectMultiStatement(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

// checkAdvisory is a message-quality helper, not a boundary. This pins that it
// still catches the obvious case — and documents that it does NOT catch the CTE,
// which is exactly why the read_only flag exists.
func TestCheckAdvisoryIsNotABoundary(t *testing.T) {
	s := DefaultSQLFunctionSettings()

	if err := s.checkAdvisory("DELETE FROM t"); err == nil {
		t.Error("a leading DELETE should be reported with a helpful message")
	}
	if err := s.checkAdvisory("SELECT 1"); err != nil {
		t.Errorf("SELECT is allowed by default: %v", err)
	}
	if err := s.checkAdvisory("WITH gone AS (DELETE FROM t RETURNING *) SELECT 1"); err != nil {
		t.Fatal("checkAdvisory unexpectedly caught a CTE write — if this is now a real " +
			"boundary the comment on it needs updating")
	}
	if !s.writesEnabled() {
		return // the CTE above is stopped by read_only at the server; nothing else to assert
	}
	t.Error("defaults should not enable writes")
}
