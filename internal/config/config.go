package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Host            string
	Port            int
	S3Endpoint      string
	S3Region        string
	S3Bucket        string
	S3UseSSL        bool
	AWSAccessKey    string
	AWSSecretKey    string
	CleanupInterval time.Duration
	MaxPasteSize    int64
	DefaultTTL      time.Duration
	LogFormat       string
	LogLevel        string
	SecureCookies   bool
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:            getEnv("PASTEBIN_HOST", "127.0.0.1"),
		Port:            getEnvInt("PASTEBIN_PORT", 8080),
		S3Endpoint:      getEnv("S3_ENDPOINT", "s3.amazonaws.com"),
		S3Region:        getEnv("S3_REGION", "us-east-1"),
		S3Bucket:        os.Getenv("S3_BUCKET"),
		S3UseSSL:        getEnvBool("S3_USE_SSL", true),
		AWSAccessKey:    os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretKey:    os.Getenv("AWS_SECRET_ACCESS_KEY"),
		CleanupInterval: getEnvDuration("CLEANUP_INTERVAL", time.Hour),
		MaxPasteSize:    getEnvInt64("MAX_PASTE_SIZE", 1024*1024), // 1MB
		DefaultTTL:      getEnvDuration("DEFAULT_TTL", 365*24*time.Hour),
		LogFormat:       getEnv("LOG_FORMAT", "text"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		SecureCookies:   getEnvBool("SECURE_COOKIES", false),
	}

	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET environment variable is required")
	}

	return cfg, nil
}

func (c *Config) S3EndpointURL() string {
	scheme := "https"
	if !c.S3UseSSL {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, c.S3Endpoint)
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
