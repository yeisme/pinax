package delivery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFeishuWebhookDeliveryAndReceipt(t *testing.T) {
	root := t.TempDir()
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"StatusCode":0}`))
	}))
	defer server.Close()
	receipt, err := DeliverFeishu(context.Background(), root, FeishuRequest{WebhookURL: server.URL + "/hook/raw-token", SecretRef: "env://FEISHU_WEBHOOK", Title: "Daily briefing", Text: "AI tooling update", Yes: true})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if received["msg_type"] != "text" || receipt.Status != "delivered" {
		t.Fatalf("received=%#v receipt=%#v", received, receipt)
	}
	if strings.Contains(receipt.Webhook, "raw-token") || strings.Contains(receipt.SecretRef, "FEISHU_WEBHOOK") {
		t.Fatalf("receipt leaked secret: %#v", receipt)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "briefing", "delivery-receipts", receipt.ReceiptID+".json")); err != nil {
		t.Fatalf("receipt asset missing: %v", err)
	}
}

func TestFeishuDryRunDoesNotPost(t *testing.T) {
	root := t.TempDir()
	receipt, err := DeliverFeishu(context.Background(), root, FeishuRequest{WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/raw-token", SecretRef: "env://FEISHU_WEBHOOK", Title: "Daily briefing", Text: "AI tooling update", DryRun: true})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if receipt.Status != "dry_run" || receipt.RemoteWrite {
		t.Fatalf("dry-run receipt = %#v", receipt)
	}
}
