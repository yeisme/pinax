package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishDocDownloadRemoteAssetSVG(t *testing.T) {
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte("<svg><rect/></svg>"))
	}))
	defer svc.Close()
	assetURL := useSafeRemoteAssetTestServer(t, svc)

	source, err := publishDocDownloadRemoteAsset(context.Background(), assetURL+"/diagram.svg")
	if err != nil {
		t.Fatalf("download svg: %v", err)
	}
	if !strings.Contains(source, "<svg>") {
		t.Fatalf("downloaded svg source = %q", source)
	}
}

func TestPublishDocDownloadRemoteAssetRejectsNonSVG(t *testing.T) {
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer svc.Close()
	assetURL := useSafeRemoteAssetTestServer(t, svc)

	if _, err := publishDocDownloadRemoteAsset(context.Background(), assetURL+"/diagram.svg"); err == nil {
		t.Fatalf("expected error for non-SVG content-type")
	}
}

func TestPublishDocDownloadRemoteAssetToFile(t *testing.T) {
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer svc.Close()
	assetURL := useSafeRemoteAssetTestServer(t, svc)

	root := t.TempDir()
	relPath, err := publishDocDownloadRemoteAssetToFile(context.Background(), root, assetURL+"/logo.png")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("read cached file: %v", err)
	}
	if string(body) != "PNGDATA" {
		t.Fatalf("cached body = %q", body)
	}
}

func TestPublishDocDownloadRemoteRejectsUnsafeTargets(t *testing.T) {
	urls := []string{
		"http://example.com/x.png",
		"https://127.0.0.1/x.png",
		"https://[::1]/x.png",
		"https://10.0.0.1/x.png",
		"https://172.16.0.1/x.png",
		"https://192.168.0.1/x.png",
		"https://169.254.1.1/x.png",
		"https://[fc00::1]/x.png",
		"https://[fe80::1]/x.png",
		"https://0.0.0.0/x.png",
		"https://224.0.0.1/x.png",
		"file:///etc/passwd",
	}
	for _, assetURL := range urls {
		if _, _, err := publishDocFetchRemote(context.Background(), assetURL); err == nil {
			t.Fatalf("%s must be rejected", assetURL)
		}
	}
}

func TestPublishDocDownloadRemoteRejectsResolvedPrivateTargets(t *testing.T) {
	prevLookup := remoteAssetLookupIP
	remoteAssetLookupIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("192.168.1.20")}}, nil
	}
	defer func() { remoteAssetLookupIP = prevLookup }()

	if _, _, err := publishDocFetchRemote(context.Background(), "https://assets.example/x.png"); err == nil {
		t.Fatalf("domain resolving to private IP must be rejected")
	}
}

func TestPublishDocDownloadRemoteRejectsRedirectToUnsafeTarget(t *testing.T) {
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://127.0.0.1/private.png", http.StatusFound)
	}))
	defer svc.Close()
	assetURL := useSafeRemoteAssetTestServer(t, svc)

	if _, _, err := publishDocFetchRemote(context.Background(), assetURL+"/redirect.png"); err == nil {
		t.Fatalf("redirect to loopback must be rejected")
	}
}

func TestPublishDocDownloadRemoteAllowsSafeHTTPSTestServer(t *testing.T) {
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer svc.Close()
	assetURL := useSafeRemoteAssetTestServer(t, svc)

	body, mediaType, err := publishDocFetchRemote(context.Background(), assetURL+"/safe.png")
	if err != nil {
		t.Fatalf("fetch safe test server: %v", err)
	}
	if string(body) != "PNGDATA" || mediaType != "image/png" {
		t.Fatalf("body/mediaType = %q/%q", body, mediaType)
	}
}

func TestPublishDocDownloadRemoteRejectsHTTPNonLoopback(t *testing.T) {
	if _, _, err := publishDocFetchRemote(context.Background(), "http://example.com/x.png"); err == nil {
		t.Fatalf("http must be rejected")
	}
	if _, _, err := publishDocFetchRemote(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatalf("file scheme must be rejected")
	}
}

