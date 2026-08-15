package index

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/index/model"
	"github.com/yeisme/pinax/internal/index/query"
)

func indexedTokenMatches(rows []*model.SearchTokenRecord, queryTokens []string) map[string]indexedTokenMatch {
	matches := map[string]indexedTokenMatch{}
	for _, row := range rows {
		objectID := firstNonEmptyIndexValue(row.ObjectID, row.NotePath)
		match := matches[objectID]
		if match.Fields == nil {
			match.Fields = map[string]bool{}
			match.Tokens = map[string]bool{}
		}
		match.Fields[row.Field] = true
		match.Tokens[row.Token] = true
		match.Score += row.Weight * row.Count * 10
		matches[objectID] = match
	}
	for path, match := range matches {
		if len(queryTokens) > 1 && len(match.Tokens) < len(queryTokens) {
			delete(matches, path)
			continue
		}
		if len(queryTokens) > 1 {
			match.Score += len(queryTokens) * 20
			matches[path] = match
		}
	}
	return matches
}

func indexedTokenMatchesForQuery(q *query.Query, ctx context.Context, queryTokens []string) (map[string]indexedTokenMatch, error) {
	if len(queryTokens) == 0 {
		return map[string]indexedTokenMatch{}, nil
	}
	orderedTokens, err := tokenLookupsByRarity(q, ctx, queryTokens)
	if err != nil {
		return nil, err
	}
	if len(orderedTokens) == 0 {
		return map[string]indexedTokenMatch{}, nil
	}
	rows := make([]*model.SearchTokenRecord, 0)
	candidateObjectIDs := []string(nil)
	for i, lookup := range orderedTokens {
		search := q.SearchTokenRecord.WithContext(ctx)
		if lookup.like {
			search = search.Where(q.SearchTokenRecord.Token.Like("%" + lookup.token + "%"))
		} else {
			search = search.Where(q.SearchTokenRecord.Token.Eq(lookup.token))
		}
		if i > 0 {
			if len(candidateObjectIDs) == 0 {
				return map[string]indexedTokenMatch{}, nil
			}
			search = search.Where(q.SearchTokenRecord.ObjectID.In(candidateObjectIDs...))
		}
		tokenRows, err := search.Find()
		if err != nil {
			return nil, err
		}
		if len(tokenRows) == 0 {
			return map[string]indexedTokenMatch{}, nil
		}
		rows = append(rows, tokenRows...)
		candidateObjectIDs = tokenRowObjectIDs(tokenRows)
	}
	return indexedTokenMatches(rows, queryTokens), nil
}

func tokenLookupsByRarity(q *query.Query, ctx context.Context, queryTokens []string) ([]tokenLookup, error) {
	type tokenCount struct {
		lookup tokenLookup
		count  int64
	}
	counts := make([]tokenCount, 0, len(queryTokens))
	for _, token := range queryTokens {
		count, err := q.SearchTokenRecord.WithContext(ctx).Where(q.SearchTokenRecord.Token.Eq(token)).Count()
		if err != nil {
			return nil, err
		}
		lookup := tokenLookup{token: token}
		if count == 0 || containsNonASCII(token) {
			likeCount, err := q.SearchTokenRecord.WithContext(ctx).Where(q.SearchTokenRecord.Token.Like("%" + token + "%")).Count()
			if err != nil {
				return nil, err
			}
			if likeCount == 0 {
				return nil, nil
			}
			if likeCount > count {
				count = likeCount
				lookup.like = true
			}
		}
		counts = append(counts, tokenCount{lookup: lookup, count: count})
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].count == counts[j].count {
			return counts[i].lookup.token < counts[j].lookup.token
		}
		return counts[i].count < counts[j].count
	})
	ordered := make([]tokenLookup, 0, len(counts))
	for _, item := range counts {
		ordered = append(ordered, item.lookup)
	}
	return ordered, nil
}

func containsNonASCII(value string) bool {
	for _, r := range value {
		if r > unicode.MaxASCII {
			return true
		}
	}
	return false
}

func tokenRowObjectIDs(rows []*model.SearchTokenRecord) []string {
	seen := map[string]bool{}
	objectIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		objectID := firstNonEmptyIndexValue(row.ObjectID, row.NotePath)
		if objectID == "" || seen[objectID] {
			continue
		}
		seen[objectID] = true
		objectIDs = append(objectIDs, objectID)
	}
	sort.Strings(objectIDs)
	return objectIDs
}

func indexedMatchObjectIDs(matches map[string]indexedTokenMatch) []string {
	objectIDs := make([]string, 0, len(matches))
	for objectID := range matches {
		objectIDs = append(objectIDs, objectID)
	}
	sort.Strings(objectIDs)
	return objectIDs
}

func noteRecordObjectIDs(records []NoteRecord) []string {
	objectIDs := make([]string, 0, len(records))
	for _, record := range records {
		objectIDs = append(objectIDs, record.ObjectID)
	}
	return objectIDs
}

func findTagsForObjectIDs(q *query.Query, ctx context.Context, objectIDs []string) ([]*model.TagRecord, error) {
	if len(objectIDs) == 0 {
		return nil, nil
	}
	return q.TagRecord.WithContext(ctx).Where(q.TagRecord.ObjectID.In(objectIDs...)).Find()
}

func findTextsForObjectIDs(q *query.Query, ctx context.Context, objectIDs []string) ([]*model.NoteTextRecord, error) {
	if len(objectIDs) == 0 {
		return nil, nil
	}
	return q.NoteTextRecord.WithContext(ctx).Where(q.NoteTextRecord.ObjectID.In(objectIDs...)).Find()
}

