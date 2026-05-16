package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
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

// SupabaseClient manages Supabase PostgreSQL connections and queries.
type SupabaseClient struct {
	connStr  string
	db       *sql.DB
	settings *SQLFunctionSettings
}

// NewSupabaseClient creates a new Supabase client from environment variables.
// Uses sslmode=require as Supabase mandates SSL.
// Set SUPABASE_DB_HOST to use Session Pooler (IPv4); omit to use direct connection.
func NewSupabaseClient() (*SupabaseClient, error) {
	projectRef := os.Getenv("SUPABASE_PROJECT_REF")
	password := os.Getenv("SUPABASE_DB_PASSWORD")
	settings := parseSQLFunctionSettings()

	// SUPABASE_DB_HOST takes priority; otherwise construct direct host from project ref.
	host := os.Getenv("SUPABASE_DB_HOST")
	if host == "" {
		if projectRef == "" {
			log.Println("Warning: SUPABASE_PROJECT_REF and SUPABASE_DB_HOST not set. Tools will return errors until configured.")
			return &SupabaseClient{settings: settings}, nil
		}
		host = fmt.Sprintf("db.%s.supabase.co", projectRef)
	}

	port := os.Getenv("SUPABASE_DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("SUPABASE_DB_USER")
	if user == "" {
		user = "postgres"
	}
	database := os.Getenv("SUPABASE_DB_DATABASE")
	if database == "" {
		database = "postgres"
	}

	// Supabase shared pooler (*.pooler.supabase.com) requires the project ref
	// appended to the username: postgres.{project_ref}
	if strings.Contains(host, ".pooler.supabase.com") && projectRef != "" && !strings.Contains(user, ".") {
		user = fmt.Sprintf("%s.%s", user, projectRef)
	}

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=require",
		host, port, user, database,
	)
	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}

	return &SupabaseClient{connStr: connStr, settings: settings}, nil
}

// ensureConnected opens the DB connection on first use. Safe to call multiple times.
func (c *SupabaseClient) ensureConnected() error {
	if c.db != nil {
		return nil
	}
	if c.connStr == "" {
		return fmt.Errorf("Supabase DB not configured: set SUPABASE_PROJECT_REF and SUPABASE_DB_PASSWORD")
	}
	db, err := sql.Open("postgres", c.connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to connect to Supabase: %w", err)
	}
	c.db = db
	return nil
}

// Close closes the database connection.
func (c *SupabaseClient) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// ExecuteSQL executes a raw SQL query and returns results as a formatted string.
func (c *SupabaseClient) ExecuteSQL(ctx context.Context, query string, limit int) (string, error) {
	if err := c.ensureConnected(); err != nil {
		return "", err
	}

	upperQuery := strings.ToUpper(strings.TrimSpace(query))

	type sqlOperation struct {
		prefix  string
		allowed bool
	}

	operations := []sqlOperation{
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

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("failed to get columns: %w", err)
	}

	if len(columns) == 0 {
		return "Query executed successfully. No columns returned.", nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Query returned %d columns\n\n", len(columns)))

	header := strings.Join(columns, " | ")
	result.WriteString(header + "\n")
	result.WriteString(strings.Repeat("-", len(header)) + "\n")

	rowCount := 0
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return result.String(), fmt.Errorf("failed to scan row: %w", err)
		}

		rowVals := make([]string, len(columns))
		for i, v := range values {
			if v == nil {
				rowVals[i] = "NULL"
			} else {
				rowVals[i] = fmt.Sprintf("%v", v)
			}
		}

		result.WriteString(strings.Join(rowVals, " | ") + "\n")
		rowCount++
	}

	result.WriteString(fmt.Sprintf("\nTotal rows: %d", rowCount))
	return result.String(), nil
}

// ListTables returns all tables in the current database.
func (c *SupabaseClient) ListTables(ctx context.Context) (string, error) {
	if err := c.ensureConnected(); err != nil {
		return "", err
	}
	query := `
		SELECT schemaname, tablename
		FROM pg_catalog.pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY schemaname, tablename;
	`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return "", fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var schema, table string
		if err := rows.Scan(&schema, &table); err != nil {
			return "", fmt.Errorf("failed to scan table: %w", err)
		}
		tables = append(tables, fmt.Sprintf("%s.%s", schema, table))
	}

	if len(tables) == 0 {
		return "No tables found in the database.", nil
	}

	return fmt.Sprintf("Tables (%d):\n\n%s", len(tables), strings.Join(tables, "\n")), nil
}

// DescribeTable returns column info for a table.
func (c *SupabaseClient) DescribeTable(ctx context.Context, schema, table string) (string, error) {
	if err := c.ensureConnected(); err != nil {
		return "", err
	}
	query := `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position;
	`

	rows, err := c.db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return "", fmt.Errorf("failed to describe table: %w", err)
	}
	defer rows.Close()

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Table: %s.%s\n\n", schema, table))
	result.WriteString("Column Name          | Data Type            | Nullable | Default\n")
	result.WriteString(strings.Repeat("-", 80) + "\n")

	count := 0
	for rows.Next() {
		var colName, dataType, isNullable string
		var defaultVal sql.NullString

		if err := rows.Scan(&colName, &dataType, &isNullable, &defaultVal); err != nil {
			return "", fmt.Errorf("failed to scan column: %w", err)
		}

		defaultStr := "NULL"
		if defaultVal.Valid {
			defaultStr = defaultVal.String
		}

		result.WriteString(fmt.Sprintf("%-20s | %-20s | %-10s | %s\n",
			colName, dataType, isNullable, defaultStr))
		count++
	}

	if count == 0 {
		return fmt.Sprintf("Table %s.%s not found or has no columns.", schema, table), nil
	}

	return result.String(), nil
}
