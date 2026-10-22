package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func graphHasEdge(graph GraphNeighborhood, kind string) bool {
	for _, edge := range graph.Edges {
		if edge.Kind == kind {
			return true
		}
	}
	return false
}

func TestGraphNeighborhoodUsesStoredTargetScopedRelationships(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	artifact := model.Artifact{
		ID:     model.Digest([]byte("graph source")),
		SHA256: model.Digest([]byte("graph source")), Size: 12,
	}
	target := model.Target{
		ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID,
		Build: "18A373", Board: "n104ap", Components: map[string]string{
			"KernelCache": "kernelcache", "DeviceTree": "tree",
			"RestoreDeviceTree": "tree-copy",
		},
	}
	other := model.Target{
		ID: model.ID(artifact.ID, "1"), ArtifactID: artifact.ID,
		Build: "18A373", Board: "n104ap", Components: target.Components,
	}
	if err := s.Register(ctx, artifact, []model.Target{target, other}); err != nil {
		t.Fatal(err)
	}
	makeObs := func(subject, property, presence string, value any,
		member, coordinate, selector string,
	) model.Observation {
		t.Helper()
		o, err := model.NewObservationWithPresence(target, subject, property,
			presence, value, model.Location{
				ArtifactID: artifact.ID,
				Member:     member, Coordinate: coordinate, Selector: selector,
				Length: 12,
			}, model.NewBlob([]byte(subject+property+selector)), "graph-fixture/v1")
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	observations := []model.Observation{
		makeObs("component/KernelCache", "archive_member", "present",
			map[string]any{"path": "kernelcache", "present": true}, "BuildManifest.plist", "plist-selector", "/Manifest/KernelCache"),
		makeObs("devicetree/root", "name", "present", "root", "tree", "decoded-payload-byte", "/root/name"),
		makeObs("devicetree/root", "reg", "present", map[string]any{"type": "number", "decimal": "18446744073709551615"},
			"tree", "decoded-payload-byte", "/root/reg"),
		makeObs("driver/com.example.A", "OSBundleLibraries", "present",
			map[string]string{"com.example.B": "1.0"}, "kernelcache", "plist-selector", "/drivers/A/libraries"),
		makeObs("filesystem/volume/vol-1/object/9", "metadata", "present",
			map[string]any{"path": "/System/Library/Extensions/Foo.plist"},
			"OS.dmg", "apfs-file-object", "partition/1/volume/vol-1/view/1/object/9/metadata"),
		makeObs("filesystem/volume/vol-1/object/9", "content", "unexamined",
			map[string]any{"state": "unsupported"}, "OS.dmg", "apfs-file-object",
			"partition/1/volume/vol-1/view/1/object/9/data"),
		makeObs("devicetree/root", "name", "present", "root", "tree-copy",
			"decoded-payload-byte", "/root/name"),
	}
	run := model.Run{
		ID: model.ID("graph-run", target.ID), TargetID: target.ID,
		State: "partial", Started: time.Now(), Coverage: []model.Coverage{
			{Component: "filesystem-content", State: "partial", Reason: "unsupported file"},
		},
	}
	if err := s.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(ctx, run, observations); err != nil {
		t.Fatal(err)
	}
	root, err := s.GraphNeighborhood(ctx, GraphQuery{TargetID: target.ID, Limit: 3})
	if err != nil || !root.Truncated || len(root.Edges) != 3 ||
		!graphHasEdge(root, "belongs_to_artifact") || !graphHasEdge(root, "declares_component") {
		t.Fatalf("bounded root: %+v %v", root, err)
	}
	if len(root.Gaps) == 0 {
		t.Fatalf("missing incomplete coverage: %+v", root)
	}
	deviceRoots, err := s.GraphNeighborhood(ctx, GraphQuery{
		TargetID: target.ID, EdgeKind: "contains_device_tree_node", Limit: 20,
	})
	if err != nil || len(deviceRoots.Edges) != 2 ||
		deviceRoots.Edges[0].To == deviceRoots.Edges[1].To {
		t.Fatalf("same device label from distinct sources collapsed: %+v %v", deviceRoots, err)
	}
	for _, tc := range []struct {
		kind, key, edge string
	}{
		{"device", graphScopeKey(observations[1]), "has_property"},
		{"driver", graphScopeKey(observations[3]), "declares_driver_dependency"},
		{"driver_dependency", observations[3].ID + "/com.example.B", "declared_by_driver"},
		{"apfs_path", observations[4].ID, "resolves_to_apfs_object"},
		{"apfs_file", observations[4].ID, "has_property"},
		{"property", observations[2].ID, "records_observation"},
	} {
		graph, err := s.GraphNeighborhood(ctx, GraphQuery{
			TargetID: target.ID,
			NodeID:   GraphID(target.ID, tc.kind, tc.key), Limit: 20,
		})
		if err != nil || !graphHasEdge(graph, tc.edge) {
			t.Fatalf("%s graph: %+v %v", tc.kind, graph, err)
		}
		if tc.kind == "apfs_file" {
			foundGap := false
			for _, node := range graph.Nodes {
				if node.Kind == "property" && node.Label == "content" &&
					node.State == "unexamined" {
					foundGap = true
				}
			}
			if !foundGap {
				t.Fatalf("unsupported APFS content lost: %+v", graph)
			}
		}
		for _, edge := range graph.Edges {
			if edge.Basis.RecordKind == "observation" &&
				(edge.Basis.Source == nil || edge.Basis.Extractor == "" ||
					len(edge.Basis.RunIDs) == 0) {
				t.Fatalf("edge lost source/run/parser: %+v", edge)
			}
		}
	}
	// A name from the same archive's other BuildManifest identity cannot be used
	// to reach this target's nodes.
	if _, err := s.GraphNeighborhood(ctx, GraphQuery{
		TargetID: other.ID,
		NodeID:   GraphID(target.ID, "device", graphScopeKey(observations[1])), Limit: 10,
	}); !errors.Is(err, ErrInvalidGraphRequest) {
		t.Fatalf("cross-target node accepted: %v", err)
	}
	chains, err := s.FindingChains(ctx, target.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	var observedRevision string
	for _, chain := range chains {
		if chain.Subject == observations[2].Subject && chain.Property == observations[2].Property {
			observedRevision = chain.Revisions[0].ID
			break
		}
	}
	if observedRevision == "" {
		t.Fatal("missing observed revision")
	}
	for _, tc := range []struct{ kind, recordID string }{
		{"finding", chains[0].ID},
		{"observation", observations[2].ID},
	} {
		graph, err := s.GraphNeighborhood(ctx, GraphQuery{
			TargetID: target.ID,
			NodeID:   GraphID(target.ID, tc.kind, tc.recordID), Limit: 20,
		})
		if err != nil || graph.Center.RecordID != tc.recordID {
			t.Fatalf("%s detail reference: %+v %v", tc.kind, graph.Center, err)
		}
	}
	receipt, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: observations[2].Subject, Property: observations[2].Property,
		Basis: "inferred", Presence: "present", Value: json.RawMessage(`{"type":"number","decimal":"0"}`),
		Rationale: "review fixture", Author: "fixture", Supersedes: observedRevision,
		References: []model.BasisReference{
			{Role: "supporting", ObservationID: observations[2].ID},
			{Role: "contradicting", RevisionID: observedRevision},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	event, err := s.ReviewRevision(ctx, model.AssessmentDraft{
		FindingID: receipt.FindingID, RevisionID: receipt.RevisionID, Basis: "inferred",
		State: "accepted", Reviewer: "fixture-reviewer", Reason: "basis checked", ExpectedID: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := s.GraphNeighborhood(ctx, GraphQuery{
		TargetID: target.ID,
		NodeID:   GraphID(target.ID, "revision", receipt.RevisionID), Limit: 20,
	})
	if err != nil || !graphHasEdge(revision, "supporting_observation") ||
		!graphHasEdge(revision, "supporting_evidence") ||
		!graphHasEdge(revision, "contradicting_revision") ||
		!graphHasEdge(revision, "supersedes_revision") ||
		!graphHasEdge(revision, "belongs_to_finding") ||
		!graphHasEdge(revision, "assessed_as") {
		t.Fatalf("revision basis: %+v %v", revision, err)
	}
	assessment, err := s.GraphNeighborhood(ctx, GraphQuery{
		TargetID: target.ID,
		NodeID:   GraphID(target.ID, "assessment", fmt.Sprint(event.ID)), Limit: 20,
	})
	if err != nil || !graphHasEdge(assessment, "assesses_revision") ||
		assessment.Center.State != "accepted" {
		t.Fatalf("assessment event: %+v %v", assessment, err)
	}
	evidence, err := s.GraphNeighborhood(ctx, GraphQuery{
		TargetID: target.ID,
		NodeID:   GraphID(target.ID, "evidence", observations[2].EvidenceID), Limit: 20,
	})
	if err != nil || !graphHasEdge(evidence, "supports_observation") {
		t.Fatalf("evidence basis: %+v %v", evidence, err)
	}
	// Basis and evidence expansion must release a result cursor before making
	// another database query, even with a single available pool connection.
	var schema string
	if err := s.pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("GOIOSVM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	params := u.Query()
	params.Set("search_path", schema)
	params.Set("pool_max_conns", "1")
	u.RawQuery = params.Encode()
	one, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close()
	deadline, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, selection := range []string{
		GraphID(target.ID, "revision", receipt.RevisionID),
		GraphID(target.ID, "evidence", observations[2].EvidenceID),
	} {
		if _, err := one.GraphNeighborhood(deadline, GraphQuery{
			TargetID: target.ID, NodeID: selection, Limit: 20,
		}); err != nil {
			t.Fatalf("single-connection graph: %v", err)
		}
	}
}

func TestGraphLargeNeighborhoodIsBoundedAndNamesStayDistinct(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	artifact := model.Artifact{
		ID:     model.Digest([]byte("large graph")),
		SHA256: model.Digest([]byte("large graph")), Size: 11,
	}
	target := model.Target{ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID}
	if err := s.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	for batch := range 2 {
		run := model.Run{
			ID:       fmt.Sprintf("large-graph-run-%d", batch),
			TargetID: target.ID, Started: time.Now().Add(time.Duration(batch) * time.Second),
			State: "partial", Coverage: []model.Coverage{{Component: "device-tree", State: "partial"}},
		}
		if err := s.Start(ctx, run); err != nil {
			t.Fatal(err)
		}
		items := make([]model.Observation, 0, 70)
		for i := range 70 {
			n := batch*70 + i
			subject := fmt.Sprintf("devicetree/root/node-%03d", n)
			o, err := model.NewObservation(target, subject, "name", "duplicate",
				model.Location{
					ArtifactID: artifact.ID, Member: "tree",
					Coordinate: "decoded-payload-byte", Selector: subject + "/name",
				},
				model.NewBlob([]byte(subject)), "graph-fixture/v1")
			if err != nil {
				t.Fatal(err)
			}
			items = append(items, o)
		}
		if err := s.Publish(ctx, run, items); err != nil {
			t.Fatal(err)
		}
	}
	graph, err := s.GraphNeighborhood(ctx, GraphQuery{
		TargetID: target.ID,
		EdgeKind: "contains_device_tree_node", Limit: 20,
	})
	if err != nil || !graph.Truncated || len(graph.Edges) != 20 {
		t.Fatalf("large graph cap: %d truncated=%v err=%v", len(graph.Edges), graph.Truncated, err)
	}
	ids := map[string]bool{}
	for _, edge := range graph.Edges {
		if ids[edge.To] {
			t.Fatal("duplicate labels collapsed")
		}
		ids[edge.To] = true
	}
	if _, _, _, ok := ParseGraphID(strings.Repeat("x", 8193)); ok {
		t.Fatal("overlong graph ID accepted")
	}
}
