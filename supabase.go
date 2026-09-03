package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SQLFunctionSettings holds configuration for which SQL operations are allowed.
type SQLFunctionSettings struct {
	AllowSelect   bool
	AllowInsert   bool
	AllowUpdate   bool
	AllowDelete   bool
	AllowCreate   bool
	AllowAlter    bool
	AllowDrop     bool
	AllowTruncate bool
}

// DefaultSQLFunctionSettings returns settings with only SELECT enabled by default.
func DefaultSQLFunctionSettings() *SQLFunctionSettings {
	return &SQLFunctionSettings{
		AllowSelect:   true,
		AllowInsert:   false,
		AllowUpdate:   false,
		AllowDelete:   false,
		AllowCreate:   false,
		AllowAlter:    false,
		AllowDrop:     false,
		AllowTruncate: false,
	}
}

// parseSQLFunctionSettings reads SUPABASE_ALLOW_* env vars to configure permissions.
func parseSQLFunctionSettings() *SQLFunctionSettings {
	settings := DefaultSQLFunctionSettings()

	parseBool := func(key string) (bool, bool) {
		if val := os.Getenv(key); val != "" {
			if b, err := strconv.ParseBool(val); err == nil {
				return b, true
			}
		}
		return false, false
	}

	if b, ok := parseBool("SUPABASE_ALLOW_SELECT"); ok {
		settings.AllowSelect = b
	}
	if b, ok := parseBool("SUPABASE_ALLOW_INSERT"); ok {
		settings.AllowInsert = b
	}
	if b, ok := parseBool("SUPABASE_ALLOW_UPDATE"); ok {
		settings.AllowUpdate = b
	}
	if b, ok := parseBool("SUPABASE_ALLOW_DELETE"); ok {
		settings.AllowDelete = b
	}
	if b, ok := parseBool("SUPABASE_ALLOW_CREATE"); ok {
		settings.AllowCreate = b
	}
	if b, ok := parseBool("SUPABASE_ALLOW_ALTER"); ok {
		settings.AllowAlter = b
	}
	if b, ok := parseBool("SUPABASE_ALLOW_DROP"); ok {
		settings.AllowDrop = b
	}
	if b, ok := parseBool("SUPABASE_ALLOW_TRUNCATE"); ok {
		settings.AllowTruncate = b
	}

	return settings
}

// SupabaseClient talks to Supabase over the PostgREST REST API using the
// service_role key. Raw SQL is run through the exec_sql RPC (see OIDO.md for
// the one-time function to create).
type SupabaseClient struct {
	baseURL  string // e.g. https://host or http://host:8000, no trailing slash
	key      string // service_role secret
	rpc      string // RPC function name, default "exec_sql"
	http     *http.Client
	settings *SQLFunctionSettings
}

// NewSupabaseClient creates a REST-backed Supabase client from environment vars.
// Requires SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY.
func NewSupabaseClient() (*SupabaseClient, error) {
	settings := parseSQLFunctionSettings()

	url := strings.TrimRight(os.Getenv("SUPABASE_URL"), "/")
	key := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if url == "" || key == "" {
		log.Println("Warning: SUPABASE_URL / SUPABASE_SERVICE_ROLE_KEY not set. Tools will return errors until configured.")
		return &SupabaseClient{settings: settings}, nil
	}

	rpc := os.Getenv("SUPABASE_EXEC_SQL_FN")
	if rpc == "" {
		rpc = "exec_sql"
	}

	return &SupabaseClient{
		baseURL:  url,
		key:      key,
		rpc:      rpc,
		http:     &http.Client{Timeout: 30 * time.Second},
		settings: settings,
	}, nil
}

// Close is a no-op for the REST client (kept for interface compatibility).
func (c *SupabaseClient) Close() error { return nil }

// runSQL POSTs a query to the exec_sql RPC and returns the decoded JSON.
// The RPC returns a JSON array of rows for row-returning statements, or a
// status object for others.
//
// readOnly is the actual write barrier. There is no client-side transaction to
// open over REST, so the function sets transaction_read_only for the statement
// instead; a deployed function that predates the read_only parameter cannot
// enforce anything, so this fails closed rather than silently running without
// the barrier.
func (c *SupabaseClient) runSQL(ctx context.Context, query string, readOnly bool) (json.RawMessage, error) {
	if c.baseURL == "" || c.key == "" {
		return nil, fmt.Errorf("Supabase not configured: set SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY")
	}

	body, _ := json.Marshal(map[string]any{"query": query, "read_only": readOnly})
	endpoint := fmt.Sprintf("%s/rest/v1/rpc/%s", c.baseURL, c.rpc)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.key)
	req.Header.Set("Authorization", "Bearer "+c.key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("exec_sql RPC not found (HTTP 404) — the %s function is missing, or is the older single-argument version that cannot enforce read-only. Re-run the function definition in OIDO.md, which takes (query text, read_only boolean). Body: %s",
				c.rpc, strings.TrimSpace(string(data)))
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.RawMessage(data), nil
}

