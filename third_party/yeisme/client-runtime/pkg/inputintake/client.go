package inputintake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// TransferClient consumes only a transient input grant, never service credentials.
// Control-plane prepare/renew/status remain with the owner's MCP identity.
type TransferClient struct {
	target, grant string
	http          *http.Client
}

func NewTransferClient(ownerBase, link string, client *http.Client) (*TransferClient, error) {
	if !CheckTransientLink(link) {
		return nil, ErrInvalid
	}
	target, fragment, ok := strings.Cut(link, "#grant=")
	if !ok || !strings.HasPrefix(target, strings.TrimRight(ownerBase, "/")+Prefix) {
		return nil, ErrDenied
	}
	remainder := strings.TrimPrefix(target, strings.TrimRight(ownerBase, "/")+Prefix)
	if !validID(remainder) {
		return nil, ErrDenied
	}
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	copy.Jar = nil
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &TransferClient{target: target, grant: fragment, http: &copy}, nil
}
func (c *TransferClient) call(ctx context.Context, method, operation string, body io.Reader, length int64) (View, error) {
	req, e := http.NewRequestWithContext(ctx, method, c.target+"/"+operation, body)
	if e != nil {
		return View{}, ErrInvalid
	}
	req.ContentLength = length
	req.Header.Set(GrantHeader, c.grant)
	req.Header.Set("Content-Type", "application/json")
	if operation == "content" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	response, e := c.http.Do(req)
	if e != nil {
		return View{}, errors.New("input transfer interrupted; query the original request")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return View{}, errors.New("input transfer rejected; query or renew the original request")
	}
	var result View
	if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result) != nil || result.SchemaVersion != Schema {
		return View{}, errors.New("input response contract is invalid")
	}
	return result, nil
}
func (c *TransferClient) Bind(ctx context.Context, file File) (View, error) {
	b, e := json.Marshal(file)
	if e != nil {
		return View{}, ErrInvalid
	}
	return c.call(ctx, "POST", "file", bytes.NewReader(b), int64(len(b)))
}
func (c *TransferClient) Put(ctx context.Context, size int64, body io.Reader) (View, error) {
	if size <= 0 || body == nil {
		return View{}, ErrInvalid
	}
	return c.call(ctx, "PUT", "content", io.LimitReader(body, size), size)
}
func (c *TransferClient) Complete(ctx context.Context) (View, error) {
	return c.call(ctx, "POST", "complete", strings.NewReader("{}"), 2)
}
