package csrf

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Token should be 64 characters (32 bytes hex encoded)
	if len(token) != 64 {
		t.Errorf("expected token length 64, got %d", len(token))
	}

	// Tokens should be unique
	token2, err := GenerateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token == token2 {
		t.Error("tokens should be unique")
	}
}

func TestSetCookie(t *testing.T) {
	w := httptest.NewRecorder()
	token := "test-token-value"

	SetCookie(w, token, false)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != cookieName {
		t.Errorf("expected cookie name %q, got %q", cookieName, cookie.Name)
	}

	if cookie.Value != token {
		t.Errorf("expected cookie value %q, got %q", token, cookie.Value)
	}

	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}

	if cookie.SameSite != http.SameSiteStrictMode {
		t.Error("cookie should have SameSite=Strict")
	}

	if cookie.Secure {
		t.Error("cookie should not be Secure when secure=false")
	}
}

func TestSetCookie_Secure(t *testing.T) {
	w := httptest.NewRecorder()
	token := "test-token-value"

	SetCookie(w, token, true)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if !cookie.Secure {
		t.Error("cookie should be Secure when secure=true")
	}
}

func TestGetTokenFromCookie(t *testing.T) {
	token := "test-token-from-cookie"
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  cookieName,
		Value: token,
	})

	got := GetTokenFromCookie(req)
	if got != token {
		t.Errorf("expected %q, got %q", token, got)
	}
}

func TestGetTokenFromCookie_NoCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	got := GetTokenFromCookie(req)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetTokenFromForm(t *testing.T) {
	token := "test-token-from-form"
	form := url.Values{}
	form.Set(formField, token)

	req := httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ParseForm()

	got := GetTokenFromForm(req)
	if got != token {
		t.Errorf("expected %q, got %q", token, got)
	}
}

func TestValidate_Success(t *testing.T) {
	token := "matching-token-value"
	form := url.Values{}
	form.Set(formField, token)

	req := httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  cookieName,
		Value: token,
	})
	req.ParseForm()

	if !Validate(req) {
		t.Error("expected validation to pass")
	}
}

func TestValidate_MismatchedTokens(t *testing.T) {
	form := url.Values{}
	form.Set(formField, "form-token")

	req := httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  cookieName,
		Value: "cookie-token",
	})
	req.ParseForm()

	if Validate(req) {
		t.Error("expected validation to fail with mismatched tokens")
	}
}

func TestValidate_MissingCookie(t *testing.T) {
	form := url.Values{}
	form.Set(formField, "form-token")

	req := httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ParseForm()

	if Validate(req) {
		t.Error("expected validation to fail with missing cookie")
	}
}

func TestValidate_MissingFormToken(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  cookieName,
		Value: "cookie-token",
	})

	if Validate(req) {
		t.Error("expected validation to fail with missing form token")
	}
}

func TestFormField(t *testing.T) {
	if FormField() != formField {
		t.Errorf("expected %q, got %q", formField, FormField())
	}
}
