package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// callLogPath 由测试通过 PINAX_FAKE_LARK_CALLLOG 环境变量指定。
// fake 把每次被调用的命令摘要追加到该文件，供 e2e 断言 renderer=native-docx 时
// 没有走 markdown +create/+overwrite（spec 要求拒绝 silent fallback）。
func main() {
	args := os.Args
	logCall(args)

	if len(args) > 2 && args[1] == "drive" && args[2] == "+create-folder" {
		fmt.Println(`{"status":"ok","id":"fake_folder_token","url":"https://example.test/folder/fake_folder_token"}`)
		return
	}
	if len(args) > 2 && args[1] == "drive" && args[2] == "+move" {
		fmt.Println(`{"status":"ok"}`)
		return
	}
	// drive +inspect 返回原生文档类型，模拟真实 lark-cli drive +inspect 的 data.type。
	if len(args) > 2 && args[1] == "drive" && args[2] == "+inspect" {
		fmt.Println(`{"ok":true,"data":{"type":"docx","token":"fake_docx_token","url":"https://example.test/docx/fake_docx_token"}}`)
		return
	}
	if len(args) > 2 && args[1] == "auth" && args[2] == "status" {
		fmt.Println(`{"identities":{"user":{"status":"ready","available":true},"bot":{"status":"ready","available":true}}}`)
		return
	}
	// docs +create：原生文档创建。支持 --dry-run（capability probe）和真实创建。
	// 响应结构模拟真实 lark-cli：data.document.{document_id,url}，URL 指向 /docx/。
	if len(args) > 2 && args[1] == "docs" && args[2] == "+create" {
		if hasFlag(args, "--dry-run") {
			fmt.Println(`{"ok":true,"data":{"dry_run":true}}`)
			return
		}
		fmt.Println(`{"ok":true,"data":{"document":{"document_id":"fake_docx_token","revision_id":2,"url":"https://example.test/docx/fake_docx_token"}}}`)
		return
	}
	// docs +update：原生文档更新。支持 --command append（whiteboard 插入）和 overwrite（正文重写）。
	if len(args) > 2 && args[1] == "docs" && args[2] == "+update" {
		token := flagValue(args, "--doc")
		if token == "" {
			token = "fake_docx_token"
		}
		rev := 3
		if flagValue(args, "--command") == "append" {
			rev = 4
		}
		fmt.Printf(`{"ok":true,"data":{"document":{"document_id":%q,"revision_id":%d,"url":"https://example.test/docx/%s"}}}`+"\n", token, rev, token)
		return
	}
	// docs +fetch：返回带 whiteboard/img/figure 的 XML 内容（供资产插入管线提取 token / 清理 block）。
	if len(args) > 2 && args[1] == "docs" && args[2] == "+fetch" {
		token := flagValue(args, "--doc")
		if token == "" {
			token = "fake_docx_token"
		}
		fmt.Printf(`{"ok":true,"data":{"document":{"document_id":%q,"content":"<title>Doc</title><p>body</p><whiteboard id=\"wb_1\" token=\"fake_whiteboard_token\"></whiteboard><img id=\"img_1\" name=\"a.png\"></img><figure id=\"fig_1\"><source token=\"fake_file_token\" mime=\"application/pdf\"/></figure>","revision_id":4}}}`+"\n", token)
		return
	}
	// whiteboard +update：服务端渲染 Mermaid/SVG 到白板。
	if len(args) > 2 && args[1] == "whiteboard" && args[2] == "+update" {
		fmt.Println(`{"ok":true,"data":{"created_node_id":"t1:1"}}`)
		return
	}
	// docs +media-insert：上传本地图片到文档。
	if len(args) > 2 && args[1] == "docs" && args[2] == "+media-insert" {
		fmt.Println(`{"ok":true,"data":{"block_id":"fake_image_block","file_token":"fake_media_token","type":"image"}}`)
		return
	}
	if len(args) > 2 && args[1] == "docs" && args[2] == "+media-upload" {
		fmt.Println(`{"ok":true,"data":{"file_token":"fake_media_token"}}`)
		return
	}
	if len(args) > 2 && args[1] == "markdown" && (args[2] == "+create" || args[2] == "+overwrite") {
		fmt.Println(`{"status":"ok","id":"fake_lark_doc","url":"https://example.test/lark/fake_lark_doc","type":"file"}`)
		return
	}
	if len(args) > 1 && args[1] == "status" {
		fmt.Println(`{"status":"ok","provider":"feishu"}`)
		return
	}

	hasEvents := false
	for _, arg := range args {
		if arg == "--events" {
			hasEvents = true
		}
	}

	if hasEvents {
		fmt.Println(`{"type":"start","seq":1}`)
		fmt.Println(`{"type":"event","seq":2,"data":{"synced":true}}`)
		fmt.Println(`{"type":"end","seq":3,"status":"success"}`)
		return
	}

	fmt.Println(`{"mocked":true,"cli":"lark-cli"}`)
}

func logCall(args []string) {
	path := os.Getenv("PINAX_FAKE_LARK_CALLLOG")
	if path == "" {
		return
	}
	summary := commandSummary(args)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_ = json.NewEncoder(f).Encode(summary)
}

// commandSummary 提取被调用命令的可读摘要，用于断言。
func commandSummary(args []string) string {
	if len(args) < 3 {
		return strings.Join(args[1:], " ")
	}
	return strings.Join(args[1:3], " ")
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
