package pinaxclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

const WriterEditorSourceSchemaV1Alpha1 = catalogschema.WriterEditorSourceV1Alpha1

// WriterEditorSource is exact Markdown body plus protected metadata. Metadata
// is never writable through the body; only a confirmed proposal may apply it.
type WriterEditorSource struct {
	SchemaVersion        string               `json:"schema_version"`
	ObjectID             string               `json:"object_id"`
	Revision             string               `json:"revision"`
	Body                 string               `json:"body"`
	BodyDigest           string               `json:"body_digest"`
	Editable             bool                 `json:"editable"`
	ReadonlyReason       string               `json:"readonly_reason"`
	FrontmatterProtected bool                 `json:"frontmatter_protected"`
	ProtectedMetadata    WriterEditorMetadata `json:"protected_metadata"`
	Budget               WriterEditorBudget   `json:"budget"`
}

type WriterEditorMetadata struct {
	NoteID string   `json:"note_id"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags"`
	Status string   `json:"status"`
	Kind   string   `json:"kind"`
}

type WriterEditorBudget struct {
	MaxSourceBytes int `json:"max_source_bytes"`
	MaxBodyBytes   int `json:"max_body_bytes"`
	SourceBytes    int `json:"source_bytes"`
	BodyBytes      int `json:"body_bytes"`
}

// WriterEditorSource fetches one exact live revision; no export, AST rewrite,
// mutation, implicit retry or access to owner-local storage is involved.
func (c *Client) WriterEditorSource(ctx context.Context, objectID, revision string) (WriterEditorSource, error) {
	var empty WriterEditorSource
	if strings.ContainsAny(objectID, "/\\\x00") || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterEditorSourceRequestV1Alpha1, map[string]any{"object_id": objectID, "revision": revision}) != nil {
		return empty, newError(CodeRequestInvalid, "Editor source requires an object id and exact sha256 revision", nil)
	}
	projection, err := c.getReaderProjection(ctx, "/v1/writer/notes/"+url.PathEscape(objectID)+"/editor-source", url.Values{"revision": {revision}})
	if err != nil {
		return empty, err
	}
	return decodeWriterEditorSource(projection, objectID, revision)
}

func decodeWriterEditorSource(projection Projection, objectID, revision string) (WriterEditorSource, error) {
	var empty WriterEditorSource
	var raw any
	if json.Unmarshal(projection.Data, &raw) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterEditorSourceV1Alpha1, raw) != nil {
		return empty, newError(CodeUpstreamInvalidResponse, "Editor source violated its frozen schema", nil)
	}
	var source WriterEditorSource
	if err := decodeProjectionData(projection, &source); err != nil {
		return empty, err
	}
	digest := sha256.Sum256([]byte(source.Body))
	if source.ObjectID != objectID || source.Revision != revision || source.ProtectedMetadata.NoteID != objectID || source.BodyDigest != "sha256:"+hex.EncodeToString(digest[:]) || source.Budget.BodyBytes != len(source.Body) || source.Budget.SourceBytes < source.Budget.BodyBytes || source.Editable != (source.ReadonlyReason == "none") || !utf8.ValidString(source.Body) || strings.ContainsRune(source.Body, 0) {
		return empty, newError(CodeUpstreamInvalidResponse, "Editor source identity, digest or budget did not match", nil)
	}
	return source, nil
}
