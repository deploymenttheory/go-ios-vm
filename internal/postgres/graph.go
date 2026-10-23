package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	ErrInvalidGraphRequest      = errors.New("invalid graph request")
	errGraphObservationIdentity = errors.New("graph observation identity mismatch")
)

// Graph IDs encode the exact target as well as the kind and key. A label is
// never used as identity, so duplicate names in different images stay apart.
func GraphID(target, kind, key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(target + "\x00" + kind + "\x00" + key))
}

func ParseGraphID(id string) (target, kind, key string, ok bool) {
	if len(id) > 8192 {
		return "", "", "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", "", "", false
	}
	parts := strings.Split(string(b), "\x00")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

type GraphNode struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	TargetID string `json:"target_id"`
	State    string `json:"state,omitempty"`
	RecordID string `json:"record_id,omitempty"`
}

type GraphBasis struct {
	RecordKind    string          `json:"record_kind"`
	RecordID      string          `json:"record_id"`
	Origin        string          `json:"origin"`
	Extractor     string          `json:"extractor,omitempty"`
	RunIDs        []string        `json:"run_ids,omitempty"`
	RunsTruncated bool            `json:"runs_truncated,omitempty"`
	Source        *model.Location `json:"source,omitempty"`
	CoverageState string          `json:"coverage_state,omitempty"`
}

type GraphEdge struct {
	ID    string     `json:"id"`
	Kind  string     `json:"kind"`
	From  string     `json:"from"`
	To    string     `json:"to"`
	Basis GraphBasis `json:"basis"`
}

