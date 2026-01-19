package tugo

import (
	"time"

	"github.com/jmoiron/sqlx"
)

// Config holds the complete configuration for TuGo engine.
type Config struct {
	// DB is an existing sqlx database connection.
	// Either DB or DatabaseURL must be provided.
	DB *sqlx.DB

	// DatabaseURL is a PostgreSQL connection string.
	// Used when DB is nil to create a new connection.
	DatabaseURL string

	// Discovery configures how tables are discovered and exposed.
	Discovery DiscoveryConfig

	// Auth configures authentication methods.
	Auth AuthConfig

	// Storage configures file storage providers.
	Storage StorageConfig

	// Server configures the HTTP server (standalone mode only).
	Server ServerConfig

	// Mount configures route mounting behavior.
	Mount MountOptions

	// Seed configures user seeding on first run.
	Seed SeedConfig

	// SchemaWatch configures automatic schema change detection.
	SchemaWatch SchemaWatchConfig
}

// DiscoveryConfig configures table discovery behavior.
type DiscoveryConfig struct {
	// Mode determines discovery strategy: "prefix", "config", or "hybrid".
	// Default: "prefix"
	Mode string

	// Prefix is the table name prefix for auto-discovery.
	// Default: "api_"
	Prefix string

	// AutoDiscover enables automatic exposure of discovered tables.
	// Default: false (requires explicit enable)
	AutoDiscover bool

	// Blacklist contains table names to always exclude.
	Blacklist []string

	// Config provides per-collection configuration overrides.
	Config CollectionConfigMap
}

// CollectionConfigMap maps collection names to their configuration.
type CollectionConfigMap map[string]CollectionItemConfig

// CollectionItemConfig holds configuration for a single collection.
type CollectionItemConfig struct {
	// Enabled determines if this collection is exposed via API.
	Enabled bool

	// PublicFields limits which fields are visible.
	// nil means all fields are visible.
	PublicFields []string
}

// AuthConfig configures authentication.
type AuthConfig struct {
	// Methods lists enabled authentication methods: "jwt", "cookie", "totp".
	Methods []string

	// JWT configures JWT authentication.
	JWT JWTConfig

	// Cookie configures cookie-based sessions.
	Cookie CookieConfig

	// TOTP configures time-based one-time passwords.
	TOTP TOTPConfig

	// CustomUserStore allows injecting a custom UserStore implementation.
	// If provided, TuGo will use this instead of the default DBUserStore.
	// This enables apps to use custom user tables and add business logic.
	//
	// The custom store must implement the auth.UserStore interface.
	// See auth/types.go for the interface definition.
	//
	// Example with embed pattern:
	//
	//	type Employee struct {
	//	    auth.User                    // Embed the base User
	//	    DepartmentID string          `db:"department_id" json:"department_id"`
	//	    HireDate     time.Time       `db:"hire_date" json:"hire_date"`
	//	}
	//
	//	type EmployeeStore struct {
	//	    db          *sqlx.DB
	//	    emailClient *email.Client
	//	}
	//
	//	func (s *EmployeeStore) Create(ctx context.Context, user *auth.User, hash string) error {
	//	    // Custom business logic: can access embedded User fields
	//	    _, err := s.db.ExecContext(ctx, "INSERT INTO employees ...", user.ID, user.Username, hash)
	//	    if err != nil { return err }
	//	    s.emailClient.SendWelcome(user.Email)
	//	    return nil
	//	}
	//
	//	func (s *EmployeeStore) GetByUsername(ctx context.Context, username string) (*auth.User, error) {
	//	    var emp Employee // Employee embeds auth.User
	//	    if err := s.db.GetContext(ctx, &emp, "SELECT * FROM employees WHERE username = $1", username); err != nil {
	//	        return nil, err
	//	    }
	//	    return &emp.User, nil // Return embedded User
	//	}
	//
	// Pass to TuGo config:
	//
	//	tugo.New(tugo.Config{
	//	    Auth: tugo.AuthConfig{
	//	        CustomUserStore: &EmployeeStore{db: db, emailClient: emailClient},
	//	    },
	//	})
	//
	CustomUserStore any // Must implement auth.UserStore interface
}

// JWTConfig configures JWT authentication.
type JWTConfig struct {
	// Secret is the signing key for HS256.
	Secret string

	// Expiry is the token expiry time in seconds.
	// Default: 86400 (24 hours)
	Expiry int

	// RefreshExp is the refresh token expiry in seconds.
	// Default: 604800 (7 days)
	RefreshExp int

	// Issuer is the JWT issuer claim.
	Issuer string
}

