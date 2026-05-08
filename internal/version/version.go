package version

import "runtime/debug"

// Info describes the current build version metadata.
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Get returns version info derived from Go build metadata when available.
func Get() Info {
	info := Info{Version: "v0.5.0-dev"}
	buildInfo, ok := debug.ReadBuildInfo()
	if ok && buildInfo != nil {
		if buildInfo.Main.Version != "" && buildInfo.Main.Version != "(devel)" {
			info.Version = buildInfo.Main.Version
		}
		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				info.GitCommit = setting.Value
			case "vcs.time":
				info.BuildDate = setting.Value
			}
		}
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
