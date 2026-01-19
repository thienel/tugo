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
- **Permission System**: Policy-based access control with row-level filtering
- **Auto-migrations**: Internal migration system (no external tools required)
- **Schema Watching**: Auto-detect database changes via polling or PostgreSQL LISTEN/NOTIFY
- **User Seeding**: Built-in mechanism to seed admin users from config or environment
- **Custom UserStore**: Support for custom user tables with embed pattern

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
        // Auto-register admin routes
        Mount: tugo.MountOptions{
            IncludeAdmin: true,
        },
        // Seed default admin user
        Seed: tugo.SeedConfig{
            Enabled: true,
            AdminUser: &tugo.SeedUser{
                Username: "admin",
                Email:    "admin@example.com",
                Password: "changeme",
                Role:     "admin",
            },
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
        Mount: tugo.MountOptions{
            IncludeAdmin:     true,
            AdminPath:        "/admin",
            RequireAdminAuth: true,
        },
    })
    engine.Init(context.Background())

    // Mount TuGo routes (includes admin routes if configured)
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

### Migrations

TuGo automatically runs migrations during `Init()`. No external migration tools required.

```go
// Migrations run automatically
engine.Init(context.Background())
```

To check migration status programmatically:

```go
import "github.com/thienel/tugo/pkg/migrate"

migrator := migrate.NewMigrator(db, logger)
status, _ := migrator.Status(ctx)
for _, s := range status {
    fmt.Printf("%s: applied=%v\n", s.Name, s.Applied)
}
```

## User Seeding

### From Configuration

```go
engine, _ := tugo.New(tugo.Config{
    Seed: tugo.SeedConfig{
        Enabled: true,
        AdminUser: &tugo.SeedUser{
            Username: "admin",
            Email:    "admin@example.com",
            Password: "secure-password",
            Role:     "admin",
        },
    },
})
```

### From Environment Variables

Set these environment variables and TuGo will seed the admin user automatically:

```bash
export TUGO_ADMIN_USERNAME=admin
export TUGO_ADMIN_EMAIL=admin@example.com
export TUGO_ADMIN_PASSWORD=secure-password
```

## Schema Watching

Auto-detect database schema changes:

```go
engine, _ := tugo.New(tugo.Config{
    SchemaWatch: tugo.SchemaWatchConfig{
        Enabled:      true,
        Mode:         "poll",         // "poll" or "notify"
        PollInterval: 30 * time.Second,
    },
})
```

Or trigger manually:

```go
engine.TriggerSchemaRefresh(ctx)
```

## Permission System

TuGo includes a policy-based permission system with row-level filtering:

```go
import "github.com/thienel/tugo/pkg/permission"

// Create permission checker
checker := permission.NewChecker(db, logger)

// Set policy: users can only read their own records
checker.SetPolicy(ctx, userRoleID, "posts", permission.ActionRead,
    map[string]any{"author_id": "$USER_ID"}, // Row-level filter
    nil, // Field permissions
    nil, // Presets
)

// Use middleware
router.Use(permission.Middleware(checker))
```

### Filter Variables

| Variable | Description |
|----------|-------------|
| `$USER_ID` | Current user's ID |
| `$ROLE_ID` | Current user's role ID |
| `$ROLE` | Current user's role name |
| `$USERNAME` | Current user's username |
| `$EMAIL` | Current user's email |

## Custom UserStore

Use custom user tables with the embed pattern:

```go
// Define custom user type embedding auth.User
type Employee struct {
    auth.User                    // Embed base User
    DepartmentID string          `db:"department_id" json:"department_id"`
    HireDate     time.Time       `db:"hire_date" json:"hire_date"`
}

// Implement auth.UserStore interface
type EmployeeStore struct {
    db *sqlx.DB
}

func (s *EmployeeStore) GetByUsername(ctx context.Context, username string) (*auth.User, error) {
    var emp Employee
    err := s.db.GetContext(ctx, &emp,
        "SELECT * FROM employees WHERE username = $1", username)
    return &emp.User, err // Return embedded User
}

func (s *EmployeeStore) Create(ctx context.Context, user *auth.User, hash string) error {
    _, err := s.db.ExecContext(ctx,
        "INSERT INTO employees (id, username, email, password_hash) VALUES ($1, $2, $3, $4)",
        user.ID, user.Username, user.Email, hash)
    return err
}

// ... implement other UserStore methods

// Use in config
engine, _ := tugo.New(tugo.Config{
    Auth: tugo.AuthConfig{
        Methods:         []string{"jwt"},
        CustomUserStore: &EmployeeStore{db: db},
    },
})
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

### Admin Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/admin/collections` | List all collections |
| GET | `/admin/collections/:name` | Get collection details |
| POST | `/admin/collections` | Create new collection |
| DELETE | `/admin/collections/:name` | Drop collection |
| POST | `/admin/collections/:name/fields` | Add field |
| PATCH | `/admin/collections/:name/fields/:field` | Alter field |
| DELETE | `/admin/collections/:name/fields/:field` | Drop field |
| POST | `/admin/sync-schema` | Refresh schema |

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

### Field Selection

```
GET /api/v1/products?fields=id,name,price
```

### Search

```
GET /api/v1/products?search=iphone
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
        Methods         []string  // "jwt", "cookie", "totp"
        CustomUserStore any       // Custom auth.UserStore implementation
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

    // Route mounting
    Mount MountOptions{
        IncludeAdmin     bool   // Auto-register admin routes
        AdminPath        string // Default: "/admin"
        RequireAdminAuth bool   // Require admin role (default: true)
    }

    // User seeding
    Seed SeedConfig{
        Enabled   bool
        AdminUser *SeedUser{
            Username string
            Email    string
            Password string
            Role     string
        }
    }

    // Schema watching
    SchemaWatch SchemaWatchConfig{
        Enabled      bool
        Mode         string        // "poll" or "notify"
        PollInterval time.Duration // Default: 30s
        Channel      string        // PG channel (default: "tugo_schema_change")
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

TuGo uses the following system tables (created automatically):

| Table | Purpose |
|-------|---------|
| `tugo_roles` | Role definitions |
| `tugo_users` | User accounts |
| `tugo_sessions` | Session tokens |
| `tugo_permissions` | Role-based permissions |
| `tugo_collections` | Collection metadata |
| `tugo_fields` | Field definitions |
| `tugo_relationships` | Relationship metadata |
| `tugo_migrations` | Migration tracking |
| `tugo_audit_log` | Audit trail |
| `autoapi_files` | File storage metadata |

## License

MIT License
