package search

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/yeisme/pinax/internal/domain"
)

type Result struct {
	Engine string        `json:"engine"`
	Notes  []domain.Note `json:"notes"`
}

func Notes(ctx context.Context, root, query string, fallback []domain.Note) Result {
	_ = root
	if strings.TrimSpace(query) == "" {
		return Result{Engine: "native", Notes: fallback}
	}
	select {
	case <-ctx.Done():
		return Result{Engine: "native", Notes: []domain.Note{}}
	default:
	}
	return Result{Engine: "native", Notes: scanNotes(ctx, query, fallback)}
}

func scanNotes(ctx context.Context, query string, notes []domain.Note) []domain.Note {
	query = strings.ToLower(query)
	if len(notes) == 0 {
		return nil
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan domain.Note)
	matches := make(chan domain.Note, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for note := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if noteMatches(query, note) {
					select {
					case matches <- note:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, note := range notes {
			select {
			case jobs <- note:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(matches)
	}()
	matched := make([]domain.Note, 0)
	for note := range matches {
		matched = append(matched, note)
	}
	if ctx.Err() != nil {
		return []domain.Note{}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Path < matched[j].Path })
	return matched
}

func noteMatches(query string, note domain.Note) bool {
	haystack := strings.ToLower(note.Title + "\n" + strings.Join(note.Tags, " ") + "\n" + note.Body)
	return strings.Contains(haystack, query)
}
