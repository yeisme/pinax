// Package provider 定义 provider adapter 的稳定内部接口和数据契约。
// 本包不执行 lark-cli 命令；实际 CLI 调用在 internal/app 的 adapter 实现中完成。
// 把契约类型独立出来，便于在不依赖 app Service 的情况下测试 adapter 行为和 provider 选择。
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// NativeDocCreateInput 是创建飞书原生文档的 provider-neutral 输入。
// PlanJSON 是序列化后的 publishdocast.NativePlan；adapter 负责翻译成 provider 命令。
type NativeDocCreateInput struct {
	Title       string
	FolderToken string
	PlanJSON    []byte
	ContentPath string
}

// NativeDocUpdateInput 是更新飞书原生文档的输入。
type NativeDocUpdateInput struct {
	Title       string
	DocToken    string
	PlanJSON    []byte
	ContentPath string
	ExistingURL string
}

// MediaUploadInput 描述一次资产上传。
type MediaUploadInput struct {
	Root      string
	DocToken  string
	Path      string // vault-relative artifact 路径
	MediaType string
	Caption   string
	Width     int
}

// NativeDocResult 是 create/update 的归一化结果。
type NativeDocResult struct {
	Token string // doc token / file token
	URL   string
	Type  string // docx / doc / file
}

// CapabilityMissingError 表示 provider 暂不支持原生文档写入。
// app 层不得静默降级到 markdown-file，必须把该错误透传给用户。
var ErrCapabilityMissing = errors.New("provider_capability_missing")

// CapabilityErrorCode 返回 capability missing 的稳定 error code。
const CapabilityErrorCode = "provider_capability_missing"

// LarkNativeDocAdapter 定义 lark-cli 原生文档能力的稳定内部接口。
// app 层的 adapter 实现封装实际 lark-cli 命令名，保证调用方只依赖此接口。
type LarkNativeDocAdapter interface {
	// CheckCapability 探测 lark-cli 是否支持原生文档写入。
	CheckCapability(ctx context.Context) error
	// CreateNativeDoc 创建原生文档，返回 docx/doc 对象。
	CreateNativeDoc(ctx context.Context, input NativeDocCreateInput) (NativeDocResult, error)
	// UpdateNativeDoc 更新已有原生文档正文。
	UpdateNativeDoc(ctx context.Context, input NativeDocUpdateInput) (NativeDocResult, error)
	// UploadMedia 上传图片/渲染资产，返回 provider media token。
	UploadMedia(ctx context.Context, input MediaUploadInput) (string, error)
	// InspectObjectType 查询远端对象类型（docx/doc/file）。
	InspectObjectType(ctx context.Context, docToken string) (string, error)
}

// LarkNativeCLICommand 是 adapter 交给外部 CLI runner 的完整命令描述。
type LarkNativeCLICommand struct {
	Dir   string
	Name  string
	Args  []string
	Stdin string
}

// LarkNativeCLIRunner 执行 provider CLI。测试可注入 fake runner，避免依赖真实 lark-cli。
type LarkNativeCLIRunner interface {
	Run(ctx context.Context, cmd LarkNativeCLICommand) ([]byte, error)
}

// LarkNativeDocCLIAdapter 封装 lark-cli native-docx 命令参数和 JSON 结果解析。
type LarkNativeDocCLIAdapter struct {
	profile domain.PublishDocProfile
	runner  LarkNativeCLIRunner
}

// NewLarkNativeDocCLIAdapter 创建 lark-cli 原生文档 adapter。
func NewLarkNativeDocCLIAdapter(profile domain.PublishDocProfile, runner LarkNativeCLIRunner) *LarkNativeDocCLIAdapter {
	return &LarkNativeDocCLIAdapter{profile: profile, runner: runner}
}

func (a *LarkNativeDocCLIAdapter) CheckCapability(ctx context.Context) error {
	_, err := a.run(ctx, LarkNativeCLICommand{Args: a.larkArgs("docs", "+create", "--dry-run", "--content", "pinax capability probe", "--doc-format", "markdown", "--json")})
	if err != nil {
		return ErrCapabilityMissing
	}
	return nil
}

func (a *LarkNativeDocCLIAdapter) CreateNativeDoc(ctx context.Context, input NativeDocCreateInput) (NativeDocResult, error) {
	args := []string{"docs", "+create", "--title", input.Title, "--content", "@" + input.ContentPath, "--doc-format", "markdown", "--json"}
	if strings.TrimSpace(input.FolderToken) != "" {
		args = append(args, "--parent-token", input.FolderToken)
	}
	args = a.appendIdentity(args)
	out, err := a.run(ctx, LarkNativeCLICommand{Args: args})
	if err != nil {
		return NativeDocResult{}, err
	}
	return parseNativeDocResult(out), nil
}

