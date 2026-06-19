package redaction

import "regexp"

type SensitiveClass string

const (
	SensitiveAuthorization SensitiveClass = "authorization_header"
	SensitiveCookie        SensitiveClass = "cookie_header"
	SensitiveWebhook       SensitiveClass = "webhook_url"
	SensitiveProvider      SensitiveClass = "provider_payload"
	SensitiveSecret        SensitiveClass = "secret_pattern"
	SensitiveAbsolutePath  SensitiveClass = "absolute_path"
	SensitivePinaxInternal SensitiveClass = "pinax_internal_reference"
	SensitivePrivateBody   SensitiveClass = "private_body_leak"
)

var (
	sensitiveAuthorizationPattern = regexp.MustCompile(`(?i)Authorization\s*[:=]?\s*Bearer\s+[^\s]+|\bBearer\s+[^\s]+`)
	sensitiveCookiePattern        = regexp.MustCompile(`(?i)\bCookie\s*[:=]\s*[^\s]+`)
	sensitiveWebhookPattern       = regexp.MustCompile(`(?i)https?://[^\s]*(webhook|callback|hooks?)[^\s]*|\bwebhook\b.*https?://`)
	sensitiveProviderPattern      = regexp.MustCompile(`(?i)\b(raw_prompt|hidden_prompt|system_prompt|provider_payload|raw_provider_payload|provider raw payload)\b`)
	sensitiveSecretPattern        = regexp.MustCompile(`(?i)\b(token|secret|api_key|access_key|secret_key|password)\s*[:=][^\s&;]+|secret_|(^|[/._-])(token|secret|api[_-]?key|access[_-]?key|secret[_-]?key|password)([/._=-]|$)`)
	sensitiveAbsolutePathPattern  = regexp.MustCompile(`(?i)(/Users/|/home/|[A-Z]:\\)`) // publish evidence must not reveal local machine paths.
	sensitivePinaxInternalPattern = regexp.MustCompile(`(^|[/\s"'(<])\.pinax(/|[\s"')>]|$)`)
	sensitivePrivateBodyPattern   = regexp.MustCompile(`(?i)\b(private_body|raw_body|private body)\b`)
)

func ScanSensitiveClasses(input string) []SensitiveClass {
	checks := []struct {
		class   SensitiveClass
		pattern *regexp.Regexp
	}{
		{SensitiveAuthorization, sensitiveAuthorizationPattern},
		{SensitiveCookie, sensitiveCookiePattern},
		{SensitiveWebhook, sensitiveWebhookPattern},
		{SensitiveProvider, sensitiveProviderPattern},
		{SensitiveSecret, sensitiveSecretPattern},
		{SensitiveAbsolutePath, sensitiveAbsolutePathPattern},
		{SensitivePinaxInternal, sensitivePinaxInternalPattern},
		{SensitivePrivateBody, sensitivePrivateBodyPattern},
	}
	out := make([]SensitiveClass, 0)
	seen := map[SensitiveClass]bool{}
	for _, check := range checks {
		if check.pattern.MatchString(input) && !seen[check.class] {
			out = append(out, check.class)
			seen[check.class] = true
		}
	}
	return out
}
