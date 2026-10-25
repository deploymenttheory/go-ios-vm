package daemon

import (
	"context"
	"debug/macho"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
	"github.com/deploymenttheory/go-ios-vm/internal/guest/protocol"
	"github.com/deploymenttheory/go-ios-vm/internal/testenv"
)

var iphone = guest.DeviceInfo{ProductType: "iPhone17,3", Version: "27.0.1", Build: "24A446"}

func TestPingReportsActualReadiness(t *testing.T) {
	ok := New(Platform{Identity: func() (guest.DeviceInfo, error) { return iphone, nil }}, "test")
	res, err := ok.Ping(context.Background())
	if err != nil || !res.Ready || res.Identity != iphone || res.Daemon.PID != os.Getpid() || res.Daemon.Version != "test" {
		t.Fatalf("ready daemon: %+v, %v", res, err)
	}
	for name, identity := range map[string]func() (guest.DeviceInfo, error){
		"unreadable": func() (guest.DeviceInfo, error) { return guest.DeviceInfo{}, errors.New("no SystemVersion.plist") },
		"incomplete": func() (guest.DeviceInfo, error) { return guest.DeviceInfo{ProductType: "iPhone17,3"}, nil },
	} {
		res, err := New(Platform{Identity: identity}, "test").Ping(context.Background())
		if err != nil || res.Ready || len(res.Reasons) == 0 {
			t.Errorf("%s identity: %+v, %v; want not ready with a reason", name, res, err)
		}
	}
}

func TestDeviceInfo(t *testing.T) {
	got, err := New(Platform{Identity: func() (guest.DeviceInfo, error) { return iphone, nil }}, "t").DeviceInfo(context.Background())
	if err != nil || got != iphone {
		t.Fatalf("DeviceInfo = %+v, %v", got, err)
	}
	_, err = New(Platform{Identity: func() (guest.DeviceInfo, error) { return guest.DeviceInfo{}, errors.New("x") }}, "t").DeviceInfo(context.Background())
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Code != protocol.CodeOperationFailed {
		t.Fatalf("DeviceInfo failure = %v", err)
	}
}

func TestShutdownPreconditionsAndHaltFailure(t *testing.T) {
	halts := 0
	var logged []string
	p := Platform{
		IsRoot: func() bool { return false },
		Halt:   func() error { halts++; return errors.New("EPERM") },
		Logf:   func(f string, a ...any) { logged = append(logged, f) },
	}
	s := New(p, "t")
	for name, params := range map[string]protocol.ShutdownParams{"without force": {}, "not root": {Force: true}} {
		_, after, err := s.Shutdown(context.Background(), params)
		var pe *protocol.Error
		if !errors.As(err, &pe) || after != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	p.IsRoot = func() bool { return true }
	res, after, err := New(p, "t").Shutdown(context.Background(), protocol.ShutdownParams{Force: true})
	if err != nil || !res.Accepted || after == nil || halts != 0 {
		t.Fatalf("shutdown: %+v %v halts=%d", res, err, halts)
	}
	after()
	if halts != 1 || len(logged) != 1 {
		t.Fatalf("halt failure after acknowledgment: halts=%d logged=%v", halts, logged)
	}
	p.Halt = nil
	if _, _, err := New(p, "t").Shutdown(context.Background(), protocol.ShutdownParams{Force: true}); err == nil {
		t.Fatal("shutdown without a halt implementation was accepted")
	}
}

func TestHaltIsUnavailableOutsideTheIOSBuild(t *testing.T) {
	if runtime.GOOS == "ios" {
		t.Skip("the iOS build halts for real")
	}
	if err := Halt(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Halt = %v, want ErrUnsupported", err)
	}
}

func TestReadIdentityOnThisHost(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("reads Darwin's SystemVersion.plist")
	}
	id, err := ReadIdentity()
	if err != nil || id.ProductType == "" || id.Version == "" || id.Build == "" {
		t.Fatalf("ReadIdentity = %+v, %v", id, err)
	}
	t.Logf("host identity read by the guest code path: %+v", id)
}

// TestNativeGuestCrossBuild builds the daemon for iOS with external linking and
// records its Mach-O platform, architecture and linked libraries.
func TestNativeGuestCrossBuild(t *testing.T) {
	testenv.Require(t, testenv.Native, testenv.AppleSilicon(27), testenv.Prerequisite{
		Name: "iPhoneOS SDK",
		Check: func() error {
			return exec.CommandContext(t.Context(), "xcrun", "--sdk", "iphoneos", "--show-sdk-path").Run()
		},
	})
	bin := filepath.Join(t.TempDir(), "go-ios-vm-guestd")
	build := exec.CommandContext(t.Context(), "make", "-C", "../../..", "guest", "GUEST_BIN="+bin)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("make guest: %v\n%s", err, out)
	}
	f, err := macho.Open(bin)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Cpu != macho.CpuArm64 || f.Type != macho.TypeExec {
		t.Fatalf("Mach-O %v %v, want an arm64 executable", f.Cpu, f.Type)
	}
	const lcBuildVersion, platformIOS = 0x32, 2
	platform := uint32(0)
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) >= 12 && f.ByteOrder.Uint32(raw[0:4]) == lcBuildVersion {
			platform = binary.LittleEndian.Uint32(raw[8:12])
		}
	}
	if platform != platformIOS {
		t.Fatalf("LC_BUILD_VERSION platform %d, want iOS (%d)", platform, platformIOS)
	}
	libs, err := f.ImportedLibraries()
	if err != nil {
		t.Fatal(err)
	}
	allowed := []string{
		"/usr/lib/libSystem.B.dylib",
		"/usr/lib/libresolv.9.dylib",
		"/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation",
		"/System/Library/Frameworks/Security.framework/Security",
	}
	for _, lib := range libs {
		if !slices.Contains(allowed, lib) {
			t.Errorf("guest links %s, outside the Apple system libraries", lib)
		}
	}
	t.Logf("arm64 iOS executable linking %s", strings.Join(libs, ", "))
}
