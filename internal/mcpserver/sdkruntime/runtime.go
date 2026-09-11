// Package sdkruntime 提供 Pinax MCP 的官方 Go SDK candidate 运行时。
//
// 设计（pinax-mcp-official-sdk-v1）：本包不携带业务逻辑。工具、资源、
// schema 校验、错误与脱敏语义全部委托 internal/mcpserver.Server.Handle
// 单点 dispatch，两个 runtime（legacy 手写 stdio 与本包）对同一注册目录
// 投影，保证行为同源。4.2 真实客户端验收（2026-09-11）完成后，本包为
// 默认 runtime（pinax-mcp-official-sdk-v1 §4.4）；legacy 手写实现保留为
// 兼容窗口内的回退路径，经 PINAX_MCP_RUNTIME=legacy 显式启用。
package sdkruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/mcpserver"
)

// RuntimeName 是启用本运行时的 PINAX_MCP_RUNTIME 取值。
const RuntimeName = "official-sdk"

const (
	// negotiatedProtocolVersion 必须与 internal/mcpserver 的 modern 协议
	// 版本一致。SDK 完成 initialize 协商后，本包以该版本进入 Handle 的
	// modern 校验路径，与真实 2026-07-28 客户端等价。
	negotiatedProtocolVersion = "2026-07-28"
	defaultInstructions       = "Pinax exposes bounded read-only local-vault tools and resources."
)

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// eofReader 记录输入流是否已到达 EOF。stdio 服务器以 stdin 关闭为正常
// 会话终止，此时官方 SDK Run 返回的关闭错误应归一化为 nil，与 legacy
// runtime 的退出语义对齐。
type eofReader struct {
	io.Reader
	sawEOF *bool
}

func (r eofReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		*r.sawEOF = true
	}
	return n, err
}

// Serve 以官方 Go SDK stdio 运行时服务一次 MCP 会话。签名与
// mcpserver.ServeWithOptions 对齐，便于 CLI 层在同一装配线下切换。
func Serve(ctx context.Context, service *app.Service, vault string, in io.Reader, out io.Writer, options mcpserver.ServerOptions) error {
	server := New(service, vault, options)
	var sawEOF bool
	err := server.Run(ctx, &mcp.IOTransport{Reader: io.NopCloser(eofReader{Reader: in, sawEOF: &sawEOF}), Writer: nopWriteCloser{out}})
	if err != nil && sawEOF && ctx.Err() == nil {
		// 客户端关闭 stdin 属正常终止（EOF 竞态下 Read 可能不再被调用，
		// 此时保留原始错误），不向 CLI 上抛非零退出。
		return nil
	}
	return err
}

// New 基于统一注册目录构造官方 SDK server。所有 handler 委托
// mcpserver.Server.Handle，schema 校验与错误码与 legacy runtime 同源。
func New(service *app.Service, vault string, options mcpserver.ServerOptions) *mcp.Server {
	inner := mcpserver.NewServerWithOptions(service, vault, options)
	instructions := defaultInstructions
	if options.Collaboration {
		instructions = mcpserver.CollaborationInstructions()
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "pinax", Version: "dev"}, &mcp.ServerOptions{
		Instructions: instructions,
		// 正文/查询是用户内容：candidate 运行时默认丢弃 SDK 结构化日志，
		// 禁止 vault 内容经日志面外泄。
		Logger: slog.New(slog.DiscardHandler),
	})
	toolHandler := func(name string) mcp.ToolHandler {
		return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			params := map[string]any{"name": name}
			if args := decodeArguments(req.Params.Arguments); args != nil {
				params["arguments"] = args
			}
			resp, err := inner.Handle(ctx, mcpserver.Request{Method: "tools/call", Params: injectModernMeta(params)})
			if err != nil {
				return nil, wireError(err)
			}
			if resp.Error != nil {
				return nil, mcpErrorToWire(resp.Error)
			}
			return toolResult(resp.Result), nil
		}
	}
	tools := mcpserver.ToolInventory()
	if options.Collaboration {
		tools = append(tools, inner.CollaborationToolInventory()...)
	}
	if options.Input != nil {
		tools = append(tools, mcpserver.InputToolInventory()...)
	}
	for _, tool := range tools {
		server.AddTool(sdkTool(tool), toolHandler(tool.Name))
	}

	resourceHandler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params := map[string]any{"uri": req.Params.URI}
		resp, err := inner.Handle(ctx, mcpserver.Request{Method: "resources/read", Params: injectModernMeta(params)})
		if err != nil {
			return nil, wireError(err)
		}
		if resp.Error != nil {
			return nil, mcpErrorToWire(resp.Error)
		}
		return resourceResult(resp.Result)
	}
	resources := mcpserver.ConcreteResourceInventory()
	templates := mcpserver.ResourceTemplateInventory()
	if options.Collaboration {
		resources = append(resources, mcpserver.CollaborationResourceInventory()...)
		templates = append(templates, mcpserver.CollaborationResourceTemplateInventory()...)
	}
	if options.Input != nil {
		resources = append(resources, mcpserver.Resource{URI: "pinax://input/capabilities", Name: "Input capabilities"})
	}
	for _, resource := range resources {
		server.AddResource(sdkResource(resource), resourceHandler)
	}
	for _, template := range templates {
		server.AddResourceTemplate(sdkResourceTemplate(template), resourceHandler)
	}
	return server
}

