---
name: oido-supabase
description: Execute SQL queries, list tables, describe Supabase PostgreSQL schemas, and explore database structure
---

# Oido Supabase Extension

## Overview

The Oido Supabase extension provides tools to query and explore Supabase PostgreSQL databases. Use these tools when users ask about Supabase database content, schema structure, or want to run SQL queries.

## Available Tools

### `sp_execute_sql`

Execute a SQL query against the Supabase database.

**Parameters:**
- `query` (string, required): SQL query to execute
- `limit` (number, optional): Maximum rows to return (default: 100)

**When to use:**
- User asks to query the Supabase database
- User wants to see data from a specific table
- User asks for data analysis or reporting
- User wants to filter or aggregate data

**Example Usage:**
```
User: "Show me all active users"
-> Call sp_execute_sql with query: "SELECT * FROM public.users WHERE status = 'active'"

User: "How many orders were placed last month?"
-> Call sp_execute_sql with query: "SELECT COUNT(*) FROM public.orders WHERE created_at >= ..."
```

### `sp_list_tables`

List all tables in the Supabase database.

**Parameters:** None

**When to use:**
- User asks "what tables are in the Supabase database?"
- User wants to explore database structure
- First step before querying unknown database

**Example Usage:**
```
User: "What tables do we have?"
-> Call sp_list_tables
```

### `sp_describe_table`

Describe a table's columns, types, and constraints.

**Parameters:**
- `schema` (string, optional): Database schema name (default: public)
- `table` (string, required): Table name to describe

**When to use:**
- User asks about table structure
- User wants to know column names or types
- User needs schema documentation

**Example Usage:**
```
User: "What columns are in the users table?"
-> Call sp_describe_table with schema: "public", table: "users"
```

## Best Practices

1. **Use full table names**: Always use `schema.table` format (e.g., `public.users`)
2. **Default to public schema**: If schema not specified, use `public`
3. **Check permissions**: Operations blocked by `SUPABASE_ALLOW_*` flags return a clear error
4. **Limit results**: Default 100 rows, respect user-specified limits
5. **Supabase schemas**: Supabase also has `auth` and `storage` schemas — sp_list_tables shows all

## Triggers

Use these tools when you see:
- "query" or "SQL" or "SELECT"
- "Supabase" + "tables" or "database"
- "columns" or "schema" or "table structure"
- "show me data" or "fetch records"
- Supabase database questions

## Related Commands

- `/sql-query` - Execute a SQL query
- `/list-tables` - List all tables
- `/describe-table` - Describe table structure
