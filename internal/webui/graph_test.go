package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

func TestGraphHTTPIsReadOnlyScopedAndBounded(t *testing.T) {
	store := resultTestStore(t)
	data := []byte("graph HTTP image")
	artifact := model.Artifact{
		ID: model.Digest(data), SHA256: model.Digest(data),
		Size: int64(len(data)),
	}
	target := model.Target{
		ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID,
		Build: "18A373", Board: "n104ap",
	}
	other := model.Target{
		ID: model.ID(artifact.ID, "1"), ArtifactID: artifact.ID,
		Build: "18A373", Board: "n104ap",
	}
	if err := store.Register(context.Background(), artifact, []model.Target{target, other}); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/graph?target_id=" + url.QueryEscape(target.ID) + "&limit=1"
	status, body := resultRequest(t, store, path)
	if status != http.StatusOK {
		t.Fatalf("graph status %d: %s", status, body)
	}
	var envelope struct {
		ReadOnly bool                       `json:"read_only"`
		Data     postgres.GraphNeighborhood `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.ReadOnly || len(envelope.Data.Edges) != 1 ||
		envelope.Data.Edges[0].Kind != "belongs_to_artifact" ||
		len(envelope.Data.Gaps) != 1 || envelope.Data.Gaps[0].State != "unexamined" {
		t.Fatalf("graph response: %+v", envelope)
	}
	wrong := path + "&node_id=" + url.QueryEscape(postgres.GraphID(other.ID, "target", other.ID))
	if status, _ := resultRequest(t, store, wrong); status != http.StatusBadRequest {
		t.Fatalf("cross-target node status %d", status)
	}
	for _, invalid := range []string{
		path + "&limit=2", path + "&unexpected=1",
		"/api/v1/graph?target_id=" + target.ID + "&limit=101",
	} {
		if status, _ := resultRequest(t, store, invalid); status != http.StatusBadRequest {
			t.Fatalf("invalid graph filter status %d: %s", status, invalid)
		}
	}
	if status, _ := resultRequest(t, store, "/api/v1/graph?target_id=missing"); status != http.StatusNotFound {
		t.Fatalf("missing target status %d", status)
	}
}
