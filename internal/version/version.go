// Package version reports the build identity of the running binary: the
// release it was cut from and the commit it was built at.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Version is set at build time with
// -ldflags "-X github.com/espebra/pastebin/internal/version.Version=v1.0.0".
// When it is empty the value is recovered from the embedded build info.
var Version = ""

// Info describes the running build.
type Info struct {
	Version  string // release tag, or "dev" for an untagged build
	Commit   string // short commit hash, or "unknown"
	Modified string // "-dirty" when built from a dirty working tree
}

// Get returns the build identity of the running binary.
func Get() Info {
	info := Info{Version: Version, Commit: "unknown"}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		if info.Version == "" {
			info.Version = "dev"
		}
		return info
	}

	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
			if len(info.Commit) > shortCommitLength {
				info.Commit = info.Commit[:shortCommitLength]
			}
		case "vcs.modified":
			if setting.Value == "true" {
				info.Modified = "-dirty"
			}
		}
	}

	if info.Version == "" {
		info.Version = moduleVersion(bi.Main.Version)
	}

	return info
}

// shortCommitLength is how much of the commit hash is shown.
const shortCommitLength = 7

// moduleVersion interprets the module version recorded in the build info,
// which is only meaningful when the binary was built from a tagged release.
func moduleVersion(v string) string {
	// A pseudo-version such as v0.0.0-20060102150405-abcdef123456 carries no
	// release information, so it reads as a development build.
	isPseudo := strings.HasPrefix(v, "v0.0.0-") || strings.Contains(v, "-0.")
	if v == "" || v == "(devel)" || isPseudo {
		return "dev"
	}
	return v
}

// Commitish returns the commit with its dirty marker, e.g. "abc1234-dirty".
func (i Info) Commitish() string {
	return i.Commit + i.Modified
}

// String renders the build identity, e.g. "v1.0.0 (commit: abc1234)".
func (i Info) String() string {
	return fmt.Sprintf("%s (commit: %s)", i.Version, i.Commitish())
}
