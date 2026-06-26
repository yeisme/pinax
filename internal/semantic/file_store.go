package semantic

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type storeMetadata struct {
	SchemaVersion string `json:"schema_version"`
	Backend       string `json:"backend"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	EmbeddingDim  int    `json:"embedding_dim,omitempty"`
	IndexedAt     string `json:"indexed_at"`
}

func writeStoreMetadata(root, backend string, chunks []Chunk) error {
	meta := storeMetadata{SchemaVersion: SidecarSchema, Backend: normalizedBackend(backend), Provider: DefaultProvider, Model: DefaultModel, IndexedAt: time.Now().UTC().Format(time.RFC3339)}
	if len(chunks) > 0 {
		meta.Provider = chunks[0].Provider
		meta.Model = chunks[0].EmbeddingModel
		meta.EmbeddingDim = chunks[0].EmbeddingDim
		meta.IndexedAt = chunks[0].IndexedAt
	}
	path := filepath.Join(sidecarStorePath(root, backend), "metadata.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}

func readStoreProvider(root, backend string) (string, string) {
	meta := readStoreMetadata(root, backend)
	return meta.Provider, meta.Model
}

func readStoreMetadata(root, backend string) storeMetadata {
	fallback := storeMetadata{SchemaVersion: SidecarSchema, Backend: normalizedBackend(backend), Provider: DefaultProvider, Model: DefaultModel}
	payload, err := os.ReadFile(filepath.Join(sidecarStorePath(root, backend), "metadata.json"))
	if err != nil {
		return fallback
	}
	var meta storeMetadata
	if err := json.Unmarshal(payload, &meta); err != nil {
		return fallback
	}
	if meta.Provider == "" {
		meta.Provider = fallback.Provider
	}
	if meta.Model == "" {
		meta.Model = fallback.Model
	}
	if meta.Backend == "" {
		meta.Backend = fallback.Backend
	}
	return meta
}

type FileStore struct {
	Root    string
	Backend string
}

func NewFileStore(root, backend string) FileStore {
	return FileStore{Root: root, Backend: normalizedBackend(backend)}
}

func (s FileStore) Path() string {
	return filepath.Join(s.Root, ".pinax", "kb", s.Backend, "chunks.jsonl")
}

func (s FileStore) Save(chunks []Chunk) error {
	path := s.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	for _, chunk := range chunks {
		if err := enc.Encode(chunk); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s FileStore) Load() ([]Chunk, error) {
	path := s.Path()
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &domain.CommandError{Code: "kb_index_missing", Message: "KB semantic index is missing", Hint: "Run pinax kb rebuild --vault <vault> first"}
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	dec := json.NewDecoder(file)
	chunks := []Chunk{}
	for {
		var chunk Chunk
		if err := dec.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}
