package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// Run with GOIOSVM_BROWSER pointing at Chrome for Testing or Chromium. The
// injected driver uses native buttons and the same assets served in production.
func TestGraphBrowserInteractions(t *testing.T) {
	browser := os.Getenv("GOIOSVM_BROWSER")
	if browser == "" {
		t.Skip("set GOIOSVM_BROWSER to run graph browser interactions")
	}
	const target = "target-exact-image-1"
	var mu sync.Mutex
	requests := []url.Values{}
	base := NewHandler(func(context.Context) error { return nil })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/assets/graph.js" {
			data, err := assets.ReadFile("assets/graph.js")
			if err != nil {
				http.Error(w, "asset unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(data)
			_, _ = w.Write([]byte(graphBrowserDriver))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			var value any
			switch r.URL.Path {
			case "/api/v1/health":
				value = map[string]any{}
			case "/api/v1/targets":
				value = map[string]any{"items": []any{map[string]any{"target": map[string]any{
					"id": target, "products": []string{"iPhone12,1"}, "build": "18A373", "identity": 1,
				}}}}
			case "/api/v1/targets/" + target:
				value = map[string]any{"target": map[string]any{
					"id": target, "products": []string{"iPhone12,1"}, "build": "18A373", "identity": 1,
					"board": "n104ap", "version": "14.0", "components": map[string]string{"DeviceTree": "tree"},
				}, "artifact": map[string]any{"sha256": strings.Repeat("a", 64), "source_availability": "registered_unverified"}}
			case "/api/v1/graph":
				if r.URL.Query().Get("target_id") != target {
					http.Error(w, "wrong target", http.StatusBadRequest)
					return
				}
				mu.Lock()
				requests = append(requests, r.URL.Query())
				mu.Unlock()
				value = graphBrowserNeighborhood(r.URL.Query())
			default:
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_version": "v1", "state": "ready", "read_only": true, "data": value,
			})
			return
		}
		base.ServeHTTP(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, browser,
		"--headless", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--disable-background-networking", "--no-first-run", "--no-default-browser-check",
		"--user-data-dir="+t.TempDir(), "--virtual-time-budget=14000", "--dump-dom",
		server.URL+"/?screen=graph&target="+url.QueryEscape(target),
	)
	output, err := command.Output()
	if err != nil && (ctx.Err() != context.DeadlineExceeded || !strings.Contains(string(output), "</html>")) {
		t.Fatalf("browser: %v; DOM tail: %s", err, tail(string(output), 2000))
	}
	dom := string(output)
	if !strings.Contains(dom, `data-graph-test="pass"`) {
		t.Fatalf("graph browser interaction failure; DOM tail: %s", tail(dom, 4000))
	}
	for _, want := range []string{
		"Unsupported coverage", "unknown codec", "Neighborhood limit reached",
	} {
		if !strings.Contains(dom, want) {
			t.Fatalf("graph browser missing %q; DOM tail: %s", want, tail(dom, 3000))
		}
	}
	mu.Lock()
	defer mu.Unlock()
	var expanded, filtered, component, large, finding, revision, driver bool
	for _, query := range requests {
		if query.Get("node_id") == "opaque:device:1" {
			expanded = true
		}
		if query.Get("edge_kind") == "has_property" {
			filtered = true
		}
		if query.Get("node_id") == "opaque:component:DeviceTree" {
			component = true
		}
		if query.Get("limit") == "100" {
			large = true
		}
		if query.Get("node_id") == "opaque:finding:1" {
			finding = true
		}
		if query.Get("node_id") == "opaque:revision:1" {
			revision = true
		}
		if query.Get("node_id") == "opaque:driver:1" {
			driver = true
		}
	}
	if !expanded || !filtered || !component || !large || !finding || !revision || !driver {
		t.Fatalf("missing bounded graph requests: device=%t filter=%t component=%t large=%t finding=%t revision=%t driver=%t", expanded, filtered, component, large, finding, revision, driver)
	}
}

