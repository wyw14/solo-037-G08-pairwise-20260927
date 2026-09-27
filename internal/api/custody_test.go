package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wyw14/solo-037/internal/service"
)

func postJSON(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return recorder
}

func TestShipModelCustodyHTTPFlow(t *testing.T) {
	runtime, err := service.OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewRouter(runtime).Handler()

	registered := postJSON(t, server, "/api/ship-models", `{"entity_no":"SM-100","name":"hull 100"}`)
	if registered.Code != http.StatusOK {
		t.Fatalf("register: %d %s", registered.Code, registered.Body)
	}

	created := postJSON(t, server, "/api/measured-runs", `{"name":"tow 1","model_name":"hull 100","section":"section-a","carriage":"carriage-1","model_entity":"SM-100"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("create bound run: %d %s", created.Code, created.Body)
	}
	var runResponse struct {
		Data service.MeasuredRun `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &runResponse); err != nil {
		t.Fatal(err)
	}
	runID := string(runResponse.Data.Operation.ID)

	// The active run occupies the model: a second bound run conflicts.
	conflict := postJSON(t, server, "/api/measured-runs", `{"name":"tow 2","model_name":"hull 100","section":"section-b","carriage":"carriage-2","model_entity":"SM-100"}`)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d %s", conflict.Code, conflict.Body)
	}

	offered := postJSON(t, server, "/api/ship-models/SM-100/handovers", `{"to":"testing","packaging":"sealed","handed_over_by":"operator-li","run":"`+runID+`"}`)
	if offered.Code != http.StatusOK {
		t.Fatalf("offer: %d %s", offered.Code, offered.Body)
	}
	var handoverResponse struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(offered.Body.Bytes(), &handoverResponse); err != nil {
		t.Fatal(err)
	}

	// The pending handover blocks new runs even after the first run closes.
	closed := postJSON(t, server, "/api/measured-runs/"+runID+"/close", `{"outcome":"complete"}`)
	if closed.Code != http.StatusOK {
		t.Fatalf("close: %d %s", closed.Code, closed.Body)
	}
	blocked := postJSON(t, server, "/api/measured-runs", `{"name":"tow 3","model_name":"hull 100","section":"section-a","carriage":"carriage-1","model_entity":"SM-100"}`)
	if blocked.Code != http.StatusConflict {
		t.Fatalf("expected 409 for pending handover, got %d %s", blocked.Code, blocked.Body)
	}

	confirmed := postJSON(t, server, "/api/handovers/"+handoverResponse.Data.ID+"/confirm", `{"received_by":"operator-wang"}`)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", confirmed.Code, confirmed.Body)
	}
	free := postJSON(t, server, "/api/measured-runs", `{"name":"tow 3","model_name":"hull 100","section":"section-a","carriage":"carriage-1","model_entity":"SM-100"}`)
	if free.Code != http.StatusOK {
		t.Fatalf("expected create after confirm, got %d %s", free.Code, free.Body)
	}

	chain := httptest.NewRecorder()
	server.ServeHTTP(chain, httptest.NewRequest(http.MethodGet, "/api/ship-models/SM-100/handovers", nil))
	if chain.Code != http.StatusOK || !strings.Contains(chain.Body.String(), "operator-wang") {
		t.Fatalf("chain: %d %s", chain.Code, chain.Body)
	}
	models := httptest.NewRecorder()
	server.ServeHTTP(models, httptest.NewRequest(http.MethodGet, "/api/ship-models", nil))
	if models.Code != http.StatusOK || !strings.Contains(models.Body.String(), `"location":"testing"`) {
		t.Fatalf("models: %d %s", models.Code, models.Body)
	}

	missing := postJSON(t, server, "/api/ship-models/GHOST/handovers", `{"to":"storage","packaging":"sealed","handed_over_by":"operator-li"}`)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", missing.Code, missing.Body)
	}
}
