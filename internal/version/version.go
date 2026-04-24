// Package version exposes build-time identity information.
// Values are set via -ldflags at build time; defaults are reasonable for
// development builds run directly from source.
package version

import "runtime/debug"

var (
	// Version is the semver string of this build, e.g. "0.1.0".
	Version = "0.0.0-dev"
	// Commit is the git SHA the binary was built from.
	Commit = "unknown"
	// BuildTime is the UTC RFC3339 timestamp set at build time.
	BuildTime = "unknown"
)

// Info captures the build identity for logs, /metrics, and the dashboard.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// Get returns the current build identity. When ldflags are not supplied it
// falls back to go-build-info so development builds still carry a module hash.
func Get() Info {
	commit := Commit
	if commit == "unknown" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && s.Value != "" {
					commit = s.Value
				}
			}
		}
	}
	goVer := "unknown"
	if bi, ok := debug.ReadBuildInfo(); ok {
		goVer = bi.GoVersion
	}
	return Info{
		Version:   Version,
		Commit:    commit,
		BuildTime: BuildTime,
		GoVersion: goVer,
	}
}
