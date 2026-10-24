package inspection

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestAPFSExtractorMatchesLinkedModule(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Fatal("Go build information unavailable")
	}
	for _, dependency := range info.Deps {
		if dependency.Path != "github.com/deploymenttheory/go-apfs-v3" {
			continue
		}
		if dependency.Version != goAPFSVersion {
			t.Fatalf("APFS extractor records %q but linked module is %q", goAPFSVersion, dependency.Version)
		}
		for _, extractor := range []string{diskExtractor, filesystemExtractor, RunPlanVersion} {
			if !strings.Contains(extractor, "go-apfs-v3="+dependency.Version) {
				t.Fatalf("extractor %q omits linked APFS version %q", extractor, dependency.Version)
			}
		}
		return
	}
	t.Fatal("go-apfs-v3 is absent from linked module dependencies")
}
