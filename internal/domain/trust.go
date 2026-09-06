package domain

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OKF 对齐的信任与生命周期信号。frontmatter 字段全部可选、additive；
// 派生分级（TrustTier/Freshness）只在消费时计算，绝不写回 vault。
const (
	TrustTierUnverified = "unverified"
	TrustTierMachine    = "machine"
	TrustTierHuman      = "human"

	FreshnessFresh = "fresh"
	FreshnessStale = "stale"

	// ActorPrefixHuman 是 human 分级判定前缀（同 OKF actor 约定）。
	ActorPrefixHuman = "human:"
)

// TrustActorEvent 记录一次 actor 事件（generated 或 verified 中的一个条目）。
type TrustActorEvent struct {
	By   string `json:"by"`
	At   string `json:"at"`
	Note string `json:"note,omitempty"`
}

// TrustSignals 是 note frontmatter 中信任/生命周期字段的类型化投影。
// 未知 actor 前缀与非时间戳字段不报错；非法时间戳由 ParseTrustSignals fail-closed。
type TrustSignals struct {
	Generated    TrustActorEvent   `json:"generated,omitempty"`
	HasGenerated bool              `json:"has_generated,omitempty"`
	Verified     []TrustActorEvent `json:"verified,omitempty"`
	StaleAfter   string            `json:"stale_after,omitempty"`
}

// IsZero 表示 note 未携带任何信任字段（存量 note 常态）。
func (t TrustSignals) IsZero() bool {
	return !t.HasGenerated && len(t.Verified) == 0 && strings.TrimSpace(t.StaleAfter) == ""
}

// TrustFieldError 指向具体 frontmatter 字段的解析错误（fail-closed）。
type TrustFieldError struct {
	Field  string
	Value  string
	Reason string
}

func (e *TrustFieldError) Error() string {
	return fmt.Sprintf("invalid trust field %s=%q: %s", e.Field, e.Value, e.Reason)
}

// IsHumanActor 报告 actor 是否为 human 前缀约定。
func IsHumanActor(actor string) bool {
	return strings.HasPrefix(strings.TrimSpace(actor), ActorPrefixHuman)
}

// ValidateTrustTimestamp 校验带 UTC offset 的 ISO8601 时间戳（RFC3339）。
func ValidateTrustTimestamp(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return &TrustFieldError{Field: field, Value: value, Reason: "timestamp is empty"}
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return &TrustFieldError{Field: field, Value: value, Reason: "expected ISO8601 with UTC offset (RFC3339)"}
	}
	return nil
}

// ParseTrustSignals 从完整 note 内容解析信任/生命周期字段。
//
// - 未携带字段返回零值 signals（存量 note 不受影响）。
// - bare mapping `verified: {by, at}` 兼容为单元素列表（同 OKF）。
// - 未知 actor 前缀保留原文，不报错。
// - generated.at / verified[].at / stale_after 非法时间戳返回 *TrustFieldError。
func ParseTrustSignals(content []byte) (TrustSignals, error) {
	signals := TrustSignals{}
	raw, ok := trustFrontmatterBlock(content)
	if !ok {
		return signals, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return signals, &TrustFieldError{Field: "frontmatter", Value: "", Reason: err.Error()}
	}
	root := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return signals, nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		value := root.Content[i+1]
		switch key {
		case "generated":
			event, hasBy, hasAt, err := trustEventFromMapping(value)
			if err != nil {
				return signals, err
			}
			if hasBy || hasAt {
				signals.HasGenerated = true
				signals.Generated = event
			}
		case "verified":
			events, err := trustEventsFromNode(value)
			if err != nil {
				return signals, err
			}
			signals.Verified = append(signals.Verified, events...)
		case "stale_after":
			if strings.TrimSpace(value.Value) == "" && value.Kind == yaml.ScalarNode {
				continue
			}
			if value.Kind != yaml.ScalarNode {
				continue
			}
			if err := ValidateTrustTimestamp("stale_after", value.Value); err != nil {
				return signals, err
			}
			signals.StaleAfter = strings.TrimSpace(value.Value)
		}
	}
	return signals, nil
}

// TrustTierOf 派生信任分级：verified 为空 ⇒ unverified；
// 存在事件但全部非 human: ⇒ machine；任一 human: ⇒ human。绝不存储。
func TrustTierOf(signals *TrustSignals) string {
	if signals == nil || len(signals.Verified) == 0 {
		return TrustTierUnverified
	}
	for _, event := range signals.Verified {
		if IsHumanActor(event.By) {
			return TrustTierHuman
		}
	}
	return TrustTierMachine
}