// formatRows renders a JSON array of row objects as a text table.
func formatRows(raw json.RawMessage) (string, error) {
	var rows []map[string]interface{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		// Not an array of rows (e.g. status object); return raw JSON.
		return strings.TrimSpace(string(raw)), nil
	}
	if len(rows) == 0 {
		return "Query executed successfully. 0 rows.", nil
	}

	// Stable column order from the first row.
	var columns []string
	for k := range rows[0] {
		columns = append(columns, k)
	}
	// ponytail: map iteration is unordered; sort so output is deterministic.
	sortStrings(columns)

	var b strings.Builder
	header := strings.Join(columns, " | ")
	b.WriteString(header + "\n")
	b.WriteString(strings.Repeat("-", len(header)) + "\n")
	for _, row := range rows {
		vals := make([]string, len(columns))
		for i, col := range columns {
			v := row[col]
			if v == nil {
				vals[i] = "NULL"
			} else {
				vals[i] = fmt.Sprintf("%v", v)
			}
		}
		b.WriteString(strings.Join(vals, " | ") + "\n")
	}
	b.WriteString(fmt.Sprintf("\nTotal rows: %d", len(rows)))
	return b.String(), nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// writesEnabled reports whether any write operation is permitted. When nothing
// is, every statement runs with transaction_read_only set and the database
// enforces it for us.
func (s *SQLFunctionSettings) writesEnabled() bool {
	return s.AllowInsert || s.AllowUpdate || s.AllowDelete ||
		s.AllowCreate || s.AllowAlter || s.AllowDrop || s.AllowTruncate
}

// limitClause matches a row cap already present at the end of the query, so a
// LIMIT inside a subquery does not stop us capping the outer result.
var limitClause = regexp.MustCompile(`(?is)\bLIMIT\s+\d+\s*(OFFSET\s+\d+\s*)?$`)

// prepareQuery trims the statement and appends a row cap to an uncapped SELECT.
// The trailing semicolon has to go first: a model emits "SELECT 1;" by default,
// and pasting " LIMIT 100" after it is a syntax error.
func prepareQuery(query string, limit int, s *SQLFunctionSettings) string {
	q := strings.TrimRight(strings.TrimSpace(query), "; \t\n\r")
	if !s.AllowSelect || !strings.HasPrefix(strings.ToUpper(q), "SELECT") {
		return q
	}
	if limitClause.MatchString(q) {
		return q
	}
	if limit <= 0 {
		limit = 100
	}
	return fmt.Sprintf("%s LIMIT %d", q, limit)
}

// rejectMultiStatement blocks a second statement smuggled in after the first.
// Without this, a query like "SET LOCAL transaction_read_only = off; DROP
// TABLE t" fails the row-wrapped attempt (a SET can't sit in a subquery),
// falls into exec_sql's "when others" fallback, and runs as a plain
// multi-statement EXECUTE -- which flips the read-only barrier off from
// inside the same call it was meant to guard. read_only is set per-call, not
// per-statement, so a second statement runs under whatever the first one left
// it as.
//
// ponytail: naive semicolon scan, so a semicolon inside a string literal
// (e.g. SELECT 'a;b') false-positives as multi-statement. Reject-on-doubt is
// the right tradeoff for a security barrier; switch to a real SQL statement
// splitter if that false positive becomes a real complaint.
func rejectMultiStatement(query string) error {
	trimmed := strings.TrimRight(strings.TrimSpace(query), "; \t\n\r")
	if strings.Contains(trimmed, ";") {
		return fmt.Errorf("blocked: multiple statements in one query are not allowed " +
			"(found a ';' before the end) -- send one statement per call")
	}
	return nil
}

// checkAdvisory rejects a disallowed operation early so the caller gets a
// message naming the setting to flip.
//
// It is NOT the security boundary and must never be treated as one: it matches
// the statement's leading keyword, so "WITH gone AS (DELETE ...) SELECT" matches
// nothing here and sails through. Enforcement is the read_only flag passed to
// the RPC, which the server applies to CTEs and every other shape alike.
func (s *SQLFunctionSettings) checkAdvisory(query string) error {
	upper := strings.ToUpper(strings.TrimSpace(query))
	operations := []struct {
		prefix  string
		allowed bool
	}{
		{"SELECT", s.AllowSelect},
		{"INSERT", s.AllowInsert},
		{"UPDATE", s.AllowUpdate},
		{"DELETE", s.AllowDelete},
		{"CREATE", s.AllowCreate},
		{"ALTER", s.AllowAlter},
		{"DROP", s.AllowDrop},
		{"TRUNCATE", s.AllowTruncate},
	}
	for _, op := range operations {
		if strings.HasPrefix(upper, op.prefix) && !op.allowed {
			return fmt.Errorf("blocked: %s operations are not allowed (enable with SUPABASE_ALLOW_%s=true)",
				op.prefix, op.prefix)
		}
	}
	return nil
}

// ExecuteSQL runs a single SQL statement via the exec_sql RPC.
//
// Gating is the read_only argument sent to the function, not the keyword check
// below. Inspecting the query string cannot work: exec_sql wraps the statement
// in "select ... from (query) t", a top-level data-modifying CTE cannot sit in a
// subquery, and the function's exception handler then runs the statement raw —
// so "WITH gone AS (DELETE ...) SELECT" reached the table no matter what the
// keyword check said. With transaction_read_only set, that fallback path fails
// at the server like every other write.
//
// Per-verb permissions (INSERT yes, DROP no) cannot be expressed this way. Once
// any write is enabled only checkAdvisory stands between the model and the other
// write verbs, so scope the key or the function's role to what it should reach.
//
// rejectMultiStatement runs before any of that: exec_sql's read_only barrier is
// per-call, not per-statement, so a query smuggling in a second statement (e.g.
// "SET LOCAL transaction_read_only = off; DROP TABLE t") could flip it off from
// inside the same call meant to be guarded by it. The RPC enforces this too
// (defense in depth for anyone bypassing this client); rejecting here just gives
// a clearer error without a round trip.
func (c *SupabaseClient) ExecuteSQL(ctx context.Context, query string, limit int) (string, error) {
	query = prepareQuery(query, limit, c.settings)
	if err := rejectMultiStatement(query); err != nil {
		return "", err
	}
	if err := c.settings.checkAdvisory(query); err != nil {
		return "", err
	}

	raw, err := c.runSQL(ctx, query, !c.settings.writesEnabled())
	if err != nil {
		return "", err
	}
	return formatRows(raw)
}

// ListTables returns all user tables in the database.
func (c *SupabaseClient) ListTables(ctx context.Context) (string, error) {
	query := `SELECT schemaname || '.' || tablename AS name
		FROM pg_catalog.pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY schemaname, tablename`

	raw, err := c.runSQL(ctx, query, true) // introspection only
	if err != nil {
		return "", err
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return "", fmt.Errorf("unexpected response: %s", strings.TrimSpace(string(raw)))
	}
	if len(rows) == 0 {
		return "No tables found in the database.", nil
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = fmt.Sprintf("%v", r["name"])
	}
	return fmt.Sprintf("Tables (%d):\n\n%s", len(names), strings.Join(names, "\n")), nil
}

// DescribeTable returns column info for a table.
func (c *SupabaseClient) DescribeTable(ctx context.Context, schema, table string) (string, error) {
	// ponytail: schema/table interpolated into SQL run server-side under the
	// service_role via exec_sql. Values come from the caller's own MCP request,
	// not untrusted input; quote to avoid breakage on odd names.
	query := fmt.Sprintf(`SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = %s AND table_name = %s
		ORDER BY ordinal_position`, quoteLiteral(schema), quoteLiteral(table))

	raw, err := c.runSQL(ctx, query, true) // introspection only
	if err != nil {
		return "", err
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return "", fmt.Errorf("unexpected response: %s", strings.TrimSpace(string(raw)))
	}
	if len(rows) == 0 {
		return fmt.Sprintf("Table %s.%s not found or has no columns.", schema, table), nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Table: %s.%s\n\n", schema, table))
	b.WriteString("Column Name          | Data Type            | Nullable | Default\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	for _, r := range rows {
		def := "NULL"
		if r["column_default"] != nil {
			def = fmt.Sprintf("%v", r["column_default"])
		}
		b.WriteString(fmt.Sprintf("%-20v | %-20v | %-10v | %v\n",
			r["column_name"], r["data_type"], r["is_nullable"], def))
	}
	return b.String(), nil
}

// quoteLiteral wraps a value as a single-quoted SQL string literal.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
