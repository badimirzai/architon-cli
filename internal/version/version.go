package version

import (
	"runtime/debug"
	"strings"
)

// Version is set at link time by release builds that have no VCS metadata,
// such as the container image:
//
//	-ldflags "-X github.com/badimirzai/architon-cli/internal/version.Version=v0.15.0"
//
// When set, it takes precedence over the module version from build info.
var Version string

// Info describes the current build version metadata.
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Get returns version info derived from Go build metadata when available.
func Get() Info {
	info := Info{Version: "v0.7.0-dev"}
	buildInfo, ok := debug.ReadBuildInfo()
	if ok && buildInfo != nil {
		moduleVersion := strings.TrimSpace(buildInfo.Main.Version)
		modified := false
		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.GitCommit = setting.Value
			case "vcs.time":
				info.BuildDate = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
		if moduleVersion != "" && moduleVersion != "(devel)" && !modified && !strings.Contains(moduleVersion, "+dirty") {
			info.Version = moduleVersion
		}
	}
	if linked := strings.TrimSpace(Version); linked != "" {
		info.Version = linked
	}
	return info
}

// Line returns the formatted version string for "rv version".
func Line() string {
	info := Get()
	if info.GitCommit == "" || info.BuildDate == "" {
		return "rv version: " + info.Version
	}
	shortCommit := info.GitCommit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}
	return "rv version: " + info.Version + " (" + shortCommit + ", " + info.BuildDate + ")"
}
