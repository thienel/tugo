// Package main demonstrates TuGo with full authentication including JWT and TOTP.
//
// This example shows how to configure TuGo with:
//   - JWT authentication for API access
//   - TOTP (Time-based One-Time Password) for two-factor authentication
//   - Protected routes requiring authentication
//   - Admin routes requiring admin role
//
// Prerequisites:
//   - PostgreSQL database with:
//     - Tables prefixed with "api_" (e.g., api_products)
//     - TuGo system tables (run migrations first)
//   - Environment variables:
//     - DATABASE_URL: PostgreSQL connection string
//     - JWT_SECRET: Secret key for JWT signing (min 32 characters)
//
// Run:
//
//	DATABASE_URL="postgres://user:pass@localhost/mydb?sslmode=disable" \
//	JWT_SECRET="your-super-secret-jwt-key-min-32-chars" \
//	go run main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/thienel/tugo"
)

func main() {
	// Get configuration from environment
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// For development only - in production, always require a secret
		log.Println("WARNING: JWT_SECRET not set, using default (NOT FOR PRODUCTION)")
		jwtSecret = "development-secret-key-min-32-characters"
	}

	// Create TuGo engine with full auth configuration
	engine, err := tugo.New(tugo.Config{
		DatabaseURL: databaseURL,
		Discovery: tugo.DiscoveryConfig{
			Mode:         "prefix",
			Prefix:       "api_",
			AutoDiscover: true,
		},
		Auth: tugo.AuthConfig{
			// Enable multiple auth methods
			Methods: []string{"jwt", "totp"},

			// JWT configuration
			JWT: tugo.JWTConfig{
				Secret:     jwtSecret,
				Expiry:     3600,   // 1 hour
				RefreshExp: 604800, // 7 days
				Issuer:     "tugo-example",
			},

			// TOTP configuration for 2FA
			TOTP: tugo.TOTPConfig{
				Issuer: "TuGo Example App",
				Period: 30,
				Digits: 6,
			},
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

	// Initialize the engine
	ctx := context.Background()
	if err := engine.Init(ctx); err != nil {
		log.Fatalf("Failed to initialize TuGo: %v", err)
	}

	// Get the internal router for custom routes
	router := engine.Router()

	// Add custom public routes
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "TuGo API with Authentication",
			"docs": gin.H{
				"auth_endpoints": []string{
					"POST /api/v1/auth/login    - Login with username/password",
					"POST /api/v1/auth/refresh  - Refresh access token",
					"POST /api/v1/auth/logout   - Logout and revoke token",
					"GET  /api/v1/auth/me       - Get current user (requires auth)",
				},
				"totp_endpoints": []string{
					"POST /api/v1/auth/totp/setup   - Generate TOTP secret (requires auth)",
					"POST /api/v1/auth/totp/enable  - Enable 2FA with code",
					"POST /api/v1/auth/totp/disable - Disable 2FA with code",
				},
			},
		})
	})

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Log discovered collections
	collections := engine.GetCollections()
	log.Printf("Discovered %d collections:", len(collections))
	for _, c := range collections {
		log.Printf("  - %s", c.Name)
	}

	// Start the server
	log.Println("")
	log.Println("Starting TuGo server with authentication on :8080")
	log.Println("")
	log.Println("Authentication Flow:")
	log.Println("  1. Create a user in tugo_users table with hashed password")
	log.Println("  2. POST /api/v1/auth/login with {\"username\": \"...\", \"password\": \"...\"}")
	log.Println("  3. Use returned access_token in Authorization header: Bearer <token>")
	log.Println("  4. (Optional) Enable TOTP for 2FA on your account")
	log.Println("")
	log.Println("Public endpoints (no auth required):")
	log.Println("  GET  /")
	log.Println("  GET  /health")
	log.Println("  POST /api/v1/auth/login")
	log.Println("  POST /api/v1/auth/refresh")
	log.Println("")
	log.Println("Protected endpoints (auth required):")
	log.Println("  GET/POST/PATCH/DELETE /api/v1/{collection}")
	log.Println("  GET  /api/v1/auth/me")
	log.Println("  POST /api/v1/auth/totp/*")
	log.Println("")
	log.Println("Admin endpoints (admin role required):")
	log.Println("  GET/POST /api/admin/collections")
	log.Println("  POST     /api/admin/schema/migrate")

	if err := engine.Run(":8080"); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
