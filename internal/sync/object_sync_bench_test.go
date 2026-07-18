package syncplan

import (
	"fmt"
	"testing"

	"github.com/yeisme/pinax/internal/remote"
)

func BenchmarkObjectSync10K(b *testing.B) {
	baseEntries := make([]remote.ManifestEntry, 10_000)
	localEntries := make([]remote.ManifestEntry, 10_000)
	remoteEntries := make([]remote.ManifestEntry, 10_000)
	for index := range baseEntries {
		objectID := fmt.Sprintf("01982d84-%04x-7000-8000-%012x", index&0xffff, index)
		baseEntries[index] = remote.ManifestEntry{ObjectID: objectID, ObjectKind: "note", Path: fmt.Sprintf("notes/%05d.md", index), RevisionID: "r1", BlobID: fmt.Sprintf("b1-%d", index)}
		localEntries[index] = baseEntries[index]
		remoteEntries[index] = baseEntries[index]
		if index%100 == 0 {
			remoteEntries[index].Path = fmt.Sprintf("archive/%05d.md", index)
			remoteEntries[index].RevisionID = "r2"
			remoteEntries[index].BlobID = fmt.Sprintf("b2-%d", index)
		}
	}
	request := Request{Direction: DirectionPull, BaseManifest: remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: baseEntries}, LocalManifest: remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: localEntries}, RemoteManifest: remote.Manifest{SchemaVersion: remote.ManifestSchemaVersionV2, Entries: remoteEntries}, Yes: true}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		plan, err := BuildPlan(request)
		if err != nil || len(plan.Operations) != 100 {
			b.Fatalf("operations=%d err=%v", len(plan.Operations), err)
		}
	}
}
