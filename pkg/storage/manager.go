package storage

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

// Manager manages multiple storage providers and file metadata.
type Manager struct {
	providers      map[string]Provider
	defaultName    string
	db             *sqlx.DB
	mu             sync.RWMutex
}

// NewManager creates a new storage manager.
func NewManager(defaultProvider string, db *sqlx.DB) *Manager {
	return &Manager{
		providers:   make(map[string]Provider),
		defaultName: defaultProvider,
		db:          db,
	}
}

// RegisterProvider registers a storage provider.
func (m *Manager) RegisterProvider(name string, provider Provider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[name] = provider
}

// GetProvider returns a provider by name.
func (m *Manager) GetProvider(name string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if name == "" {
		name = m.defaultName
	}

	provider, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("storage provider not found: %s", name)
	}
	return provider, nil
}

// DefaultProvider returns the default storage provider.
func (m *Manager) DefaultProvider() (Provider, error) {
	return m.GetProvider(m.defaultName)
}

// Upload uploads a file using the specified or default provider.
func (m *Manager) Upload(ctx context.Context, providerName string, file io.Reader, filename string, opts *UploadOptions) (*FileRecord, error) {
	provider, err := m.GetProvider(providerName)
	if err != nil {
		return nil, err
	}

	if providerName == "" {
		providerName = m.defaultName
	}

	info, err := provider.Upload(ctx, file, filename, opts)
	if err != nil {
		return nil, err
	}

	// Save file metadata to database
	record := &FileRecord{
		ID:          info.ID,
		Filename:    info.Filename,
		StoragePath: info.StoragePath,
		Provider:    providerName,
		Size:        info.Size,
		ContentType: info.ContentType,
		URL:         info.URL,
		CreatedAt:   info.UploadedAt,
	}

	if m.db != nil {
		if err := m.saveFileRecord(ctx, record); err != nil {
			// Try to delete the uploaded file
			_ = provider.Delete(ctx, info.StoragePath)
			return nil, fmt.Errorf("failed to save file record: %w", err)
		}
	}

	return record, nil
}

// Download downloads a file by ID.
func (m *Manager) Download(ctx context.Context, fileID string) (io.ReadCloser, *FileRecord, error) {
	record, err := m.GetFileRecord(ctx, fileID)
	if err != nil {
		return nil, nil, err
	}

	provider, err := m.GetProvider(record.Provider)
	if err != nil {
		return nil, nil, err
	}

	reader, err := provider.Download(ctx, record.StoragePath)
	if err != nil {
		return nil, nil, err
	}

	return reader, record, nil
}

// Delete deletes a file by ID.
func (m *Manager) Delete(ctx context.Context, fileID string) error {
	record, err := m.GetFileRecord(ctx, fileID)
	if err != nil {
		return err
	}

	provider, err := m.GetProvider(record.Provider)
	if err != nil {
		return err
	}

	// Delete from storage
	if err := provider.Delete(ctx, record.StoragePath); err != nil {
		return err
	}

	// Delete from database
	if m.db != nil {
		if err := m.deleteFileRecord(ctx, fileID); err != nil {
			return fmt.Errorf("failed to delete file record: %w", err)
		}
	}

	return nil
}

// GetURL returns the URL for a file.
func (m *Manager) GetURL(ctx context.Context, fileID string) (string, error) {
	record, err := m.GetFileRecord(ctx, fileID)
	if err != nil {
		return "", err
	}
	return record.URL, nil
}

// FileRecord represents a file metadata record in the database.
type FileRecord struct {
	ID          string    `db:"id" json:"id"`
	Filename    string    `db:"filename" json:"filename"`
	StoragePath string    `db:"storage_path" json:"storage_path"`
	Provider    string    `db:"provider" json:"provider"`
	Size        int64     `db:"size" json:"size"`
	ContentType string    `db:"content_type" json:"content_type"`
	URL         string    `db:"url" json:"url"`
	UploadedBy  *string   `db:"uploaded_by" json:"uploaded_by,omitempty"`
	Metadata    *string   `db:"metadata" json:"metadata,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// saveFileRecord saves a file record to the database.
func (m *Manager) saveFileRecord(ctx context.Context, record *FileRecord) error {
	query := `
		INSERT INTO autoapi_files (id, filename, storage_path, provider, size, content_type, url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	now := time.Now()
	_, err := m.db.ExecContext(ctx, query,
		record.ID,
		record.Filename,
		record.StoragePath,
		record.Provider,
		record.Size,
		record.ContentType,
		record.URL,
		now,
		now,
	)
	return err
}

// GetFileRecord retrieves a file record by ID.
func (m *Manager) GetFileRecord(ctx context.Context, fileID string) (*FileRecord, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	var record FileRecord
	query := `SELECT * FROM autoapi_files WHERE id = $1`
	err := m.db.GetContext(ctx, &record, query, fileID)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	return &record, nil
}

// deleteFileRecord deletes a file record from the database.
func (m *Manager) deleteFileRecord(ctx context.Context, fileID string) error {
	query := `DELETE FROM autoapi_files WHERE id = $1`
	_, err := m.db.ExecContext(ctx, query, fileID)
	return err
}

// ListFiles lists files with pagination.
func (m *Manager) ListFiles(ctx context.Context, limit, offset int) ([]*FileRecord, int, error) {
	if m.db == nil {
		return nil, 0, fmt.Errorf("database not configured")
	}

	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM autoapi_files`
	if err := m.db.GetContext(ctx, &total, countQuery); err != nil {
		return nil, 0, err
	}

	// Get files
	var records []*FileRecord
	query := `SELECT * FROM autoapi_files ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	if err := m.db.SelectContext(ctx, &records, query, limit, offset); err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// EnsureTable creates the autoapi_files table if it doesn't exist.
func (m *Manager) EnsureTable(ctx context.Context) error {
	if m.db == nil {
		return nil
	}

	query := `
		CREATE TABLE IF NOT EXISTS autoapi_files (
			id VARCHAR(36) PRIMARY KEY,
			filename VARCHAR(255) NOT NULL,
			storage_path VARCHAR(512) NOT NULL,
			provider VARCHAR(50) NOT NULL,
			size BIGINT NOT NULL DEFAULT 0,
			content_type VARCHAR(100),
			url TEXT,
			uploaded_by VARCHAR(36),
			metadata JSONB,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_autoapi_files_created_at ON autoapi_files(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_autoapi_files_uploaded_by ON autoapi_files(uploaded_by);
	`
	_, err := m.db.ExecContext(ctx, query)
	return err
}
