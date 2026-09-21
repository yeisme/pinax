// Package judgment is a provider-neutral SDK for structured judgment
// services: capability discovery, strict typed request/result validation,
// canonical digesting, structured error classification, and injected
// transports (fixture, HTTP, optional local stdio).
//
// Contract owner: openspec/changes/aigora-structured-judgment-sdk-v1
// (Aigora). Wire format: snake_case UTF-8 JSON, schema_version "1.0".
//
// Boundaries (enforced by design and by TestSourceHygiene):
//   - the package never imports Aigora internal packages, never starts or
//     talks to the Aigora gateway by default, and holds no credentials;
//   - transports are injected by the caller; nothing is read from the
//     process environment;
//   - the SDK never retries: an Evaluate call maps to exactly one transport
//     call, and post-submit uncertainty is reported as outcome_unknown with
//     retry_class reconcile_first.
//
// The module is pure Go and builds with CGO_ENABLED=0.
package judgment
