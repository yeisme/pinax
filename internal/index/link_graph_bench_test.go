package index

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/notelinks"
)

func BenchmarkLinkGraph100K(b *testing.B) {
	notes := make([]domain.Note, 1_000)
	for index := range notes {
		links := make([]string, 100)
		for link := range links {
			links[link] = fmt.Sprintf("[[Note %04d]]", (index+link+1)%len(notes))
		}
		notes[index] = domain.Note{ID: fmt.Sprintf("note-%04d", index), Title: fmt.Sprintf("Note %04d", index), Path: fmt.Sprintf("notes/%04d.md", index), Body: strings.Join(links, " ")}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		outgoing, _ := notelinks.BuildGraph(notes)
		if len(outgoing) != len(notes) {
			b.Fatalf("outgoing = %d", len(outgoing))
		}
	}
}