// injectModernMeta 以协商出的 modern 协议版本与客户端能力元数据进入
// Handle 的 modern 校验路径。SDK 自身持有真实协商状态；Handle 只需要
// 等价的 modern 请求形态。
func injectModernMeta(params map[string]any) map[string]any {
	meta, _ := params["_meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		params["_meta"] = meta
	}
	meta["io.modelcontextprotocol/protocolVersion"] = negotiatedProtocolVersion
	if _, ok := meta["io.modelcontextprotocol/clientCapabilities"]; !ok {
		meta["io.modelcontextprotocol/clientCapabilities"] = map[string]any{}
	}
	return params
}

func decodeArguments(arguments any) map[string]any {
	switch value := arguments.(type) {
	case nil:
		return nil
	case map[string]any:
		return value
	case json.RawMessage:
		if len(value) == 0 || string(value) == "null" {
			return nil
		}
		var decoded map[string]any
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil
		}
		return decoded
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil
		}
		return decoded
	}
}

// sdkTool 把注册目录工具映射为官方 SDK 工具定义。annotations 投影与
// legacy runtime 的 standardToolDefinitions 一致。
func sdkTool(tool mcpserver.Tool) *mcp.Tool {
	readonly := tool.Readonly
	destructive := !tool.Readonly
	idempotent := true
	openWorld := false
	return &mcp.Tool{
		Name:         tool.Name,
		Description:  tool.Description,
		InputSchema:  tool.InputSchema,
		OutputSchema: tool.OutputSchema,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    readonly,
			DestructiveHint: &destructive,
			IdempotentHint:  idempotent,
			OpenWorldHint:   &openWorld,
		},
	}
}

func sdkResource(resource mcpserver.Resource) *mcp.Resource {
	return &mcp.Resource{URI: resource.URI, Name: resource.Name, Description: resource.Description, MIMEType: resource.MIMEType}
}

func sdkResourceTemplate(template map[string]any) *mcp.ResourceTemplate {
	uri, _ := template["uriTemplate"].(string)
	name, _ := template["name"].(string)
	description, _ := template["description"].(string)
	mimeType, _ := template["mimeType"].(string)
	return &mcp.ResourceTemplate{URITemplate: uri, Name: name, Description: description, MIMEType: mimeType}
}

// toolResult 把 Handle 的 tools/call 结果映射为官方 SDK 结果。Handle 已经
// 产出标准 tool-call envelope（content/structuredContent/isError），此处
// 只做结构搬运，不重复包装。
func toolResult(result map[string]any) *mcp.CallToolResult {
	if result == nil {
		result = map[string]any{}
	}
	out := &mcp.CallToolResult{}
	if structured, ok := result["structuredContent"]; ok {
		out.StructuredContent = structured
	}
	if isError, ok := result["isError"].(bool); ok {
		out.IsError = isError
	}
	appendText := func(text string) {
		out.Content = append(out.Content, &mcp.TextContent{Text: text})
	}
	switch contents := result["content"].(type) {
	case []map[string]any:
		for _, content := range contents {
			if content["type"] == "text" {
				if text, ok := content["text"].(string); ok {
					appendText(text)
				}
			}
		}
	case []any:
		for _, content := range contents {
			if typed, ok := content.(map[string]any); ok && typed["type"] == "text" {
				if text, ok := typed["text"].(string); ok {
					appendText(text)
				}
			}
		}
	}
	if len(out.Content) == 0 {
		raw, err := json.Marshal(result["structuredContent"])
		if err != nil {
			raw = []byte("{}")
		}
		appendText(string(raw))
	}
	return out
}

func resourceResult(result map[string]any) (*mcp.ReadResourceResult, error) {
	if result == nil {
		return nil, &jsonrpc.Error{Code: -32603, Message: "resource_result_missing"}
	}
	raw, err := json.Marshal(result["contents"])
	if err != nil {
		return nil, &jsonrpc.Error{Code: -32603, Message: "resource_encoding_failed"}
	}
	var contents []*mcp.ResourceContents
	if err := json.Unmarshal(raw, &contents); err != nil {
		return nil, &jsonrpc.Error{Code: -32603, Message: "resource_encoding_failed"}
	}
	if len(contents) == 0 {
		return nil, &jsonrpc.Error{Code: -32603, Message: "resource_encoding_failed"}
	}
	return &mcp.ReadResourceResult{Contents: contents}, nil
}

// wireError 保留 mcpserver 的错误码与结构化 data，其余错误以内部错误
// 上抛，避免正文或路径进错误消息。
func wireError(err error) error {
	if mcpErr, ok := err.(*mcpserver.MCPError); ok {
		return mcpErrorToWire(mcpErr)
	}
	return &jsonrpc.Error{Code: -32603, Message: "internal_error"}
}

func mcpErrorToWire(mcpErr *mcpserver.MCPError) error {
	wire := &jsonrpc.Error{Code: int64(mcpErr.Code), Message: mcpErr.Message}
	if mcpErr.Data != nil {
		if raw, err := json.Marshal(mcpErr.Data); err == nil {
			wire.Data = raw
		}
	}
	return wire
}

// String 返回运行时名，供诊断输出。
func String() string { return fmt.Sprintf("pinax mcp runtime=%s", RuntimeName) }
