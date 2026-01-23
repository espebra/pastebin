package paste

import (
	"testing"
	"time"
)

func TestNewPaste(t *testing.T) {
	content := "Hello, World!"
	p := NewPaste(content)

	if p.Content != content {
		t.Errorf("expected content %q, got %q", content, p.Content)
	}

	if p.Checksum == "" {
		t.Error("expected checksum to be set")
	}

	// SHA256 of "Hello, World!" should be consistent
	expectedChecksum := "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"
	if p.Checksum != expectedChecksum {
		t.Errorf("expected checksum %q, got %q", expectedChecksum, p.Checksum)
	}
}

func TestNewPaste_DifferentContent(t *testing.T) {
	p1 := NewPaste("content1")
	p2 := NewPaste("content2")

	if p1.Checksum == p2.Checksum {
		t.Error("different content should produce different checksums")
	}
}

func TestNewPaste_SameContent(t *testing.T) {
	p1 := NewPaste("same content")
	p2 := NewPaste("same content")

	if p1.Checksum != p2.Checksum {
		t.Error("same content should produce same checksum")
	}
}

func TestNewMeta(t *testing.T) {
	checksum := "abc123"
	size := int64(100)
	ttl := time.Hour

	meta := NewMeta(checksum, size, ttl)

	if meta.Checksum != checksum {
		t.Errorf("expected checksum %q, got %q", checksum, meta.Checksum)
	}

	if meta.Size != size {
		t.Errorf("expected size %d, got %d", size, meta.Size)
	}

	if meta.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	if meta.ExpiresAt.IsZero() {
		t.Error("expected ExpiresAt to be set")
	}

	// ExpiresAt should be approximately CreatedAt + TTL
	expectedExpiry := meta.CreatedAt.Add(ttl)
	if !meta.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("expected ExpiresAt %v, got %v", expectedExpiry, meta.ExpiresAt)
	}
}

func TestMeta_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		wait     time.Duration
		expected bool
	}{
		{
			name:     "not expired",
			ttl:      time.Hour,
			wait:     0,
			expected: false,
		},
		{
			name:     "expired",
			ttl:      -time.Hour, // Already in the past
			wait:     0,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := NewMeta("test", 100, tt.ttl)
			time.Sleep(tt.wait)

			if got := meta.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTTLOptions(t *testing.T) {
	options := TTLOptions(24 * time.Hour)

	if len(options) == 0 {
		t.Error("expected TTL options to be non-empty")
	}

	for _, opt := range options {
		if opt.Label == "" {
			t.Error("TTL option label should not be empty")
		}
		if opt.Duration <= 0 {
			t.Errorf("TTL option %q has invalid duration %v", opt.Label, opt.Duration)
		}
	}
}

func TestTTLOptions_DefaultMarked(t *testing.T) {
	defaultTTL := 365 * 24 * time.Hour
	options := TTLOptions(defaultTTL)

	foundDefault := false
	for _, opt := range options {
		if opt.Duration == defaultTTL {
			if !opt.IsDefault {
				t.Error("expected 1 year option to be marked as default")
			}
			foundDefault = true
		} else {
			if opt.IsDefault {
				t.Errorf("option %q should not be marked as default", opt.Label)
			}
		}
	}

	if !foundDefault {
		t.Error("expected to find 1 year option")
	}
}

func TestComputeChecksum(t *testing.T) {
	tests := []struct {
		content  string
		expected string
	}{
		{
			content:  "Hello, World!",
			expected: "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f",
		},
		{
			content:  "",
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			content:  "test",
			expected: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			got := ComputeChecksum(tt.content)
			if got != tt.expected {
				t.Errorf("ComputeChecksum(%q) = %q, want %q", tt.content, got, tt.expected)
			}
		})
	}
}

func TestComputeChecksum_Consistency(t *testing.T) {
	content := "same content multiple times"
	checksum1 := ComputeChecksum(content)
	checksum2 := ComputeChecksum(content)

	if checksum1 != checksum2 {
		t.Error("ComputeChecksum should return consistent results for same input")
	}
}
