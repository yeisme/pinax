package output

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

func renderAgent(w io.Writer, p domain.Projection) error {
	lines := []string{
		"spec_version=" + p.SpecVersion,
		"mode=agent",
		"command=" + p.Command,
		"status=" + p.Status,
	}
	keys := make([]string, 0, len(p.Facts))
	for key := range p.Facts {
		keys = append(keys, key)
	}
	sortFactKeys(keys)
	for _, key := range keys {
		lines = append(lines, "fact."+key+"="+quoteAgentValue(p.Facts[key]))
	}
	if p.Error != nil {
		lines = append(lines, "error.code="+quoteAgentValue(p.Error.Code))
		if p.Error.Message != "" {
			lines = append(lines, "error.message="+quoteAgentValue(p.Error.Message))
		}
		if p.Error.Hint != "" {
			lines = append(lines, "error.hint="+quoteAgentValue(p.Error.Hint))
		}
	}
	if data, ok := p.Data.(map[string]any); ok {
		if candidates, ok := data["candidates"].([]domain.Note); ok {
			for i, note := range candidates {
				prefix := fmt.Sprintf("candidate.%d.", i+1)
				lines = append(lines, prefix+"path="+quoteAgentValue(note.Path))
				lines = append(lines, prefix+"note_id="+quoteAgentValue(note.ID))
				lines = append(lines, prefix+"title="+quoteAgentValue(note.Title))
			}
		}
		if candidates, ok := data["candidates"].([]domain.VaultObjectCandidate); ok {
			for i, candidate := range candidates {
				prefix := fmt.Sprintf("candidate.%d.", i+1)
				lines = append(lines, prefix+"object_kind="+quoteAgentValue(string(candidate.ObjectKind)))
				lines = append(lines, prefix+"path="+quoteAgentValue(candidate.Path))
				if candidate.NoteID != "" {
					lines = append(lines, prefix+"note_id="+quoteAgentValue(candidate.NoteID))
				}
				if candidate.Title != "" {
					lines = append(lines, prefix+"title="+quoteAgentValue(candidate.Title))
				}
				if candidate.ManagedStatus != "" {
					lines = append(lines, prefix+"managed_status="+quoteAgentValue(candidate.ManagedStatus))
				}
			}
		}
		if recommendations := agentRecommendationMaps(data["recommendations"]); len(recommendations) > 0 {
			for i, recommendation := range recommendations {
				prefix := fmt.Sprintf("recommendation.%d.", i)
				keys := make([]string, 0, len(recommendation))
				for key := range recommendation {
					keys = append(keys, key)
				}
				sortFactKeys(keys)
				for _, key := range keys {
					lines = append(lines, prefix+key+"="+quoteAgentValue(agentScalarValue(recommendation[key])))
				}
			}
		}
	}
	lines = appendAgentDataListLines(lines, p)
	lines = appendNoteLinkAgentLines(lines, p)
	lines = appendRepairPlanAgentLines(lines, p)
	lines = appendRemoteVaultAgentLines(lines, p)
	lines = appendAssetDetailAgentLines(lines, p)
	lines = appendDatabaseSchemaAgentLines(lines, p)
	lines = appendProfileDetailAgentLines(lines, p)
	lines = appendBackendDetailAgentLines(lines, p)
	lines = appendBackendCapabilitiesAgentLines(lines, p)
	lines = appendSyncOperationAgentLines(lines, p)
	lines = appendPublishPlanAgentLines(lines, p)
	lines = appendPublishThemeEjectAgentLines(lines, p)
	lines = appendCollectionPlanAgentLines(lines, p)
	lines = appendPlanningAgentLines(lines, p)
	lines = appendBrainAgentLines(lines, p)
	lines = appendMetadataRecordAgentLines(lines, p)
	lines = appendProjectItemAgentLines(lines, p)
	lines = appendFolderPlanAgentLines(lines, p)
	lines = appendLearningProjectAgentLines(lines, p)
	for _, action := range p.Actions {
		lines = append(lines, "action."+action.Name+"="+quoteAgentValue(agentActionCommand(p.Command, action.Command)))
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func appendRepairPlanAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "repair.list" {
		return lines
	}
	plans := dataListMaps(p.Data, "plans")
	limit := len(plans)
	if limit > 10 {
		limit = 10
	}
	for i, plan := range plans[:limit] {
		prefix := fmt.Sprintf("repair_plan.%d.", i+1)
		for _, field := range []agentListField{{"plan_id", []string{"plan_id"}}, {"status", []string{"status"}}, {"created_at", []string{"created_at"}}, {"expires_at", []string{"expires_at"}}, {"saved_path", []string{"saved_path"}}} {
			value := firstDataPathString(plan, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
		lines = append(lines, prefix+"operations="+fmt.Sprint(dataPathLen(plan, "operations")))
	}
	return lines
}

func appendRemoteVaultAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "vault.remote.list" {
		return lines
	}
	items := remoteVaultRows(p.Data)
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i, item := range items[:limit] {
		prefix := fmt.Sprintf("remote_vault.%d.", i+1)
		for _, field := range []agentListField{{"profile", []string{"profile"}}, {"selector", []string{"selector"}}, {"label", []string{"label"}}, {"workspace", []string{"workspace"}}, {"revision", []string{"revision"}}} {
			value := firstDataPathString(item, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func appendAssetDetailAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "asset.show" {
		return lines
	}
	asset := assetMapFromData(p.Data)
	if asset == nil {
		return lines
	}
	for _, field := range []agentListField{{"path", []string{"path"}}, {"filename", []string{"filename"}}, {"media_type", []string{"media_type"}}, {"size_bytes", []string{"size_bytes", "size"}}, {"managed_status", []string{"managed_status"}}, {"sha256", []string{"sha256"}}, {"display_path", []string{"display_path"}}} {
		value := firstDataPathString(asset, field.Paths...)
		if value != "" {
			lines = append(lines, "asset_detail."+field.Key+"="+quoteAgentValue(value))
		}
	}
	return lines
}

func appendDatabaseSchemaAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "database.schema.show" {
		return lines
	}
	root, ok := dataMap(p.Data)
	if !ok {
		return lines
	}
	property, ok := dataMap(root["property"])
	if !ok {
		return lines
	}
	validation, _ := dataMap(root["validation"])
	fields := []struct {
		key   string
		value string
	}{
		{key: "name", value: p.Facts["property"]},
		{key: "type", value: firstDataPathString(property, "type")},
		{key: "values", value: firstDataPathString(property, "values")},
		{key: "updated_at", value: firstDataPathString(property, "updated_at")},
		{key: "validation_status", value: firstDataPathString(validation, "status")},
		{key: "checked_values", value: firstDataPathString(validation, "checked_values")},
		{key: "invalid_values", value: firstDataPathString(validation, "invalid_values")},
	}
	for _, field := range fields {
		if field.value != "" {
			lines = append(lines, "schema_property."+field.key+"="+quoteAgentValue(field.value))
		}
	}
	return lines
}

func appendProfileDetailAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "profile.show" {
		return lines
	}
	profile := profileMapFromData(p.Data)
	if profile == nil {
		return lines
	}
	for _, field := range []agentListField{{"name", []string{"name"}}, {"endpoint", []string{"endpoint"}}, {"workspace", []string{"workspace"}}, {"device", []string{"device"}}, {"default_scope", []string{"default_scope"}}} {
		value := firstDataPathString(profile, field.Paths...)
		if value != "" {
			lines = append(lines, "profile_detail."+field.Key+"="+quoteAgentValue(value))
		}
	}
	return lines
}

func agentRecommendationMaps(value any) []map[string]any {
	if value == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var recommendations []map[string]any
	if err := json.Unmarshal(payload, &recommendations); err != nil {
		return nil
	}
	return recommendations
}

type agentListField struct {
	Key   string
	Paths []string
}

type agentListSpec struct {
	Prefix string
	Path   []string
	Fields []agentListField
}

func appendAgentDataListLines(lines []string, p domain.Projection) []string {
	for _, spec := range agentListSpecs() {
		items := dataListMaps(p.Data, spec.Path...)
		if len(items) == 0 {
			continue
		}
		limit := len(items)
		if limit > 10 {
			limit = 10
		}
		for i, item := range items[:limit] {
			prefix := fmt.Sprintf("%s.%d.", spec.Prefix, i+1)
			for _, field := range spec.Fields {
				value := firstDataPathString(item, field.Paths...)
				if value == "" {
					continue
				}
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func agentListSpecs() []agentListSpec {
	return []agentListSpec{
		{Prefix: "note", Path: []string{"notes"}, Fields: []agentListField{{"path", []string{"path"}}, {"title", []string{"title"}}, {"note_id", []string{"id", "note_id"}}, {"kind", []string{"kind"}}, {"status", []string{"status"}}, {"project", []string{"project"}}, {"updated_at", []string{"updated_at"}}}},
		{Prefix: "note", Path: []string{"result", "notes"}, Fields: []agentListField{{"path", []string{"path"}}, {"title", []string{"title"}}, {"note_id", []string{"id", "note_id"}}, {"kind", []string{"kind"}}, {"status", []string{"status"}}, {"project", []string{"project"}}, {"updated_at", []string{"updated_at"}}}},
		{Prefix: "issue", Path: []string{"issues"}, Fields: []agentListField{{"code", []string{"code", "issue_code"}}, {"severity", []string{"severity"}}, {"path", []string{"path"}}, {"field", []string{"field"}}, {"operation", []string{"operation"}}, {"note_id", []string{"note_id"}}, {"message", []string{"message"}}}},
		{Prefix: "warning", Path: []string{"warnings"}, Fields: []agentListField{{"source", []string{"source"}}, {"path", []string{"path"}}, {"line", []string{"line"}}, {"message", []string{"message"}}}},
		{Prefix: "delete_candidate", Path: []string{"delete_candidates"}, Fields: []agentListField{{"run_id", []string{"run_id"}}, {"name", []string{"name"}}, {"command", []string{"command"}}, {"status", []string{"status"}}, {"template", []string{"template"}}, {"target_note", []string{"target_note"}}, {"created_at", []string{"created_at"}}}},
		{Prefix: "result", Path: []string{"results"}, Fields: []agentListField{{"prompt_asset_id", []string{"prompt_asset_id"}}, {"path", []string{"note.path", "path"}}, {"title", []string{"note.title", "title"}}, {"note_id", []string{"note.id", "note_id"}}, {"kind", []string{"note.kind", "kind"}}, {"status", []string{"note.status", "status"}}, {"snippet", []string{"snippet"}}, {"score", []string{"score"}}}},
		{Prefix: "template", Path: []string{"templates"}, Fields: []agentListField{{"name", []string{"name"}}, {"source", []string{"source"}}, {"kind", []string{"kind"}}, {"scenario_id", []string{"scenario_id"}}, {"template_kind", []string{"template_kind"}}, {"maturity", []string{"maturity"}}, {"lifecycle", []string{"lifecycle"}}, {"pack", []string{"pack.id"}}, {"write_boundary", []string{"output_policy.write_boundary"}}}},
		{Prefix: "prompt_asset", Path: []string{"prompt_assets"}, Fields: []agentListField{{"id", []string{"prompt_asset_id", "PromptAssetID"}}, {"title", []string{"title", "Title"}}, {"domain", []string{"domain", "Domain"}}, {"lifecycle", []string{"lifecycle", "Lifecycle"}}, {"permission", []string{"permission", "Permission"}}, {"owner_project", []string{"owner_project", "OwnerProject"}}, {"current_version_id", []string{"current_version_id", "CurrentVersionID"}}}},
		{Prefix: "entry", Path: []string{"entries"}, Fields: []agentListField{{"event_id", []string{"event_id"}}, {"source", []string{"source"}}, {"kind", []string{"kind"}}, {"status", []string{"status"}}, {"severity", []string{"severity"}}, {"object_ref", []string{"object_ref"}}, {"path", []string{"path"}}, {"run_id", []string{"run_id"}}, {"ts", []string{"ts", "timestamp"}}}},
		{Prefix: "trash", Path: []string{"entries"}, Fields: []agentListField{{"object_kind", []string{"object_kind"}}, {"object_id", []string{"object_id"}}, {"title", []string{"title"}}, {"trash_path", []string{"trash_path"}}, {"old_path", []string{"old_path"}}, {"deleted_at", []string{"deleted_at"}}, {"expires_at", []string{"expires_at"}}}},
		{Prefix: "event", Path: []string{"events"}, Fields: []agentListField{{"seq", []string{"seq"}}, {"type", []string{"type"}}, {"run_id", []string{"run_id"}}, {"direction", []string{"direction"}}, {"kind", []string{"kind"}}, {"path", []string{"path"}}, {"path_hash", []string{"path_hash"}}, {"from_path", []string{"from_path"}}, {"to_path", []string{"to_path"}}, {"operation_status", []string{"operation_status"}}, {"status", []string{"status"}}, {"backend_kind", []string{"backend_kind"}}, {"target", []string{"target"}}, {"ts", []string{"ts", "timestamp"}}}},
		{Prefix: "run", Path: []string{"runs"}, Fields: []agentListField{{"run_id", []string{"run_id"}}, {"command", []string{"command"}}, {"direction", []string{"direction"}}, {"status", []string{"status"}}, {"backend_kind", []string{"backend_kind"}}, {"duration_ms", []string{"duration_ms"}}, {"started_at", []string{"started_at", "created_at"}}}},
		{Prefix: "backend", Path: []string{"registry", "backends"}, Fields: []agentListField{{"name", []string{"name"}}, {"kind", []string{"kind"}}, {"bucket", []string{"bucket"}}, {"region", []string{"region"}}, {"prefix", []string{"prefix"}}, {"profile", []string{"profile"}}, {"credential_source", []string{"credential_source"}}, {"capabilities", []string{"capabilities"}}}},
		{Prefix: "asset", Path: []string{"assets"}, Fields: []agentListField{{"path", []string{"path"}}, {"filename", []string{"filename"}}, {"media_type", []string{"media_type"}}, {"size_bytes", []string{"size_bytes", "size"}}, {"managed_status", []string{"managed_status"}}}},
		{Prefix: "memory_record", Path: []string{"records"}, Fields: []agentListField{{"id", []string{"id"}}, {"type", []string{"type"}}, {"subject", []string{"subject"}}, {"predicate", []string{"predicate"}}, {"object", []string{"object"}}, {"status", []string{"status"}}, {"source_uri", []string{"source_uri"}}, {"confidence", []string{"confidence"}}}},
		{Prefix: "memory_match", Path: []string{"matches"}, Fields: []agentListField{{"id", []string{"id"}}, {"type", []string{"type"}}, {"subject", []string{"subject"}}, {"object", []string{"object"}}, {"status", []string{"status"}}, {"score", []string{"score"}}, {"recall_reason", []string{"recall_reason"}}}},
		{Prefix: "plugin", Path: []string{"plugins"}, Fields: []agentListField{{"id", []string{"id"}}, {"name", []string{"name"}}, {"version", []string{"version"}}, {"runtime", []string{"runtime"}}, {"enabled", []string{"enabled"}}, {"scope", []string{"scope"}}, {"capability_count", []string{"capability_count"}}}},
		{Prefix: "permission_grant", Path: []string{"grants"}, Fields: []agentListField{{"permission", []string{"permission"}}, {"capability", []string{"capability"}}, {"granted_at", []string{"granted_at"}}}},
		{Prefix: "property", Path: []string{"properties"}, Fields: []agentListField{{"name", []string{"name"}}, {"type", []string{"type"}}, {"values", []string{"values"}}, {"updated_at", []string{"updated_at"}}}},
		{Prefix: "provider", Path: []string{"providers"}, Fields: []agentListField{{"name", []string{"name"}}, {"default_model", []string{"default_model"}}, {"configured", []string{"configured"}}, {"credential_source", []string{"credential_source"}}, {"local_only", []string{"local_only"}}, {"requires_credential", []string{"requires_credential"}}}},
		{Prefix: "profile", Path: []string{"profiles"}, Fields: []agentListField{{"name", []string{"name"}}, {"target", []string{"target"}}, {"renderer", []string{"renderer"}}, {"title", []string{"site.title"}}, {"theme", []string{"site.theme.value"}}, {"endpoint", []string{"endpoint"}}, {"workspace", []string{"workspace"}}, {"device", []string{"device"}}, {"default_scope", []string{"default_scope"}}, {"default", []string{"default"}}}},
		{Prefix: "doc_provider", Path: []string{"providers"}, Fields: []agentListField{{"target", []string{"target"}}, {"provider", []string{"provider"}}}},
		{Prefix: "doc_mapping", Path: []string{"mappings"}, Fields: []agentListField{{"note_id", []string{"note_id"}}, {"target", []string{"target"}}, {"provider", []string{"provider"}}, {"publish_status", []string{"publish_status"}}, {"renderer", []string{"renderer"}}, {"remote_path", []string{"remote_path"}}, {"updated_at", []string{"updated_at"}}}},
		{Prefix: "theme", Path: []string{"themes"}, Fields: []agentListField{{"name", []string{"name"}}, {"source", []string{"source"}}, {"contract", []string{"contract_version"}}}},
		{Prefix: "token", Path: []string{"tokens"}, Fields: []agentListField{{"id", []string{"id"}}, {"label", []string{"label"}}, {"created_at", []string{"created_at"}}, {"scope", []string{"scope"}}, {"expires_at", []string{"expires_at"}}}},
		{Prefix: "view", Path: []string{"views"}, Fields: []agentListField{{"name", []string{"name"}}, {"group", []string{"group"}}, {"folder", []string{"folder"}}, {"kind", []string{"kind"}}, {"status", []string{"status"}}, {"sort", []string{"sort"}}, {"display", []string{"display.mode"}}, {"query", []string{"query"}}}},
		{Prefix: "route", Path: []string{"routes"}, Fields: []agentListField{{"id", []string{"id", "route_id"}}, {"method", []string{"method"}}, {"path", []string{"path"}}, {"rpc_method", []string{"rpc_method"}}, {"command", []string{"command"}}, {"surface", []string{"surface"}}}},
		{Prefix: "object", Path: []string{"objects"}, Fields: []agentListField{{"key", []string{"key"}}, {"path", []string{"path"}}, {"size_bytes", []string{"size_bytes"}}, {"updated_at", []string{"updated_at"}}}},
		{Prefix: "subproject", Path: []string{"subprojects"}, Fields: []agentListField{{"slug", []string{"subproject", "slug"}}, {"title", []string{"title"}}, {"workspace_path", []string{"workspace_path"}}, {"status", []string{"status"}}}},
		{Prefix: "item", Path: []string{"board", "items"}, Fields: []agentListField{{"item_id", []string{"item_id"}}, {"title", []string{"title"}}, {"column", []string{"column"}}, {"path", []string{"path"}}, {"project", []string{"project"}}, {"subproject", []string{"subproject"}}, {"workspace_path", []string{"workspace_path"}}, {"source_kind", []string{"source_kind"}}, {"source_status", []string{"source_status"}}, {"note_id", []string{"note_id"}}, {"status", []string{"status"}}, {"priority", []string{"priority"}}, {"labels", []string{"labels"}}, {"writable", []string{"writable"}}}},
	}
}

func appendNoteLinkAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "note.links" && p.Command != "note.backlinks" {
		return lines
	}
	links := dataListMaps(p.Data, "links")
	if p.Command == "note.backlinks" {
		links = dataListMaps(p.Data, "backlinks")
	}
	limit := len(links)
	if limit > 10 {
		limit = 10
	}
	for i, link := range links[:limit] {
		prefix := fmt.Sprintf("link.%d.", i+1)
		for _, field := range []agentListField{{"source_path", []string{"source_path"}}, {"kind", []string{"kind"}}, {"status", []string{"status"}}, {"target", []string{"target"}}, {"target_path", []string{"target_path"}}, {"target_note_id", []string{"target_note_id"}}, {"target_title", []string{"target_title"}}, {"line", []string{"line"}}} {
			value := firstDataPathString(link, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
		candidates := agentLinkCandidateMaps(link["candidates"])
		if len(candidates) == 0 {
			continue
		}
		lines = append(lines, prefix+"candidate_count="+quoteAgentValue(fmt.Sprint(len(candidates))))
		candidateLimit := len(candidates)
		if candidateLimit > 5 {
			candidateLimit = 5
		}
		for c, candidate := range candidates[:candidateLimit] {
			candidatePrefix := fmt.Sprintf("%scandidate.%d.", prefix, c+1)
			for _, field := range []agentListField{{"path", []string{"path"}}, {"title", []string{"title"}}, {"note_id", []string{"note_id"}}} {
				value := firstDataPathString(candidate, field.Paths...)
				if value != "" {
					lines = append(lines, candidatePrefix+field.Key+"="+quoteAgentValue(value))
				}
			}
		}
	}
	return lines
}

func agentLinkCandidateMaps(value any) []map[string]any {
	if value == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var items []map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil
	}
	return items
}

func appendBackendDetailAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "backend.show" {
		return lines
	}
	profile := profileMapFromData(p.Data)
	if profile == nil {
		return lines
	}
	for _, field := range []agentListField{{"name", []string{"name"}}, {"kind", []string{"kind"}}, {"bucket", []string{"bucket"}}, {"region", []string{"region"}}, {"prefix", []string{"prefix"}}, {"profile", []string{"profile"}}, {"credential_source", []string{"credential_source"}}, {"capabilities", []string{"capabilities"}}} {
		value := firstDataPathString(profile, field.Paths...)
		if value != "" {
			lines = append(lines, "backend_detail."+field.Key+"="+quoteAgentValue(value))
		}
	}
	return lines
}

func appendBackendCapabilitiesAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "backend.capabilities" {
		return lines
	}
	capabilities := dataListMaps(p.Data, "capabilities")
	limit := len(capabilities)
	if limit > 10 {
		limit = 10
	}
	for i, capability := range capabilities[:limit] {
		prefix := fmt.Sprintf("capability.%d.", i+1)
		for _, field := range []agentListField{{"name", []string{"name"}}, {"supported", []string{"supported"}}} {
			value := firstDataPathString(capability, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func appendSyncOperationAgentLines(lines []string, p domain.Projection) []string {
	switch p.Command {
	case "sync.diff", "sync.push", "sync.pull":
	default:
		return lines
	}
	operations := dataListMaps(p.Data, "plan", "operations")
	limit := len(operations)
	if limit > 10 {
		limit = 10
	}
	for i, operation := range operations[:limit] {
		prefix := fmt.Sprintf("operation.%d.", i+1)
		for _, field := range []agentListField{{"kind", []string{"kind"}}, {"path", []string{"path"}}, {"status", []string{"status"}}, {"blob_id", []string{"blob_id"}}, {"path_hash", []string{"path_hash"}}} {
			value := firstDataPathString(operation, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func appendPublishPlanAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "publish.plan" {
		return lines
	}
	items := dataListMaps(p.Data, "plan", "selected")
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i, item := range items[:limit] {
		prefix := fmt.Sprintf("publish_item.%d.", i+1)
		for _, field := range []agentListField{{"id", []string{"id"}}, {"kind", []string{"kind"}}, {"title", []string{"title"}}, {"source_path", []string{"source_path"}}, {"output_path", []string{"output_path"}}} {
			value := firstDataPathString(item, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func appendPublishThemeEjectAgentLines(lines []string, p domain.Projection) []string {
	if p.Command != "publish.theme.eject" {
		return lines
	}
	files := dataListScalars(p.Data, "files")
	limit := len(files)
	if limit > 20 {
		limit = 20
	}
	for i, file := range files[:limit] {
		lines = append(lines, fmt.Sprintf("file.%d.path=%s", i+1, quoteAgentValue(file)))
	}
	return lines
}

func appendCollectionPlanAgentLines(lines []string, p domain.Projection) []string {
	switch p.Command {
	case "collection.import", "collection.diff":
	default:
		return lines
	}
	items := dataListMaps(p.Data, "plan", "plans")
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i, item := range items[:limit] {
		prefix := fmt.Sprintf("collection_item.%d.", i+1)
		for _, field := range []agentListField{{"item_id", []string{"item_id"}}, {"title", []string{"title"}}, {"note_path", []string{"note_path"}}, {"prompt_asset_id", []string{"prompt_asset_id"}}, {"note_status", []string{"note_status"}}, {"prompt_status", []string{"prompt_status"}}} {
			value := firstDataPathString(item, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func appendPlanningAgentLines(lines []string, p domain.Projection) []string {
	switch p.Command {
	case "plan.daily", "plan.weekly", "plan.monthly":
		items := planningSelectedTaskRows(p.Data)
		limit := len(items)
		if limit > 10 {
			limit = 10
		}
		for i, item := range items[:limit] {
			prefix := fmt.Sprintf("plan_task.%d.", i+1)
			for _, field := range []agentListField{{"task_id", []string{"id"}}, {"title", []string{"title"}}, {"status", []string{"status"}}, {"source", []string{"source"}}, {"priority", []string{"priority"}}, {"reason", []string{"reason"}}, {"section_id", []string{"section_id"}}, {"section_title", []string{"section_title"}}} {
				value := firstDataPathString(item, field.Paths...)
				if value != "" {
					lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
				}
			}
		}
	case "plan.actions":
		items := dataListMaps(p.Data, "draft", "tasks")
		limit := len(items)
		if limit > 10 {
			limit = 10
		}
		for i, item := range items[:limit] {
			prefix := fmt.Sprintf("action_task.%d.", i+1)
			for _, field := range []agentListField{{"action_id", []string{"action_id"}}, {"task_id", []string{"task_id"}}, {"title", []string{"title"}}, {"kind", []string{"kind"}}, {"priority", []string{"priority"}}, {"project_slug", []string{"project_slug"}}, {"reason", []string{"reason"}}, {"requires_confirmation", []string{"requires_confirmation"}}} {
				value := firstDataPathString(item, field.Paths...)
				if value != "" {
					lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
				}
			}
		}
	}
	return lines
}

func appendBrainAgentLines(lines []string, p domain.Projection) []string {
	switch p.Command {
	case "brain.answer":
		items := dataListMaps(p.Data, "sources")
		limit := len(items)
		if limit > 10 {
			limit = 10
		}
		for i, item := range items[:limit] {
			prefix := fmt.Sprintf("brain_source.%d.", i+1)
			for _, field := range []agentListField{{"kind", []string{"kind"}}, {"id", []string{"id"}}, {"path", []string{"path"}}, {"title", []string{"title"}}} {
				value := firstDataPathString(item, field.Paths...)
				if value != "" {
					lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
				}
			}
		}
	case "brain.maintenance_plan":
		items := dataListMaps(p.Data, "operations")
		limit := len(items)
		if limit > 10 {
			limit = 10
		}
		for i, item := range items[:limit] {
			prefix := fmt.Sprintf("brain_operation.%d.", i+1)
			for _, field := range []agentListField{{"kind", []string{"kind"}}, {"risk", []string{"risk"}}, {"status", []string{"status"}}, {"evidence", []string{"evidence"}}, {"next_action", []string{"next_action.command"}}} {
				value := firstDataPathString(item, field.Paths...)
				if value != "" {
					lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
				}
			}
		}
	}
	return lines
}

func appendMetadataRecordAgentLines(lines []string, p domain.Projection) []string {
	switch p.Command {
	case "metadata.plan":
		items := dataListMaps(p.Data, "operations")
		limit := len(items)
		if limit > 10 {
			limit = 10
		}
		for i, item := range items[:limit] {
			prefix := fmt.Sprintf("operation.%d.", i+1)
			for _, field := range []agentListField{{"kind", []string{"kind"}}, {"path", []string{"path"}}, {"target", []string{"target"}}, {"reason", []string{"reason"}}, {"status", []string{"status"}}} {
				value := firstDataPathString(item, field.Paths...)
				if value != "" {
					lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
				}
			}
		}
	case "record.history":
		record, ok := dataMap(p.Data)
		if !ok {
			return lines
		}
		for _, field := range []agentListField{{"note_id", []string{"note_id"}}, {"path", []string{"path"}}, {"title", []string{"title"}}, {"lifecycle", []string{"lifecycle"}}, {"record_version", []string{"record_version"}}, {"ledger_seq", []string{"ledger_seq"}}} {
			value := firstDataPathString(record, field.Paths...)
			if value != "" {
				lines = append(lines, "record_detail."+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func appendProjectItemAgentLines(lines []string, p domain.Projection) []string {
	switch p.Command {
	case "project.item.add", "project.item.move", "project.item.archive", "project.item.plan", "project.item.show":
	default:
		return lines
	}
	root, ok := dataMap(p.Data)
	if !ok {
		return lines
	}
	item, ok := dataMap(root["item"])
	if !ok {
		return lines
	}
	for _, field := range []agentListField{{"item_id", []string{"item_id"}}, {"title", []string{"title"}}, {"column", []string{"column"}}, {"source_kind", []string{"source_kind"}}, {"note_id", []string{"note_id"}}, {"path", []string{"path"}}, {"project", []string{"project"}}, {"subproject", []string{"subproject"}}, {"status", []string{"status"}}, {"priority", []string{"priority"}}, {"labels", []string{"labels"}}, {"writable", []string{"writable"}}} {
		value := firstDataPathString(item, field.Paths...)
		if value != "" {
			lines = append(lines, "project_item."+field.Key+"="+quoteAgentValue(value))
		}
	}
	return lines
}

func appendFolderPlanAgentLines(lines []string, p domain.Projection) []string {
	switch p.Command {
	case "folder.create", "folder.adopt", "folder.delete", "folder.rename", "folder.move":
	default:
		return lines
	}
	items := dataListMaps(p.Data, "plan", "effects")
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i, item := range items[:limit] {
		prefix := fmt.Sprintf("folder_effect.%d.", i+1)
		for _, field := range []agentListField{{"kind", []string{"kind"}}, {"path", []string{"path"}}, {"target", []string{"target"}}, {"reason", []string{"reason"}}, {"status", []string{"status"}}} {
			value := firstDataPathString(item, field.Paths...)
			if value != "" {
				lines = append(lines, prefix+field.Key+"="+quoteAgentValue(value))
			}
		}
	}
	return lines
}

func appendLearningProjectAgentLines(lines []string, p domain.Projection) []string {
	data, ok := p.Data.(map[string]any)
	if !ok {
		return lines
	}
	learning, ok := dataMap(data["learning_project"])
	if !ok {
		return lines
	}
	for _, field := range []agentListField{
		{"project", []string{"project"}},
		{"subproject", []string{"subproject"}},
		{"preset", []string{"preset"}},
		{"workspace_path", []string{"workspace.workspace_path"}},
		{"workspace_title", []string{"workspace.title"}},
		{"workspace_status", []string{"workspace.status"}},
	} {
		value := firstDataPathString(learning, field.Paths...)
		if value != "" {
			lines = append(lines, "learning."+field.Key+"="+quoteAgentValue(value))
		}
	}
	lines = appendAgentScalarListLines(lines, "learning.column", learning["columns"])
	lines = appendAgentScalarListLines(lines, "learning.starter_note", learning["starter_notes"])
	lines = appendAgentScalarListLines(lines, "learning.starter_item", learning["starter_items"])
	return lines
}

func appendAgentScalarListLines(lines []string, prefix string, value any) []string {
	items := agentScalarList(value)
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i, item := range items[:limit] {
		if item == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s.%d=%s", prefix, i+1, quoteAgentValue(item)))
	}
	return lines
}

func agentScalarList(value any) []string {
	if value == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var raw []any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil
	}
	items := make([]string, 0, len(raw))
	for _, item := range raw {
		items = append(items, agentScalarValue(item))
	}
	return items
}

func dataMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, false
	}
	return out, true
}

func firstDataPathString(item map[string]any, paths ...string) string {
	for _, path := range paths {
		value := dataPathString(item, path)
		if value != "" {
			return value
		}
	}
	return ""
}

func agentScalarValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := agentScalarValue(item); value != "" {
				items = append(items, value)
			}
		}
		return strings.Join(items, ",")
	case bool:
		return fmt.Sprint(typed)
	case float64:
		return fmt.Sprint(typed)
	default:
		payload, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(payload)
	}
}

func quoteAgentValue(value string) string {
	if strings.ContainsAny(value, " \t\n\"") {
		b, _ := json.Marshal(value)
		quoted := strings.ReplaceAll(string(b), "\\u003c", "<")
		quoted = strings.ReplaceAll(quoted, "\\u003e", ">")
		quoted = strings.ReplaceAll(quoted, "\\u0026", "&")
		return quoted
	}
	return value
}

var agentVaultFlagPattern = regexp.MustCompile(`--vault\s+("[^"]+"|'[^']+'|\S+)`)

func agentActionCommand(command, action string) string {
	if strings.HasPrefix(command, "project.") {
		return agentVaultFlagPattern.ReplaceAllString(action, "--vault <vault>")
	}
	return action
}
