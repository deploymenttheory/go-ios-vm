// Package buildinfo reports the running binary's version and the upstream
// source revisions this build is pinned to.
package buildinfo

import "runtime/debug"

// Source pins agreed for this release.
const (
	// VPhoneCLIRevision is the vphone-cli revision the runner is ported from.
	VPhoneCLIRevision = "b2804adb45a653ff3981f0705108975414dc8d5d"
	// MacOSBindingsRevision is the go-bindings-macosplatform revision required
	// in go.mod.
	MacOSBindingsRevision = "d5b3d0ecb48ed106ddac46095bd91ec945b2833c"
)

// version is set at link time with -ldflags "-X <package>.version=v1.2.3".
var version string

// Info describes the running binary.
type Info struct {
	Version   string
	Revision  string
	Modified  bool
	GoVersion string
}

// Read returns the running binary's build information.
func Read() Info {
	info := Info{Version: version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		if info.Version == "" {
			info.Version = "unknown"
		}
		return info
	}
	info.GoVersion = bi.GoVersion
	if info.Version == "" {
		info.Version = bi.Main.Version
	}
	if info.Version == "" || info.Version == "(devel)" {
		info.Version = "devel"
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}
