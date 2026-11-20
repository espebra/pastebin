package requestid

import (
	"strings"
	"testing"
)

// FuzzValid checks that client supplied request IDs are only accepted when
// they are safe to echo in a response header and write to the logs
func FuzzValid(f *testing.F) {
	f.Add("")
	f.Add("abc-123_DEF.456")
	f.Add(New())
	f.Add(strings.Repeat("a", maxLength))
	f.Add(strings.Repeat("a", maxLength+1))
	f.Add("id\r\nSet-Cookie: x=y")
	f.Add("id with spaces")
	f.Add("id\x00")
	f.Add("idé")
	f.Add(`id" level=ERROR msg="forged`)

	f.Fuzz(func(t *testing.T, id string) {
		// An independent statement of the rule in Valid
		want := len(id) > 0 && len(id) <= maxLength
		for i := 0; i < len(id) && want; i++ {
			c := id[i]
			want = c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
				c == '-' || c == '_' || c == '.'
		}
		if got := Valid(id); got != want {
			t.Errorf("Valid(%q) = %v, want %v", id, got, want)
		}
	})
}
