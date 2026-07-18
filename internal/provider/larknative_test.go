package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

type recordedLarkNativeCommand struct {
	dir   string
	name  string
	args  []string
	stdin string
}

type recordingLarkNativeRunner struct {
	commands []recordedLarkNativeCommand
	outputs  [][]byte
	err      error
}

func (r *recordingLarkNativeRunner) Run(_ context.Context, cmd LarkNativeCLICommand) ([]byte, error) {
	r.commands = append(r.commands, recordedLarkNativeCommand{dir: cmd.Dir, name: cmd.Name, args: append([]string{}, cmd.Args...), stdin: cmd.Stdin})
	if r.err != nil {
		return nil, r.err
	}
	if len(r.outputs) == 0 {
		return nil, nil
	}
	out := r.outputs[0]
	r.outputs = r.outputs[1:]
	return out, nil
}

// fakeAdapter 是测试用 in-memory LarkNativeDocAdapter。
// 它模拟 lark-cli 原生文档能力，供 app 层和 provider 契约测试使用。
type fakeAdapter struct {
	capable       bool
	createdInputs []NativeDocCreateInput
	updatedInputs []NativeDocUpdateInput
	uploaded      []MediaUploadInput
	resultType    string
}

func (f *fakeAdapter) CheckCapability(context.Context) error {
	if !f.capable {
		return ErrCapabilityMissing
	}
	return nil
}

func (f *fakeAdapter) CreateNativeDoc(_ context.Context, input NativeDocCreateInput) (NativeDocResult, error) {
	if !f.capable {
		return NativeDocResult{}, ErrCapabilityMissing
	}
	f.createdInputs = append(f.createdInputs, input)
	objType := f.resultType
	if objType == "" {
		objType = domain.PublishDocObjectTypeDocx
	}
	return NativeDocResult{Token: "fake_docx_token", URL: "https://example.test/docx/fake_docx_token", Type: objType}, nil
}

func (f *fakeAdapter) UpdateNativeDoc(_ context.Context, input NativeDocUpdateInput) (NativeDocResult, error) {
	if !f.capable {
		return NativeDocResult{}, ErrCapabilityMissing
	}
	f.updatedInputs = append(f.updatedInputs, input)
	return NativeDocResult{Token: input.DocToken, URL: "https://example.test/docx/" + input.DocToken, Type: domain.PublishDocObjectTypeDocx}, nil
}

func (f *fakeAdapter) UploadMedia(_ context.Context, input MediaUploadInput) (string, error) {
	if !f.capable {
		return "", ErrCapabilityMissing
	}
	f.uploaded = append(f.uploaded, input)
	return "fake_media_token", nil
}

func (f *fakeAdapter) InspectObjectType(context.Context, string) (string, error) {
	return domain.PublishDocObjectTypeDocx, nil
}

