package webui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

// TestBrowserDeepLinkRetainsExactEvidence is opt-in because CI hosts do not
// install a browser. Point GOIOSVM_BROWSER at Chromium or Chrome for Testing.
func TestBrowserDeepLinkRetainsExactEvidence(t *testing.T) {
	browser := os.Getenv("GOIOSVM_BROWSER")
	if browser == "" {
		t.Skip("set GOIOSVM_BROWSER to run the browser smoke test")
	}
	store := resultTestStore(t)
	ctx := context.Background()
	cachePath := filepath.Join(t.TempDir(), "removed.ipsw")
	archive := []byte("browser fixture archive")
	if err := os.WriteFile(cachePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := model.Artifact{
		ID: model.Digest(archive), SHA256: model.Digest(archive),
		Size: int64(len(archive)), Path: cachePath,
	}
	target := model.Target{
		ID: model.ID(artifact.ID, "identity-1"), ArtifactID: artifact.ID,
		Identity: 1, Products: []string{"iPhone12,1"}, Board: "n104ap",
		Version: "14.0", Build: "18A373",
		Components: map[string]string{"DeviceTree": "tree"},
	}
	otherTarget := target
	otherTarget.ID = model.ID(artifact.ID, "identity-0")
	otherTarget.Identity = 0
	if err := store.Register(ctx, artifact, []model.Target{target, otherTarget}); err != nil {
		t.Fatal(err)
	}
	evidence := []byte("source <script>alert(1)</script> survives cache deletion")
	observation, err := model.NewObservation(
		target, "device", "register",
		map[string]any{
			"counter":   uint64(18446744073709551615),
			"long_text": strings.Repeat("x", 2048) + "<script>unsafe()</script>",
		},
		model.Location{
			ArtifactID: artifact.ID, Member: "tree", Coordinate: "plist-selector",
			Selector: "/device/value",
		},
		model.NewBlob(evidence), "fixture/browser-v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	zero, err := model.NewObservation(
		target, "device", "register", uint64(0),
		model.Location{
			ArtifactID: artifact.ID, Member: "tree", Coordinate: "plist-selector",
			Selector: "/device/zero",
		},
		model.NewBlob([]byte("zero reading")), "fixture/browser-v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	absent, err := model.NewObservationWithPresence(
		target, "device", "register", "absent", nil,
		model.Location{
			ArtifactID: artifact.ID, Member: "tree", Coordinate: "plist-selector",
			Selector: "/device/absent",
		},
		model.NewBlob([]byte("absence reading")), "fixture/browser-v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	run := model.Run{
		ID: model.ID("browser-run", target.ID), TargetID: target.ID,
		State: "partial", Started: time.Now().UTC(),
		Coverage: []model.Coverage{
			{Component: "DeviceTree", State: "complete"},
			{Component: "filesystem-content", State: "unexamined", Reason: "unsupported codec"},
		},
	}
	if err := store.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(ctx, run, []model.Observation{observation, zero, absent}); err != nil {
		t.Fatal(err)
	}
	failedRun := model.Run{
		ID: model.ID("failed-browser-run", target.ID), TargetID: target.ID,
		Started: time.Now().UTC().Add(time.Second),
	}
	if err := store.Start(ctx, failedRun); err != nil {
		t.Fatal(err)
	}
	if err := store.Fail(ctx, failedRun.ID, "failed"); err != nil {
		t.Fatal(err)
	}
	otherObservation, err := model.NewObservation(
		otherTarget, "device", "register", "other-target-only",
		model.Location{ArtifactID: artifact.ID, Member: "tree", Coordinate: "plist-selector", Selector: "/device/value"},
		model.NewBlob([]byte("other target evidence")), "fixture/browser-v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	otherRun := model.Run{
		ID: model.ID("other-browser-run", otherTarget.ID), TargetID: otherTarget.ID,
		State: "complete", Started: time.Now().UTC(),
		Coverage: []model.Coverage{{Component: "DeviceTree", State: "complete"}},
	}
	if err := store.Start(ctx, otherRun); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(ctx, otherRun, []model.Observation{otherObservation}); err != nil {
		t.Fatal(err)
	}
	findings, err := store.FindingChains(ctx, target.ID, "")
	if err != nil || len(findings) != 1 {
		t.Fatalf("seed finding: %v, count=%d", err, len(findings))
	}
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}
	serverHandler := NewReadHandler(store)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/assets/app.js" {
			data, readErr := assets.ReadFile("assets/app.js")
			if readErr != nil {
				http.Error(w, "asset unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(data)
			_, _ = w.Write([]byte(`
setInterval(() => {
  document.documentElement.dataset.viewportWidth = document.documentElement.clientWidth;
  document.documentElement.dataset.scrollWidth = document.documentElement.scrollWidth;
  document.documentElement.dataset.activeId = document.activeElement && document.activeElement.id || "";
}, 100);
`))
			return
		}
		serverHandler.ServeHTTP(w, r)
	}))
	defer server.Close()
	query := url.Values{
		"product": {"iPhone12,1"}, "build": {"18A373"},
		"target": {target.ID}, "run": {run.ID}, "view": {"findings"},
		"subject": {"device"}, "property": {"register"},
		"component": {"DeviceTree"}, "finding": {findings[0].ID},
		"evidence": {observation.EvidenceID},
	}
	browserCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pageURL := server.URL + "/?" + query.Encode()
	command := exec.CommandContext(browserCtx, browser,
		"--headless", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--disable-background-networking", "--no-first-run", "--no-default-browser-check",
		"--user-data-dir="+t.TempDir(), "--virtual-time-budget=10000", "--window-size=390,900", "--dump-dom",
		pageURL,
	)
	output, err := command.Output()
	// Chrome for Testing on macOS can emit the complete DOM and then leave a
	// background process alive. The deadline bounds that process; the DOM is
	// still checked below. Any other early exit remains a test failure.
	if err != nil && (browserCtx.Err() != context.DeadlineExceeded ||
		!strings.Contains(string(output), "</html>")) {
		t.Fatalf("browser: %v; DOM tail: %s", err, tail(string(output), 2000))
	}
	dom := string(output)
	viewport := regexp.MustCompile(`data-viewport-width="(\d+)"`).FindStringSubmatch(dom)
	scroll := regexp.MustCompile(`data-scroll-width="(\d+)"`).FindStringSubmatch(dom)
	if len(viewport) != 2 || len(scroll) != 2 {
		t.Fatalf("browser did not report mobile layout metrics")
	}
	viewportWidth, _ := strconv.Atoi(viewport[1])
	scrollWidth, _ := strconv.Atoi(scroll[1])
	if viewportWidth > 540 || scrollWidth > viewportWidth {
		t.Fatalf("narrow layout overflow: viewport=%d scroll=%d", viewportWidth, scrollWidth)
	}
	if !strings.Contains(dom, `data-active-id="detail-title"`) {
		t.Fatalf("deep link did not focus the result detail")
	}
	for _, want := range []string{
		"iPhone12,1 · 18A373", "Identity", "DeviceTree", "Committed checkpoints",
		"device · register", "Basis: observed", "Assessment: unreviewed",
		"18446744073709551615", "/device/value", "Digest verified",
		"Absent: no value was recorded.", "<pre class=\"value mono\">0</pre>",
		"unsupported codec", "data-tone=\"bad\">failed</span>",
		"source &lt;script&gt;alert(1)&lt;/script&gt; survives cache deletion",
		strings.Repeat("x", 128),
	} {
		if !strings.Contains(dom, want) {
			t.Fatalf("browser deep link missing %q; DOM tail: %s", want, tail(dom, 2000))
		}
	}
	if strings.Contains(dom, "<script>alert(1)</script>") ||
		strings.Contains(dom, "<script>unsafe()</script>") ||
		strings.Contains(dom, "other-target-only") ||
		strings.Contains(dom, cachePath) {
		t.Fatalf("unsafe text or removed cache path appeared in DOM")
	}
	if screenshotDir := os.Getenv("GOIOSVM_BROWSER_SCREENSHOT_DIR"); screenshotDir != "" {
		if err := os.MkdirAll(screenshotDir, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, capture := range []struct{ name, size string }{
			{"desktop.png", "1360,900"}, {"mobile.png", "500,900"},
		} {
			path := filepath.Join(screenshotDir, capture.name)
			captureCtx, stop := context.WithTimeout(ctx, 15*time.Second)
			cmd := exec.CommandContext(captureCtx, browser,
				"--headless", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
				"--disable-background-networking", "--no-first-run", "--no-default-browser-check",
				"--user-data-dir="+t.TempDir(), "--virtual-time-budget=10000",
				"--window-size="+capture.size, "--screenshot="+path, pageURL,
			)
			output, captureErr := cmd.CombinedOutput()
			stop()
			info, statErr := os.Stat(path)
			if statErr != nil || info.Size() == 0 {
				t.Fatalf("capture %s: %v: %s", capture.name, captureErr, tail(string(output), 600))
			}
		}
	}
}

func tail(value string, n int) string {
	if len(value) <= n {
		return value
	}
	return value[len(value)-n:]
}
