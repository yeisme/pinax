package templateengine

import (
	"regexp"
	"strings"
)

type ManagedBlock struct {
	Name         string `json:"name"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	ContentStart int    `json:"-"`
	ContentEnd   int    `json:"-"`
}

var (
	managedBlockOpenPattern  = regexp.MustCompile(`<!--\s*pinax:managed\s+name=([A-Za-z0-9_.:-]+)\s*-->`)
	managedBlockClosePattern = regexp.MustCompile(`<!--\s*/pinax:managed\s*-->`)
)

func InspectManagedBlocks(body string) ([]ManagedBlock, error) {
	blocks := []ManagedBlock{}
	seen := map[string]bool{}
	pos := 0
	for {
		open := managedBlockOpenPattern.FindStringSubmatchIndex(body[pos:])
		if open == nil {
			return blocks, nil
		}
		start := pos + open[0]
		contentStart := pos + open[1]
		name := body[pos+open[2] : pos+open[3]]
		close := managedBlockClosePattern.FindStringIndex(body[contentStart:])
		if close == nil {
			return nil, &Error{Code: "managed_block_unclosed", Message: "托管区块缺少结束标记"}
		}
		contentEnd := contentStart + close[0]
		end := contentStart + close[1]
		if seen[name] {
			return nil, &Error{Code: "managed_block_ambiguous", Message: "托管区块名称重复: " + name}
		}
		seen[name] = true
		blocks = append(blocks, ManagedBlock{Name: name, Start: start, End: end, ContentStart: contentStart, ContentEnd: contentEnd})
		pos = end
	}
}

func ReplaceManagedBlock(body, name, replacement string) (string, error) {
	blocks, err := InspectManagedBlocks(body)
	if err != nil {
		return "", err
	}
	for _, block := range blocks {
		if block.Name != name {
			continue
		}
		// 只替换托管区块内部内容，保留用户正文和边界标记；缺失/重复/未闭合时 fail closed，避免猜测写入位置。
		return body[:block.ContentStart] + managedBlockReplacement(replacement) + body[block.ContentEnd:], nil
	}
	return "", &Error{Code: "managed_block_missing", Message: "未找到托管区块: " + name}
}

func managedBlockReplacement(replacement string) string {
	replacement = strings.Trim(replacement, "\n")
	if replacement == "" {
		return "\n"
	}
	return "\n" + replacement + "\n"
}