type GraphGap struct {
	Component string `json:"component"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
}

type GraphNeighborhood struct {
	Center    GraphNode   `json:"center"`
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
	Gaps      []GraphGap  `json:"gaps"`
	RunID     string      `json:"run_id,omitempty"`
	Truncated bool        `json:"truncated"`
}

type GraphQuery struct {
	TargetID string
	NodeID   string
	EdgeKind string
	Limit    int
}

type graphBuilder struct {
	target, edgeKind string
	limit            int
	out              GraphNeighborhood
	seen             map[string]bool
	nodeSeen         map[string]bool
}

func (b *graphBuilder) node(kind, key, label, state string) GraphNode {
	node := GraphNode{
		ID: GraphID(b.target, kind, key), Kind: kind, Label: label,
		TargetID: b.target, State: state,
	}
	if kind == "finding" || kind == "observation" {
		node.RecordID = key
	}
	return node
}

func (b *graphBuilder) add(kind string, from, to GraphNode, basis GraphBasis) {
	if b.edgeKind != "" && b.edgeKind != kind {
		return
	}
	id := model.ID("graph/v1", b.target, kind, from.ID, to.ID,
		basis.RecordKind, basis.RecordID)
	if b.seen[id] {
		return
	}
	b.seen[id] = true
	if len(b.out.Edges) >= b.limit {
		b.out.Truncated = true
		return
	}
	b.out.Edges = append(b.out.Edges, GraphEdge{
		ID: id, Kind: kind, From: from.ID, To: to.ID, Basis: basis,
	})
	if from.ID != b.out.Center.ID && !b.nodeSeen[from.ID] {
		b.out.Nodes = append(b.out.Nodes, from)
		b.nodeSeen[from.ID] = true
	}
	if to.ID != b.out.Center.ID && !b.nodeSeen[to.ID] {
		b.out.Nodes = append(b.out.Nodes, to)
		b.nodeSeen[to.ID] = true
	}
}

func graphObservationBasis(o model.Observation, runs []string) GraphBasis {
	source := o.Source
	truncated := len(runs) > 5
	if truncated {
		runs = runs[:5]
	}
	return GraphBasis{
		RecordKind: "observation", RecordID: o.ID,
		Origin: "stored_extraction", Extractor: o.Extractor, RunIDs: runs,
		RunsTruncated: truncated, Source: &source,
	}
}

type graphObserved struct {
	Observation model.Observation
	RunIDs      []string
}

// graphObservations uses only predicates selected by this package. Its LIMIT
// applies in SQL; large real-image graphs are never loaded wholesale.
func (s *Store) graphObservations(ctx context.Context, target, predicate, key string,
	limit int, distinctSubject bool,
) ([]graphObserved, bool, error) {
	allowed := map[string]string{
		"component": "o.document->>'subject' LIKE 'component/%' AND o.document->>'property'='archive_member' AND $2::text IS NOT NULL",
		"device":    "o.document->>'subject' LIKE 'devicetree/%' AND $2::text IS NOT NULL",
		"driver":    "o.document->>'subject' LIKE 'driver/%' AND $2::text IS NOT NULL",
		"file":      "o.document->>'subject' LIKE 'filesystem/volume/%/object/%' AND o.document->>'property'='metadata' AND $2::text IS NOT NULL",
		"subject":   "o.document->>'subject'=$2",
		"member":    "o.document #>> '{source,member}'=$2",
		"evidence":  "o.evidence_id=$2",
		"subject_scope": `(o.document->>'subject')=($2::jsonb->>'subject')
		 AND (o.document #>> '{source,member}')=($2::jsonb->>'member')
		 AND (o.document #>> '{source,entry}')=($2::jsonb->>'entry')`,
		"file_scope": `(o.document->>'subject')=($2::jsonb->>'subject')
		 AND (o.document #>> '{source,member}')=($2::jsonb->>'member')
		 AND (o.document #>> '{source,entry}')=($2::jsonb->>'entry')
		 AND left(o.document #>> '{source,selector}',length($2::jsonb->>'selector_prefix')+1)
		 = ($2::jsonb->>'selector_prefix')||'/'`,
	}
	where, ok := allowed[predicate]
	if !ok {
		return nil, false, ErrInvalidGraphRequest
	}
	// A graph scan needs identity and provenance, not large extracted values.
	// Only the two relationship-bearing values are projected into memory.
	projection := `jsonb_build_object('id',o.id,'target_id',o.target_id,
	 'subject',o.document->>'subject','property',o.document->>'property',
	 'presence',o.document->>'presence','source',o.document->'source',
	 'extractor',o.document->>'extractor','evidence_id',o.evidence_id,
	 'value',CASE
	  WHEN o.document->>'property'='OSBundleLibraries' THEN o.document->'value'
	  WHEN o.document->>'property'='metadata' AND
	   o.document->>'subject' LIKE 'filesystem/volume/%/object/%'
	   THEN jsonb_build_object('path',o.document #>> '{value,path}')
	  ELSE 'null'::jsonb END)`
	selectClause := "SELECT " + projection + ",o.id"
	order := " ORDER BY o.id"
	if distinctSubject {
		if predicate == "device" || predicate == "driver" {
			selectClause = `SELECT DISTINCT ON (o.document->>'subject',
			 o.document #>> '{source,member}',o.document #>> '{source,entry}') ` + projection + ",o.id"
			order = ` ORDER BY o.document->>'subject',o.document #>> '{source,member}',
			 o.document #>> '{source,entry}',o.id`
		} else {
			selectClause = "SELECT DISTINCT ON (o.document->>'subject') " + projection + ",o.id"
			order = " ORDER BY o.document->>'subject',o.id"
		}
	}
	query := selectClause + " FROM observation o WHERE o.target_id=$1 AND " + where + order + " LIMIT $3"
	rows, err := s.pool.Query(ctx, query, target, key, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("graph observations: %w", err)
	}
	defer rows.Close()
	items := []graphObserved{}
	for rows.Next() {
		var raw []byte
		var id string
		if err := rows.Scan(&raw, &id); err != nil {
			return nil, false, fmt.Errorf("graph observation scan: %w", err)
		}
		var o model.Observation
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, false, fmt.Errorf("graph observation decode: %w", err)
		}
		if o.ID != id {
			return nil, false, errGraphObservationIdentity
		}
		items = append(items, graphObserved{Observation: o})
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("graph observation rows: %w", err)
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	for i := range items {
		runs, err := s.graphRunIDs(ctx, items[i].Observation.ID)
		if err != nil {
			return nil, false, fmt.Errorf("graph observations: %w", err)
		}
		items[i].RunIDs = runs
	}
	return items, more, nil
}

func (s *Store) graphRunIDs(ctx context.Context, observationID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT run_id FROM run_observation
	 WHERE observation_id=$1 ORDER BY run_id LIMIT 6`, observationID)
	if err != nil {
		return nil, fmt.Errorf("graph run IDs: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("graph run ID scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph run ID rows: %w", err)
	}
	return ids, nil
}

func graphValuePath(raw json.RawMessage) string {
	var v struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return v.Path
}

type graphSourceScope struct {
	Subject        string `json:"subject"`
	Member         string `json:"member"`
	Entry          int    `json:"entry"`
	SelectorPrefix string `json:"selector_prefix,omitempty"`
}

func graphScopeKey(o model.Observation) string {
	b, _ := json.Marshal(graphSourceScope{
		Subject: o.Subject, Member: o.Source.Member, Entry: o.Source.Entry,
	})
	return string(b)
}

func parseGraphScope(key string) (graphSourceScope, bool) {
	var scope graphSourceScope
	if json.Unmarshal([]byte(key), &scope) != nil || scope.Subject == "" ||
		scope.Member == "" || scope.Entry < 0 || scope.SelectorPrefix != "" {
		return graphSourceScope{}, false
	}
	canonical, _ := json.Marshal(scope)
	if string(canonical) != key {
		return graphSourceScope{}, false
	}
	return scope, true
}

func (s *Store) graphCoverage(ctx context.Context, b *graphBuilder) error {
	page, err := s.BrowseRuns(ctx, RunFilter{
		TargetID: b.target, Page: PageOptions{Limit: 1},
	})
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	if len(page.Items) == 0 {
		b.out.Gaps = []GraphGap{{
			Component: "inspection", State: "unexamined",
			Reason: "no inspection run for this target",
		}}
		return nil
	}
	run, err := s.BrowseRun(ctx, page.Items[0].ID)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.out.RunID = run.ID
	if !run.FinalCoverageAvailable {
		b.out.Gaps = append(b.out.Gaps, GraphGap{
			Component: "inspection", State: "unexamined",
			Reason: "latest run has no final coverage ledger",
		})
	}
	for _, c := range run.FinalCoverage {
		if c.State != "complete" {
			b.out.Gaps = append(b.out.Gaps, GraphGap(c))
		}
	}
	return nil
}

//nolint:gocyclo // Each node type is an explicit, bounded read projection.
func (s *Store) GraphNeighborhood(ctx context.Context, q GraphQuery) (GraphNeighborhood, error) {
	if q.TargetID == "" || q.Limit < 1 || q.Limit > 100 {
		return GraphNeighborhood{}, ErrInvalidGraphRequest
	}
	if q.EdgeKind != "" {
		allowed := map[string]bool{
			"belongs_to_artifact": true, "contains_target": true,
			"declares_component": true, "contains_device_tree_node": true,
			"declares_driver": true, "observed_apfs_path": true,
			"has_finding": true, "has_property": true,
			"declares_driver_dependency": true, "declared_by_driver": true,
			"resolves_to_apfs_object": true, "records_observation": true,
			"has_revision": true, "supersedes_revision": true,
			"belongs_to_finding":     true,
			"supporting_observation": true, "contradicting_observation": true,
			"supporting_evidence": true, "contradicting_evidence": true,
			"supporting_revision": true, "contradicting_revision": true,
			"assessed_as": true, "assesses_revision": true,
			"retains_evidence": true, "used_by_revision": true,
			"supports_observation": true,
		}
		if !allowed[q.EdgeKind] {
			return GraphNeighborhood{}, ErrInvalidGraphRequest
		}
	}
	target, kind, key := q.TargetID, "target", q.TargetID
	if q.NodeID != "" {
		var ok bool
		target, kind, key, ok = ParseGraphID(q.NodeID)
		if !ok || target != q.TargetID {
			return GraphNeighborhood{}, ErrInvalidGraphRequest
		}
	}
	record, err := s.BrowseTarget(ctx, q.TargetID)
	if err != nil {
		return GraphNeighborhood{}, err
	}
	b := &graphBuilder{
		target: target, edgeKind: q.EdgeKind, limit: q.Limit,
		seen: map[string]bool{}, nodeSeen: map[string]bool{},
	}
	b.out.Center = b.node(kind, key, key, "")
	b.out.Nodes = []GraphNode{}
	b.out.Edges = []GraphEdge{}
	b.out.Gaps = []GraphGap{}
	if err := s.graphCoverage(ctx, b); err != nil {
		return GraphNeighborhood{}, err
	}
	switch kind {
	case "target":
		if key != target {
			return GraphNeighborhood{}, ErrInvalidGraphRequest
		}
		b.out.Center.Label = record.Target.Build + " · " + record.Target.Board
		if err := s.graphTarget(ctx, b, record); err != nil {
			return GraphNeighborhood{}, err
		}
	case "artifact":
		if key != record.Artifact.ID {
			return GraphNeighborhood{}, pgx.ErrNoRows
		}
		b.out.Center.Label = record.Artifact.SHA256
		b.add("contains_target", b.out.Center, b.node("target", target,
			record.Target.Build+" · "+record.Target.Board, ""),
			GraphBasis{RecordKind: "firmware_target", RecordID: target, Origin: "stored_record"})
	case "component", "device", "driver", "apfs_path", "apfs_file":
		if err := s.graphSubject(ctx, b, kind, key, record); err != nil {
			return GraphNeighborhood{}, err
		}
	case "driver_dependency":
		if err := s.graphDriverDependency(ctx, b, key); err != nil {
			return GraphNeighborhood{}, err
		}
	case "property":
		if err := s.graphProperty(ctx, b, key); err != nil {
			return GraphNeighborhood{}, err
		}
	case "finding":
		if err := s.graphFinding(ctx, b, key); err != nil {
			return GraphNeighborhood{}, err
		}
	case "revision":
		if err := s.graphRevision(ctx, b, key); err != nil {
			return GraphNeighborhood{}, err
		}
	case "observation":
		if err := s.graphObservation(ctx, b, key); err != nil {
			return GraphNeighborhood{}, err
		}
	case "evidence":
		if err := s.graphEvidence(ctx, b, key); err != nil {
			return GraphNeighborhood{}, err
		}
	case "assessment":
		if err := s.graphAssessment(ctx, b, key); err != nil {
			return GraphNeighborhood{}, err
		}
	default:
		return GraphNeighborhood{}, ErrInvalidGraphRequest
	}
	return b.out, nil
}

func (s *Store) graphTarget(ctx context.Context, b *graphBuilder, record TargetRecord) error {
	b.add("belongs_to_artifact", b.out.Center,
		b.node("artifact", record.Artifact.ID, record.Artifact.SHA256, ""),
		GraphBasis{RecordKind: "firmware_target", RecordID: b.target, Origin: "stored_record"})
	types := []struct {
		predicate, kind, edge string
	}{
		{"component", "component", "declares_component"},
		{"device", "device", "contains_device_tree_node"},
		{"driver", "driver", "declares_driver"},
		{"file", "apfs_path", "observed_apfs_path"},
	}
	for _, group := range types {
		if b.edgeKind != "" && b.edgeKind != group.edge {
			continue
		}
		remaining := max(1, b.limit-len(b.out.Edges))
		items, more, err := s.graphObservations(ctx, b.target, group.predicate, "", remaining,
			group.kind != "apfs_path")
		if err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		b.out.Truncated = b.out.Truncated || more
		for _, item := range items {
			o := item.Observation
			key, label := o.Subject, o.Subject
			switch group.kind {
			case "component":
				key = strings.TrimPrefix(o.Subject, "component/")
				label = key
			case "device":
				key = graphScopeKey(o)
				label = strings.TrimPrefix(o.Subject, "devicetree")
			case "driver":
				key = graphScopeKey(o)
				label = strings.TrimPrefix(o.Subject, "driver/")
			case "apfs_path":
				key = o.ID
				label = graphValuePath(o.Value)
				if label == "" {
					continue
				}
			}
			b.add(group.edge, b.out.Center, b.node(group.kind, key, label, o.Presence),
				graphObservationBasis(o, item.RunIDs))
		}
	}
	if b.edgeKind == "" || b.edgeKind == "has_finding" {
		rows, err := s.pool.Query(ctx, `SELECT id,subject,property FROM finding
		 WHERE target_id=$1 ORDER BY id LIMIT $2`, b.target, b.limit+1)
		if err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var id, subject, property string
			if err := rows.Scan(&id, &subject, &property); err != nil {
				return fmt.Errorf("graph projection: %w", err)
			}
			count++
			if count > b.limit {
				b.out.Truncated = true
				break
			}
			b.add("has_finding", b.out.Center, b.node("finding", id,
				subject+" / "+property, ""), GraphBasis{
				RecordKind: "finding", RecordID: id, Origin: "stored_record",
			})
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
	}
	return nil
}

//nolint:gocyclo // Each structural node kind validates its source scope before its bounded read.
func (s *Store) graphSubject(ctx context.Context, b *graphBuilder,
	kind, key string, record TargetRecord,
) error {
	var subject string
	predicate := "subject"
	switch kind {
	case "component":
		if _, ok := record.Target.Components[key]; !ok {
			return pgx.ErrNoRows
		}
		subject = "component/" + key
	case "device":
		scope, ok := parseGraphScope(key)
		if !ok || !strings.HasPrefix(scope.Subject, "devicetree/") {
			return ErrInvalidGraphRequest
		}
		subject, predicate = key, "subject_scope"
	case "driver":
		scope, ok := parseGraphScope(key)
		if !ok || !strings.HasPrefix(scope.Subject, "driver/") {
			return ErrInvalidGraphRequest
		}
		subject, predicate = key, "subject_scope"
	case "apfs_path":
		o, err := s.ObservationForTarget(ctx, b.target, key)
		if err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		if o.Property != "metadata" || !strings.HasPrefix(o.Subject, "filesystem/volume/") {
			return ErrInvalidGraphRequest
		}
		path := graphValuePath(o.Value)
		if path == "" {
			return ErrInvalidGraphRequest
		}
		b.out.Center.Label = path
		b.out.Center.State = o.Presence
		runs, err := s.graphRunIDs(ctx, o.ID)
		if err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		b.add("resolves_to_apfs_object", b.out.Center,
			b.node("apfs_file", o.ID, path, o.Presence),
			graphObservationBasis(o, runs))
		return nil
	case "apfs_file":
		metadata, err := s.ObservationForTarget(ctx, b.target, key)
		if err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		if metadata.Property != "metadata" ||
			!strings.HasPrefix(metadata.Subject, "filesystem/volume/") ||
			!strings.Contains(metadata.Subject, "/object/") ||
			!strings.HasSuffix(metadata.Source.Selector, "/metadata") {
			return ErrInvalidGraphRequest
		}
		scope := graphSourceScope{
			Subject: metadata.Subject, Member: metadata.Source.Member,
			Entry:          metadata.Source.Entry,
			SelectorPrefix: strings.TrimSuffix(metadata.Source.Selector, "/metadata"),
		}
		encoded, _ := json.Marshal(scope)
		subject, predicate = string(encoded), "file_scope"
		b.out.Center.Label = graphValuePath(metadata.Value)
	}
	items, more, err := s.graphObservations(ctx, b.target, predicate, subject, b.limit, false)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	if len(items) == 0 {
		return pgx.ErrNoRows
	}
	b.out.Truncated = b.out.Truncated || more
	switch kind {
	case "component":
		b.out.Center.Label = key
	case "device", "driver":
		scope, _ := parseGraphScope(key)
		b.out.Center.Label = scope.Subject
	}
	for _, item := range items {
		o := item.Observation
		property := b.node("property", o.ID, o.Property, o.Presence)
		b.add("has_property", b.out.Center, property,
			graphObservationBasis(o, item.RunIDs))
		if kind == "driver" && o.Property == "OSBundleLibraries" && o.Presence == "present" {
			var dependencies map[string]json.RawMessage
			if json.Unmarshal(o.Value, &dependencies) == nil {
				names := make([]string, 0, len(dependencies))
				for name := range dependencies {
					names = append(names, name)
				}
				slices.Sort(names)
				for _, name := range names {
					if len(b.out.Edges) >= b.limit {
						b.out.Truncated = true
						break
					}
					b.add("declares_driver_dependency", b.out.Center,
						b.node("driver_dependency", o.ID+"/"+name, name, "declared"),
						graphObservationBasis(o, item.RunIDs))
				}
			}
		}
	}
	return nil
}

func (s *Store) graphProperty(ctx context.Context, b *graphBuilder, id string) error {
	o, err := s.ObservationForTarget(ctx, b.target, id)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.out.Center.Label = o.Property
	b.out.Center.State = o.Presence
	runs, err := s.graphRunIDs(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.add("records_observation", b.out.Center,
		b.node("observation", o.ID, o.Subject+" / "+o.Property, o.Presence),
		graphObservationBasis(o, runs))
	return nil
}

func (s *Store) graphDriverDependency(ctx context.Context, b *graphBuilder, key string) error {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrInvalidGraphRequest
	}
	o, err := s.ObservationForTarget(ctx, b.target, parts[0])
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	if !strings.HasPrefix(o.Subject, "driver/") || o.Property != "OSBundleLibraries" ||
		o.Presence != "present" {
		return ErrInvalidGraphRequest
	}
	var dependencies map[string]json.RawMessage
	if json.Unmarshal(o.Value, &dependencies) != nil {
		return ErrInvalidGraphRequest
	}
	if _, exists := dependencies[parts[1]]; !exists {
		return pgx.ErrNoRows
	}
	b.out.Center.Label = parts[1]
	b.out.Center.State = "declared"
	runs, err := s.graphRunIDs(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.add("declared_by_driver", b.out.Center,
		b.node("driver", graphScopeKey(o),
			strings.TrimPrefix(o.Subject, "driver/"), "observed"),
		graphObservationBasis(o, runs))
	return nil
}

func (s *Store) graphFinding(ctx context.Context, b *graphBuilder, id string) error {
	var subject, property string
	if err := s.pool.QueryRow(ctx, `SELECT subject,property FROM finding
	 WHERE id=$1 AND target_id=$2`, id, b.target).Scan(&subject, &property); err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.out.Center.Label = subject + " / " + property
	rows, err := s.pool.Query(ctx, `SELECT id,basis,presence FROM finding_revision
	 WHERE finding_id=$1 ORDER BY created,id LIMIT $2`, id, b.limit+1)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var revisionID, basis, presence string
		if err := rows.Scan(&revisionID, &basis, &presence); err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		count++
		if count > b.limit {
			b.out.Truncated = true
			break
		}
		b.add("has_revision", b.out.Center,
			b.node("revision", revisionID, basis+" · "+presence, presence),
			GraphBasis{
				RecordKind: "finding_revision", RecordID: revisionID,
				Origin: "stored_record",
			})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("graph rows: %w", err)
	}
	return nil
}

func (s *Store) graphRevision(ctx context.Context, b *graphBuilder, id string) error {
	var basis, presence, findingID, subject, property string
	var supersedes *string
	if err := s.pool.QueryRow(ctx, `SELECT r.basis,r.presence,r.supersedes,f.id,f.subject,f.property
	 FROM finding_revision r JOIN finding f ON f.id=r.finding_id
	 WHERE r.id=$1 AND f.target_id=$2`, id, b.target).
		Scan(&basis, &presence, &supersedes, &findingID, &subject, &property); err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.out.Center.Label = basis + " · " + presence
	b.out.Center.State = presence
	b.add("belongs_to_finding", b.out.Center,
		b.node("finding", findingID, subject+" / "+property, ""),
		GraphBasis{RecordKind: "finding_revision", RecordID: id, Origin: "stored_record"})
	if supersedes != nil {
		b.add("supersedes_revision", b.out.Center,
			b.node("revision", *supersedes, *supersedes, ""),
			GraphBasis{RecordKind: "finding_revision", RecordID: id, Origin: "stored_record"})
	}
	rows, err := s.pool.Query(ctx, `SELECT role,observation_id,reference_revision_id
	 FROM finding_basis WHERE revision_id=$1
	 ORDER BY role,observation_id,reference_revision_id LIMIT $2`, id, b.limit+1)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	type reference struct {
		role                      string
		observationID, revisionID *string
	}
	references := make([]reference, 0, b.limit)
	count := 0
	for rows.Next() {
		var ref reference
		if err := rows.Scan(&ref.role, &ref.observationID, &ref.revisionID); err != nil {
			rows.Close()
			return fmt.Errorf("graph projection: %w", err)
		}
		count++
		if count > b.limit {
			b.out.Truncated = true
			break
		}
		references = append(references, ref)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("graph projection: %w", err)
	}
	rows.Close()
	for _, ref := range references {
		if ref.observationID != nil {
			o, err := s.ObservationForTarget(ctx, b.target, *ref.observationID)
			if err != nil {
				return fmt.Errorf("graph projection: %w", err)
			}
			runs, err := s.graphRunIDs(ctx, o.ID)
			if err != nil {
				return fmt.Errorf("graph projection: %w", err)
			}
			b.add(ref.role+"_observation", b.out.Center,
				b.node("observation", o.ID, o.Subject+" / "+o.Property, o.Presence),
				graphObservationBasis(o, runs))
			b.add(ref.role+"_evidence", b.out.Center,
				b.node("evidence", o.EvidenceID, o.EvidenceID, ""),
				graphObservationBasis(o, runs))
		} else if ref.revisionID != nil {
			b.add(ref.role+"_revision", b.out.Center,
				b.node("revision", *ref.revisionID, *ref.revisionID, ""),
				GraphBasis{
					RecordKind: "finding_basis", RecordID: id + "/" + *ref.revisionID,
					Origin: "stored_record",
				})
		}
	}
	rows, err = s.pool.Query(ctx, `SELECT id,state FROM finding_assessment_event
	 WHERE revision_id=$1 ORDER BY id DESC LIMIT $2`, id, b.limit+1)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	defer rows.Close()
	count = 0
	for rows.Next() {
		var eventID int64
		var state string
		if err := rows.Scan(&eventID, &state); err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		count++
		if count > b.limit {
			b.out.Truncated = true
			break
		}
		eventKey := fmt.Sprint(eventID)
		b.add("assessed_as", b.out.Center,
			b.node("assessment", eventKey, state, state),
			GraphBasis{
				RecordKind: "finding_assessment_event", RecordID: eventKey,
				Origin: "stored_record",
			})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("graph rows: %w", err)
	}
	return nil
}

func (s *Store) graphObservation(ctx context.Context, b *graphBuilder, id string) error {
	o, err := s.ObservationForTarget(ctx, b.target, id)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.out.Center.Label = o.Subject + " / " + o.Property
	b.out.Center.State = o.Presence
	runs, err := s.graphRunIDs(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.add("retains_evidence", b.out.Center,
		b.node("evidence", o.EvidenceID, o.EvidenceID, ""),
		graphObservationBasis(o, runs))
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT r.id,r.basis FROM finding_revision r
	 JOIN finding f ON f.id=r.finding_id
	 LEFT JOIN finding_basis b ON b.revision_id=r.id
	 WHERE f.target_id=$1 AND (r.observation_id=$2 OR b.observation_id=$2)
	 ORDER BY r.id LIMIT $3`, b.target, id, b.limit+1)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var revisionID, basis string
		if err := rows.Scan(&revisionID, &basis); err != nil {
			return fmt.Errorf("graph projection: %w", err)
		}
		count++
		if count > b.limit {
			b.out.Truncated = true
			break
		}
		b.add("used_by_revision", b.out.Center,
			b.node("revision", revisionID, basis, ""),
			graphObservationBasis(o, runs))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("graph rows: %w", err)
	}
	return nil
}

