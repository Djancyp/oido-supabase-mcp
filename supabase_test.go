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
