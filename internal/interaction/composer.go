package interaction

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	stateReady            = "ready"
	statePartial          = "partial"
	stateBlocked          = "blocked"
	stateApprovalRequired = "approval_required"
	stateUnknown          = "unknown"
)

var vaultArgumentPattern = regexp.MustCompile(`--vault\s+("[^"]*"|'[^']*'|[^\s]+)`)

// Decorate derives the daily interaction projection from an existing
// projection. It deliberately does not inspect vault files, indexes, or
// command arguments, so the output remains a bounded view of owner facts.
func Decorate(p *domain.Projection) {
	if p == nil || !isSupported(p.Command) {
		return
	}
	if p.Facts == nil {
		p.Facts = map[string]string{}
	}

	phase := phaseFor(p.Command)
	writes := factBool(p.Facts, "writes") || factBool(p.Facts, "local_write")
	if !writes && knownWriteCommand(p.Command) && !factBool(p.Facts, "dry_run") && p.Facts["ledger_status"] != "preview" {
		writes = true
	}
	remoteEffect := factBool(p.Facts, "remote_write") || factBool(p.Facts, "remote_writes")
	sourceStatus := firstFact(p.Facts, "source_status", "evidence_status", "index_status", "pack_status")
	if sourceStatus == "" {
		sourceStatus = "not_measured"
	}
	freshness := firstFact(p.Facts, "freshness_status", "fresh", "source_freshness")
	if freshness == "" {
		freshness = "not_measured"
	}
	state := stateFor(p, sourceStatus, freshness, writes)

	recommended, recovery := actionsFor(p, phase, state)
	warnings := warningCodes(p, sourceStatus, freshness, state)
	interaction := &domain.DailyInteraction{
		SchemaVersion:         domain.DailyInteractionSchemaVersion,
		Phase:                 phase,
		State:                 state,
		ReadOnly:              !writes && !remoteEffect,
		Writes:                writes,
		RemoteEffect:          remoteEffect,
		Scope:                 firstFact(p.Facts, "scope", "project", "binding_id_digest"),
		SourceStatus:          sourceStatus,
		FreshnessStatus:       freshness,
		RecommendedNextAction: recommended,
		RecoveryActions:       recovery,
		WarningCodes:          warnings,
	}
	p.Interaction = interaction
	// Keep the existing machine facts additive and easy to inspect with
	// --agent, while the structured contract lives under interaction.
	p.Facts["interaction_schema_version"] = domain.DailyInteractionSchemaVersion
	p.Facts["interaction_phase"] = phase
	p.Facts["interaction_state"] = state
	p.Facts["interaction_read_only"] = boolString(interaction.ReadOnly)
	p.Facts["interaction_writes"] = boolString(writes)
	p.Facts["interaction_remote_effect"] = boolString(remoteEffect)
	if recommended != nil {
		p.Facts["interaction_next_action"] = recommended.Command
	}
}

