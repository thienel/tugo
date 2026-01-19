package migrate

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

// Migration represents a single migration.
type Migration struct {
	Version     string
	Name        string
	UpSQL       string
	DownSQL     string
	Checksum    string
	AppliedAt   *time.Time
	ExecutionMs int64
}

// MigrationRecord stores migration history in the database.
type MigrationRecord struct {
	ID          int64     `db:"id"`
	Version     string    `db:"version"`
	Name        string    `db:"name"`
	Checksum    string    `db:"checksum"`
	AppliedAt   time.Time `db:"applied_at"`
	ExecutionMs int64     `db:"execution_ms"`
}

// Migrator handles database migrations.
type Migrator struct {
	db        *sqlx.DB
	logger    *zap.SugaredLogger
	tableName string
}

// NewMigrator creates a new migrator.
func NewMigrator(db *sqlx.DB, logger *zap.SugaredLogger) *Migrator {
	return &Migrator{
		db:        db,
		logger:    logger,
		tableName: "tugo_migrations",
	}
}

// EnsureMigrationTable creates the migration tracking table if it doesn't exist.
func (m *Migrator) EnsureMigrationTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			version VARCHAR(50) NOT NULL UNIQUE,
			name VARCHAR(255) NOT NULL,
			checksum VARCHAR(64) NOT NULL,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			execution_ms BIGINT DEFAULT 0
		)
	`, m.tableName)

	_, err := m.db.ExecContext(ctx, query)
	return err
}

// GetAppliedMigrations returns all applied migrations.
func (m *Migrator) GetAppliedMigrations(ctx context.Context) (map[string]MigrationRecord, error) {
	var records []MigrationRecord
	query := fmt.Sprintf("SELECT id, version, name, checksum, applied_at, execution_ms FROM %s ORDER BY version", m.tableName)

	if err := m.db.SelectContext(ctx, &records, query); err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	result := make(map[string]MigrationRecord)
	for _, r := range records {
		result[r.Version] = r
	}
	return result, nil
}

// LoadMigrations loads all migration files.
func (m *Migrator) LoadMigrations() ([]Migration, error) {
	entries, err := sqlFiles.ReadDir("sql")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	migrations := make(map[string]*Migration)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		content, err := sqlFiles.ReadFile("sql/" + name)
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		// Parse filename: 000001_init_tugo_tables.up.sql
		parts := strings.SplitN(name, "_", 2)
		if len(parts) != 2 {
			continue
		}
		version := parts[0]

		// Get migration name and direction
		remainder := parts[1]
		var migrationName, direction string
		if strings.HasSuffix(remainder, ".up.sql") {
			migrationName = strings.TrimSuffix(remainder, ".up.sql")
			direction = "up"
		} else if strings.HasSuffix(remainder, ".down.sql") {
			migrationName = strings.TrimSuffix(remainder, ".down.sql")
			direction = "down"
		} else {
			continue
		}

		// Create or update migration
		if _, ok := migrations[version]; !ok {
			migrations[version] = &Migration{
				Version: version,
				Name:    migrationName,
			}
		}

		if direction == "up" {
			migrations[version].UpSQL = string(content)
			migrations[version].Checksum = checksumSQL(string(content))
		} else {
			migrations[version].DownSQL = string(content)
		}
	}

	// Convert map to sorted slice
	result := make([]Migration, 0, len(migrations))
	for _, mig := range migrations {
		result = append(result, *mig)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result, nil
}

// MigrateUp runs all pending migrations.
func (m *Migrator) MigrateUp(ctx context.Context) error {
	if err := m.EnsureMigrationTable(ctx); err != nil {
		return fmt.Errorf("failed to ensure migration table: %w", err)
	}

	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	migrations, err := m.LoadMigrations()
	if err != nil {
		return err
	}

	for _, mig := range migrations {
		if _, ok := applied[mig.Version]; ok {
			// Check for checksum mismatch
			if applied[mig.Version].Checksum != mig.Checksum {
				m.logger.Warnw("Migration checksum mismatch",
					"version", mig.Version,
					"expected", mig.Checksum,
					"actual", applied[mig.Version].Checksum)
			}
			continue
		}

		if mig.UpSQL == "" {
			continue
		}

		m.logger.Infow("Running migration", "version", mig.Version, "name", mig.Name)

		start := time.Now()
		if err := m.runMigration(ctx, mig.UpSQL); err != nil {
			return fmt.Errorf("migration %s failed: %w", mig.Version, err)
		}
		executionMs := time.Since(start).Milliseconds()

		// Record migration
		if err := m.recordMigration(ctx, mig, executionMs); err != nil {
			return fmt.Errorf("failed to record migration %s: %w", mig.Version, err)
		}

		m.logger.Infow("Migration completed", "version", mig.Version, "duration_ms", executionMs)
	}

	return nil
}

// MigrateDown rolls back the last migration.
func (m *Migrator) MigrateDown(ctx context.Context) error {
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		return nil
	}

	// Find the last applied migration
	var lastVersion string
	for version := range applied {
		if version > lastVersion {
			lastVersion = version
		}
	}

	migrations, err := m.LoadMigrations()
	if err != nil {
		return err
	}

	var target *Migration
	for i := range migrations {
		if migrations[i].Version == lastVersion {
			target = &migrations[i]
			break
		}
	}

	if target == nil || target.DownSQL == "" {
		return fmt.Errorf("no down migration found for version %s", lastVersion)
	}

	m.logger.Infow("Rolling back migration", "version", target.Version, "name", target.Name)

	if err := m.runMigration(ctx, target.DownSQL); err != nil {
		return fmt.Errorf("rollback %s failed: %w", target.Version, err)
	}

	// Remove migration record
	query := fmt.Sprintf("DELETE FROM %s WHERE version = $1", m.tableName)
	if _, err := m.db.ExecContext(ctx, query, target.Version); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	m.logger.Infow("Rollback completed", "version", target.Version)
	return nil
}

// runMigration executes a migration SQL.
func (m *Migrator) runMigration(ctx context.Context, sql string) error {
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, sql)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// recordMigration records a successful migration.
func (m *Migrator) recordMigration(ctx context.Context, mig Migration, executionMs int64) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (version, name, checksum, execution_ms)
		VALUES ($1, $2, $3, $4)
	`, m.tableName)

	_, err := m.db.ExecContext(ctx, query, mig.Version, mig.Name, mig.Checksum, executionMs)
	return err
}

