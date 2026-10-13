package requestid

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	a, b := New(), New()
	if len(a) != 32 {
		t.Errorf("expected 32 character ID, got %q", a)
	}
	if a == b {
		t.Error("expected unique IDs")
	}
	if !Valid(a) {
		t.Errorf("expected generated ID %q to be valid", a)
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"abc-123_DEF.4", true},
		{"", false},
		{strings.Repeat("a", maxLength), true},
		{strings.Repeat("a", maxLength+1), false},
		{"abc def", false},
		{"abc\ninjected", false},
		{"abc\"", false},
	}
	for _, tt := range tests {
		if got := Valid(tt.id); got != tt.valid {
			t.Errorf("Valid(%q) = %v, want %v", tt.id, got, tt.valid)
		}
	}
}

func TestFromContext(t *testing.T) {
	if id := FromContext(context.Background()); id != "" {
		t.Errorf("expected empty ID, got %q", id)
	}
	ctx := WithID(context.Background(), "abc")
	if id := FromContext(ctx); id != "abc" {
		t.Errorf("expected abc, got %q", id)
	}
}

func TestLogHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewLogHandler(slog.NewJSONHandler(&buf, nil))).With("component", "test")

	logger.InfoContext(WithID(context.Background(), "abc"), "with id")
	logger.InfoContext(context.Background(), "without id")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	var first, second map[string]any
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first["request_id"] != "abc" {
		t.Errorf("expected request_id abc, got %v", first["request_id"])
	}
	if first["component"] != "test" {
		t.Errorf("expected attributes from With to be kept, got %v", first["component"])
	}
	if _, ok := second["request_id"]; ok {
		t.Error("expected no request_id without one in context")
	}
}
