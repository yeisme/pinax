package pinaxclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncControlClientUsesOwnerRPCContract(t *testing.T) {
	seen := make([]RPCRequest, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected sync request: %s %s", r.Method, r.URL.Path)
		}
		var request RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, request)
		writeTestProjection(t, w, request.Method, nil)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SyncStatus(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SyncDiff(context.Background(), SyncRequest{ContentDiff: true}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0].Method != "Pinax.Sync.Status" || seen[1].Method != "Pinax.Sync.Diff" {
		t.Fatalf("sync RPC calls = %#v", seen)
	}
	if seen[1].Params["target"] != "capsa" || seen[1].Params["content_diff"] != true {
		t.Fatalf("sync diff params = %#v", seen[1].Params)
	}
}