func graphBrowserNeighborhood(query url.Values) map[string]any {
	const target = "target-exact-image-1"
	nodeID := query.Get("node_id")
	edgeKind := query.Get("edge_kind")
	limit := 25
	if query.Get("limit") == "100" {
		limit = 100
	}
	targetNode := graphBrowserNode("opaque:target", "target", "18A373 · n104ap", target, "", "")
	component := graphBrowserNode("opaque:component:DeviceTree", "component", "DeviceTree", target, "present", "")
	if edgeKind == "declares_component" && nodeID == "" {
		return graphBrowserData(targetNode, []any{component}, []any{
			graphBrowserEdge("component-edge", "declares_component", "opaque:target", "opaque:component:DeviceTree", "observation", "obs-component", "stored_extraction"),
		}, false)
	}
	if nodeID == "opaque:device:1" {
		center := graphBrowserNode(nodeID, "device", "/chosen-device", target, "present", "")
		property := graphBrowserNode("opaque:property:1", "property", "register", target, "present", "")
		return graphBrowserData(center, []any{property}, []any{
			graphBrowserEdge("device-property", "has_property", nodeID, "opaque:property:1", "observation", "obs-device", "stored_extraction"),
		}, false)
	}
	if nodeID == "opaque:driver:1" {
		center := graphBrowserNode(nodeID, "driver", "com.apple.driver.Test", target, "present", "")
		dependency := graphBrowserNode("opaque:dependency:1", "driver_dependency", "com.apple.iokit.IOUSBHostFamily", target, "declared", "")
		return graphBrowserData(center, []any{dependency}, []any{
			graphBrowserEdge("driver-dependency", "declares_driver_dependency", nodeID, "opaque:dependency:1", "observation", "obs-driver", "stored_extraction"),
		}, false)
	}
	if nodeID == "opaque:finding:1" {
		center := graphBrowserNode(nodeID, "finding", "device / register", target, "", "finding-1")
		revision := graphBrowserNode("opaque:revision:1", "revision", "inferred · present", target, "present", "")
		return graphBrowserData(center, []any{revision}, []any{
			graphBrowserEdge("finding-revision", "has_revision", nodeID, "opaque:revision:1", "finding_revision", "revision-1", "stored_record"),
		}, false)
	}
	if nodeID == "opaque:revision:1" {
		center := graphBrowserNode(nodeID, "revision", "inferred · present", target, "present", "")
		observation := graphBrowserNode("opaque:observation:1", "observation", "device / register", target, "present", "obs-device")
		evidence := graphBrowserNode("opaque:evidence:1", "evidence", "evidence-1", target, "", "")
		finding := graphBrowserNode("opaque:finding:1", "finding", "device / register", target, "", "finding-1")
		return graphBrowserData(center, []any{finding, observation, evidence}, []any{
			graphBrowserEdge("revision-finding", "belongs_to_finding", nodeID, "opaque:finding:1", "finding", "finding-1", "stored_record"),
			graphBrowserEdge("revision-observation", "supporting_observation", nodeID, "opaque:observation:1", "observation", "obs-device", "stored_extraction"),
			graphBrowserEdge("revision-evidence", "supporting_evidence", nodeID, "opaque:evidence:1", "observation", "obs-device", "stored_extraction"),
		}, false)
	}
	if nodeID == "opaque:component:DeviceTree" {
		center := component
		property := graphBrowserNode("opaque:property:component", "property", "archive_member", target, "present", "")
		return graphBrowserData(center, []any{property}, []any{
			graphBrowserEdge("component-property", "has_property", center["id"].(string), "opaque:property:component", "observation", "obs-component", "stored_extraction"),
		}, false)
	}
	nodes := []any{}
	edges := []any{}
	finding := graphBrowserNode("opaque:finding:1", "finding", "device / register", target, "", "finding-1")
	nodes = append(nodes, finding)
	edges = append(edges, graphBrowserEdge("finding-edge", "has_finding", "opaque:target", "opaque:finding:1", "finding", "finding-1", "stored_record"))
	device := graphBrowserNode("opaque:device:1", "device", "/chosen-device", target, "present", "")
	nodes = append(nodes, device)
	edges = append(edges, graphBrowserEdge("device-edge", "contains_device_tree_node", "opaque:target", "opaque:device:1", "observation", "obs-device", "stored_extraction"))
	nodes = append(nodes, component)
	edges = append(edges, graphBrowserEdge("component-edge", "declares_component", "opaque:target", "opaque:component:DeviceTree", "observation", "obs-component", "stored_extraction"))
	driver := graphBrowserNode("opaque:driver:1", "driver", "com.apple.driver.Test", target, "present", "")
	nodes = append(nodes, driver)
	edges = append(edges, graphBrowserEdge("driver-edge", "declares_driver", "opaque:target", "opaque:driver:1", "observation", "obs-driver", "stored_extraction"))
	for i := 0; i < 66; i++ {
		id := fmt.Sprintf("opaque:device:%d", i+2)
		nodes = append(nodes, graphBrowserNode(id, "device", fmt.Sprintf("/device-%02d", i+2), target, "present", ""))
		edges = append(edges, graphBrowserEdge(fmt.Sprintf("device-edge-%02d", i+2), "contains_device_tree_node", "opaque:target", id, "observation", fmt.Sprintf("obs-%02d", i+2), "stored_extraction"))
	}
	if edgeKind != "" {
		filteredNodes, filteredEdges := []any{}, []any{}
		for i, raw := range edges {
			if raw.(map[string]any)["kind"] == edgeKind {
				filteredNodes = append(filteredNodes, nodes[i])
				filteredEdges = append(filteredEdges, raw)
			}
		}
		nodes, edges = filteredNodes, filteredEdges
	}
	truncated := len(edges) > limit
	if truncated {
		nodes, edges = nodes[:limit], edges[:limit]
	}
	return graphBrowserData(targetNode, nodes, edges, truncated)
}

