package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_RequiredS3Bucket(t *testing.T) {
	// Clear any existing PASTEBIN_S3_BUCKET
	os.Unsetenv("PASTEBIN_S3_BUCKET")

	_, err := Load()
	if err == nil {
		t.Error("expected error when PASTEBIN_S3_BUCKET is not set")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("PASTEBIN_S3_BUCKET", "test-bucket")
	defer os.Unsetenv("PASTEBIN_S3_BUCKET")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected default host 127.0.0.1, got %s", cfg.Host)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}

	if cfg.S3Endpoint != "s3.amazonaws.com" {
		t.Errorf("expected default S3 endpoint, got %s", cfg.S3Endpoint)
	}

	if cfg.S3Region != "us-east-1" {
		t.Errorf("expected default S3 region us-east-1, got %s", cfg.S3Region)
	}

	if !cfg.S3UseSSL {
		t.Error("expected S3UseSSL to default to true")
	}

	if cfg.CleanupInterval != time.Hour {
		t.Errorf("expected default cleanup interval 1h, got %v", cfg.CleanupInterval)
	}

	if cfg.MaxPasteSize != 1024*1024 {
		t.Errorf("expected default max paste size 1MB, got %d", cfg.MaxPasteSize)
	}

	if cfg.DefaultTTL != 365*24*time.Hour {
		t.Errorf("expected default TTL 1 year, got %v", cfg.DefaultTTL)
	}

	if cfg.SecureCookies {
		t.Error("expected SecureCookies to default to false")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	envVars := map[string]string{
		"PASTEBIN_S3_BUCKET":        "my-bucket",
		"PASTEBIN_HOST":             "0.0.0.0",
		"PASTEBIN_PORT":             "9000",
		"PASTEBIN_S3_ENDPOINT":      "minio.local",
		"PASTEBIN_S3_REGION":        "eu-west-1",
		"PASTEBIN_S3_USE_SSL":       "false",
		"PASTEBIN_CLEANUP_INTERVAL": "30m",
		"PASTEBIN_MAX_PASTE_SIZE":   "2097152",
		"PASTEBIN_DEFAULT_TTL":      "48h",
		"PASTEBIN_SECURE_COOKIES":   "true",
	}

	for k, v := range envVars {
		os.Setenv(k, v)
		defer os.Unsetenv(k)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.S3Bucket != "my-bucket" {
		t.Errorf("expected bucket my-bucket, got %s", cfg.S3Bucket)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Host)
	}

	if cfg.Port != 9000 {
		t.Errorf("expected port 9000, got %d", cfg.Port)
	}

	if cfg.S3Endpoint != "minio.local" {
		t.Errorf("expected endpoint minio.local, got %s", cfg.S3Endpoint)
	}

	if cfg.S3Region != "eu-west-1" {
		t.Errorf("expected region eu-west-1, got %s", cfg.S3Region)
	}

	if cfg.S3UseSSL {
		t.Error("expected S3UseSSL to be false")
	}

	if cfg.CleanupInterval != 30*time.Minute {
		t.Errorf("expected cleanup interval 30m, got %v", cfg.CleanupInterval)
	}

	if cfg.MaxPasteSize != 2097152 {
		t.Errorf("expected max paste size 2097152, got %d", cfg.MaxPasteSize)
	}

	if cfg.DefaultTTL != 48*time.Hour {
		t.Errorf("expected default TTL 48h, got %v", cfg.DefaultTTL)
	}

	if !cfg.SecureCookies {
		t.Error("expected SecureCookies to be true")
	}
}

func TestS3EndpointURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		useSSL   bool
		expected string
	}{
		{
			name:     "with SSL",
			endpoint: "s3.amazonaws.com",
			useSSL:   true,
			expected: "https://s3.amazonaws.com",
		},
		{
			name:     "without SSL",
			endpoint: "localhost:9000",
			useSSL:   false,
			expected: "http://localhost:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				S3Endpoint: tt.endpoint,
				S3UseSSL:   tt.useSSL,
			}

			if got := cfg.S3EndpointURL(); got != tt.expected {
				t.Errorf("S3EndpointURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
