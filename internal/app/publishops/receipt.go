package publishops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const PublishReceiptSchemaVersion = "pinax.publish_receipt.v1"

func WritePublishReceipt(root string, receipt domain.PublishReceipt) (string, error) {
	runID := strings.TrimSpace(receipt.RunID)
	if runID == "" || strings.ContainsAny(runID, `/\`) || strings.Contains(runID, "..") {
		return "", fmt.Errorf("publish receipt run id is unsafe")
	}
	receipt.SchemaVersion = PublishReceiptSchemaVersion
	rel := filepath.ToSlash(filepath.Join(".pinax", "publish", "runs", runID, "receipt.json"))
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".receipt-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return "", err
	}
	return rel, nil
}
