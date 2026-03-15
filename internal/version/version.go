// Package version provides build version information.
package version

import (
	"runtime/debug"
)

// Info holds version metadata embedded at build time.
type Info struct {
	Version    string
	GoVersion  string
	Commit     string
	CommitTime string
	Modified   bool
}

// Get returns the current build version information.
func Get() Info {
	info := Info{
		Version:   "(unknown)",
		GoVersion: "(unknown)",
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}

	info.GoVersion = bi.GoVersion
	if bi.Main.Version != "" {
		info.Version = bi.Main.Version
	}

	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Commit = s.Value
		case "vcs.time":
			info.CommitTime = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}

	return info
}
