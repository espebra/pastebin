package version

import "testing"

func TestModuleVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"release tag", "v1.2.3", "v1.2.3"},
		{"empty", "", "dev"},
		{"devel placeholder", "(devel)", "dev"},
		{"pseudo-version", "v0.0.0-20060102150405-abcdef123456", "dev"},
		{"pseudo-version after tag", "v1.2.3-0.20060102150405-abcdef123456", "dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := moduleVersion(tt.input); got != tt.want {
				t.Errorf("moduleVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInfoCommitish(t *testing.T) {
	clean := Info{Commit: "abc1234"}
	if got := clean.Commitish(); got != "abc1234" {
		t.Errorf("expected %q, got %q", "abc1234", got)
	}

	dirty := Info{Commit: "abc1234", Modified: "-dirty"}
	if got := dirty.Commitish(); got != "abc1234-dirty" {
		t.Errorf("expected %q, got %q", "abc1234-dirty", got)
	}
}

func TestInfoString(t *testing.T) {
	info := Info{Version: "v1.0.0", Commit: "abc1234"}
	if got := info.String(); got != "v1.0.0 (commit: abc1234)" {
		t.Errorf("unexpected rendering: %q", got)
	}
}

func TestGetShortensCommit(t *testing.T) {
	// Under `go test` the build info is present, so Get must always return a
	// commit no longer than the short form and a non-empty version.
	info := Get()

	if len(info.Commit) > shortCommitLength {
		t.Errorf("commit %q is longer than %d characters", info.Commit, shortCommitLength)
	}

	if info.Version == "" {
		t.Error("expected a non-empty version")
	}
}
