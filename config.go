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
	Upload(ctx interface{}, file interface{}, filename string, opts *UploadOptions) (string, error)

	// Download retrieves a file by its storage path.
	Download(ctx interface{}, path string) (interface{}, error)

	// Delete removes a file by its storage path.
	Delete(ctx interface{}, path string) error

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
