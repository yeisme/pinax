package records

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func BenchmarkIdentityLedgerReplay10K(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, ".pinax", "records", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	writer := bufio.NewWriter(file)
	for index := 0; index < 10_000; index++ {
		objectID := fmt.Sprintf("01982d84-%04x-7000-8000-%012x", index&0xffff, index)
		event := domain.RecordEvent{SchemaVersion: EventSchemaVersion, EventID: fmt.Sprintf("event-%d", index), Seq: uint64(index + 1), IdempotencyKey: fmt.Sprintf("create-%d", index), Kind: domain.RecordEventNoteCreated, ObjectID: objectID, ObjectKind: "note", NoteID: objectID, Path: fmt.Sprintf("notes/%05d.md", index), CurrentPath: fmt.Sprintf("notes/%05d.md", index)}
		payload, _ := json.Marshal(event)
		_, _ = writer.Write(append(payload, '\n'))
	}
	if err := writer.Flush(); err != nil {
		b.Fatal(err)
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}
	service := NewService(root)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		state, err := service.ReplayReadOnly(context.Background())
		if err != nil || len(state.Records) != 10_000 {
			b.Fatalf("records=%d err=%v", len(state.Records), err)
		}
	}
}