func findLinksForObjectIDs(q *query.Query, ctx context.Context, objectIDs []string) ([]*model.LinkRecord, error) {
	if len(objectIDs) == 0 {
		return nil, nil
	}
	return q.LinkRecord.WithContext(ctx).Where(q.LinkRecord.SourceObjectID.In(objectIDs...)).Find()
}

func findAttachmentsForObjectIDs(q *query.Query, ctx context.Context, objectIDs []string) ([]*model.AttachmentRecord, error) {
	if len(objectIDs) == 0 {
		return nil, nil
	}
	return q.AttachmentRecord.WithContext(ctx).Where(q.AttachmentRecord.ObjectID.In(objectIDs...)).Find()
}

func sortResults(items []ResultItem, mode string) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "relevance"
	}
	sort.Slice(items, func(i, j int) bool {
		a := items[i].Note
		b := items[j].Note
		switch mode {
		case "title":
			if a.Title == b.Title {
				return a.Path < b.Path
			}
			return a.Title < b.Title
		case "path":
			return a.Path < b.Path
		case "created":
			return timestampDesc(a.CreatedAt, b.CreatedAt, a.Path, b.Path)
		case "updated":
			return timestampDesc(a.UpdatedAt, b.UpdatedAt, a.Path, b.Path)
		default:
			if items[i].Score == items[j].Score {
				return a.Path < b.Path
			}
			return items[i].Score > items[j].Score
		}
	})
}

func timestampDesc(a, b, pathA, pathB string) bool {
	at, aErr := parseDate(a)
	bt, bErr := parseDate(b)
	if aErr != nil || bErr != nil || at.Equal(bt) {
		return pathA < pathB
	}
	return at.After(bt)
}

func tokens(text string) []string {
	tokens := make([]string, 0)
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, strings.ToLower(b.String()))
			b.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func snippet(text NoteTextRecord, query string) string {
	if query == "" {
		if text.Excerpt != "" {
			return text.Excerpt
		}
		return text.TitleText
	}
	haystack := text.BodyText
	idx := strings.Index(strings.ToLower(haystack), query)
	if idx < 0 {
		return text.TitleText
	}
	start := idx - 30
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 60
	if end > len(haystack) {
		end = len(haystack)
	}
	return strings.TrimSpace(haystack[start:end])
}

func scoreRecord(record NoteRecord, text NoteTextRecord, tags []string, query string) (int, []string) {
	if query == "" {
		return 1, []string{"filter"}
	}
	score := 0
	fields := make([]string, 0)
	if strings.Contains(strings.ToLower(record.Title), query) {
		score += 50
		fields = append(fields, "title")
	}
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			score += 30
			fields = append(fields, "tag")
			break
		}
	}
	if strings.Contains(strings.ToLower(record.Path), query) {
		score += 10
		fields = append(fields, "path")
	}
	if strings.Contains(strings.ToLower(text.BodyText), query) {
		score += 5
		fields = append(fields, "body")
	}
	return score, fields
}

func scoreIndexedRecord(record NoteRecord, text NoteTextRecord, tags []string, query string, tokenMatch indexedTokenMatch) (int, []string) {
	if query == "" {
		return scoreRecord(record, text, tags, query)
	}
	fields := map[string]bool{}
	score := tokenMatch.Score
	for field := range tokenMatch.Fields {
		fields[field] = true
	}
	containsScore, containsFields := scoreRecord(record, text, tags, query)
	score += containsScore
	for _, field := range containsFields {
		fields[field] = true
	}
	if score == 0 {
		return 0, nil
	}
	ordered := make([]string, 0, len(fields))
	for _, field := range []string{"title", "tag", "path", "body"} {
		if fields[field] {
			ordered = append(ordered, field)
		}
	}
	return score, ordered
}

func recordMatchesFilters(record NoteRecord, tags []string, links []LinkRecord, attachments []AttachmentRecord, req SearchRequest) bool {
	if req.Group != "" && record.Group != req.Group && record.Project != req.Group {
		return false
	}
	if req.Folder != "" && record.Folder != req.Folder {
		return false
	}
	if req.Kind != "" && record.Kind != req.Kind {
		return false
	}
	if req.Status != "" && record.Status != req.Status {
		return false
	}
	if req.CreatedAfter != "" && !timestampAfterOrEqual(record.CreatedAt, req.CreatedAfter) {
		return false
	}
	if req.UpdatedAfter != "" && !timestampAfterOrEqual(record.UpdatedAt, req.UpdatedAfter) {
		return false
	}
	for _, want := range req.Tags {
		if !containsTag(tags, want) {
			return false
		}
	}
	if req.LinkTarget != "" {
		found := false
		for _, link := range links {
			if strings.Contains(strings.ToLower(link.Target), strings.ToLower(req.LinkTarget)) || strings.Contains(strings.ToLower(link.TargetPath), strings.ToLower(req.LinkTarget)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if req.HasAttachment && len(attachments) == 0 {
		return false
	}
	return true
}

func timestampAfterOrEqual(value, boundary string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	valueTime, err := parseDate(value)
	if err != nil {
		return false
	}
	boundaryTime, err := parseDate(boundary)
	if err != nil {
		return false
	}
	return valueTime.Equal(boundaryTime) || valueTime.After(boundaryTime)
}

func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}

func containsTag(tags []string, want string) bool {
	want = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(want)), "#")
	for _, tag := range tags {
		if strings.ToLower(tag) == want {
			return true
		}
	}
	return false
}

func projectFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) >= 3 && parts[0] == "notes" {
		return parts[1]
	}
	return ""
}
