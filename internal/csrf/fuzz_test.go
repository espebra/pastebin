package csrf

import (
	"crypto/subtle"
	"testing"
)

func FuzzTokenComparison(f *testing.F) {
	// Add seed corpus with various token patterns
	f.Add("", "")
	f.Add("abc", "abc")
	f.Add("abc", "def")
	f.Add("a]b[c", "a]b[c") // special chars
	f.Add("token1", "token2")
	f.Add("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	f.Fuzz(func(t *testing.T, token1, token2 string) {
		// Test that constant-time comparison doesn't panic
		result := subtle.ConstantTimeCompare([]byte(token1), []byte(token2))

		// Verify result is consistent with equality
		if token1 == token2 && result != 1 {
			t.Errorf("equal tokens should return 1, got %d", result)
		}
		if token1 != token2 && result != 0 {
			t.Errorf("unequal tokens should return 0, got %d", result)
		}
	})
}