// FreshnessOf 派生新鲜度：now >= stale_after ⇒ stale，否则 fresh。
// 未携带 stale_after 的存量 note 一律 fresh（不惩罚存量）。
func FreshnessOf(signals *TrustSignals, now time.Time) string {
	if signals == nil {
		return FreshnessFresh
	}
	deadline := strings.TrimSpace(signals.StaleAfter)
	if deadline == "" {
		return FreshnessFresh
	}
	parsed, err := time.Parse(time.RFC3339, deadline)
	if err != nil {
		// 解析层已 fail-closed；这里防御性按 fresh 处理，避免消费路径 panic。
		return FreshnessFresh
	}
	if !now.Before(parsed) {
		return FreshnessStale
	}
	return FreshnessFresh
}

// LatestVerifiedAt 返回 verified 列表中按时间排序最新的 at（无事件返回空）。
func (signals TrustSignals) LatestVerifiedAt() string {
	best := ""
	var bestTime time.Time
	for _, event := range signals.Verified {
		at := strings.TrimSpace(event.At)
		if at == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, at)
		if err != nil {
			continue
		}
		if best == "" || parsed.After(bestTime) {
			best = at
			bestTime = parsed
		}
	}
	return best
}

// LatestHumanVerified 返回最新一条 human: 前缀验证事件。
func (signals TrustSignals) LatestHumanVerified() (TrustActorEvent, bool) {
	var best TrustActorEvent
	var bestTime time.Time
	found := false
	for _, event := range signals.Verified {
		if !IsHumanActor(event.By) {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(event.At))
		if err != nil {
			continue
		}
		if !found || parsed.After(bestTime) {
			best = event
			bestTime = parsed
			found = true
		}
	}
	return best, found
}

// FindVerifiedEventSameDay 返回同 actor 同 UTC 日已存在的验证事件（幂等判定）。
func (signals TrustSignals) FindVerifiedEventSameDay(actor string, now time.Time) (TrustActorEvent, bool) {
	actor = strings.TrimSpace(actor)
	day := now.UTC().Format("2006-01-02")
	for _, event := range signals.Verified {
		if strings.TrimSpace(event.By) != actor {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(event.At))
		if err != nil {
			continue
		}
		if parsed.UTC().Format("2006-01-02") == day {
			return event, true
		}
	}
	return TrustActorEvent{}, false
}

// AppendTrustVerifiedEvent 向 note 内容追加一条 verified 事件，保留其余 frontmatter 与正文。
// 返回更新后的完整内容。写入方必须走 atomic write。
func AppendTrustVerifiedEvent(content []byte, event TrustActorEvent) ([]byte, error) {
	if strings.TrimSpace(event.By) == "" {
		return nil, &TrustFieldError{Field: "verified[].by", Value: event.By, Reason: "actor is empty"}
	}
	if err := ValidateTrustTimestamp("verified[].at", event.At); err != nil {
		return nil, err
	}
	return patchTrustSignals(content, func(root *yaml.Node) {
		entry := trustEventMappingNode(event)
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value != "verified" {
				continue
			}
			value := root.Content[i+1]
			switch value.Kind {
			case yaml.SequenceNode:
				value.Content = append(value.Content, entry)
			case yaml.MappingNode:
				sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				sequence.Content = append(sequence.Content, value, entry)
				root.Content[i+1] = sequence
			default:
				sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				sequence.Content = append(sequence.Content, entry)
				root.Content[i+1] = sequence
			}
			return
		}
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "verified"}
		sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		sequence.Content = append(sequence.Content, entry)
		root.Content = append(root.Content, key, sequence)
	})
}

