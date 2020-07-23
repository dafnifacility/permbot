package app

// PermbotVersion is used during the build (via -ldflags=-X to set the version)
var PermbotVersion string

func Version() string {
	if PermbotVersion == "" {
		return "DEV:UNRELEASED"
	}
	return PermbotVersion
}