func graphBrowserNode(id, kind, label, target, state, recordID string) map[string]any {
	return map[string]any{"id": id, "kind": kind, "label": label, "target_id": target,
		"state": state, "record_id": recordID}
}

func graphBrowserEdge(id, kind, from, to, recordKind, recordID, origin string) map[string]any {
	basis := map[string]any{"record_kind": recordKind, "record_id": recordID, "origin": origin}
	if recordKind == "observation" {
		basis["extractor"] = "fixture/browser-v1"
		basis["run_ids"] = []string{"run-1"}
		basis["source"] = map[string]any{"artifact_id": strings.Repeat("a", 64), "member": "tree",
			"coordinate": "plist-selector", "selector": "/device/register", "offset": "0", "length": "4"}
	}
	return map[string]any{"id": id, "kind": kind, "from": from, "to": to, "basis": basis}
}

func graphBrowserData(center map[string]any, nodes, edges []any, truncated bool) map[string]any {
	return map[string]any{"center": center, "nodes": nodes, "edges": edges,
		"gaps":   []any{map[string]any{"component": "filesystem-content", "state": "unsupported", "reason": "unknown codec"}},
		"run_id": "run-1", "truncated": truncated}
}

const graphBrowserDriver = `
window.addEventListener("load", () => {
  const pause = () => new Promise((resolve) => setTimeout(resolve, 30));
  const until = async (test) => { for (let i = 0; i < 200; i++) { if (test()) return; await pause(); } throw new Error("timeout waiting for graph UI"); };
  (async () => {
    await until(() => document.querySelectorAll("#graph-map .graph-lane").length === 25);
    const count = () => [document.querySelectorAll("#graph-map .graph-lane").length, document.querySelectorAll("#graph-table-wrap tbody tr").length];
    if (count().join() !== "25,25" || document.getElementById("graph-truncated").hidden) throw new Error("bounded map/table mismatch");
    const finding = document.querySelector('[data-node-id="opaque:finding:1"]'); finding.click();
    if (!document.querySelector('#graph-detail a[href*="finding=finding-1"]')) throw new Error("finding detail link missing");
    const search = document.getElementById("graph-search"); search.value = "/chosen-device";
    search.dispatchEvent(new Event("input", { bubbles: true }));
    if (count().join() !== "1,1") throw new Error("search map/table mismatch");
    search.value = ""; search.dispatchEvent(new Event("input", { bubbles: true }));
    document.getElementById("graph-limit").value = "100";
    document.getElementById("graph-limit").dispatchEvent(new Event("change", { bubbles: true }));
    await until(() => count().join() === "70,70");
    document.querySelectorAll(".graph-sort")[2].click();
    const firstAscending = document.querySelector("#graph-table-wrap tbody tr td:nth-child(3)").textContent;
    document.querySelectorAll(".graph-sort")[2].click();
    const firstDescending = document.querySelector("#graph-table-wrap tbody tr td:nth-child(3)").textContent;
    if (firstAscending === firstDescending || document.querySelectorAll("#graph-table-wrap th")[2].getAttribute("aria-sort") !== "descending") throw new Error("relationship table sorting failed");
    document.querySelector('[data-node-id="opaque:finding:1"]').click();
    document.querySelector("#graph-detail button.button-primary").click();
    await until(() => document.getElementById("graph-scope").textContent.includes("device / register"));
    if (!document.querySelector('#graph-detail a[href*="finding=finding-1"]')) throw new Error("expanded finding lost detail link");
    if (!document.querySelector('.graph-lane[data-semantic="inferred"]')) throw new Error("inferred revision not distinguished");
    document.querySelector('[data-node-id="opaque:revision:1"]').click();
    document.querySelector("#graph-detail button.button-primary").click();
    await until(() => document.getElementById("graph-scope").textContent.includes("inferred · present"));
    document.querySelector('[data-node-id="opaque:finding:1"]').click();
    if (!document.querySelector('#graph-detail a[href*="finding=finding-1"]')) throw new Error("revision lost exact finding link");
    document.querySelector('[data-edge-id="revision-observation"] .graph-relation').click();
    if (!document.getElementById("graph-detail").textContent.includes("Member: tree")) throw new Error("finding basis source missing");
    document.getElementById("graph-reset").click();
    await until(() => count().join() === "70,70");
    document.querySelector('[data-node-id="opaque:driver:1"]').click();
    document.querySelector("#graph-detail button.button-primary").click();
    await until(() => document.getElementById("graph-scope").textContent.includes("com.apple.driver.Test"));
    document.querySelector("#graph-map .graph-relation").click();
    if (!document.getElementById("graph-detail").textContent.includes("Declared, runtime unverified")) throw new Error("driver declaration overstated");
    document.getElementById("graph-reset").click();
    await until(() => count().join() === "70,70");
    const device = document.querySelector('[data-node-id="opaque:device:1"]'); device.focus(); device.click();
    if (document.activeElement !== device) throw new Error("node not keyboard focusable");
    document.querySelector("#graph-detail button.button-primary").click();
    await until(() => document.getElementById("graph-scope").textContent.includes("/chosen-device"));
    const edge = document.querySelector("#graph-table-wrap .graph-table-select"); edge.focus(); edge.click();
    if (document.activeElement !== edge || !document.getElementById("graph-detail").textContent.includes("Member: tree")) throw new Error("table selection or source detail failed");
    document.getElementById("graph-edge-kind").value = "has_property";
    document.getElementById("graph-edge-kind").dispatchEvent(new Event("change", { bubbles: true }));
    await until(() => count().join() === "1,1");
    document.getElementById("graph-component").value = "opaque:component:DeviceTree";
    document.getElementById("graph-component").dispatchEvent(new Event("change", { bubbles: true }));
    await until(() => document.getElementById("graph-scope").textContent.includes("DeviceTree"));
    document.documentElement.dataset.graphTest = "pass";
  })().catch((error) => { document.documentElement.dataset.graphTest = "fail:" + error.message; });
});
`
