package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIO implements Provider for MinIO/S3-compatible storage.
type MinIO struct {
	client   *minio.Client
	bucket   string
	endpoint string
	useSSL   bool
}

// MinIOConfig holds configuration for MinIO storage.
type MinIOConfig struct {
	// Endpoint is the MinIO server endpoint (e.g., "localhost:9000").
	Endpoint string

	// AccessKey is the access key ID.
	AccessKey string

	// SecretKey is the secret access key.
	SecretKey string

	// Bucket is the bucket name to use.
	Bucket string

	// UseSSL enables HTTPS connections.
	UseSSL bool

	// Region is the bucket region (optional).
	Region string

	// CreateBucket creates the bucket if it doesn't exist.
	CreateBucket bool
}

// NewMinIO creates a new MinIO storage provider.
func NewMinIO(cfg MinIOConfig) (*MinIO, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Check if bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}

	if !exists {
		if cfg.CreateBucket {
			err = client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{
				Region: cfg.Region,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create bucket: %w", err)
			}
		} else {
			return nil, fmt.Errorf("bucket %s does not exist", cfg.Bucket)
		}
	}

	return &MinIO{
		client:   client,
		bucket:   cfg.Bucket,
		endpoint: cfg.Endpoint,
		useSSL:   cfg.UseSSL,
	}, nil
}

// Upload stores a file in MinIO.
func (m *MinIO) Upload(ctx context.Context, file io.Reader, filename string, opts *UploadOptions) (*FileInfo, error) {
	if opts == nil {
		opts = DefaultUploadOptions()
	}

	// Generate unique ID
	fileID := uuid.New().String()

	// Prepare object name
	safeFilename := sanitizeFilename(filename)
	objectName := fileID + filepath.Ext(safeFilename)
	if opts.PreserveName {
		objectName = safeFilename
	}
	if opts.Directory != "" {
		objectName = opts.Directory + "/" + objectName
	}

	// Set content type
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Upload options
	putOpts := minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: opts.Metadata,
	}

	// Upload with size limit if specified
	var size int64 = -1
	if opts.MaxSize > 0 {
		size = opts.MaxSize
	}

	info, err := m.client.PutObject(ctx, m.bucket, objectName, file, size, putOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return &FileInfo{
		ID:          fileID,
		Filename:    filename,
		StoragePath: objectName,
		URL:         m.GetURL(objectName),
		Size:        info.Size,
		ContentType: contentType,
		UploadedAt:  time.Now(),
	}, nil
}

// Download retrieves a file from MinIO.
func (m *MinIO) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	object, err := m.client.GetObject(ctx, m.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	// Check if object exists by reading stat
	_, err = object.Stat()
	if err != nil {
		object.Close()
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return nil, fmt.Errorf("file not found")
		}
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return object, nil
}

// Delete removes a file from MinIO.
func (m *MinIO) Delete(ctx context.Context, path string) error {
	err := m.client.RemoveObject(ctx, m.bucket, path, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// GetURL returns the public URL for a file.
func (m *MinIO) GetURL(path string) string {
	protocol := "http"
	if m.useSSL {
		protocol = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", protocol, m.endpoint, m.bucket, path)
}

// Exists checks if a file exists in MinIO.
func (m *MinIO) Exists(ctx context.Context, path string) (bool, error) {
	_, err := m.client.StatObject(ctx, m.bucket, path, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat object: %w", err)
	}
	return true, nil
}

// GetPresignedURL generates a presigned URL for temporary access.
func (m *MinIO) GetPresignedURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	url, err := m.client.PresignedGetObject(ctx, m.bucket, path, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return url.String(), nil
}
