package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	tokenLength = 32
	cookieName  = "csrf_token"
	formField   = "csrf_token"
)

// GenerateToken creates a new random CSRF token
func GenerateToken() (string, error) {
	bytes := make([]byte, tokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SetCookie sets the CSRF token cookie on the response
func SetCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
		MaxAge:   int(24 * time.Hour / time.Second),
	})
}

// GetTokenFromCookie retrieves the CSRF token from the request cookie
func GetTokenFromCookie(r *http.Request) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// GetTokenFromForm retrieves the CSRF token from the form data
func GetTokenFromForm(r *http.Request) string {
	return r.FormValue(formField)
}

// Validate checks if the form token matches the cookie token
func Validate(r *http.Request) bool {
	cookieToken := GetTokenFromCookie(r)
	formToken := GetTokenFromForm(r)

	if cookieToken == "" || formToken == "" {
		return false
	}

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(formToken)) == 1
}

// FormField returns the name of the form field for the CSRF token
func FormField() string {
	return formField
}
