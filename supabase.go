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
func (c *SupabaseClient) runSQL(ctx context.Context, query string) (json.RawMessage, error) {
	if c.baseURL == "" || c.key == "" {
		return nil, fmt.Errorf("Supabase not configured: set SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY")
	}

	body, _ := json.Marshal(map[string]string{"query": query})
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
			return nil, fmt.Errorf("exec_sql RPC not found (HTTP 404) — create the %s function (see OIDO.md). Body: %s", c.rpc, strings.TrimSpace(string(data)))
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

// ExecuteSQL runs a SQL query via the exec_sql RPC, subject to SUPABASE_ALLOW_* gating.
func (c *SupabaseClient) ExecuteSQL(ctx context.Context, query string, limit int) (string, error) {
	upperQuery := strings.ToUpper(strings.TrimSpace(query))

	operations := []struct {
		prefix  string
		allowed bool
	}{
		{"SELECT", c.settings.AllowSelect},
		{"INSERT", c.settings.AllowInsert},
		{"UPDATE", c.settings.AllowUpdate},
		{"DELETE", c.settings.AllowDelete},
		{"CREATE", c.settings.AllowCreate},
		{"ALTER", c.settings.AllowAlter},
		{"DROP", c.settings.AllowDrop},
		{"TRUNCATE", c.settings.AllowTruncate},
	}
	for _, op := range operations {
		if strings.HasPrefix(upperQuery, op.prefix) {
			if !op.allowed {
				return "", fmt.Errorf("blocked: %s operations are not allowed (enable with SUPABASE_ALLOW_%s=true)",
					op.prefix, op.prefix)
			}
			break
		}
	}

	if strings.HasPrefix(upperQuery, "SELECT") && c.settings.AllowSelect && !strings.Contains(upperQuery, "LIMIT") {
		if limit <= 0 {
			limit = 100
		}
		query = fmt.Sprintf("%s LIMIT %d", query, limit)
	}

	raw, err := c.runSQL(ctx, query)
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

	raw, err := c.runSQL(ctx, query)
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

	raw, err := c.runSQL(ctx, query)
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
