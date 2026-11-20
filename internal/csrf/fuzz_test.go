package csrf

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// FuzzValidate checks that a request is accepted only when the form token
// is non-empty and equal to the token in the cookie, for any cookie header
// and form value
func FuzzValidate(f *testing.F) {
	token := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	f.Add(token, token)
	f.Add(token, token[:63])
	f.Add(token, "")
	f.Add("", "")
	f.Add("abc", "def")
	f.Add(`"quoted"`, "quoted")
	f.Add("a; csrf_token=b", "b")
	f.Add("a\r\nSet-Cookie: x=y", "a")

	f.Fuzz(func(t *testing.T, cookie, form string) {
		body := url.Values{formField: {form}}.Encode()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Cookie", cookieName+"="+cookie)

		got := Validate(r)

		// The form value survives URL encoding unchanged, while the cookie
		// is whatever net/http parses out of the header
		parsed := GetTokenFromCookie(r)
		if GetTokenFromForm(r) != form {
			t.Fatalf("form token changed in transit: %q became %q", form, GetTokenFromForm(r))
		}
		want := parsed != "" && form != "" && parsed == form
		if got != want {
			t.Errorf("Validate = %v, want %v (cookie %q parsed as %q, form %q)", got, want, cookie, parsed, form)
		}
	})
}
