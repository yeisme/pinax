package app

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/app/syncops"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

const (
	syncContentDiffFileLimit = 10
	syncContentDiffFileBytes = 64 * 1024
	syncContentDiffRunBytes  = 256 * 1024
)

type syncContentDiffPayload struct {
	SchemaVersion string                `json:"schema_version"`
	Files         []syncContentDiffFile `json:"files,omitempty"`
	FileCount     int                   `json:"file_count"`
	Shown         int                   `json:"shown"`
	TotalLines    int                   `json:"total_lines"`
	TotalBytes    int                   `json:"total_bytes"`
	Truncated     bool                  `json:"truncated"`
}

type syncContentDiffFile struct {
	Path      string   `json:"path,omitempty"`
	Hunks     []string `json:"hunks,omitempty"`
	Added     int      `json:"added"`
	Removed   int      `json:"removed"`
	LineCount int      `json:"line_count"`
	Bytes     int      `json:"bytes"`
	Truncated bool     `json:"truncated"`
}

var sensitiveDiffLine = regexp.MustCompile(`(?i)(password|passphrase|secret|token|authorization|access[_-]?key|private[_-]?key)\s*[:=]`)

func buildSyncContentDiff(root string, plan syncplan.Plan, base, local, remote pinaxcloud.Manifest, pathPolicy string) syncContentDiffPayload {
	payload := syncContentDiffPayload{SchemaVersion: "pinax.sync.content-diff.v1"}
	for _, operation := range plan.Operations {
		if isManifestOperation(operation.Kind) || len(payload.Files) >= syncContentDiffFileLimit {
			if !isManifestOperation(operation.Kind) && len(payload.Files) >= syncContentDiffFileLimit {
				payload.Truncated = true
			}
			continue
		}
		before, after := syncContentDiffSides(root, operation, base, local, remote)
		if bytes.Equal(before, after) {
			continue
		}
		file := buildSyncContentDiffFile(operation, before, after, pathPolicy)
		if file.Bytes == 0 && len(file.Hunks) == 0 {
			continue
		}
		remaining := syncContentDiffRunBytes - payload.TotalBytes
		if remaining <= 0 {
			payload.Truncated = true
			break
		}
		if file.Bytes > remaining {
			file.Hunks = truncateDiffLines(file.Hunks, remaining)
			file.Bytes = diffBytes(file.Hunks)
			file.Truncated = true
			payload.Truncated = true
		}
		payload.Files = append(payload.Files, file)
		payload.TotalBytes += file.Bytes
		payload.TotalLines += file.LineCount
	}
	payload.FileCount = len(payload.Files)
	payload.Shown = len(payload.Files)
	return payload
}

func syncContentDiffSides(root string, operation syncplan.Operation, base, local, remote pinaxcloud.Manifest) ([]byte, []byte) {
	path := operation.Path
	if path == "" {
		path = operation.ToPath
	}
	switch operation.Kind {
	case "move":
		return readSyncLocalContent(root, syncManifestEntryForPath(base, operation.FromPath)), readSyncLocalContent(root, syncManifestEntryForPath(local, operation.ToPath))
	case "delete_remote":
		return readSyncContent(root, syncManifestEntryForPath(local, path)), nil
	case "delete_local":
		return readSyncLocalContent(root, syncManifestEntryForPath(local, path)), readSyncBlobContent(root, syncManifestEntryForPath(remote, path))
	case "upload_blob":
		return readSyncBlobContent(root, syncManifestEntryForPath(base, path)), readSyncLocalContent(root, syncManifestEntryForPath(local, path))
	case "download_blob":
		return readSyncLocalContent(root, syncManifestEntryForPath(local, path)), readSyncBlobContent(root, syncManifestEntryForPath(remote, path))
	case "conflict", "revision_conflict", "path_collision":
		return readSyncBlobContent(root, syncManifestEntryForPath(base, path)), readSyncLocalContent(root, syncManifestEntryForPath(local, path))
	default:
		return nil, nil
	}
}