func (a *LarkNativeDocCLIAdapter) UpdateNativeDoc(ctx context.Context, input NativeDocUpdateInput) (NativeDocResult, error) {
	out, err := a.run(ctx, LarkNativeCLICommand{Args: a.larkArgs("docs", "+update", "--doc", input.DocToken, "--command", "overwrite", "--content", "@"+input.ContentPath, "--doc-format", "markdown", "--json")})
	if err != nil {
		return NativeDocResult{}, err
	}
	result := parseNativeDocResult(out)
	if result.Token == "" {
		result.Token = input.DocToken
	}
	if result.URL == "" {
		result.URL = input.ExistingURL
	}
	return result, nil
}

func (a *LarkNativeDocCLIAdapter) UploadMedia(ctx context.Context, input MediaUploadInput) (string, error) {
	relPath := strings.TrimSpace(strings.ReplaceAll(input.Path, "\\", "/"))
	if relPath == "" {
		return "", nil
	}
	mediaType := strings.TrimSpace(input.MediaType)
	if mediaType == "" {
		mediaType = "image"
	}
	args := a.larkArgs("docs", "+media-insert", "--doc", input.DocToken, "--file", "./"+relPath, "--type", mediaType, "--json")
	if input.Caption != "" {
		args = append(args, "--caption", input.Caption)
	}
	if input.Width > 0 {
		args = append(args, "--width", strconv.Itoa(input.Width))
	}
	out, err := a.run(ctx, LarkNativeCLICommand{Dir: input.Root, Args: args})
	if err != nil {
		return "", err
	}
	return parseMediaInsertBlockID(out), nil
}

func (a *LarkNativeDocCLIAdapter) InspectObjectType(ctx context.Context, docToken string) (string, error) {
	target := strings.TrimSpace(docToken)
	if target == "" {
		return "", nil
	}
	if !strings.HasPrefix(target, "http") {
		target = "https://www.feishu.cn/docx/" + target
	}
	out, err := a.run(ctx, LarkNativeCLICommand{Args: a.larkArgs("drive", "+inspect", "--json", "--url", target)})
	if err != nil {
		return "", err
	}
	var payload map[string]any
	if err := json.Unmarshal(extractJSON(out), &payload); err != nil {
		return "", err
	}
	data, _ := payload["data"].(map[string]any)
	if t, ok := data["type"].(string); ok && t != "" {
		return t, nil
	}
	return firstString(payload, "type", "object_type"), nil
}

func (a *LarkNativeDocCLIAdapter) run(ctx context.Context, cmd LarkNativeCLICommand) ([]byte, error) {
	if a.runner == nil {
		return nil, errors.New("lark native cli runner is nil")
	}
	cmd.Name = "lark-cli"
	return a.runner.Run(ctx, cmd)
}

func (a *LarkNativeDocCLIAdapter) larkArgs(args ...string) []string {
	out := append([]string{}, args...)
	return a.appendIdentity(out)
}

func (a *LarkNativeDocCLIAdapter) appendIdentity(args []string) []string {
	if a.profile.As != "" && a.profile.As != "auto" {
		args = append(args, "--as", a.profile.As)
	}
	return args
}

// ResolveExternalObjectType 根据 renderer 和 provider 返回类型决定 external_object.type。
// native-docx 应得到 docx/doc；markdown-file 得到 file。
func ResolveExternalObjectType(renderer domain.PublishDocRenderer, providerType string) string {
	if renderer == domain.PublishDocRendererMarkdownFile {
		return domain.PublishDocObjectTypeFile
	}
	switch providerType {
	case domain.PublishDocObjectTypeDoc, domain.PublishDocObjectTypeDocx:
		return providerType
	default:
		return domain.PublishDocObjectTypeDocx
	}
}

func parseNativeDocResult(body []byte) NativeDocResult {
	var payload map[string]any
	_ = json.Unmarshal(extractJSON(body), &payload)
	data, _ := payload["data"].(map[string]any)
	doc, _ := data["document"].(map[string]any)
	id, _ := doc["document_id"].(string)
	url, _ := doc["url"].(string)
	objType, _ := doc["type"].(string)
	if id == "" {
		id = firstString(payload, "id", "document_id", "obj_token")
	}
	if url == "" {
		url = firstString(payload, "url", "external_url")
	}
	if objType == "" {
		objType = firstString(payload, "type", "object_type")
	}
	return NativeDocResult{Token: id, URL: url, Type: objType}
}

func parseMediaInsertBlockID(body []byte) string {
	var payload map[string]any
	_ = json.Unmarshal(extractJSON(body), &payload)
	return firstString(payload, "block_id", "id")
}

func extractJSON(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] == '{' {
		return trimmed
	}
	if i := bytes.IndexByte(trimmed, '{'); i >= 0 {
		return trimmed[i:]
	}
	return trimmed
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
		if data, ok := payload["data"].(map[string]any); ok {
			if value, ok := data[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return ""
}
