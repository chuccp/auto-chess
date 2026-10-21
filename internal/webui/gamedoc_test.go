package webui_test

import (
	"encoding/json"
	"testing"

	"github.com/chuccp/auto-chess/internal/registry"
	"github.com/chuccp/auto-chess/internal/webui"
)

func TestGameDocsAreComplete(t *testing.T) {
	reg := registry.New()
	for _, meta := range reg.List() {
		engine, ok := reg.Get(meta.ID)
		if !ok {
			t.Fatalf("%s: engine not found", meta.ID)
		}
		doc := webui.BuildGameDoc(&meta, engine)
		if doc.RequestJSON == "" || doc.MoveResponseJSON == "" {
			t.Fatalf("%s: missing move examples", meta.ID)
		}
		if doc.ApplyRequestJSON == "" || doc.ApplyResponseJSON == "" {
			t.Fatalf("%s: missing apply examples", meta.ID)
		}
		if doc.LegalRequestJSON == "" || doc.LegalResponseJSON == "" {
			t.Fatalf("%s: missing legal-move examples", meta.ID)
		}
		if len(doc.Fields) < 5 || len(doc.Notes) < 3 {
			t.Fatalf("%s: docs too thin: fields=%d notes=%d", meta.ID, len(doc.Fields), len(doc.Notes))
		}

		var request map[string]any
		if err := json.Unmarshal([]byte(doc.RequestJSON), &request); err != nil {
			t.Fatalf("%s: request JSON: %v", meta.ID, err)
		}
		if request["game"] != meta.ID {
			t.Fatalf("%s: request game = %v", meta.ID, request["game"])
		}
		if _, ok := request["position"].(map[string]any); !ok {
			t.Fatalf("%s: request has no position object", meta.ID)
		}

		var apply map[string]any
		if err := json.Unmarshal([]byte(doc.ApplyResponseJSON), &apply); err != nil {
			t.Fatalf("%s: apply JSON: %v", meta.ID, err)
		}
		data, ok := apply["data"].(map[string]any)
		if !ok {
			t.Fatalf("%s: apply response has no data", meta.ID)
		}
		if _, ok := data["position"].(map[string]any); !ok {
			t.Fatalf("%s: apply response has no position", meta.ID)
		}
	}
}