func TestPublishDocReadImageDimensionsPNG(t *testing.T) {
	root := t.TempDir()
	rel := "test.png"
	if err := os.WriteFile(filepath.Join(root, rel), buildTestPNGHeader(3, 5), 0o644); err != nil {
		t.Fatal(err)
	}
	w, h := publishDocReadImageDimensions(root, rel)
	if w != 3 || h != 5 {
		t.Fatalf("dimensions = %dx%d, want 3x5", w, h)
	}
}

func TestPublishDocReadImageDimensionsNonPNG(t *testing.T) {
	root := t.TempDir()
	rel := "notimage.txt"
	if err := os.WriteFile(filepath.Join(root, rel), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, h := publishDocReadImageDimensions(root, rel)
	if w != 0 || h != 0 {
		t.Fatalf("non-PNG should return 0x0, got %dx%d", w, h)
	}
}

func useSafeRemoteAssetTestServer(t *testing.T, svc *httptest.Server) string {
	t.Helper()

	serverURL, err := url.Parse(svc.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	_, port, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatalf("split test server host: %v", err)
	}
	const host = "example.com"

	prevClient := remoteAssetClient
	prevLookup := remoteAssetLookupIP
	client := svc.Client()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport %T", client.Transport)
	}
	clonedTransport := transport.Clone()
	clonedTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, svc.Listener.Addr().String())
	}
	client.Transport = clonedTransport
	remoteAssetClient = client
	remoteAssetLookupIP = func(ctx context.Context, lookupHost string) ([]net.IPAddr, error) {
		if lookupHost == host {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return prevLookup(ctx, lookupHost)
	}
	t.Cleanup(func() {
		remoteAssetClient = prevClient
		remoteAssetLookupIP = prevLookup
	})

	return "https://" + net.JoinHostPort(host, port)
}

// buildTestPNGHeader 构造 PNG 文件前 24 字节（signature + IHDR chunk 头 + width + height），
// 足够让 publishDocReadImageDimensions 读取宽高。dimension reader 只看前 24 字节。
func buildTestPNGHeader(width, height int) []byte {
	sig := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	// IHDR chunk length = 13（big-endian）。
	length := []byte{0, 0, 0, 13}
	chunkType := []byte{'I', 'H', 'D', 'R'}
	w := []byte{byte(width >> 24), byte(width >> 16), byte(width >> 8), byte(width)}
	h := []byte{byte(height >> 24), byte(height >> 16), byte(height >> 8), byte(height)}
	out := append([]byte{}, sig...)
	out = append(out, length...)
	out = append(out, chunkType...)
	out = append(out, w...)
	out = append(out, h...)
	return out
}

func TestPublishDocParseManagedAssetBlockIDs(t *testing.T) {
	content := `<title>Doc</title><whiteboard id="user_wb" token="user"></whiteboard><img id="user_img" name="manual.png"></img><figure id="user_fig"></figure>`
	if got := publishDocManagedAssetBlockIDs(content, nil); len(got) != 0 {
		t.Fatalf("unmanaged cloud blocks must not be selected for cleanup: %+v", got)
	}
	got := publishDocManagedAssetBlockIDs(content, []string{"pinax_img", "pinax_fig"})
	if strings.Join(got, ",") != "pinax_img,pinax_fig" {
		t.Fatalf("manifest IDs should drive cleanup, got %+v", got)
	}
}

func TestParsePublishDocMediaInsertBlockID(t *testing.T) {
	body := []byte(`{"ok":true,"data":{"block_id":"blk_img","file_token":"tok"}}`)
	if got := parsePublishDocMediaInsertBlockID(body); got != "blk_img" {
		t.Fatalf("block id = %q", got)
	}
}
