package inspection

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/testfixture"
)

func TestProbeManifestMatchesExactBuildWithoutWholeImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.ipsw")
	if err := testfixture.IPSW(path, "14.0", 42); err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(image)
	probe, err := ProbeManifest(
		context.Background(),
		reader,
		int64(len(image)),
		"iPhone12,1",
		"14.0",
		"synthetic-14.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Version != "14.0" || probe.Build != "synthetic-14.0" ||
		probe.ManifestSHA256 == "" || len(probe.RawBuildManifest) == 0 || probe.Identities != 2 {
		t.Fatal(probe)
	}
	if len(probe.Products) != 1 || probe.Products[0] != "iPhone12,1" ||
		len(probe.Boards) != 1 || probe.Boards[0] != "n104ap" ||
		len(probe.Variants) != 2 || probe.Variants[0] != "Erase" || probe.Variants[1] != "Update" {
		t.Fatal("manifest identities were not reported exactly", probe)
	}
	if len(probe.BuildIdentities) != 2 ||
		probe.BuildIdentities[0] != (ManifestIdentity{Index: 0, Board: "n104ap", Variant: "Erase"}) ||
		probe.BuildIdentities[1] != (ManifestIdentity{Index: 1, Board: "n104ap", Variant: "Update"}) {
		t.Fatal("board and variant pairing was lost", probe.BuildIdentities)
	}
	if _, err = ProbeManifest(
		context.Background(),
		reader,
		int64(len(image)),
		"iPhone12,1",
		"14.0",
		"18A373",
	); !errors.Is(
		err,
		errManifestTargetMismatch,
	) {
		t.Fatal("wrong build accepted", err)
	}
}
