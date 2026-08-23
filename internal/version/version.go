package version

import "strings"

var (
	Version = "1.0.2-dev"
	Commit  = "unknown"
	BuiltAt = "unknown"
)

type Info struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	BuiltAt     string `json:"builtAt"`
	Development bool   `json:"development"`
}

func Current() Info {
	trimmed := strings.TrimSpace(Version)
	development := trimmed == "" || trimmed == "unknown" || strings.Contains(trimmed, "-") || Commit == "unknown" || BuiltAt == "unknown"
	return Info{Version: trimmed, Commit: Commit, BuiltAt: BuiltAt, Development: development}
}
