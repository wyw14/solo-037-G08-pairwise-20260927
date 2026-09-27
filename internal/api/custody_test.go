package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wyw14/solo-037/internal/service"
)

func TestShipModelCustodyHTTPFlow(t *testing.T) {
	runtime, err := service.OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewRouter(runtime).Handler()

	registered := httptest.NewRecorder()
	server.ServeHTTP(registered, httptest.NewRequest(http.MethodPost, "/api/ship-models", strings.NewReader(`{"id":"SM-100","name":"hull A","home":"preparation"}`)))
	if registered.Code != http.StatusOK {
		t.Fatalf("register: %d %s", registered.Code, registered.Body)
	}

	offered := httptest.NewRecorder()
	server.ServeHTTP(offered, httptest.NewRequest(http.MethodPost, "/api/ship-models/SM-100/transfers", strings.NewReader(`{"to":"test_area","seal":"sealed","handed_over_by":"alice"}`)))
	if offered.Code != http.StatusOK {
		t.Fatalf("offer: %d %s", offered.Code, offered.Body)
	}
	var offerResponse struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(offered.Body.Bytes(), &offerResponse); err != nil || offerResponse.Data.ID == "" {
		t.Fatalf("offer response: %v %s", err, offered.Body)
	}

	conflict := httptest.NewRecorder()
	server.ServeHTTP(conflict, httptest.NewRequest(http.MethodPost, "/api/ship-models/SM-100/transfers", strings.NewReader(`{"to":"storage","seal":"sealed","handed_over_by":"carol"}`)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("unconfirmed model occupied by another test: %d %s", conflict.Code, conflict.Body)
	}

	confirmed := httptest.NewRecorder()
	server.ServeHTTP(confirmed, httptest.NewRequest(http.MethodPost, "/api/transfers/"+offerResponse.Data.ID+"/confirm", strings.NewReader(`{"received_by":"bob"}`)))
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", confirmed.Code, confirmed.Body)
	}

	afterConfirm := httptest.NewRecorder()
	server.ServeHTTP(afterConfirm, httptest.NewRequest(http.MethodPost, "/api/ship-models/SM-100/transfers", strings.NewReader(`{"to":"storage","seal":"opened","handed_over_by":"bob"}`)))
	if afterConfirm.Code != http.StatusOK {
		t.Fatalf("offer after confirm: %d %s", afterConfirm.Code, afterConfirm.Body)
	}

	chain := httptest.NewRecorder()
	server.ServeHTTP(chain, httptest.NewRequest(http.MethodGet, "/api/ship-models/SM-100/transfers", nil))
	if chain.Code != http.StatusOK || !strings.Contains(chain.Body.String(), `"received_by":"bob"`) {
		t.Fatalf("chain: %d %s", chain.Code, chain.Body)
	}

	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/ship-models/SM-999/transfers", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown model: %d %s", missing.Code, missing.Body)
	}
}

func TestShipModelOfferRejectsUnknownRun(t *testing.T) {
	runtime, err := service.OpenRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := NewRouter(runtime).Handler()
	if _, err := runtime.RegisterShipModel("SM-200", "hull B", "storage"); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/ship-models/SM-200/transfers", strings.NewReader(`{"to":"test_area","seal":"sealed","handed_over_by":"alice","run":"missing-run"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown associated test accepted: %d %s", recorder.Code, recorder.Body)
	}
}
