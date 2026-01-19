package storage

import (
	"context"
	"io"
	"time"
)

// Provider is the interface for file storage backends.
type Provider interface {
	// Upload stores a file and returns the storage path.
	Upload(ctx context.Context, file io.Reader, filename string, opts *UploadOptions) (*FileInfo, error)

	// Download retrieves a file by its storage path.
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete removes a file by its storage path.
	Delete(ctx context.Context, path string) error

	// GetURL returns a public URL for the file.
	GetURL(path string) string

	// Exists checks if a file exists at the given path.
	Exists(ctx context.Context, path string) (bool, error)
}

// UploadOptions provides options for file uploads.
type UploadOptions struct {
	// ContentType is the MIME type of the file.
	ContentType string

	// MaxSize is the maximum file size in bytes. 0 means no limit.
	MaxSize int64

	// Directory is the subdirectory to store the file in.
	Directory string

	// PreserveName keeps the original filename instead of generating a unique one.
	PreserveName bool

	// Metadata is additional metadata to store with the file.
	Metadata map[string]string
}

// FileInfo contains information about an uploaded file.
type FileInfo struct {
	// ID is the unique identifier for the file.
	ID string `json:"id"`

	// Filename is the original filename.
	Filename string `json:"filename"`

	// StoragePath is the path where the file is stored.
	StoragePath string `json:"storage_path"`

	// URL is the public URL to access the file.
	URL string `json:"url"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// ContentType is the MIME type of the file.
	ContentType string `json:"content_type"`

	// UploadedAt is the upload timestamp.
	UploadedAt time.Time `json:"uploaded_at"`
}

// DefaultUploadOptions returns default upload options.
func DefaultUploadOptions() *UploadOptions {
	return &UploadOptions{
		ContentType:  "application/octet-stream",
		MaxSize:      50 * 1024 * 1024, // 50MB default
		Directory:    "",
		PreserveName: false,
		Metadata:     make(map[string]string),
	}
}
