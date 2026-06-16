package output

import (
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// projection_redaction.go 是 Pinax 所有渲染模式（default/json/agent/events/explain）
// 与 evidence sidecar 共享的脱敏门禁。渲染前先对 projection 做递归扫描，把受保护内容
// 替换成有界占位符，避免任何命令把 note body、token、Authorization、cookie、webhook、
// provider payload 或 raw/hidden prompt 泄漏到 stdout/stderr/evidence。
//
// 门禁只做“最后一道防线”的替换：命令层仍应尽量不产出受保护内容；这里保证即便某个
// 投影意外携带，也不会原样渲染出去。

var (
	// authorizationBearerPattern 匹配 Authorization: Bearer <token> 或单独的 Bearer <token>。
	authorizationBearerPattern = regexp.MustCompile(`(?i)Authorization\s*[:=]?\s*Bearer\s+[A-Za-z0-9._~+/=-]+`)
	bearerTokenPattern         = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	// tokenKVPattern 匹配 token=<value> / secret=<value> / api_key=<value> 等 KV。
	tokenKVPattern = regexp.MustCompile(`(?i)\b(token|secret|api_key|access_key|secret_key|password|cookie|webhook)(=|:\s*)[^\s&;]+`)
	// webhookURLPattern 匹配常见 webhook/callback URL。
	webhookURLPattern = regexp.MustCompile(`(?i)https?://[^\s]*(webhook|callback|hooks?)[^\s]*`)
	// providerPayloadPattern 匹配 provider payload 标记，避免输出原始 prompt/provider 包体。
	providerPayloadPattern = regexp.MustCompile(`(?i)\b(raw_prompt|hidden_prompt|system_prompt|provider_payload)\b[^\n]*`)
)

const (
	redactedValue        = "[REDACTED]"
	redactedPathValue    = "[REDACTED_PATH]"
	redactedWebhookValue = "[REDACTED_WEBHOOK]"
)

// sensitiveFieldNames 是承载 prompt/provider/凭证的字段名（大小写不敏感），
// 其值无论内容都必须脱敏，避免 raw/hidden system prompt、provider payload、token 或
// webhook URL 外泄。note 正文（body/content）由各命令的有界投影自行控制：preview/show
// 等命令会合法展示正文，而只读有界投影不产出正文——门禁不做全局清空以免误伤合法渲染。
var sensitiveFieldNames = map[string]bool{
	"raw_prompt": true, "hidden_prompt": true, "system_prompt": true, "provider_payload": true,
	"authorization": true, "cookie": true, "webhook_url": true,
	"api_key": true, "access_key": true, "secret_key": true, "secret": true, "password": true,
	"bearer_token": true, "session_token": true,
}

// ApplyProjectionRedaction 就地递归脱敏 projection 的全部渲染面。
// 它在 RenderWithOptions 渲染前调用，保证 default/json/agent/events/explain 与 evidence
// sidecar 都经过同一道门禁。返回的 projection 可安全渲染。
func ApplyProjectionRedaction(p *domain.Projection) {
	if p == nil {
		return
	}
	p.Summary = redactString(p.Summary)
	for key, value := range p.Facts {
		p.Facts[key] = redactString(value)
	}
	for idx, action := range p.Actions {
		action.Name = redactString(action.Name)
		action.Command = redactString(action.Command)
		p.Actions[idx] = action
	}
	for idx, evidence := range p.Evidence {
		p.Evidence[idx] = redactString(evidence)
	}
	if p.Data != nil {
		p.Data = redactValue(p.Data)
	}
	if p.Error != nil {
		p.Error.Message = redactString(p.Error.Message)
		p.Error.Hint = redactString(p.Error.Hint)
	}
}

// redactValue 递归脱敏任意 JSON 可序列化值（map/slice/string）。
func redactValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for key, child := range val {
			lk := strings.ToLower(key)
			if sensitiveFieldNames[lk] {
				val[key] = redactSensitiveField(child)
				continue
			}
			val[key] = redactValue(val[key])
		}
		return val
	case []any:
		for idx, child := range val {
			val[idx] = redactValue(child)
		}
		return val
	case []map[string]any:
		for idx, child := range val {
			val[idx] = redactValue(child).(map[string]any)
		}
		return val
	case string:
		return redactString(val)
	default:
		return v
	}
}

// redactSensitiveField 把敏感字段值整体替换为占位符（string 直接替换，其它类型递归脱敏）。
func redactSensitiveField(v any) any {
	if _, ok := v.(string); ok {
		return redactedValue
	}
	return redactValue(v)
}

// redactString 对单个字符串做受保护内容替换。
func redactString(s string) string {
	if s == "" {
		return s
	}
	out := authorizationBearerPattern.ReplaceAllString(s, "Authorization: Bearer "+redactedValue)
	out = bearerTokenPattern.ReplaceAllString(out, "Bearer "+redactedValue)
	out = tokenKVPattern.ReplaceAllStringFunc(out, func(m string) string {
		// 保留 key 和分隔符，只替换值。
		loc := tokenKVPattern.FindStringSubmatchIndex(m)
		if loc == nil {
			return redactedValue
		}
		key := m[loc[2]:loc[3]]
		sep := m[loc[4]:loc[5]]
		return key + sep + redactedValue
	})
	out = webhookURLPattern.ReplaceAllString(out, redactedWebhookValue)
	out = providerPayloadPattern.ReplaceAllStringFunc(out, func(m string) string {
		// 保留标记名，截断随后的内容。
		idx := regexp.MustCompile(`(?i)\b(raw_prompt|hidden_prompt|system_prompt|provider_payload)\b`).FindStringIndex(m)
		if idx == nil {
			return redactedValue
		}
		return m[:idx[1]] + "=" + redactedValue
	})
	return out
}
