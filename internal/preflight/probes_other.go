//go:build !darwin

package preflight

// DefaultProbes reports the platform only; Run stops at the platform check
// anywhere but darwin/arm64.
func DefaultProbes(goos, goarch string) Probes {
	return Probes{GOOS: goos, GOARCH: goarch}
}
