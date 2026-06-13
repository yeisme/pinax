package delivery

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ReceiptSchemaVersion = "pinax.delivery.receipt.v1"

type FeishuRequest struct {
	WebhookURL string
	SecretRef  string
	Title      string
	Text       string
	DryRun     bool
	Yes        bool
}

type Receipt struct {
	SchemaVersion string `json:"schema_version"`
	ReceiptID     string `json:"receipt_id"`
	Status        string `json:"status"`
	Provider      string `json:"provider"`
	Webhook       string `json:"webhook"`
	SecretRef     string `json:"secret_ref"`
	RemoteWrite   bool   `json:"remote_write"`
	CreatedAt     string `json:"created_at"`
}

func DeliverFeishu(ctx context.Context, root string, req FeishuRequest) (Receipt, error) {
	if strings.TrimSpace(req.WebhookURL) == "" {
		return Receipt{}, fmt.Errorf("feishu webhook required")
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = "Pinax Briefing"
	}
	if strings.TrimSpace(req.Text) == "" {
		req.Text = req.Title
	}
	now := time.Now().UTC()
	receipt := Receipt{SchemaVersion: ReceiptSchemaVersion, ReceiptID: receiptID(req.WebhookURL, now), Status: "dry_run", Provider: "feishu", Webhook: RedactWebhook(req.WebhookURL), SecretRef: RedactSecretRef(req.SecretRef), RemoteWrite: false, CreatedAt: now.Format(time.RFC3339)}
	if req.DryRun || !req.Yes {
		return receipt, nil
	}
	payload := map[string]any{"msg_type": "text", "content": map[string]string{"text": req.Title + "\n" + req.Text}}
	b, err := json.Marshal(payload)
	if err != nil {
		return Receipt{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.WebhookURL, bytes.NewReader(b))
	if err != nil {
		return Receipt{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return Receipt{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return Receipt{}, fmt.Errorf("feishu webhook status %d", resp.StatusCode)
	}
	receipt.Status = "delivered"
	receipt.RemoteWrite = true
	return receipt, writeReceipt(root, receipt)
}

func RedactWebhook(raw string) string {
	if before, _, ok := strings.Cut(raw, "/hook/"); ok {
		return before + "/hook/[REDACTED]"
	}
	return "[REDACTED_WEBHOOK]"
}

func RedactSecretRef(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return "[REDACTED_SECRET_REF]"
}

func writeReceipt(root string, receipt Receipt) error {
	path := filepath.Join(root, ".pinax", "briefing", "delivery-receipts", receipt.ReceiptID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func receiptID(webhook string, t time.Time) string {
	h := sha1.Sum([]byte(webhook + "\x00" + t.Format(time.RFC3339Nano)))
	return "deliv_" + hex.EncodeToString(h[:])[:16]
}