// Status returns the current migration status.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := m.EnsureMigrationTable(ctx); err != nil {
		return nil, err
	}

	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	migrations, err := m.LoadMigrations()
	if err != nil {
		return nil, err
	}

	var result []MigrationStatus
	for _, mig := range migrations {
		status := MigrationStatus{
			Version: mig.Version,
			Name:    mig.Name,
			Applied: false,
		}

		if record, ok := applied[mig.Version]; ok {
			status.Applied = true
			status.AppliedAt = &record.AppliedAt
			status.ExecutionMs = record.ExecutionMs
		}

		result = append(result, status)
	}

	return result, nil
}

// MigrationStatus represents the status of a single migration.
type MigrationStatus struct {
	Version     string
	Name        string
	Applied     bool
	AppliedAt   *time.Time
	ExecutionMs int64
}

// checksumSQL generates a checksum for SQL content.
func checksumSQL(sql string) string {
	// Normalize whitespace for consistent checksums
	normalized := strings.TrimSpace(sql)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// RunInternalMigrations runs all internal TuGo migrations.
// This is called during engine initialization.
func RunInternalMigrations(ctx context.Context, db *sqlx.DB, logger *zap.SugaredLogger) error {
	migrator := NewMigrator(db, logger)
	return migrator.MigrateUp(ctx)
}
