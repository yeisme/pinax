package remote

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestFakeServerStoresManifestAndBlob(t *testing.T) {
	server := NewFakeServer()
	t.Cleanup(server.Close)
	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	putManifest := FakeManifestPutRequest{BaseRevision: "", Manifest: map[string]any{"ciphertext": "encrypted-manifest"}}
	body, _ := json.Marshal(putManifest)
	resp, err = http.Post(server.URL+"/v1/workspaces/ws_123/manifest", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("put manifest: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put manifest status = %d", resp.StatusCode)
	}
	var putOut FakeManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&putOut); err != nil {
		t.Fatalf("decode put: %v", err)
	}
	_ = resp.Body.Close()
	if putOut.Revision == "" {
		t.Fatalf("revision missing: %#v", putOut)
	}
	resp, err = http.Get(server.URL + "/v1/workspaces/ws_123/manifest")
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	var got FakeManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	_ = resp.Body.Close()
	if got.Revision != putOut.Revision || got.Manifest["ciphertext"] != "encrypted-manifest" {
		t.Fatalf("manifest = %#v", got)
	}
	blobReq, _ := http.NewRequest(http.MethodPut, server.URL+"/v1/workspaces/ws_123/blobs/blob_1", bytes.NewBufferString("encrypted-blob"))
	resp, err = http.DefaultClient.Do(blobReq)
	if err != nil {
		t.Fatalf("put blob: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put blob status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	resp, err = http.Get(server.URL + "/v1/workspaces/ws_123/blobs/blob_1")
	if err != nil {
		t.Fatalf("get blob: %v", err)
	}
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	_ = resp.Body.Close()
	if buf.String() != "encrypted-blob" {
		t.Fatalf("blob = %q", buf.String())
	}
}

func TestFakeServerRevisionConflict(t *testing.T) {
	server := NewFakeServer()
	t.Cleanup(server.Close)
	first, _ := json.Marshal(FakeManifestPutRequest{BaseRevision: "", Manifest: map[string]any{"ciphertext": "one"}})
	resp, err := http.Post(server.URL+"/v1/workspaces/ws_123/manifest", "application/json", bytes.NewReader(first))
	if err != nil {
		t.Fatalf("first put: %v", err)
	}
	_ = resp.Body.Close()
	stale, _ := json.Marshal(FakeManifestPutRequest{BaseRevision: "stale", Manifest: map[string]any{"ciphertext": "two"}})
	resp, err = http.Post(server.URL+"/v1/workspaces/ws_123/manifest", "application/json", bytes.NewReader(stale))
	if err != nil {
		t.Fatalf("stale put: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale put status = %d", resp.StatusCode)
	}
	var conflict FakeErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&conflict); err != nil {
		t.Fatalf("decode conflict: %v", err)
	}
	if conflict.Code != "REVISION_CONFLICT" || conflict.CurrentRevision == "" {
		t.Fatalf("conflict = %#v", conflict)
	}
}
