package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Assignment struct {
	KeyID          string `json:"key_ref"`
	Owner          string `json:"owner"`
	Project        string `json:"project"`
	ProfileID      string `json:"profile_id"`
	TargetRevision uint64 `json:"target_revision"`
	Version        uint64 `json:"version"`
	Enabled        bool   `json:"enabled"`
}
type Configuration struct {
	Configuration struct {
		Profile ExecutionProfile `json:"profile"`
		Digest  string           `json:"digest"`
	} `json:"configuration"`
	Assignment Assignment `json:"assignment"`
	Activation string     `json:"activation"`
}

// ManagementClient is called only by an explicit host sync/report command.
// It never executes a provider or activates the returned configuration.
type ManagementClient struct {
	BaseURL      string
	Owner        string
	Project      string
	Token        func(context.Context) (string, error)
	RoundTripper http.RoundTripper
}

func (c ManagementClient) exchange(ctx context.Context, method, path string, input, out any) error {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || c.Token == nil || c.Owner == "" || c.Project == "" {
		return errors.New("judgment_management_configuration_invalid")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return errors.New("judgment_management_https_required")
	}
	var body []byte
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil || len(body) > 16<<10 {
			return errors.New("judgment_management_payload_invalid")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	token, err := c.Token(ctx)
	if err != nil || token == "" || strings.ContainsAny(token, "\r\n\t ") {
		return errors.New("judgment_management_credential_unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("judgment_management_request_invalid")
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Transport: c.RoundTripper, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return errors.New("judgment_management_outcome_unknown")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("judgment_management_request_rejected")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return errors.New("judgment_management_response_invalid")
	}
	if json.Unmarshal(raw, out) != nil {
		return errors.New("judgment_management_response_invalid")
	}
	return nil
}
func (c ManagementClient) Fetch(ctx context.Context) (Configuration, error) {
	var result Configuration
	if err := c.exchange(ctx, http.MethodGet, "/v1/judgments/configurations", nil, &result); err != nil {
		return result, err
	}
	p, a := result.Configuration.Profile, result.Assignment
	d, err := p.Digest()
	if err != nil || d != result.Configuration.Digest || a.Owner != c.Owner || a.Project != c.Project || a.KeyID == "" || a.Version == 0 || !a.Enabled || a.ProfileID != p.ID || a.TargetRevision != p.Revision || result.Activation != "explicit_sync" {
		return Configuration{}, errors.New("judgment_management_snapshot_invalid")
	}
	return result, nil
}
func (c ManagementClient) Applied(ctx context.Context, cfg Configuration) error {
	a := cfg.Assignment
	d, err := cfg.Configuration.Profile.Digest()
	if err != nil || d != cfg.Configuration.Digest || a.Owner != c.Owner || a.Project != c.Project || a.ProfileID != cfg.Configuration.Profile.ID || a.TargetRevision != cfg.Configuration.Profile.Revision {
		return errors.New("judgment_management_snapshot_invalid")
	}
	var out map[string]any
	if err := c.exchange(ctx, http.MethodPost, "/v1/judgments/reports", map[string]any{"applied": map[string]any{"owner": a.Owner, "project": a.Project, "key_ref": a.KeyID, "profile_id": a.ProfileID, "revision": a.TargetRevision, "digest": d, "assignment_version": a.Version}}, &out); err != nil {
		return err
	}
	return checkManagementAcknowledgment(out)
}
func (c ManagementClient) Report(ctx context.Context, summary Summary) error {
	if summary.Owner != c.Owner || summary.Project != c.Project {
		return errors.New("judgment_management_scope_invalid")
	}
	var out map[string]any
	if err := c.exchange(ctx, http.MethodPost, "/v1/judgments/reports", map[string]any{"receipt": summary}, &out); err != nil {
		return err
	}
	return checkManagementAcknowledgment(out)
}

func checkManagementAcknowledgment(out map[string]any) error {
	if out["status"] != "recorded" || out["provenance"] != "owner_reported" {
		return errors.New("judgment_management_report_unconfirmed")
	}
	return nil
}
