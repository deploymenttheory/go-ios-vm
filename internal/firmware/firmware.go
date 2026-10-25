// Package firmware defines the firmware builds the runner accepts and the
// contract for acquiring verified archives into the IPSW cache.
package firmware

import (
	"context"
	"path/filepath"

	"github.com/deploymenttheory/go-ios-vm/internal/progress"
)

// Identity names one firmware build exactly. Selection always matches the
// build; a marketing version alone never selects an archive.
type Identity struct {
	// OS is the operating system family, for example "iOS" or "cloudOS".
	OS string `json:"os"`
	// Device is the product type the build is selected for. It is empty when
	// the device is resolved from the archive's manifest.
	Device  string `json:"device,omitempty"`
	Version string `json:"version"`
	Build   string `json:"build"`
}

func (id Identity) String() string {
	s := id.OS + " " + id.Version + " (" + id.Build + ")"
	if id.Device != "" {
		s = id.Device + " " + s
	}
	return s
}

// Pair is an iPhone build and the cloudOS virtual-platform build restored
// together into one machine.
type Pair struct {
	IPhone  Identity `json:"iphone"`
	CloudOS Identity `json:"cloudos"`
}

// SupportedPair is the only pairing this release accepts. Other builds are
// rejected rather than substituted.
var SupportedPair = Pair{
	IPhone:  Identity{OS: "iOS", Device: "iPhone17,3", Version: "27.0.1", Build: "24A446"},
	CloudOS: Identity{OS: "cloudOS", Version: "26.4", Build: "23E5207q"},
}

// DefaultCacheDir returns the IPSW cache directory under the user's cache
// directory, as returned by os.UserCacheDir.
func DefaultCacheDir(userCacheDir string) string {
	return filepath.Join(userCacheDir, "go-ios-vm", "ipsw")
}

// Archive is a verified firmware archive in the cache.
type Archive struct {
	Identity Identity
	Path     string
	SHA256   string
}

// Resolver resolves a pair to verified archives, downloading into the cache
// when needed. An archive is returned only after its identity and digest are
// verified; existing cache entries for other builds are never modified.
type Resolver interface {
	Resolve(ctx context.Context, pair Pair, report progress.Reporter) (iphone, cloudOS Archive, err error)
}
