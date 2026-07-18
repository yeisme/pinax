package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestMetadataApplyWritesObjectRevisionReceiptWithoutBody(t *testing.T) {
	root := t.TempDir()
	path := "notes/alpha.md"
	writeAppFixture(t, filepath.Join(root, filepath.FromSlash(path)), "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-000000000051\ntitle: Alpha\n---\n\n# Alpha\n\nSECRET_BODY_SENTINEL\n")
	projection, err := NewService().ApplyMetadata(context.Background(), ApplyRequest{VaultPath: root, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	receipt := projection.Data.(map[string]any)["receipt"].(domain.ApplyReceipt)
	if receipt.ReceiptID == "" || receipt.LedgerSeq == 0 || len(receipt.Objects) != 1 || len(receipt.ChangedPaths) != 1 {
		t.Fatalf("receipt = %#v", receipt)
	}
	object := receipt.Objects[0]
	if object.ObjectID == "" || object.BeforeRevision.Hash == "" || object.AfterRevision.Hash == "" || object.AfterRecordVersion == 0 {
		t.Fatalf("receipt object = %#v", object)
	}
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(receipt.SavedPath)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "SECRET_BODY_SENTINEL") {
		t.Fatalf("receipt leaked note body: %s", payload)
	}
	var decoded domain.ApplyReceipt
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.SyncReady {
		t.Fatalf("sync readiness = %#v", decoded)
	}
}
