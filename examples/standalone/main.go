// Package main demonstrates TuGo in standalone server mode.
//
// This example shows how to use TuGo as a standalone HTTP server
// that auto-generates REST APIs from your PostgreSQL database.
//
// Prerequisites:
//   - PostgreSQL database with tables prefixed with "api_" (e.g., api_products, api_users)
//   - Environment variable DATABASE_URL set to your PostgreSQL connection string
//
// Run:
//
//	DATABASE_URL="postgres://user:pass@localhost/mydb?sslmode=disable" go run main.go
//
// The server will:
//   - Discover all tables with "api_" prefix
//   - Generate CRUD endpoints automatically
//   - Listen on port 8080
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thienel/tugo"
)

func main() {
	// Get database URL from environment
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Create TuGo engine with minimal configuration
	engine, err := tugo.New(tugo.Config{
		DatabaseURL: databaseURL,
		Discovery: tugo.DiscoveryConfig{
			Mode:         "prefix",       // Discover tables by prefix
			Prefix:       "api_",         // Tables like api_products, api_users
			AutoDiscover: true,           // Auto-expose discovered tables
		},
		Server: tugo.ServerConfig{
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	})
	if err != nil {
		log.Fatalf("Failed to create TuGo engine: %v", err)
	}
	defer engine.Close()

	// Initialize the engine (discover schema)
	ctx := context.Background()
	if err := engine.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize TuGo: %v", err)
	}

	// Log discovered collections
	collections := engine.GetCollections()
	log.Printf("Discovered %d collections:", len(collections))
	for _, c := range collections {
		log.Printf("  - %s (table: %s, fields: %d)", c.Name, c.TableName, len(c.Fields))
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")
		engine.Close()
		os.Exit(0)
	}()

	// Start the server
	log.Println("Starting TuGo server on :8080")
	log.Println("API endpoints available at:")
	log.Println("  GET    /api/v1/{collection}      - List items")
	log.Println("  GET    /api/v1/{collection}/:id  - Get item")
	log.Println("  POST   /api/v1/{collection}      - Create item")
	log.Println("  PATCH  /api/v1/{collection}/:id  - Update item")
	log.Println("  DELETE /api/v1/{collection}/:id  - Delete item")

	if err := engine.Run(":8080"); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
