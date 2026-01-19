// Package main demonstrates TuGo integration as middleware in an existing Gin application.
//
// This example shows the primary use case for TuGo: integrating auto-generated
// REST APIs into your existing application while keeping full control of routing
// and middleware.
//
// Prerequisites:
//   - PostgreSQL database with tables prefixed with "api_"
//   - Environment variable DATABASE_URL set to your PostgreSQL connection string
//
// Run:
//
//	DATABASE_URL="postgres://user:pass@localhost/mydb?sslmode=disable" go run main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/thienel/tugo"
)

func main() {
	// Get database URL from environment
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// Create your own database connection (typical in existing apps)
	db, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Create your existing Gin router
	router := gin.Default()

	// Add your existing routes
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Welcome to My API",
			"version": "1.0.0",
		})
	})

	router.GET("/health", func(c *gin.Context) {
		if err := db.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Custom business logic routes
	router.GET("/custom/stats", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"total_requests": 1234,
			"uptime":         "24h",
		})
	})

	// Create TuGo engine - reuse existing database connection
	engine, err := tugo.New(tugo.Config{
		DB: db, // Reuse existing connection
		Discovery: tugo.DiscoveryConfig{
			Mode:         "prefix",
			Prefix:       "api_",
			AutoDiscover: true,
			// Optionally exclude specific tables
			Blacklist: []string{"api_internal_logs"},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create TuGo engine: %v", err)
	}

	// Initialize TuGo (discover schema)
	ctx := context.Background()
	if err := engine.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize TuGo: %v", err)
	}

	// Mount TuGo on a specific route group
	// This keeps TuGo routes separate from your custom routes
	api := router.Group("/api/v1")
	{
		// Add any custom middleware before TuGo
		api.Use(requestLogger())
		api.Use(rateLimiter())

		// Mount TuGo routes
		engine.Mount(api)
	}

	// Log discovered collections
	collections := engine.GetCollections()
	log.Printf("TuGo discovered %d collections:", len(collections))
	for _, c := range collections {
		log.Printf("  - GET/POST/PATCH/DELETE /api/v1/%s", c.Name)
	}

	// Start the server
	log.Println("Starting server on :8080")
	log.Println("")
	log.Println("Routes:")
	log.Println("  Custom:")
	log.Println("    GET  /              - Welcome message")
	log.Println("    GET  /health        - Health check")
	log.Println("    GET  /custom/stats  - Custom stats")
	log.Println("  TuGo (auto-generated):")
	log.Println("    GET    /api/v1/{collection}      - List")
	log.Println("    GET    /api/v1/{collection}/:id  - Get")
	log.Println("    POST   /api/v1/{collection}      - Create")
	log.Println("    PATCH  /api/v1/{collection}/:id  - Update")
	log.Println("    DELETE /api/v1/{collection}/:id  - Delete")

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// requestLogger is a simple request logging middleware
func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		log.Printf("%s %s %d %v",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			time.Since(start),
		)
	}
}

// rateLimiter is a placeholder for rate limiting middleware
func rateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		// In production, implement actual rate limiting
		// e.g., using github.com/ulule/limiter/v3
		c.Next()
	}
}