func TestLarkNativeDocCreateReturnsDocxType(t *testing.T) {
	adapter := &fakeAdapter{capable: true}
	result, err := adapter.CreateNativeDoc(context.Background(), NativeDocCreateInput{Title: "T", FolderToken: "fld"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if result.Type != domain.PublishDocObjectTypeDocx {
		t.Fatalf("type = %q, want docx", result.Type)
	}
	if result.Token == "" || result.URL == "" {
		t.Fatalf("missing token/url: %+v", result)
	}
}

func TestLarkNativeDocCapabilityMissingNotSilentFallback(t *testing.T) {
	adapter := &fakeAdapter{capable: false}
	_, err := adapter.CreateNativeDoc(context.Background(), NativeDocCreateInput{Title: "T"})
	if !errors.Is(err, ErrCapabilityMissing) {
		t.Fatalf("expected ErrCapabilityMissing, got %v", err)
	}
	// 关键：capability missing 时 adapter 不得静默调用 markdown +create。
	// fakeAdapter 在 incapable 时直接返回错误，证明契约要求 fail-fast。
}

func TestPublishDocProviderCapabilityResolveObjectType(t *testing.T) {
	tests := []struct {
		renderer     domain.PublishDocRenderer
		providerType string
		want         string
	}{
		{domain.PublishDocRendererNativeDocx, "docx", "docx"},
		{domain.PublishDocRendererNativeDocx, "doc", "doc"},
		{domain.PublishDocRendererNativeDocx, "", "docx"},
		{domain.PublishDocRendererMarkdownFile, "", "file"},
	}
	for _, tc := range tests {
		got := ResolveExternalObjectType(tc.renderer, tc.providerType)
		if got != tc.want {
			t.Fatalf("ResolveExternalObjectType(%q, %q) = %q, want %q", tc.renderer, tc.providerType, got, tc.want)
		}
	}
}

func TestLarkNativeDocUploadMediaReturnsToken(t *testing.T) {
	adapter := &fakeAdapter{capable: true}
	token, err := adapter.UploadMedia(context.Background(), MediaUploadInput{Path: "assets/img.png", MediaType: "image/png"})
	if err != nil || token == "" {
		t.Fatalf("upload media failed: %v token=%q", err, token)
	}
}

func TestLarkNativeDocInspectObjectType(t *testing.T) {
	adapter := &fakeAdapter{capable: true}
	objType, err := adapter.InspectObjectType(context.Background(), "some_token")
	if err != nil || objType != domain.PublishDocObjectTypeDocx {
		t.Fatalf("inspect = %q err=%v", objType, err)
	}
}

func TestLarkNativeCLIAdapterCheckCapabilityBuildsDryRunProbe(t *testing.T) {
	runner := &recordingLarkNativeRunner{}
	adapter := NewLarkNativeDocCLIAdapter(domain.PublishDocProfile{As: "user"}, runner)

	if err := adapter.CheckCapability(context.Background()); err != nil {
		t.Fatalf("CheckCapability failed: %v", err)
	}

	want := []string{"docs", "+create", "--dry-run", "--content", "pinax capability probe", "--doc-format", "markdown", "--json", "--as", "user"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].args, want) {
		t.Fatalf("capability args = %#v, want %#v", runner.commands, want)
	}
}

func TestLarkNativeCLIAdapterCreateBuildsCommandAndParsesDocument(t *testing.T) {
	runner := &recordingLarkNativeRunner{outputs: [][]byte{[]byte(`noise {"data":{"document":{"document_id":"doc123","url":"https://www.feishu.cn/docx/doc123"}}}`)}}
	adapter := NewLarkNativeDocCLIAdapter(domain.PublishDocProfile{As: "user"}, runner)

	result, err := adapter.CreateNativeDoc(context.Background(), NativeDocCreateInput{Title: "Title", FolderToken: "fld", ContentPath: ".pinax/tmp.md"})
	if err != nil {
		t.Fatalf("CreateNativeDoc failed: %v", err)
	}

	wantArgs := []string{"docs", "+create", "--title", "Title", "--content", "@.pinax/tmp.md", "--doc-format", "markdown", "--json", "--parent-token", "fld", "--as", "user"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].args, wantArgs) {
		t.Fatalf("create args = %#v, want %#v", runner.commands, wantArgs)
	}
	if result.Token != "doc123" || result.URL != "https://www.feishu.cn/docx/doc123" || result.Type != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestLarkNativeCLIAdapterUpdateBuildsCommandAndUsesFallbacks(t *testing.T) {
	runner := &recordingLarkNativeRunner{outputs: [][]byte{[]byte(`{"ok":true}`)}}
	adapter := NewLarkNativeDocCLIAdapter(domain.PublishDocProfile{}, runner)

	result, err := adapter.UpdateNativeDoc(context.Background(), NativeDocUpdateInput{DocToken: "doc123", ExistingURL: "https://old.example/doc123", ContentPath: ".pinax/tmp.md"})
	if err != nil {
		t.Fatalf("UpdateNativeDoc failed: %v", err)
	}

	wantArgs := []string{"docs", "+update", "--doc", "doc123", "--command", "overwrite", "--content", "@.pinax/tmp.md", "--doc-format", "markdown", "--json"}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].args, wantArgs) {
		t.Fatalf("update args = %#v, want %#v", runner.commands, wantArgs)
	}
	if result.Token != "doc123" || result.URL != "https://old.example/doc123" {
		t.Fatalf("result = %+v", result)
	}
}

func TestLarkNativeCLIAdapterUploadMediaBuildsCommandAndParsesBlockID(t *testing.T) {
	runner := &recordingLarkNativeRunner{outputs: [][]byte{[]byte(`{"data":{"block_id":"img_block"}}`)}}
	adapter := NewLarkNativeDocCLIAdapter(domain.PublishDocProfile{}, runner)

	blockID, err := adapter.UploadMedia(context.Background(), MediaUploadInput{Root: "/vault", DocToken: "doc123", Path: "notes/img.png", MediaType: "image", Caption: "Alt", Width: 800})
	if err != nil {
		t.Fatalf("UploadMedia failed: %v", err)
	}

	wantArgs := []string{"docs", "+media-insert", "--doc", "doc123", "--file", "./notes/img.png", "--type", "image", "--json", "--caption", "Alt", "--width", "800"}
	if len(runner.commands) != 1 || runner.commands[0].dir != "/vault" || !reflect.DeepEqual(runner.commands[0].args, wantArgs) {
		t.Fatalf("media command = %#v, want dir /vault args %#v", runner.commands, wantArgs)
	}
	if blockID != "img_block" {
		t.Fatalf("blockID = %q, want img_block", blockID)
	}
}
