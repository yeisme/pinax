package index

import (
	"fmt"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func BenchmarkPropertyExtraction(b *testing.B) {
	notes := benchmarkNotes(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractPropertyRows(notes)
	}
}

func BenchmarkQueryLikePropertyFilter(b *testing.B) {
	rows := ExtractPropertyRows(benchmarkNotes(1000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for _, row := range rows {
			if row.Values["status"].String() == "active" {
				count++
			}
		}
		if count == 0 {
			b.Fatal("missing active rows")
		}
	}
}

func benchmarkNotes(n int) []domain.Note {
	notes := make([]domain.Note, 0, n)
	for i := 0; i < n; i++ {
		status := "done"
		if i%2 == 0 {
			status = "active"
		}
		notes = append(notes, domain.Note{ID: fmt.Sprintf("note_%d", i), Title: fmt.Sprintf("Note %d", i), Path: fmt.Sprintf("notes/%d.md", i), Status: status, Tags: []string{"bench"}, Body: fmt.Sprintf("priority:: %d\ndue:: 2026-06-%02d\n", i%5, (i%28)+1)})
	}
	return notes
}
