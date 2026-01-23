package paste

import (
	"testing"
)

func FuzzComputeChecksum(f *testing.F) {
	// Add seed corpus
	f.Add("")
	f.Add("hello world")
	f.Add("test paste content")
	f.Add("\x00\x01\x02\x03") // binary data
	f.Add("unicode: 日本語 emoji: 🎉")
	f.Add(string(make([]byte, 10000))) // large input

	f.Fuzz(func(t *testing.T, content string) {
		checksum := ComputeChecksum(content)

		// Verify checksum is always 64 hex characters (SHA256)
		if len(checksum) != 64 {
			t.Errorf("expected checksum length 64, got %d", len(checksum))
		}

		// Verify checksum is deterministic
		checksum2 := ComputeChecksum(content)
		if checksum != checksum2 {
			t.Errorf("checksum not deterministic: %s != %s", checksum, checksum2)
		}
	})
}

func FuzzNewPaste(f *testing.F) {
	f.Add("")
	f.Add("hello world")
	f.Add("test paste content with special chars: <script>alert('xss')</script>")
	f.Add("\x00\x01\x02\x03")

	f.Fuzz(func(t *testing.T, content string) {
		p := NewPaste(content)

		if p == nil {
			t.Fatal("NewPaste returned nil")
		}

		if p.Content != content {
			t.Errorf("content mismatch")
		}

		if p.Checksum != ComputeChecksum(content) {
			t.Errorf("checksum mismatch")
		}
	})
}
