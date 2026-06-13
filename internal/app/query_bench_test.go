package app

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkQueryPlannerPagination(b *testing.B) {
	ctx := context.Background()
	root := b.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		status := "done"
		if i%2 == 0 {
			status = "active"
		}
		if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: fmt.Sprintf("Note %d", i), Body: fmt.Sprintf("priority:: %d", i%5), Status: status}); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		b.Fatal(err)
	}
	req := QueryRequest{VaultPath: root, SQL: `SELECT title, priority FROM notes WHERE status = "active" ORDER BY title ASC LIMIT 20`}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.QueryRun(ctx, req); err != nil {
			b.Fatal(err)
		}
	}
}
