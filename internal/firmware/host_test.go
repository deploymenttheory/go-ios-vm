package firmware

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/progress"
	"github.com/deploymenttheory/go-ios-vm/internal/testenv"
)

// hostCacheDir is the real IPSW cache, or GO_IOS_VM_IPSW_CACHE when set.
func hostCacheDir(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("GO_IOS_VM_IPSW_CACHE"); dir != "" {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return DefaultCacheDir(base)
}

// TestNativeExistingCacheEntriesAreRejected inspects the real cache read-only:
// archives this cache did not publish never match the supported pair and are
// left exactly as found.
func TestNativeExistingCacheEntriesAreRejected(t *testing.T) {
	testenv.Require(t, testenv.Native, testenv.AppleSilicon(27))
	c := &Cache{Dir: hostCacheDir(t)}
	entries, err := c.Entries()
	if err != nil {
		t.Fatal(err)
	}
	inspected := 0
	for _, e := range entries {
		if e.Record != nil {
			continue
		}
		path := filepath.Join(c.Dir, e.Name)
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		m, err := ReadManifest(f, before.Size())
		_ = f.Close()
		if err != nil {
			t.Logf("%s: not inspectable as a restore archive: %v", e.Name, err)
			continue
		}
		inspected++
		t.Logf("%s: %s (%s) for %v, device classes %v", e.Name, m.ProductVersion, m.ProductBuildVersion, m.SupportedProductTypes, m.DeviceClasses())
		for _, id := range []Identity{SupportedPair.IPhone, SupportedPair.CloudOS} {
			if m.Match(id) == nil {
				t.Errorf("%s matches %s without a cache record", e.Name, id)
			}
		}
		after, err := os.Stat(path)
		if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			t.Errorf("%s changed during inspection", e.Name)
		}
	}
	if inspected == 0 {
		t.Skip("no existing cache entries to inspect")
	}
}

// TestLiveAcquireSupportedPair downloads and verifies both archives of the
// supported pair into the real IPSW cache. It needs network access and room
// for about 13.5 GB.
func TestLiveAcquireSupportedPair(t *testing.T) {
	dir := hostCacheDir(t)
	iphone, _ := KnownSource(SupportedPair.IPhone)
	need := iphone.Size + 2<<30 // the cloudOS archive is about 1.2 GB
	testenv.Require(t, testenv.Live, testenv.AppleSilicon(27), testenv.Prerequisite{
		Name: fmt.Sprintf("%.1f GiB free for the iPhone and cloudOS IPSWs in %s", float64(need)/(1<<30), dir),
		Check: func() error {
			free, err := DirFree(dir)
			if err != nil {
				return err
			}
			if free < need {
				return fmt.Errorf("%w: %.1f GiB free", testenv.ErrUnmet, float64(free)/(1<<30))
			}
			return nil
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
	defer cancel()
	c := &Cache{Dir: dir, Free: DirFree}
	last := time.Now()
	report := progress.ReporterFunc(func(e progress.Event) {
		if time.Since(last) > 30*time.Second || e.Done == e.Total {
			last = time.Now()
			t.Logf("%s: %d/%d %s", e.Step, e.Done, e.Total, e.Unit)
		}
	})
	a, b, err := c.Resolve(ctx, SupportedPair, report)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []Archive{a, b} {
		info, err := os.Stat(x.Path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("verified %s: %s, %d bytes, SHA-256 %s", x.Identity, x.Path, info.Size(), x.SHA256)
	}
	if !strings.EqualFold(a.SHA256, iphone.SHA256) {
		t.Fatalf("iPhone archive digest %s, want %s", a.SHA256, iphone.SHA256)
	}
	if _, err := os.Stat(filepath.Join(dir, a.SHA256+".json")); errors.Is(err, os.ErrNotExist) {
		t.Fatal("no record published beside the iPhone archive")
	}
}
