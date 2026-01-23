package paste

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// TTLOption represents a time-to-live choice for pastes
type TTLOption struct {
	Label     string
	Duration  time.Duration
	IsDefault bool
}

// Forever represents a paste that never expires (100 years)
const Forever = 100 * 365 * 24 * time.Hour

// TTLOptions returns the available TTL choices with the specified default marked
func TTLOptions(defaultTTL time.Duration) []TTLOption {
	options := []TTLOption{
		{Label: "1 day", Duration: 24 * time.Hour},
		{Label: "1 week", Duration: 7 * 24 * time.Hour},
		{Label: "1 month", Duration: 30 * 24 * time.Hour},
		{Label: "1 year", Duration: 365 * 24 * time.Hour},
		{Label: "Forever", Duration: Forever},
	}
	for i := range options {
		if options[i].Duration == defaultTTL {
			options[i].IsDefault = true
		}
	}
	return options
}

// Paste represents a paste's content
type Paste struct {
	Checksum string
	Content  string
}

// Meta represents paste metadata stored alongside the content
type Meta struct {
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Size      int64     `json:"size"`
}

// IsExpired returns true if the paste has exceeded its TTL
func (m *Meta) IsExpired() bool {
	return time.Now().After(m.ExpiresAt)
}

// NewPaste creates a new paste with the given content
func NewPaste(content string) *Paste {
	return &Paste{
		Checksum: ComputeChecksum(content),
		Content:  content,
	}
}

// NewMeta creates metadata for a paste with the specified TTL
func NewMeta(checksum string, size int64, ttl time.Duration) *Meta {
	now := time.Now()
	return &Meta{
		Checksum:  checksum,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		Size:      size,
	}
}

// ComputeChecksum calculates the SHA256 checksum of the given content
func ComputeChecksum(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
