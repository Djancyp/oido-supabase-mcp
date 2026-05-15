package main

import (
	"context"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPHandler implements MCP tool handlers.
type MCPHandler struct {
	db *SupabaseClient
}

// NewMCPHandler creates a new MCP handler for Supabase tools.
func NewMCPHandler(db *SupabaseClient) *MCPHandler {
	return &MCPHandler{db: db}
}

// ExecuteSQLArgs represents arguments for the execute_sql tool.
type ExecuteSQLArgs struct {
	Query string `json:"query" jsonschema:"SQL query to execute"`
	Limit int    `json:"limit" jsonschema:"Maximum rows to return for SELECT queries (default: 100)"`
}

// ListTablesArgs represents arguments for the list_tables tool.
type ListTablesArgs struct{}

// DescribeTableArgs represents arguments for the describe_table tool.
type DescribeTableArgs struct {
	Schema string `json:"schema" jsonschema:"Database schema name (default: public)"`
	Table  string `json:"table" jsonschema:"Table name to describe"`
}

// RunMCPServer starts the MCP server using stdio transport.
func RunMCPServer() {
	client, err := NewSupabaseClient()
	if err != nil {
		log.Printf("Warning: Supabase client init failed (tools will return errors): %v", err)
		client = &SupabaseClient{settings: DefaultSQLFunctionSettings()}
	}
	defer client.Close()

	handler := NewMCPHandler(client)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "oido-supabase",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sp_execute_sql",
		Description: "Execute a SQL query against the Supabase PostgreSQL database. Permissions are configured via SUPABASE_ALLOW_* environment variables (SELECT only by default).",
	}, handler.HandleExecuteSQL)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sp_list_tables",
		Description: "List all tables in the Supabase database. Returns schema.table names.",
	}, handler.HandleListTables)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sp_describe_table",
		Description: "Describe a table's columns, types, and constraints. Requires schema and table name.",
	}, handler.HandleDescribeTable)

	ctx := context.Background()
	log.Println("Oido Supabase MCP Server starting on stdio...")

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}

// HandleExecuteSQL executes a SQL query.
func (h *MCPHandler) HandleExecuteSQL(ctx context.Context, req *mcp.CallToolRequest, args ExecuteSQLArgs) (*mcp.CallToolResult, any, error) {
	if args.Query == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Error: query parameter is required"},
			},
			IsError: true,
		}, nil, nil
	}

	result, err := h.db.ExecuteSQL(ctx, args.Query, args.Limit)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("SQL Error: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil, nil
}

// HandleListTables lists all tables.
func (h *MCPHandler) HandleListTables(ctx context.Context, req *mcp.CallToolRequest, args ListTablesArgs) (*mcp.CallToolResult, any, error) {
	result, err := h.db.ListTables(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil, nil
}

// HandleDescribeTable describes a table.
func (h *MCPHandler) HandleDescribeTable(ctx context.Context, req *mcp.CallToolRequest, args DescribeTableArgs) (*mcp.CallToolResult, any, error) {
	if args.Table == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Error: table parameter is required"},
			},
			IsError: true,
		}, nil, nil
	}

	schema := args.Schema
	if schema == "" {
		schema = "public"
	}

	result, err := h.db.DescribeTable(ctx, schema, args.Table)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)},
			},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: result},
		},
	}, nil, nil
}
