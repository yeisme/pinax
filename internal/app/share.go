package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type ShareRequest struct {
	VaultPath string
	Profile   string
	Out       string
	Scope     string
	Host      string
	Port      int
	AllowLAN  bool
	Readonly  bool
	NoAuth    bool
	TokenFile string
	Once      bool
	View      string
	NoEmbed   bool
}

func (s *Service) ShareStart(ctx context.Context, req ShareRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("share.start", err), err
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "published"
	}
	if scope != "published" && scope != "vault-readonly" {
		cmdErr := &domain.CommandError{Code: "share_scope_invalid", Message: "share scope must be published or vault-readonly", Hint: "Use --scope published for static publish output"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	if !req.Readonly {
		cmdErr := &domain.CommandError{Code: "share_readonly_required", Message: "share start requires --readonly", Hint: "Rerun with --readonly"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	view := strings.TrimSpace(req.View)
	if view == "" {
		view = "published"
	}
	if view != "published" && view != "explore" {
		cmdErr := &domain.CommandError{Code: "share_view_invalid", Message: "share view must be published or explore", Hint: "Use --view explore with --scope vault-readonly"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	if view == "explore" && scope != "vault-readonly" {
		cmdErr := &domain.CommandError{Code: "share_view_scope_invalid", Message: "--view explore requires --scope vault-readonly", Hint: "Rerun with --scope vault-readonly --view explore --readonly"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	loopback := shareHostLoopback(host)
	if !loopback && !req.AllowLAN {
		cmdErr := &domain.CommandError{Code: "share_allow_lan_required", Message: "non-loopback share requires --allow-lan", Hint: "Rerun with --allow-lan --readonly and an explicit auth mode"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	if req.NoAuth && !loopback {
		cmdErr := &domain.CommandError{Code: "share_auth_required", Message: "--no-auth is only allowed on loopback hosts", Hint: "Use --token-file for LAN share"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	if scope == "vault-readonly" && strings.TrimSpace(req.TokenFile) == "" && !req.NoAuth {
		cmdErr := &domain.CommandError{Code: "share_auth_required", Message: "vault-readonly share requires token auth", Hint: "Pass --token-file <path> for vault-readonly share"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	authToken, err := shareAuthToken(req)
	if err != nil {
		cmdErr := &domain.CommandError{Code: "share_auth_required", Message: "share token file could not be read", Hint: "Check --token-file permissions"}
		return domain.NewErrorProjection("share.start", cmdErr), cmdErr
	}
	outDir := ""
	if scope == "published" {
		var cmdErr *domain.CommandError
		outDir, cmdErr = cleanPublishOutPath(req.Out)
		if cmdErr != nil {
			return domain.NewErrorProjection("share.start", cmdErr), cmdErr
		}
		if _, err := os.Stat(outDir); err != nil {
			cmdErr := &domain.CommandError{Code: "share_output_required", Message: "published share requires an existing publish output", Hint: "Run pinax publish build --target local before share start"}
			return domain.NewErrorProjection("share.start", cmdErr), cmdErr
		}
	}
	var port int
	var listener net.Listener
	var exploreBundle ExploreBundle
	if view == "explore" {
		// explore bundle 全内存组装：不写 vault 或 .pinax/**；prepare 阶段即产出
		// 有界计数反馈，--once 冒烟复用同一份 bundle。
		exploreBundle, err = BuildExploreBundle(root)
		if err != nil {
			return errorProjection("share.start", err), err
		}
	}
	if req.Once {
		listener, err = net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(req.Port)))
		if err != nil {
			return errorProjection("share.start", err), err
		}
		port = listener.Addr().(*net.TCPAddr).Port
	} else {
		port, err = sharePort(host, req.Port)
		if err != nil {
			return errorProjection("share.start", err), err
		}
	}
	baseURL := "http://" + net.JoinHostPort(host, fmt.Sprint(port))
	projection := domain.NewProjection("share.start", "Share endpoint prepared.")
	projection.Facts["scope"] = scope
	projection.Facts["host"] = host
	projection.Facts["port"] = fmt.Sprint(port)
	projection.Facts["readonly"] = fmt.Sprint(req.Readonly)
	projection.Facts["allow_lan"] = fmt.Sprint(req.AllowLAN)
	projection.Facts["auth"] = shareAuthMode(req)
	projection.Facts["web_url"] = baseURL + "/"
	projection.Facts["api_url"] = baseURL + "/api/"
	projection.Facts["served"] = fmt.Sprint(req.Once)
	projection.Data = map[string]any{"scope": scope, "web_url": projection.Facts["web_url"], "api_url": projection.Facts["api_url"]}
	if view == "explore" {
		projection.Facts["view"] = "explore"
		projection.Facts["explore_url"] = baseURL + "/explore"
		projection.Facts["explore_nodes"] = fmt.Sprint(exploreBundle.Counts.Nodes)
		projection.Facts["explore_edges"] = fmt.Sprint(exploreBundle.Counts.Edges)
		projection.Facts["explore_truncated"] = fmt.Sprint(exploreBundle.Counts.Truncated)
		projection.Facts["explore_embed"] = fmt.Sprint(!req.NoEmbed)
		projection.Data = map[string]any{"scope": scope, "view": "explore", "web_url": projection.Facts["web_url"], "explore_url": projection.Facts["explore_url"], "api_url": projection.Facts["api_url"]}
		if shareAuthMode(req) == "token-file" {
			projection.Actions = []domain.Action{{Name: "open_explore", Command: "open " + projection.Facts["explore_url"] + "?token=<share token>"}}
		}
	}
	if req.Once {
		var webOK, apiOK bool
		if view == "explore" {
			webOK, apiOK, err = shareServeOnceExplore(ctx, listener, shareHandler(scope, root, outDir, authToken, view, exploreBundle, !req.NoEmbed), host, port, authToken, exploreBundle)
		} else {
			webOK, apiOK, err = shareServeOnce(ctx, listener, shareHandler(scope, root, outDir, authToken, view, exploreBundle, false), host, port, authToken)
		}
		if err != nil {
			cmdErr := &domain.CommandError{Code: "share_smoke_failed", Message: err.Error(), Hint: "Check publish output and share route readiness"}
			return domain.NewErrorProjection("share.start", cmdErr), cmdErr
		}
		projection.Facts["web_smoke"] = fmt.Sprint(webOK)
		projection.Facts["api_smoke"] = fmt.Sprint(apiOK)
	}
	return projection, nil
}

func shareHandler(scope, root, outDir, authToken, view string, exploreBundle ExploreBundle, exploreEmbed bool) http.Handler {
	if scope == "published" {
		return sharePublishedHandler(outDir)
	}
	if scope == "vault-readonly" {
		if view == "explore" {
			return shareVaultExploreHandler(root, authToken, exploreBundle, exploreEmbed, time.Now().UTC())
		}
		return shareVaultReadonlyHandler(root, authToken)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/share/status", func(w http.ResponseWriter, r *http.Request) {
		writeShareJSON(w, map[string]any{"scope": scope, "readonly": true})
	})
	return shareReadOnlyHandler(mux)
}

// shareVaultExploreHandler 在既有 vault-readonly 只读面上追加 /explore* 路由：
// /explore（自包含 HTML）、/explore/data.json（有界 bundle）、/explore/note/<id>
// （有界 note 预览）。全部经既有 token/loopback 认证与 GET-only 门禁。
func shareVaultExploreHandler(root, authToken string, bundle ExploreBundle, embedData bool, now time.Time) http.Handler {
	mux := shareVaultReadonlyRoutes(root)
	mux.HandleFunc("/explore", func(w http.ResponseWriter, r *http.Request) {
		page, err := RenderExplorePage(bundle, embedData)
		if err != nil {
			http.Error(w, "explore page unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("/explore/data.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeShareJSON(w, bundle)
	})
	mux.HandleFunc("/explore/note/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/explore/note/")
		notes, err := scanNotes(root)
		if err != nil {
			http.Error(w, "explore note unavailable", http.StatusInternalServerError)
			return
		}
		note, err := exploreResolveNote(notes, id)
		if err != nil {
			http.Error(w, "explore note not found", http.StatusNotFound)
			return
		}
		writeShareJSON(w, map[string]any{
			"schema_version": ExploreBundleSchemaVersion,
			"id":             exploreNodeID(note),
			"title":          note.Title,
			"kind":           note.Kind,
			"tags":           note.Tags,
			"trust":          domain.TrustTierOf(note.Trust),
			"fresh":          domain.FreshnessOf(note.Trust, now),
			"summary":        exploreNoteSummary(note),
			"updated_at":     note.UpdatedAt,
			"preview":        exploreNotePreview(note),
		})
	})
	return shareAuthHandler(authToken, shareReadOnlyHandler(mux))
}

func shareVaultReadonlyHandler(root, authToken string) http.Handler {
	mux := shareVaultReadonlyRoutes(root)
	return shareAuthHandler(authToken, shareReadOnlyHandler(mux))
}

func shareVaultReadonlyRoutes(root string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><body><h1>Pinax vault readonly share</h1></body></html>"))
	})
	mux.HandleFunc("/api/share/status", func(w http.ResponseWriter, r *http.Request) {
		writeShareJSON(w, map[string]any{"scope": "vault-readonly", "readonly": true, "exposure": "card"})
	})
	mux.HandleFunc("/api/share/notes", func(w http.ResponseWriter, r *http.Request) {
		payload, err := shareVaultReadonlyNotesPayload(root)
		if err != nil {
			http.Error(w, "share notes unavailable", http.StatusInternalServerError)
			return
		}
		writeShareJSON(w, payload)
	})
	return mux
}

func shareVaultReadonlyNotesPayload(root string) (map[string]any, error) {
	facts, err := scanNoteFacts(root)
	if err != nil {
		return nil, err
	}
	notes := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		notes = append(notes, map[string]any{
			"id":       fact.note.ID,
			"title":    fact.note.Title,
			"path":     fact.note.Path,
			"tags":     fact.note.Tags,
			"kind":     fact.note.Kind,
			"status":   fact.note.Status,
			"display":  "card",
			"exposure": "metadata-only",
		})
	}
	return map[string]any{"scope": "vault-readonly", "notes": notes}, nil
}

func shareAuthHandler(authToken string, next http.Handler) http.Handler {
	if strings.TrimSpace(authToken) == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 浏览器导航无法携带 Authorization 头：explore 页面以 ?token= 查询参数
		// 完成同等认证（浏览器端 JS 再把它转为 Bearer 头）。
		if r.Header.Get("Authorization") != "Bearer "+authToken && r.URL.Query().Get("token") != authToken {
			http.Error(w, "share auth required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sharePublishedHandler(outDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/share/notes", func(w http.ResponseWriter, r *http.Request) {
		payload, err := sharePublishedNotesPayload(outDir)
		if err != nil {
			http.Error(w, "share notes unavailable", http.StatusInternalServerError)
			return
		}
		writeShareJSON(w, payload)
	})
	mux.Handle("/", http.FileServer(http.Dir(outDir)))
	return shareReadOnlyHandler(mux)
}

func sharePublishedNotesPayload(outDir string) (map[string]any, error) {
	body, err := os.ReadFile(filepath.Join(outDir, "pinax-data", "search-index.json"))
	if err != nil {
		return map[string]any{"scope": "published", "notes": []any{}}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	entries, _ := raw["entries"].([]any)
	notes := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		bounded := map[string]any{}
		for _, key := range []string{"id", "title", "path", "tags", "kind", "status"} {
			if value, ok := item[key]; ok {
				bounded[key] = value
			}
		}
		notes = append(notes, bounded)
	}
	return map[string]any{"scope": "published", "notes": notes}, nil
}

func shareReadOnlyHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "share is read-only", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeShareJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func shareServeOnce(ctx context.Context, listener net.Listener, handler http.Handler, host string, port int, authToken string) (bool, bool, error) {
	server := &http.Server{Handler: handler}
	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := "http://" + net.JoinHostPort(shareSmokeHost(host), fmt.Sprint(port))
	webOK, err := shareSmokeGET(client, baseURL+"/", authToken)
	if err != nil {
		_ = server.Shutdown(context.Background())
		return false, false, err
	}
	apiOK, err := shareSmokeGET(client, baseURL+"/api/share/notes", authToken)
	if err != nil {
		_ = server.Shutdown(context.Background())
		return webOK, false, err
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return webOK, apiOK, err
	}
	if err := <-errCh; err != nil {
		return webOK, apiOK, err
	}
	return webOK, apiOK, nil
}

// shareServeOnceExplore 执行 explore 冒烟：依次请求 /explore、/explore/data.json
// 与一个 note 有界预览并校验响应；对返回的 HTML 页面执行外链扫描（零命中）；
// token 模式下额外验证未认证请求被 401 拒绝且不泄漏 bundle 数据。
func shareServeOnceExplore(ctx context.Context, listener net.Listener, handler http.Handler, host string, port int, authToken string, bundle ExploreBundle) (bool, bool, error) {
	server := &http.Server{Handler: handler}
	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := "http://" + net.JoinHostPort(shareSmokeHost(host), fmt.Sprint(port))
	fail := func(webOK bool, err error) (bool, bool, error) {
		_ = server.Shutdown(context.Background())
		return webOK, false, err
	}

	webOK, pageBody, err := shareSmokeGETBody(client, baseURL+"/explore", authToken)
	if err != nil {
		return fail(webOK, err)
	}
	if refs := ScanExploreExternalRefs(pageBody); len(refs) > 0 {
		return fail(webOK, fmt.Errorf("explore page leaked external refs: %s", strings.Join(refs, ", ")))
	}
	if authToken != "" {
		unauthResp, unauthErr := client.Get(baseURL + "/explore/data.json")
		if unauthErr != nil {
			return fail(webOK, unauthErr)
		}
		leakBody, _ := io.ReadAll(io.LimitReader(unauthResp.Body, 4096))
		_ = unauthResp.Body.Close()
		if unauthResp.StatusCode != http.StatusUnauthorized {
			return fail(webOK, fmt.Errorf("unauthenticated /explore/data.json returned status %d", unauthResp.StatusCode))
		}
		if strings.Contains(string(leakBody), ExploreBundleSchemaVersion) {
			return fail(webOK, errors.New("unauthenticated /explore/data.json leaked bundle data"))
		}
	}
	apiOK, dataBody, err := shareSmokeGETBody(client, baseURL+"/explore/data.json", authToken)
	if err != nil {
		return fail(webOK, err)
	}
	var decoded ExploreBundle
	if err := json.Unmarshal(dataBody, &decoded); err != nil {
		return fail(webOK, fmt.Errorf("explore data.json is not a valid bundle: %w", err))
	}
	if decoded.SchemaVersion != ExploreBundleSchemaVersion {
		return fail(webOK, fmt.Errorf("explore data.json schema version = %q", decoded.SchemaVersion))
	}
	noteOK := false
	if len(decoded.Nodes) > 0 {
		if _, _, noteErr := shareSmokeGETBody(client, baseURL+"/explore/note/"+url.PathEscape(decoded.Nodes[0].ID), authToken); noteErr != nil {
			return fail(webOK, noteErr)
		}
		noteOK = true
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return webOK, apiOK, err
	}
	if err := <-errCh; err != nil {
		return webOK, apiOK, err
	}
	if !noteOK {
		return webOK, apiOK, errors.New("explore smoke skipped note preview (bundle has no nodes)")
	}
	return webOK, apiOK, nil
}

func shareSmokeGETBody(client *http.Client, url string, authToken string) (bool, []byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, nil, err
	}
	if strings.TrimSpace(authToken) != "" {
		request.Header.Set("Authorization", "Bearer "+authToken)
	}
	resp, err := client.Do(request)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return false, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, body, fmt.Errorf("share smoke returned status %d for %s", resp.StatusCode, url)
	}
	return true, body, nil
}

func shareSmokeGET(client *http.Client, url string, authToken string) (bool, error) {
	ok, _, err := shareSmokeGETBody(client, url, authToken)
	return ok, err
}

func shareSmokeHost(host string) string {
	if host == "0.0.0.0" || host == "::" || host == "" {
		return "127.0.0.1"
	}
	return host
}

func shareHostLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sharePort(host string, requested int) (int, error) {
	if requested != 0 {
		return requested, nil
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

func shareAuthMode(req ShareRequest) string {
	if req.NoAuth {
		return "none"
	}
	if strings.TrimSpace(req.TokenFile) != "" {
		return "token-file"
	}
	return "loopback"
}

func shareAuthToken(req ShareRequest) (string, error) {
	if strings.TrimSpace(req.TokenFile) == "" {
		return "", nil
	}
	body, err := os.ReadFile(req.TokenFile)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(body))
	if token == "" {
		return "", fmt.Errorf("share token file is empty")
	}
	return token, nil
}
