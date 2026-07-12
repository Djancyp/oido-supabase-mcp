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

## Connection

Connects over the **PostgREST REST API** using the **service_role key** — just a
URL and a secret, no direct Postgres connection. Raw SQL runs through a one-time
`exec_sql` RPC function you create in the database.

### One-time setup: create the `exec_sql` function

Run this once (Studio SQL editor, or `psql`):

```sql
create or replace function public.exec_sql(query text)
returns jsonb
language plpgsql
security definer
set search_path = public
as $$
declare
  result jsonb;
begin
  -- Row-returning statements: aggregate rows into a JSON array.
  execute format('select coalesce(jsonb_agg(row_to_json(t)), ''[]''::jsonb) from (%s) t', query)
    into result;
  return result;
exception
  when others then
    -- Non-SELECT statements can't sit in a subquery; run them directly.
    execute query;
    return jsonb_build_object('status', 'ok');
end;
$$;

-- Only the service_role should reach it.
revoke all on function public.exec_sql(text) from public, anon, authenticated;
grant execute on function public.exec_sql(text) to service_role;
```

Security note: this runs arbitrary SQL as a privileged role. It is only callable
with the service_role key, which already bypasses RLS — keep that key private.
Use `SUPABASE_ALLOW_*` to restrict which statement types the tools will send.

## Notes

- **Configurable permissions**: Each SQL operation (SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, TRUNCATE) individually toggled via `SUPABASE_ALLOW_*` env vars
- **Default read-only**: Only SELECT enabled by default
- **Row limits**: Default 100 rows for SELECT queries
- **HTTP timeout**: 30s per request

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SUPABASE_URL` | *(required)* | API URL, e.g. `https://host` or `http://host:8000` |
| `SUPABASE_SERVICE_ROLE_KEY` | *(required)* | service_role secret (bypasses RLS — keep private) |
| `SUPABASE_EXEC_SQL_FN` | `exec_sql` | Name of the SQL-executing RPC function |
| `SUPABASE_ALLOW_SELECT` | `true` | Allow SELECT |
| `SUPABASE_ALLOW_INSERT` | `false` | Allow INSERT |
| `SUPABASE_ALLOW_UPDATE` | `false` | Allow UPDATE |
| `SUPABASE_ALLOW_DELETE` | `false` | Allow DELETE |
| `SUPABASE_ALLOW_CREATE` | `false` | Allow CREATE |
| `SUPABASE_ALLOW_ALTER` | `false` | Allow ALTER |
| `SUPABASE_ALLOW_DROP` | `false` | Allow DROP |
| `SUPABASE_ALLOW_TRUNCATE` | `false` | Allow TRUNCATE |
