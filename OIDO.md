# Oido Supabase Extension

Execute SQL queries, list tables, and describe Supabase PostgreSQL database schemas. This extension provides MCP tools for Supabase database exploration and query execution.

## Available Tools

### `execute_sql`
Execute a SQL query against the Supabase PostgreSQL database.

**Parameters:**
- `query` (string, required): SQL query to execute
- `limit` (number, optional): Maximum rows to return for SELECT queries (default: 100)

**Returns:** Formatted table results with column headers and row counts.

### `list_tables`
List all tables in the Supabase database.

**Parameters:** None

**Returns:** List of schema.table names.

### `describe_table`
Describe a table's columns, types, and constraints.

**Parameters:**
- `schema` (string, optional): Database schema name (default: public)
- `table` (string, required): Table name to describe

**Returns:** Column names, data types, nullable status, and defaults.

## When to Use

- User asks to query Supabase database or run SQL
- User wants to explore Supabase database schema
- User asks about tables, columns, or data
- User wants to analyze Supabase database structure
- User asks for data analysis or reporting

## Notes

- **SSL required**: Always connects with `sslmode=require` (Supabase mandates it)
- **Configurable permissions**: Each SQL operation (SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, TRUNCATE) individually toggled via `SUPABASE_ALLOW_*` env vars
- **Default read-only**: Only SELECT enabled by default
- **Port options**: `5432` for direct connection, `6543` for connection pooler (PgBouncer)
- **Row limits**: Default 100 rows for SELECT queries
- **Connection pooling**: 10 max open, 5 idle, 5 min lifetime

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SUPABASE_PROJECT_REF` | *(required)* | Project ref from Supabase dashboard |
| `SUPABASE_DB_PASSWORD` | *(required)* | Database password |
| `SUPABASE_DB_PORT` | `5432` | `5432` direct or `6543` pooler |
| `SUPABASE_DB_USER` | `postgres` | Database user |
| `SUPABASE_DB_DATABASE` | `postgres` | Database name |
| `SUPABASE_ALLOW_SELECT` | `true` | Allow SELECT |
| `SUPABASE_ALLOW_INSERT` | `false` | Allow INSERT |
| `SUPABASE_ALLOW_UPDATE` | `false` | Allow UPDATE |
| `SUPABASE_ALLOW_DELETE` | `false` | Allow DELETE |
| `SUPABASE_ALLOW_CREATE` | `false` | Allow CREATE |
| `SUPABASE_ALLOW_ALTER` | `false` | Allow ALTER |
| `SUPABASE_ALLOW_DROP` | `false` | Allow DROP |
| `SUPABASE_ALLOW_TRUNCATE` | `false` | Allow TRUNCATE |