// CookieConfig configures cookie-based sessions.
type CookieConfig struct {
	// Name is the cookie name.
	// Default: "tugo_session"
	Name string

	// MaxAge is the cookie max age in seconds.
	MaxAge int

	// Secure sets the Secure flag.
	Secure bool

	// HttpOnly sets the HttpOnly flag.
	HttpOnly bool

	// SameSite sets the SameSite attribute.
	SameSite string
}

// TOTPConfig configures TOTP authentication.
type TOTPConfig struct {
	// Issuer is displayed in authenticator apps.
	Issuer string

	// Period is the TOTP period in seconds.
	// Default: 30
	Period int

	// Digits is the number of digits in the code.
	// Default: 6
	Digits int
}

// StorageConfig configures file storage.
type StorageConfig struct {
	// Default is the default storage provider name.
	Default string

	// Providers maps names to storage provider implementations.
	Providers map[string]StorageProvider
}

// StorageProvider is the interface for file storage backends.
type StorageProvider interface {
	// Upload stores a file and returns the storage path.
	Upload(ctx any, file any, filename string, opts *UploadOptions) (string, error)

	// Download retrieves a file by its storage path.
	Download(ctx any, path string) (any, error)

	// Delete removes a file by its storage path.
	Delete(ctx any, path string) error

	// GetURL returns a public URL for the file.
	GetURL(path string) string
}

// UploadOptions provides options for file uploads.
type UploadOptions struct {
	// ContentType is the MIME type.
	ContentType string

	// MaxSize is the maximum file size in bytes.
	MaxSize int64
}

// ServerConfig configures the HTTP server for standalone mode.
type ServerConfig struct {
	// Port is the server port.
	// Default: 8080
	Port int

	// ReadTimeout is the request read timeout.
	ReadTimeout time.Duration

	// WriteTimeout is the response write timeout.
	WriteTimeout time.Duration
}

// MountOptions configures how TuGo mounts its routes.
type MountOptions struct {
	// IncludeAdmin enables auto-registration of admin routes under /admin.
	// Default: false
	IncludeAdmin bool

	// AdminPath is the path prefix for admin routes.
	// Default: "/admin"
	AdminPath string

	// RequireAdminAuth requires admin role for admin routes.
	// Default: true
	RequireAdminAuth bool
}

// DefaultMountOptions returns default mount options.
func DefaultMountOptions() MountOptions {
	return MountOptions{
		IncludeAdmin:     false,
		AdminPath:        "/admin",
		RequireAdminAuth: true,
	}
}

// SeedConfig configures user seeding on first run.
type SeedConfig struct {
	// Enabled enables user seeding.
	Enabled bool

	// AdminUser is the default admin user configuration.
	AdminUser *SeedUser
}

// SeedUser represents a user to seed.
type SeedUser struct {
	Username string
	Email    string
	Password string
	Role     string // "admin", "user", etc.
}

// SchemaWatchConfig configures automatic schema change detection.
type SchemaWatchConfig struct {
	// Enabled enables schema watching.
	Enabled bool

	// Mode is the watch mode: "poll" or "notify".
	// "poll" uses periodic polling.
	// "notify" uses PostgreSQL LISTEN/NOTIFY (more efficient).
	// Default: "poll"
	Mode string

	// PollInterval is the interval between polls (for poll mode).
	// Default: 30s
	PollInterval time.Duration

	// Channel is the PostgreSQL notification channel (for notify mode).
	// Default: "tugo_schema_change"
	Channel string
}

// DefaultSchemaWatchConfig returns default schema watch configuration.
func DefaultSchemaWatchConfig() SchemaWatchConfig {
	return SchemaWatchConfig{
		Enabled:      false,
		Mode:         "poll",
		PollInterval: 30 * time.Second,
		Channel:      "tugo_schema_change",
	}
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Discovery: DiscoveryConfig{
			Mode:         "prefix",
			Prefix:       "api_",
			AutoDiscover: false,
			Blacklist:    []string{},
			Config:       make(CollectionConfigMap),
		},
		Auth: AuthConfig{
			Methods: []string{"jwt"},
			JWT: JWTConfig{
				Expiry:     86400,
				RefreshExp: 604800,
			},
			Cookie: CookieConfig{
				Name:     "tugo_session",
				MaxAge:   86400,
				HttpOnly: true,
				SameSite: "Lax",
			},
			TOTP: TOTPConfig{
				Period: 30,
				Digits: 6,
			},
		},
		Server: ServerConfig{
			Port:         8080,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	}
}