func syncManifestEntryForPath(manifest pinaxcloud.Manifest, path string) pinaxcloud.ManifestEntry {
	for _, entry := range manifest.Entries {
		if entry.Path == path || entry.PathHash == path {
			return entry
		}
	}
	return pinaxcloud.ManifestEntry{Path: path}
}

func readSyncContent(root string, entry pinaxcloud.ManifestEntry) []byte {
	path := strings.TrimSpace(entry.Path)
	if path != "" {
		if full, err := safeJoin(root, path); err == nil {
			if content, err := os.ReadFile(full); err == nil {
				return content
			}
		}
	}
	if entry.BlobID != "" {
		content, _ := os.ReadFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", entry.BlobID))
		return content
	}
	return nil
}

func readSyncLocalContent(root string, entry pinaxcloud.ManifestEntry) []byte {
	if entry.Path != "" {
		if full, err := safeJoin(root, entry.Path); err == nil {
			if content, err := os.ReadFile(full); err == nil {
				return content
			}
		}
	}
	return readSyncBlobContent(root, entry)
}

func readSyncBlobContent(root string, entry pinaxcloud.ManifestEntry) []byte {
	if entry.BlobID == "" {
		return nil
	}
	content, _ := os.ReadFile(filepath.Join(root, ".pinax", "cloud", "blob-cache", entry.BlobID))
	return content
}

func buildSyncContentDiffFile(operation syncplan.Operation, before, after []byte, pathPolicy string) syncContentDiffFile {
	path := operation.Path
	if path == "" {
		path = operation.ToPath
	}
	if operation.FromPath != "" && operation.ToPath != "" {
		path = operation.FromPath + " -> " + operation.ToPath
	}
	path = syncRedactDiffPath(path, pathPolicy)
	before = truncateContent(before, syncContentDiffFileBytes)
	after = truncateContent(after, syncContentDiffFileBytes)
	oldLines := redactDiffLines(splitDiffLines(before))
	newLines := redactDiffLines(splitDiffLines(after))
	hunks := []string{"@@"}
	for _, line := range oldLines {
		hunks = append(hunks, "-"+line)
	}
	for _, line := range newLines {
		hunks = append(hunks, "+"+line)
	}
	truncated := len(before) >= syncContentDiffFileBytes || len(after) >= syncContentDiffFileBytes
	if diffBytes(hunks) > syncContentDiffFileBytes {
		hunks = truncateDiffLines(hunks, syncContentDiffFileBytes)
		truncated = true
	}
	return syncContentDiffFile{Path: path, Hunks: hunks, Added: len(newLines), Removed: len(oldLines), LineCount: len(hunks), Bytes: diffBytes(hunks), Truncated: truncated}
}

func truncateContent(content []byte, limit int) []byte {
	if len(content) <= limit {
		return content
	}
	return content[:limit]
}

func splitDiffLines(content []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) == 0 && len(content) > 0 {
		return []string{string(content)}
	}
	return lines
}

func redactDiffLines(lines []string) []string {
	for i, line := range lines {
		if sensitiveDiffLine.MatchString(line) {
			lines[i] = "[REDACTED]"
		}
	}
	return lines
}

func diffBytes(lines []string) int {
	total := 0
	for _, line := range lines {
		total += len(line) + 1
	}
	return total
}

func truncateDiffLines(lines []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	total := 0
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		next := total + len(line) + 1
		if next > limit {
			break
		}
		out = append(out, line)
		total = next
	}
	if len(out) == 0 {
		return []string{fmt.Sprintf("... diff truncated at %d bytes", limit)}
	}
	return out
}

func syncRedactDiffPath(path, policy string) string {
	if strings.EqualFold(strings.TrimSpace(policy), "omitted") {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(policy), "hash") {
		return syncops.RedactPath(path, policy)
	}
	return path
}