// EnsureTrustGenerated 在 generated 缺失时填充 by/at；staleAfter 非空且 stale_after 缺失时填充。
// 已存在的字段保持不变；返回更新后的完整内容与是否有变更。
func EnsureTrustGenerated(content []byte, by, at, staleAfter string) ([]byte, bool, error) {
	signals, err := ParseTrustSignals(content)
	if err != nil {
		return nil, false, err
	}
	needGenerated := !signals.HasGenerated && strings.TrimSpace(by) != ""
	needStale := strings.TrimSpace(signals.StaleAfter) == "" && strings.TrimSpace(staleAfter) != ""
	if !needGenerated && !needStale {
		return content, false, nil
	}
	if needGenerated {
		if err := ValidateTrustTimestamp("generated.at", at); err != nil {
			return nil, false, err
		}
	}
	if needStale {
		if err := ValidateTrustTimestamp("stale_after", staleAfter); err != nil {
			return nil, false, err
		}
	}
	updated, err := patchTrustSignals(content, func(root *yaml.Node) {
		if needGenerated {
			entry := trustEventMappingNode(TrustActorEvent{By: by, At: at})
			setRootMappingValue(root, "generated", entry)
		}
		if needStale {
			value := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: staleAfter}
			setRootMappingValue(root, "stale_after", value)
		}
	})
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func setRootMappingValue(root *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			root.Content[i+1] = value
			return
		}
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func trustEventMappingNode(event TrustActorEvent) *yaml.Node {
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	byKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "by"}
	byValue := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: event.By}
	atKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "at"}
	atValue := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: event.At}
	entry.Content = append(entry.Content, byKey, byValue, atKey, atValue)
	if note := strings.TrimSpace(event.Note); note != "" {
		noteKey := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "note"}
		noteValue := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: note}
		entry.Content = append(entry.Content, noteKey, noteValue)
	}
	return entry
}

// patchTrustSignals 解析 frontmatter YAML 节点树，应用 patch，再以 2 空格缩进渲染。
// 正文字节保持不变。
func patchTrustSignals(content []byte, patch func(root *yaml.Node)) ([]byte, error) {
	raw, bodyStart, ok := splitTrustFrontmatter(content)
	var root *yaml.Node
	if ok {
		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, &TrustFieldError{Field: "frontmatter", Value: "", Reason: err.Error()}
		}
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			root = doc.Content[0]
		}
	}
	if root == nil || root.Kind != yaml.MappingNode {
		root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	patch(root)
	var rendered bytes.Buffer
	encoder := yaml.NewEncoder(&rendered)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	frontmatter := strings.TrimRight(rendered.String(), "\n")
	if frontmatter == "" {
		frontmatter = "{}"
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.WriteString(frontmatter)
	out.WriteString("\n---\n")
	if ok {
		out.Write(content[bodyStart:])
	}
	return out.Bytes(), nil
}

// trustFrontmatterBlock 返回 frontmatter 原始 YAML 字节；无 frontmatter 时 ok=false。
func trustFrontmatterBlock(content []byte) ([]byte, bool) {
	raw, _, ok := splitTrustFrontmatter(content)
	return raw, ok
}

func splitTrustFrontmatter(content []byte) (raw []byte, bodyStart int, ok bool) {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return nil, 0, false
	}
	lines := bytes.SplitAfter(content, []byte("\n"))
	offset := len(lines[0])
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if bytes.Equal(bytes.TrimSpace(line), []byte("---")) {
			return content[:offset], offset + len(line), true
		}
		offset += len(line)
	}
	return nil, 0, false
}

func trustEventFromMapping(node *yaml.Node) (TrustActorEvent, bool, bool, error) {
	event := TrustActorEvent{}
	if node == nil {
		return event, false, false, nil
	}
	if node.Kind != yaml.MappingNode {
		return event, false, false, nil
	}
	hasBy := false
	hasAt := false
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]
		switch key {
		case "by":
			event.By = strings.TrimSpace(value.Value)
			hasBy = event.By != ""
		case "at":
			event.At = strings.TrimSpace(value.Value)
			hasAt = event.At != ""
		case "note":
			event.Note = strings.TrimSpace(value.Value)
		}
	}
	return event, hasBy, hasAt, nil
}

func trustEventsFromNode(node *yaml.Node) ([]TrustActorEvent, error) {
	if node == nil {
		return nil, nil
	}
	events := make([]TrustActorEvent, 0)
	appendEvent := func(value *yaml.Node, index int) error {
		event, hasBy, hasAt, err := trustEventFromMapping(value)
		if err != nil {
			return err
		}
		if !hasBy && !hasAt {
			return nil
		}
		if hasAt {
			field := fmt.Sprintf("verified[%d].at", index)
			if err := ValidateTrustTimestamp(field, event.At); err != nil {
				return err
			}
		}
		events = append(events, event)
		return nil
	}
	switch node.Kind {
	case yaml.SequenceNode:
		for i, item := range node.Content {
			if err := appendEvent(item, i); err != nil {
				return nil, err
			}
		}
	case yaml.MappingNode:
		if err := appendEvent(node, 0); err != nil {
			return nil, err
		}
	}
	return events, nil
}
