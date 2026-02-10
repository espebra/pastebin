package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/espebra/pastebin/internal/paste"
)

const (
	pastePrefix = "pastes/"
	metaPrefix  = "meta/"
)

// S3Storage handles S3 operations for pastes
type S3Storage struct {
	client *s3.Client
	bucket string
}

// New creates a new S3Storage instance
func New(ctx context.Context, endpoint, region, bucket, accessKey, secretKey string, useSSL bool) (*S3Storage, error) {
	scheme := "https"
	if !useSSL {
		scheme = "http"
	}
	endpointURL := fmt.Sprintf("%s://%s", scheme, endpoint)

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
		o.UsePathStyle = true // Required for MinIO and most S3-compatible services
	})

	storage := &S3Storage{
		client: client,
		bucket: bucket,
	}

	// Ensure bucket exists, create if it doesn't
	if err := storage.ensureBucketExists(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket exists: %w", err)
	}

	return storage, nil
}

// ensureBucketExists checks if the bucket exists and creates it if it doesn't
func (s *S3Storage) ensureBucketExists(ctx context.Context) error {
	// Check if bucket exists using HeadBucket
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err == nil {
		// Bucket exists
		return nil
	}

	// Check if the error is because the bucket doesn't exist
	var notFound *types.NotFound
	if !errors.As(err, &notFound) {
		// Some other error occurred
		return fmt.Errorf("failed to check bucket: %w", err)
	}

	// Bucket doesn't exist, create it
	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	return nil
}

// Store saves a paste and its metadata to S3
func (s *S3Storage) Store(ctx context.Context, p *paste.Paste, meta *paste.Meta) error {
	// Store paste content
	pasteKey := pastePrefix + p.Checksum
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(pasteKey),
		Body:        strings.NewReader(p.Content),
		ContentType: aws.String("text/plain; charset=utf-8"),
	})
	if err != nil {
		return fmt.Errorf("failed to store paste: %w", err)
	}

	// Store metadata
	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metaKey := metaPrefix + p.Checksum + ".json"
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(metaKey),
		Body:        bytes.NewReader(metaData),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("failed to store metadata: %w", err)
	}

	return nil
}

// ErrChecksumMismatch is returned when retrieved content doesn't match expected checksum
var ErrChecksumMismatch = errors.New("content checksum mismatch: possible data corruption")

// Get retrieves a paste and its metadata from S3
func (s *S3Storage) Get(ctx context.Context, checksum string) (*paste.Paste, *paste.Meta, error) {
	// Get paste content
	pasteKey := pastePrefix + checksum
	pasteResult, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(pasteKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get paste: %w", err)
	}
	defer func() { _ = pasteResult.Body.Close() }()

	content, err := io.ReadAll(pasteResult.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read paste content: %w", err)
	}

	// Verify checksum to detect corruption
	computedChecksum := paste.ComputeChecksum(string(content))
	if computedChecksum != checksum {
		return nil, nil, fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, checksum, computedChecksum)
	}

	// Get metadata
	metaKey := metaPrefix + checksum + ".json"
	metaResult, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(metaKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get metadata: %w", err)
	}
	defer func() { _ = metaResult.Body.Close() }()

	var meta paste.Meta
	if err := json.NewDecoder(metaResult.Body).Decode(&meta); err != nil {
		return nil, nil, fmt.Errorf("failed to decode metadata: %w", err)
	}

	return &paste.Paste{
		Checksum: checksum,
		Content:  string(content),
	}, &meta, nil
}

// Delete removes a paste and its metadata from S3
func (s *S3Storage) Delete(ctx context.Context, checksum string) error {
	// Delete paste content
	pasteKey := pastePrefix + checksum
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(pasteKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete paste: %w", err)
	}

	// Delete metadata
	metaKey := metaPrefix + checksum + ".json"
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(metaKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	return nil
}

// MetaCallback is called for each metadata entry during iteration.
// Return an error to stop iteration early (the error will be propagated).
// Return nil to continue to the next item.
type MetaCallback func(meta *paste.Meta) error

// ForEachMeta iterates over all paste metadata, calling the callback for each entry.
// This uses a streaming approach to avoid loading all metadata into memory.
func (s *S3Storage) ForEachMeta(ctx context.Context, callback MetaCallback) error {
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(metaPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list metadata: %w", err)
		}

		for _, obj := range page.Contents {
			meta, err := s.fetchMeta(ctx, obj.Key)
			if err != nil {
				continue // Skip objects we can't read
			}

			if err := callback(meta); err != nil {
				return err
			}
		}
	}

	return nil
}

// fetchMeta retrieves and decodes a single metadata object.
// Uses defer to ensure the response body is always closed.
func (s *S3Storage) fetchMeta(ctx context.Context, key *string) (*paste.Meta, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    key,
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Body.Close() }()

	var meta paste.Meta
	if err := json.NewDecoder(result.Body).Decode(&meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// Exists checks if a paste exists
func (s *S3Storage) Exists(ctx context.Context, checksum string) (bool, error) {
	pasteKey := pastePrefix + checksum
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(pasteKey),
	})
	if err != nil {
		var notFound *types.NotFound
		if ok := errors.As(err, &notFound); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
