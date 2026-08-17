// Package app — this file is the sync-run boundary that unlocks a
// repository-encrypted S3/COS credential bundle and constructs an immutable AWS
// SDK credentials provider for one sync run. Plaintext credentials live only
// inside the short-lived projectsecrets.Snapshot and the SDK provider; nothing
// is written to global env, ~/.aws, runtime YAML, receipts or logs.
package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/remote"
)

// SyncCredentialResolver unlocks one repository-encrypted S3 credential entry
// and returns the AWS SDK credentials provider plus the open Snapshot (caller
// must Close it after the sync run). It uses the credentialctl projectsecrets
// Resolver so pinax does not own KDF/AEAD/Keychain.
type SyncCredentialResolver struct {
	projectID    string
	repositoryID string
	entryName    string
	capability   string
}

// NewSyncCredentialResolver builds a resolver for one declared S3 credential
// identity. The repository id is the vault/repository identity recorded in the
// envelope; pinax passes its sync workspace id.
func NewSyncCredentialResolver(projectID, repositoryID, entryName string) *SyncCredentialResolver {
	return &SyncCredentialResolver{
		projectID:    projectID,
		repositoryID: repositoryID,
		entryName:    entryName,
		capability:   "object-storage",
	}
}

// Resolve unlocks the bundle at repoRoot/.pinax/project-secrets.yaml using the
// supplied unlock source and returns the AWS SDK credentials provider. The
// returned Snapshot MUST be Closed by the caller after the sync operation.
func (r *SyncCredentialResolver) Resolve(ctx context.Context, repoRoot string, source projectsecrets.UnlockSource) (aws.CredentialsProvider, *projectsecrets.Snapshot, error) {
	if repoRoot == "" {
		return nil, nil, fmt.Errorf("sync credential resolver: repoRoot required")
	}
	if source == nil {
		return nil, nil, fmt.Errorf("sync credential resolver: unlock source required")
	}
	envPath := filepath.Join(repoRoot, ".pinax", "project-secrets.yaml")
	policy := projectsecrets.NewAllowlistPolicy(map[string]projectsecrets.ConsumerScope{
		"pinax": {Capabilities: []string{r.capability}, Entries: []string{r.entryName}},
	})
	resolver, err := projectsecrets.NewResolver(envPath, source, policy)
	if err != nil {
		return nil, nil, err
	}
	info, err := resolver.EnvelopeInfo()
	if err != nil {
		return nil, nil, err
	}
	grant := projectsecrets.Grant{
		Project:    firstNonEmpty(r.projectID, info.Project),
		Repository: firstNonEmpty(r.repositoryID, info.Repository),
		Consumer:   "pinax",
		Capability: r.capability,
		Operation:  "sync-run",
		Entries:    []string{r.entryName},
		Digest:     info.Digest,
	}
	snap, err := resolver.Resolve(ctx, grant)
	if err != nil {
		return nil, nil, err
	}
	payload, ok := snap.Entry(r.entryName)
	if !ok {
		_ = snap.Close()
		return nil, nil, fmt.Errorf("sync credential resolver: entry %q missing from snapshot", r.entryName)
	}
	creds, err := parseS3Provider(payload)
	if err != nil {
		_ = snap.Close()
		return nil, nil, err
	}
	// The provider holds the plaintext inside the SDK; the snapshot keeps the
	// authoritative copy so Close can wipe it after the run.
	return creds, snap, nil
}

// s3StaticProvider wraps the AWS SDK StaticCredentialsProvider so pinax can
// construct it from a typed s3_credentials.v1 bundle.

func parseS3Provider(payload []byte) (aws.CredentialsProvider, error) {
	c, err := remote.ParseS3Credentials(payload)
	if err != nil {
		return nil, err
	}
	// The SDK's StaticCredentialsProvider implements aws.CredentialsProvider;
	// plaintext lives only inside it for the duration of the sync run.
	return credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, c.SessionToken), nil
}
