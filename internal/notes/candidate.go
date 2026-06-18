package notes

import (
	"fmt"
	"strings"
)

type BriefingCandidate struct {
	Title     string
	URL       string
	Summary   string
	Topic     string
	Tags      []string
	Backlinks []string
}

func RenderBriefingCandidateMarkdown(candidate BriefingCandidate) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: pinax.note.v1\n")
	b.WriteString("title: ")
	b.WriteString(candidate.Title)
	b.WriteString("\nkind: briefing_candidate\n")
	b.WriteString("status: review\n")
	b.WriteString("tags: ")
	b.WriteString(formatTags(candidate.Tags))
	b.WriteString("\nsource_url: ")
	b.WriteString(candidate.URL)
	b.WriteString("\n---\n\n")
	b.WriteString("# ")
	b.WriteString(candidate.Title)
	b.WriteString("\n\n")
	if candidate.Topic != "" {
		b.WriteString("Topic: ")
		b.WriteString(candidate.Topic)
		b.WriteString("\n\n")
	}
	b.WriteString(candidate.Summary)
	b.WriteString("\n\n")
	if len(candidate.Backlinks) > 0 {
		b.WriteString("## Related\n\n")
		for _, backlink := range candidate.Backlinks {
			b.WriteString(fmt.Sprintf("- [[%s]]\n", backlink))
		}
	}
	return b.String()
}

func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	return "[" + strings.Join(tags, ", ") + "]"
}