func (s *Store) graphEvidence(ctx context.Context, b *graphBuilder, id string) error {
	items, more, err := s.graphObservations(ctx, b.target, "evidence", id, b.limit, false)
	if err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	if len(items) == 0 {
		return pgx.ErrNoRows
	}
	b.out.Truncated = b.out.Truncated || more
	for _, item := range items {
		o := item.Observation
		b.add("supports_observation", b.out.Center,
			b.node("observation", o.ID, o.Subject+" / "+o.Property, o.Presence),
			graphObservationBasis(o, item.RunIDs))
	}
	return nil
}

func (s *Store) graphAssessment(ctx context.Context, b *graphBuilder, key string) error {
	eventID, err := strconv.ParseInt(key, 10, 64)
	if err != nil || eventID < 1 {
		return ErrInvalidGraphRequest
	}
	var state, revisionID string
	if err := s.pool.QueryRow(ctx, `SELECT e.state,e.revision_id
	 FROM finding_assessment_event e JOIN finding f ON f.id=e.finding_id
	 WHERE e.id=$1 AND f.target_id=$2`, eventID, b.target).
		Scan(&state, &revisionID); err != nil {
		return fmt.Errorf("graph projection: %w", err)
	}
	b.out.Center.Label = state
	b.out.Center.State = state
	b.add("assesses_revision", b.out.Center,
		b.node("revision", revisionID, revisionID, ""),
		GraphBasis{
			RecordKind: "finding_assessment_event", RecordID: key,
			Origin: "stored_record",
		})
	return nil
}
