package handlers

import (
	"testing"
)

func FuzzIsValidChecksum(f *testing.F) {
	// Add seed corpus
	f.Add("")
	f.Add("abc")
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") // valid SHA256
	f.Add("E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855") // uppercase
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85")  // 63 chars
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b8555") // 65 chars
	f.Add("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz") // invalid hex
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b85g") // invalid hex char

	f.Fuzz(func(t *testing.T, input string) {
		// Just verify it doesn't panic
		_ = isValidChecksum(input)
	})
}
