package app

import (
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Reader REST wire contracts (pinax-human-reading-api-v1 P06). They extend
// the OpenAPI export with the implemented human reading surface without
// touching the legacy RemoteRoutes facade: old REST/RPC/MCP/URI contracts
// stay byte-identical, and reader operations are additive GET-only paths.

func readerRESTContracts() []RemoteRESTContract {
	contracts := make([]RemoteRESTContract, 0, 7)
	readerRESTContract := func(routeID, capabilityID, command, path string, parameters []HTTPParameterSchema, responseSchema string, byteResponses bool, mediaType string) RemoteRESTContract {
		return RemoteRESTContract{
			RouteID:             routeID,
			CapabilityID:        capabilityID,
			Command:             command,
			Method:              "GET",
			Path:                path,
			Parameters:          parameters,
			RequestSchema:       readerRequestSchemaID(capabilityID),
			SuccessSchema:       catalogschema.ProjectionV1,
			SuccessDataSchema:   responseSchema,
			ByteResponses:       byteResponses,
			SuccessMediaType:    mediaType,
			ErrorSchema:         catalogschema.ProjectionV1,
			ErrorDetailSchema:   catalogschema.ErrorV1,
			BodyExposureDefault: readerBodyExposure(capabilityID),
			Readonly:            true,
			UIGroup:             "human.reading",
			WriteGate:           "readonly",
			Stability:           "experimental",
		}
	}
	readerContract := func(routeID, capabilityID, command, path string, parameters []HTTPParameterSchema, responseSchema string) RemoteRESTContract {
		return readerRESTContract(routeID, capabilityID, command, path, parameters, responseSchema, false, "")
	}
	byteContract := func(routeID, capabilityID, command, path string, parameters []HTTPParameterSchema, responseSchema, mediaType string) RemoteRESTContract {
		return readerRESTContract(routeID, capabilityID, command, path, parameters, responseSchema, true, mediaType)
	}
	contracts = append(contracts,
		readerContract("rest.reader.source", "reader.source", "reader.source", "/v1/reader/source", []HTTPParameterSchema{}, catalogschema.ReaderSourceV1Alpha1),
		readerContract("rest.reader.notes", "reader.notes", "reader.notes", "/v1/reader/notes", readerListParameters(false), catalogschema.ReaderNoteListV1Alpha1),
		readerContract("rest.reader.search", "reader.search", "reader.search", "/v1/reader/search", readerListParameters(true), catalogschema.ReaderSearchResultsV1Alpha1),
		readerContract("rest.reader.document", "reader.document", "reader.document", "/v1/reader/notes/{object_id}/document", readerDocumentParameters(), catalogschema.ReaderDocumentV1Alpha1),
		readerContract("rest.reader.links", "reader.links", "reader.links", "/v1/reader/notes/{object_id}/links", readerLinksParameters(), catalogschema.ReaderLinkPageV1Alpha1),
		// Byte endpoints: success responses are raw byte windows (200 full
		// within one window, 206 explicit Range) with Content-Range and
		// ETag identity headers — never a JSON projection envelope.
		byteContract("rest.reader.asset", "reader.asset", "reader.asset", "/v1/reader/assets/{asset_ref}/content", readerAssetParameters(), catalogschema.ReaderAssetRefV1Alpha1, "application/octet-stream"),
		byteContract("rest.reader.source_export", "reader.source_export", "reader.source_export", "/v1/reader/notes/{object_id}/source", readerExportParameters(), catalogschema.ReaderSourceExportV1Alpha1, "text/markdown"),
	)
	return contracts
}

func readerRequestSchemaID(capabilityID string) string {
	switch capabilityID {
	case "reader.source":
		return catalogschema.ReaderSourceRequestV1Alpha1
	case "reader.notes":
		return catalogschema.ReaderNotesRequestV1Alpha1
	case "reader.search":
		return catalogschema.ReaderSearchRequestV1Alpha1
	case "reader.document":
		return catalogschema.ReaderDocumentRequestV1Alpha1
	case "reader.links":
		return catalogschema.ReaderLinksRequestV1Alpha1
	case "reader.asset":
		return catalogschema.ReaderAssetContentRequestV1Alpha1
	default:
		return catalogschema.ReaderSourceExportRequestV1Alpha1
	}
}

func readerBodyExposure(capabilityID string) string {
	switch capabilityID {
	case "reader.search", "reader.document", "reader.source_export":
		return "explicit"
	default:
		return "none"
	}
}

func readerListParameters(withQuery bool) []HTTPParameterSchema {
	parameters := []HTTPParameterSchema{
		queryString("project_id"),
		queryStringArray("tag"),
		queryString("status"),
	}
	if withQuery {
		parameters = append(parameters, queryBoundedString("q", 1, catalogschema.ReaderQueryMaxChars))
	} else {
		parameters = append(parameters, queryEnum("sort", "updated_desc", "created_desc", "title_asc"))
	}
	return append(parameters,
		queryBoundedInteger("limit", 1, catalogschema.ReaderListMaxLimit),
		queryBoundedString("cursor", 1, catalogschema.ReaderCursorMaxLength),
	)
}

func readerDocumentParameters() []HTTPParameterSchema {
	return []HTTPParameterSchema{
		pathString("object_id"),
		queryBoundedString("revision", 1, catalogschema.ReaderRevisionMaxLength),
		queryBoundedString("cursor", 1, catalogschema.ReaderCursorMaxLength),
		queryBoundedString("anchor", 1, catalogschema.ReaderAnchorMaxLength),
		queryBoundedString("outline_cursor", 1, catalogschema.ReaderCursorMaxLength),
	}
}

func readerLinksParameters() []HTTPParameterSchema {
	return []HTTPParameterSchema{
		pathString("object_id"),
		queryRequiredBoundedString("revision", 1, catalogschema.ReaderRevisionMaxLength),
		queryEnum("direction", "outgoing", "backlinks"),
		queryBoundedInteger("limit", 1, catalogschema.ReaderListMaxLimit),
		queryBoundedString("cursor", 1, catalogschema.ReaderCursorMaxLength),
	}
}

func readerAssetParameters() []HTTPParameterSchema {
	return []HTTPParameterSchema{
		pathString("asset_ref"),
		queryRequiredBoundedString("note_id", 1, 256),
		queryRequiredBoundedString("note_revision", 1, catalogschema.ReaderRevisionMaxLength),
		headerRangeParameter(),
	}
}

func readerExportParameters() []HTTPParameterSchema {
	return []HTTPParameterSchema{
		pathString("object_id"),
		queryRequiredBoundedString("revision", 1, catalogschema.ReaderRevisionMaxLength),
		headerRangeParameter(),
	}
}

// queryRequiredBoundedString marks a query parameter the handler rejects
// requests without.
func queryRequiredBoundedString(name string, min, max int) HTTPParameterSchema {
	schema := queryBoundedString(name, min, max)
	schema.Required = true
	return schema
}

// headerRangeParameter declares the RFC 7233 Range request header for byte
// endpoints: explicit windows are mandatory for content larger than one
// request window.
func headerRangeParameter() HTTPParameterSchema {
	return HTTPParameterSchema{
		Name:        "Range",
		In:          HTTPParameterHeader,
		Schema:      map[string]any{"type": "string", "pattern": `^bytes=\d+-\d*$`},
		Description: "Inclusive byte window (bytes=start-end); required when the content exceeds one request window.",
	}
}

func queryStringArray(name string) HTTPParameterSchema {
	return HTTPParameterSchema{
		Name:        name,
		In:          HTTPParameterQuery,
		Schema:      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 16},
		Description: "Repeated parameter; multiple values combine with AND semantics.",
	}
}

func queryBoundedString(name string, min, max int) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterQuery, Schema: map[string]any{"type": "string", "minLength": min, "maxLength": max}}
}

func queryBoundedInteger(name string, min, max int) HTTPParameterSchema {
	return HTTPParameterSchema{Name: name, In: HTTPParameterQuery, Schema: map[string]any{"type": "integer", "minimum": min, "maximum": max, "default": catalogschema.ReaderListDefaultLimit}}
}
