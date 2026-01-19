package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Local implements Provider for local filesystem storage.
type Local struct {
	// BasePath is the root directory for file storage.
	BasePath string

	// BaseURL is the base URL for serving files.
	BaseURL string
}

// NewLocal creates a new local filesystem storage provider.
func NewLocal(basePath, baseURL string) (*Local, error) {
	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Normalize base URL
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Local{
		BasePath: basePath,
		BaseURL:  baseURL,
	}, nil
}

// Upload stores a file on the local filesystem.
func (l *Local) Upload(ctx context.Context, file io.Reader, filename string, opts *UploadOptions) (*FileInfo, error) {
	if opts == nil {
		opts = DefaultUploadOptions()
	}

	// Generate unique ID
	fileID := uuid.New().String()

	// Sanitize and prepare filename
	safeFilename := sanitizeFilename(filename)
	if !opts.PreserveName {
		ext := filepath.Ext(safeFilename)
		safeFilename = fileID + ext
	}

	// Build storage path
	storagePath := safeFilename
	if opts.Directory != "" {
		storagePath = filepath.Join(opts.Directory, safeFilename)
	}

	// Create full file path
	fullPath := filepath.Join(l.BasePath, storagePath)

	// Ensure parent directory exists
	parentDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create the file
	dst, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	// Copy with size limit
	var size int64
	if opts.MaxSize > 0 {
		limitedReader := io.LimitReader(file, opts.MaxSize+1)
		size, err = io.Copy(dst, limitedReader)
		if err != nil {
			os.Remove(fullPath)
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
		if size > opts.MaxSize {
			os.Remove(fullPath)
			return nil, fmt.Errorf("file size exceeds maximum allowed (%d bytes)", opts.MaxSize)
		}
	} else {
		size, err = io.Copy(dst, file)
		if err != nil {
			os.Remove(fullPath)
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
	}

	return &FileInfo{
		ID:          fileID,
		Filename:    filename,
		StoragePath: storagePath,
		URL:         l.GetURL(storagePath),
		Size:        size,
		ContentType: opts.ContentType,
		UploadedAt:  time.Now(),
	}, nil
}

// Download retrieves a file from the local filesystem.
func (l *Local) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	// Prevent path traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid path")
	}

	fullPath := filepath.Join(l.BasePath, cleanPath)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found")
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// Delete removes a file from the local filesystem.
func (l *Local) Delete(ctx context.Context, path string) error {
	// Prevent path traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path")
	}

	fullPath := filepath.Join(l.BasePath, cleanPath)

	err := os.Remove(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// GetURL returns the public URL for a file.
func (l *Local) GetURL(path string) string {
	return fmt.Sprintf("%s/%s", l.BaseURL, path)
}

// Exists checks if a file exists.
func (l *Local) Exists(ctx context.Context, path string) (bool, error) {
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return false, fmt.Errorf("invalid path")
	}

	fullPath := filepath.Join(l.BasePath, cleanPath)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// sanitizeFilename removes potentially dangerous characters from filenames.
func sanitizeFilename(filename string) string {
	// Get base name to remove any path components
	filename = filepath.Base(filename)

	// Replace problematic characters
	replacer := strings.NewReplacer(
		"..", "_",
		"/", "_",
		"\\", "_",
		"\x00", "_",
	)
	return replacer.Replace(filename)
}
