package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
)

// configJSON holds settings parsed from --config flag.
type configJSON struct {
	SUPABASE_PROJECT_REF    string `json:"SUPABASE_PROJECT_REF"`
	SUPABASE_DB_HOST        string `json:"SUPABASE_DB_HOST"`
	SUPABASE_DB_PASSWORD    string `json:"SUPABASE_DB_PASSWORD"`
	SUPABASE_DB_PORT        string `json:"SUPABASE_DB_PORT"`
	SUPABASE_DB_USER        string `json:"SUPABASE_DB_USER"`
	SUPABASE_DB_DATABASE    string `json:"SUPABASE_DB_DATABASE"`
	SUPABASE_ALLOW_SELECT   string `json:"SUPABASE_ALLOW_SELECT"`
	SUPABASE_ALLOW_INSERT   string `json:"SUPABASE_ALLOW_INSERT"`
	SUPABASE_ALLOW_UPDATE   string `json:"SUPABASE_ALLOW_UPDATE"`
	SUPABASE_ALLOW_DELETE   string `json:"SUPABASE_ALLOW_DELETE"`
	SUPABASE_ALLOW_CREATE   string `json:"SUPABASE_ALLOW_CREATE"`
	SUPABASE_ALLOW_ALTER    string `json:"SUPABASE_ALLOW_ALTER"`
	SUPABASE_ALLOW_DROP     string `json:"SUPABASE_ALLOW_DROP"`
	SUPABASE_ALLOW_TRUNCATE string `json:"SUPABASE_ALLOW_TRUNCATE"`
}

// Main entry point for Oido Supabase MCP Server.
func main() {
	configStr := flag.String("config", "", "JSON string with Supabase settings (overrides env vars)")
	flag.Parse()

	if *configStr != "" {
		var cfg configJSON
		if err := json.Unmarshal([]byte(*configStr), &cfg); err != nil {
			log.Fatalf("Failed to parse --config JSON: %v", err)
		}
		setIfNotEnv := func(key, val string) {
			if val != "" && os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
		setIfNotEnv("SUPABASE_PROJECT_REF", cfg.SUPABASE_PROJECT_REF)
		setIfNotEnv("SUPABASE_DB_HOST", cfg.SUPABASE_DB_HOST)
		setIfNotEnv("SUPABASE_DB_PASSWORD", cfg.SUPABASE_DB_PASSWORD)
		setIfNotEnv("SUPABASE_DB_PORT", cfg.SUPABASE_DB_PORT)
		setIfNotEnv("SUPABASE_DB_USER", cfg.SUPABASE_DB_USER)
		setIfNotEnv("SUPABASE_DB_DATABASE", cfg.SUPABASE_DB_DATABASE)
		setIfNotEnv("SUPABASE_ALLOW_SELECT", cfg.SUPABASE_ALLOW_SELECT)
		setIfNotEnv("SUPABASE_ALLOW_INSERT", cfg.SUPABASE_ALLOW_INSERT)
		setIfNotEnv("SUPABASE_ALLOW_UPDATE", cfg.SUPABASE_ALLOW_UPDATE)
		setIfNotEnv("SUPABASE_ALLOW_DELETE", cfg.SUPABASE_ALLOW_DELETE)
		setIfNotEnv("SUPABASE_ALLOW_CREATE", cfg.SUPABASE_ALLOW_CREATE)
		setIfNotEnv("SUPABASE_ALLOW_ALTER", cfg.SUPABASE_ALLOW_ALTER)
		setIfNotEnv("SUPABASE_ALLOW_DROP", cfg.SUPABASE_ALLOW_DROP)
		setIfNotEnv("SUPABASE_ALLOW_TRUNCATE", cfg.SUPABASE_ALLOW_TRUNCATE)
	}

	log.Println("Starting Oido Supabase MCP Server v1.0.0...")
	RunMCPServer()
}
