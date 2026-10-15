package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
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
	TrustedProxies  []netip.Prefix
	MetricsHost     string
	MetricsPort     int
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:            getEnv("PASTEBIN_HOST", "127.0.0.1"),
		Port:            getEnvInt("PASTEBIN_PORT", 8080),
		S3Endpoint:      getEnv("PASTEBIN_S3_ENDPOINT", "s3.amazonaws.com"),
		S3Region:        getEnv("PASTEBIN_S3_REGION", "us-east-1"),
		S3Bucket:        os.Getenv("PASTEBIN_S3_BUCKET"),
		S3UseSSL:        getEnvBool("PASTEBIN_S3_USE_SSL", true),
		AWSAccessKey:    os.Getenv("PASTEBIN_S3_ACCESS_KEY"),
		AWSSecretKey:    os.Getenv("PASTEBIN_S3_SECRET_KEY"),
		CleanupInterval: getEnvDuration("PASTEBIN_CLEANUP_INTERVAL", time.Hour),
		MaxPasteSize:    getEnvInt64("PASTEBIN_MAX_PASTE_SIZE", 1024*1024), // 1MB
		DefaultTTL:      getEnvDuration("PASTEBIN_DEFAULT_TTL", 30*24*time.Hour),
		LogFormat:       getEnv("PASTEBIN_LOG_FORMAT", "text"),
		LogLevel:        getEnv("PASTEBIN_LOG_LEVEL", "info"),
		SecureCookies:   getEnvBool("PASTEBIN_SECURE_COOKIES", false),
		MetricsPort:     getEnvInt("PASTEBIN_METRICS_PORT", 0), // 0 disables metrics
	}
	cfg.MetricsHost = getEnv("PASTEBIN_METRICS_HOST", cfg.Host)

	if cfg.MetricsPort != 0 && cfg.MetricsHost == cfg.Host && cfg.MetricsPort == cfg.Port {
		return nil, fmt.Errorf("PASTEBIN_METRICS_PORT must differ from PASTEBIN_PORT")
	}

	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("PASTEBIN_S3_BUCKET environment variable is required")
	}

	proxies, err := parsePrefixes(os.Getenv("PASTEBIN_TRUSTED_PROXIES"))
	if err != nil {
		return nil, fmt.Errorf("invalid PASTEBIN_TRUSTED_PROXIES: %w", err)
	}
	cfg.TrustedProxies = proxies

	return cfg, nil
}

func (c *Config) S3EndpointURL() string {
	scheme := "https"
	if !c.S3UseSSL {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, c.S3Endpoint)
}

// TrustedProxiesString returns the trusted proxies as a comma separated list
func (c *Config) TrustedProxiesString() string {
	s := make([]string, len(c.TrustedProxies))
	for i, p := range c.TrustedProxies {
		s[i] = p.String()
	}
	return strings.Join(s, ",")
}

// parsePrefixes parses a comma separated list of IP addresses and CIDR
// prefixes. A plain IP address is treated as a single address prefix.
func parsePrefixes(val string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, field := range strings.Split(val, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if strings.Contains(field, "/") {
			p, err := netip.ParsePrefix(field)
			if err != nil {
				return nil, err
			}
			prefixes = append(prefixes, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(field)
		if err != nil {
			return nil, err
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return prefixes, nil
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
