package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	SetBuildInfo("v1.2.3", "abc1234")

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)

	for _, want := range []string{
		`pastebin_build_info{commit="abc1234",version="v1.2.3"} 1`,
		`pastebin_pastes_deleted_total{reason="expired"} 0`,
		`pastebin_cleanup_runs_total{result="success"} 0`,
		"go_goroutines",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("expected metrics output to contain %q", want)
		}
	}
}
