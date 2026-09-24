# Pinax external RAG ownership

## Purpose

Vector databases, embedding providers, and retrieval runtimes are not Pinax
dependencies. A separate RAG owner may choose and own those components.

## Requirements

### Requirement: External RAG SHALL be reached through an explicit handoff

External RAG integration SHALL start from an explicit Markdown export rather than
Pinax provider calls.

#### Scenario: Handoff keeps ownership separate

- **WHEN** an agent needs semantic retrieval
- **THEN** it SHALL export Markdown with a Pinax command and call the external
  RAG contract; Pinax SHALL not expose provider credentials, vectors, or raw
  provider payloads.