func isSupported(command string) bool {
	for _, prefix := range []string{
		"inbox.", "search", "note.search", "note.preview", "note.show", "review", "plan.",
		"organize.", "continue", "agent.handoff", "proof.loop",
	} {
		if command == prefix || strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func phaseFor(command string) string {
	switch {
	case command == "inbox.capture":
		return "capture"
	case command == "search" || command == "search.show" || command == "note.search" || command == "note.preview" || command == "note.show":
		return "retrieve"
	case command == "inbox.review" || command == "review" || strings.HasPrefix(command, "review."):
		return "review"
	case strings.HasPrefix(command, "plan."):
		return "plan"
	case command == "organize.apply":
		return "apply"
	case strings.HasPrefix(command, "organize."):
		return "plan"
	case command == "continue.checkpoint" || command == "agent.handoff.create":
		return "close"
	case command == "continue" || strings.HasPrefix(command, "continue."):
		return "resume"
	case strings.HasPrefix(command, "proof."):
		return "plan"
	default:
		return "retrieve"
	}
}

func stateFor(p *domain.Projection, sourceStatus, freshness string, writes bool) string {
	if p.Error != nil {
		switch p.Error.Code {
		case "approval_required", "write_disabled", "snapshot_required":
			return stateApprovalRequired
		case "plan_stale":
			return stateBlocked
		case "operation_unknown", "reconcile_required", "unknown_accept":
			return stateUnknown
		case "continuity_binding_ambiguous", "search_index_unavailable", "vault_not_initialized", "invalid_scope":
			return stateBlocked
		default:
			return stateBlocked
		}
	}
	if p.Status == "partial" || sourceStatus == "partial" || sourceStatus == "missing" || sourceStatus == "stale" || freshness == "stale" || freshness == "partial" || hasWarning(p, "handoff_missing", "source_missing", "source_stale", "decision_conflict") {
		return statePartial
	}
	if writes && factBool(p.Facts, "approval_required") {
		return stateApprovalRequired
	}
	return stateReady
}

func actionsFor(p *domain.Projection, phase, state string) (*domain.Action, []domain.Action) {
	actions := append([]domain.Action(nil), p.Actions...)
	for i := range actions {
		actions[i] = safeAction(actions[i])
	}
	if phase == "capture" {
		return &domain.Action{Name: "review", Command: "pinax inbox review --limit 5 --json"}, actions[:min(3, len(actions))]
	}
	if phase == "retrieve" && (p.Command == "search" || p.Command == "note.search") {
		if action := searchPreviewAction(p.Data); action != nil {
			return action, actions[:min(3, len(actions))]
		}
		return &domain.Action{Name: "review", Command: "pinax inbox review --limit 5 --json"}, actions[:min(3, len(actions))]
	}
	if phase == "resume" {
		if action := reflectedAction(p.Data, "RecommendedNextAction"); action != nil {
			safe := safeAction(*action)
			return &safe, actions[:min(3, len(actions))]
		}
	}
	if len(actions) == 0 {
		return nil, nil
	}
	var preferred *domain.Action
	preferredNames := map[string]bool{
		"organize_preview": true, "preview": true, "read": true, "resume": true,
		"continue": true, "review": true, "apply": true, "reconcile": true,
		"bind": true, "status": true, "next": true,
	}
	for i := range actions {
		if preferredNames[strings.ToLower(actions[i].Name)] {
			copy := actions[i]
			preferred = &copy
			break
		}
	}
	if preferred == nil {
		copy := actions[0]
		preferred = &copy
	}
	if state == stateUnknown {
		for i := range actions {
			if strings.Contains(strings.ToLower(actions[i].Name), "reconcil") || strings.Contains(strings.ToLower(actions[i].Command), "operation reconcile") {
				copy := actions[i]
				preferred = &copy
				break
			}
		}
	}
	remaining := make([]domain.Action, 0, 3)
	for _, action := range actions {
		if preferred != nil && action.Command == preferred.Command {
			continue
		}
		remaining = append(remaining, action)
		if len(remaining) == 3 {
			break
		}
	}
	return preferred, remaining
}

func safeAction(action domain.Action) domain.Action {
	action.Command = vaultArgumentPattern.ReplaceAllString(action.Command, "--vault <vault>")
	return action
}

func searchPreviewAction(data any) *domain.Action {
	ref := firstNoteRef(data)
	if ref == "" {
		return nil
	}
	return &domain.Action{Name: "preview", Command: fmt.Sprintf("pinax note preview %s --json", ref)}
}

// firstNoteRef reads only note identity fields from the bounded search
// projection. Reflection keeps this composer independent from the search
// implementation package and never reads body/snippet fields.
func firstNoteRef(data any) string {
	v := reflect.ValueOf(data)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return ""
	}
	for _, fieldName := range []string{"Results", "Notes"} {
		field := v.FieldByName(fieldName)
		if !field.IsValid() || (field.Kind() != reflect.Slice && field.Kind() != reflect.Array) || field.Len() == 0 {
			continue
		}
		item := field.Index(0)
		for item.IsValid() && (item.Kind() == reflect.Pointer || item.Kind() == reflect.Interface) {
			if item.IsNil() {
				break
			}
			item = item.Elem()
		}
		if item.IsValid() && item.Kind() == reflect.Struct {
			if note := item.FieldByName("Note"); note.IsValid() {
				item = note
			}
			for item.IsValid() && (item.Kind() == reflect.Pointer || item.Kind() == reflect.Interface) {
				if item.IsNil() {
					break
				}
				item = item.Elem()
			}
			if item.IsValid() && item.Kind() == reflect.Struct {
				for _, identity := range []string{"ID", "Path"} {
					value := item.FieldByName(identity)
					if value.IsValid() && value.Kind() == reflect.String && strings.TrimSpace(value.String()) != "" {
						return value.String()
					}
				}
			}
		}
	}
	return ""
}

func reflectedAction(data any, fieldName string) *domain.Action {
	v := reflect.ValueOf(data)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return nil
	}
	field := v.FieldByName(fieldName)
	for field.IsValid() && (field.Kind() == reflect.Pointer || field.Kind() == reflect.Interface) {
		if field.IsNil() {
			return nil
		}
		field = field.Elem()
	}
	if !field.IsValid() || field.Kind() != reflect.Struct {
		return nil
	}
	name, command := field.FieldByName("Name"), field.FieldByName("Command")
	if !name.IsValid() || !command.IsValid() || name.Kind() != reflect.String || command.Kind() != reflect.String || strings.TrimSpace(command.String()) == "" {
		return nil
	}
	return &domain.Action{Name: name.String(), Command: command.String()}
}

func warningCodes(p *domain.Projection, sourceStatus, freshness, state string) []string {
	seen := map[string]bool{}
	add := func(code string) {
		if code != "" && !seen[code] {
			seen[code] = true
		}
	}
	for _, warning := range p.Warnings {
		add(warning.Code)
	}
	for _, code := range strings.Split(p.Facts["warning_codes"], ",") {
		add(strings.TrimSpace(code))
	}
	if sourceStatus == "stale" || freshness == "stale" {
		add("source_stale")
	}
	if sourceStatus == "partial" || sourceStatus == "missing" || freshness == "partial" {
		add("source_partial")
	}
	if state == stateUnknown {
		add("operation_reconcile_required")
	}
	out := make([]string, 0, len(seen))
	for _, code := range []string{"handoff_missing", "source_missing", "source_stale", "decision_conflict", "source_partial", "operation_reconcile_required"} {
		if seen[code] {
			out = append(out, code)
		}
	}
	return out
}

func hasWarning(p *domain.Projection, codes ...string) bool {
	for _, warning := range p.Warnings {
		for _, code := range codes {
			if warning.Code == code {
				return true
			}
		}
	}
	for _, code := range codes {
		if strings.Contains(p.Facts["warning_codes"], code) {
			return true
		}
	}
	return false
}

func factBool(facts map[string]string, keys ...string) bool {
	for _, key := range keys {
		if strings.EqualFold(strings.TrimSpace(facts[key]), "true") {
			return true
		}
	}
	return false
}

func firstFact(facts map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(facts[key]); value != "" {
			return value
		}
	}
	return ""
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func knownWriteCommand(command string) bool {
	switch command {
	case "inbox.capture", "continue.checkpoint", "continue.feedback", "agent.handoff.create", "review.approve", "review.reject", "organize.apply", "plan.snapshot":
		return true
	default:
		return strings.HasSuffix(command, ".apply")
	}
}
