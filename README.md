# TuGo

**TuGo (Turbo Go)** is a Go package that auto-generates REST APIs from PostgreSQL database schemas. Inspired by Directus but lighter, focusing on simplicity and flexibility.

## Features

- **Auto-discovery**: Automatically expose tables with configurable prefix (e.g., `api_*`)
- **Full CRUD**: Generated endpoints for Create, Read, Update, Delete operations
- **Filtering & Sorting**: Rich query parameters for filtering, sorting, and pagination
- **Relationships**: Auto-discovered foreign key relationships with expansion support
- **Authentication**: Built-in JWT and session-based authentication
- **Two-Factor Auth**: TOTP support for enhanced security
- **File Storage**: Local and MinIO storage backends
- **Validation**: Automatic validation based on database constraints
- **Middleware-first**: Designed for integration into existing Gin applications

## Installation

```bash
go get github.com/thienel/tugo
```

## Quick Start

### Standalone Mode

```go
package main

import (
    "context"
    "log"

    "github.com/thienel/tugo"
)

func main() {
    engine, err := tugo.New(tugo.Config{
        DatabaseURL: "postgres://user:pass@localhost/mydb?sslmode=disable",
        Discovery: tugo.DiscoveryConfig{
            Prefix:       "api_",
            AutoDiscover: true,
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    defer engine.Close()

    if err := engine.Init(context.Background()); err != nil {
        log.Fatal(err)
    }

    log.Fatal(engine.Run(":8080"))
}
```

### Middleware Integration

```go
package main

import (
    "context"

    "github.com/gin-gonic/gin"
    "github.com/jmoiron/sqlx"
    "github.com/thienel/tugo"
)

func main() {
    db, _ := sqlx.Connect("postgres", "postgres://...")
    router := gin.Default()

    engine, _ := tugo.New(tugo.Config{
        DB: db,
        Discovery: tugo.DiscoveryConfig{
            Prefix:       "api_",
            AutoDiscover: true,
        },
    })
    engine.Init(context.Background())

    // Mount TuGo routes
    api := router.Group("/api/v1")
    engine.Mount(api)

    router.Run(":8080")
}
```

## Database Setup

### Table Naming Convention

TuGo discovers tables using a configurable prefix. By default, tables with `api_` prefix are auto-exposed:

```sql
-- This table becomes available at /api/v1/products
CREATE TABLE api_products (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10,2),
    category_id UUID,
    created_at TIMESTAMP DEFAULT NOW()
);

-- This table becomes available at /api/v1/categories
CREATE TABLE api_categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL
);
```

### Running Migrations

TuGo provides migration files for system tables. Use [golang-migrate](https://github.com/golang-migrate/migrate):

```bash
# Install migrate CLI
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Run migrations
migrate -path internal/db/migrations -database "postgres://user:pass@localhost/mydb?sslmode=disable" up
```

## API Endpoints

### Collection Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/{collection}` | List items with filtering, sorting, pagination |
| GET | `/{collection}/:id` | Get single item by ID |
| POST | `/{collection}` | Create new item |
| PATCH | `/{collection}/:id` | Update item |
| DELETE | `/{collection}/:id` | Delete item |

### Authentication Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/login` | Authenticate and get tokens |
| POST | `/auth/refresh` | Refresh access token |
| POST | `/auth/logout` | Revoke tokens |
| GET | `/auth/me` | Get current user (auth required) |
| POST | `/auth/totp/setup` | Generate TOTP secret |
| POST | `/auth/totp/enable` | Enable 2FA |
| POST | `/auth/totp/disable` | Disable 2FA |

### File Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/files` | Upload file |
| GET | `/files/:path` | Download file |
| DELETE | `/files/:path` | Delete file |

## Query Parameters

### Filtering

```
GET /api/v1/products?filter[name]=iPhone
GET /api/v1/products?filter[price:gte]=100
GET /api/v1/products?filter[category_id:in]=uuid1,uuid2
```

**Operators:**

| Operator | Description | Example |
|----------|-------------|---------|
| `eq` (default) | Equal | `filter[status]=active` |
| `ne` | Not equal | `filter[status:ne]=deleted` |
| `gt` | Greater than | `filter[price:gt]=100` |
| `gte` | Greater or equal | `filter[price:gte]=100` |
| `lt` | Less than | `filter[price:lt]=100` |
| `lte` | Less or equal | `filter[price:lte]=100` |
| `like` | Case-insensitive match | `filter[name:like]=%iphone%` |
| `in` | In list | `filter[id:in]=1,2,3` |
| `null` | Is null | `filter[deleted_at:null]=true` |
| `notnull` | Is not null | `filter[email:notnull]=true` |

### Sorting

```
GET /api/v1/products?sort=name           # ASC
GET /api/v1/products?sort=-created_at    # DESC
GET /api/v1/products?sort=-price,name    # Multiple fields
```

### Pagination

```
GET /api/v1/products?page=2&limit=20
```

**Response:**

```json
{
  "success": true,
  "data": {
    "items": [...],
    "pagination": {
      "page": 2,
      "limit": 20,
      "total": 150,
      "total_pages": 8
    }
  }
}
```

### Relationship Expansion

```
GET /api/v1/products?expand=category
GET /api/v1/products?expand=category,brand
```

## Configuration Reference

```go
type Config struct {
    // Database connection (provide one)
    DB          *sqlx.DB  // Existing connection
    DatabaseURL string    // Connection string

    // Table discovery
    Discovery DiscoveryConfig{
        Mode         string   // "prefix", "config", "hybrid"
        Prefix       string   // Default: "api_"
        AutoDiscover bool     // Auto-expose discovered tables
        Blacklist    []string // Tables to exclude
        Config       map[string]CollectionItemConfig
    }

    // Authentication
    Auth AuthConfig{
        Methods []string  // "jwt", "cookie", "totp"
        JWT JWTConfig{
            Secret     string
            Expiry     int    // Seconds (default: 86400)
            RefreshExp int    // Seconds (default: 604800)
            Issuer     string
        }
        Cookie CookieConfig{
            Name     string // Default: "tugo_session"
            MaxAge   int
            Secure   bool
            HttpOnly bool
            SameSite string
        }
        TOTP TOTPConfig{
            Issuer string
            Period int    // Default: 30
            Digits int    // Default: 6
        }
    }

    // File storage
    Storage StorageConfig{
        Default   string
        Providers map[string]StorageProvider
    }

    // Server (standalone mode)
    Server ServerConfig{
        Port         int           // Default: 8080
        ReadTimeout  time.Duration
        WriteTimeout time.Duration
    }
}
```

## Examples

See the [examples](./examples) directory:

- [Standalone Server](./examples/standalone/main.go) - Minimal standalone setup
- [Middleware Integration](./examples/middleware/main.go) - Integration with existing Gin app
- [Full Authentication](./examples/with-auth/main.go) - JWT + TOTP authentication

## Response Format

All responses follow a consistent format:

**Success:**

```json
{
  "success": true,
  "data": { ... }
}
```

**Error:**

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed",
    "details": {
      "errors": [
        {"field": "email", "message": "invalid email format", "code": "invalid_email"}
      ]
    }
  }
}
```

## System Tables

TuGo uses the following system tables (created by migrations):

| Table | Purpose |
|-------|---------|
| `tugo_roles` | Role definitions |
| `tugo_users` | User accounts |
| `tugo_sessions` | Session tokens |
| `tugo_permissions` | Role-based permissions |
| `autoapi_files` | File storage metadata |
| `tugo_audit_log` | Audit trail |

## License

MIT License
