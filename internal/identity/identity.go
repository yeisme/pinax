// Package identity 定义 Pinax durable vault object 的稳定身份合同。
package identity

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var ErrIdempotencyKeyRequired = errors.New("identity_idempotency_key_required")

type ObjectID string

func (id ObjectID) String() string { return string(id) }

type IDClass string

const (
	IDClassInvalid   IDClass = "invalid"
	IDClassCanonical IDClass = "canonical"
	IDClassLegacy    IDClass = "legacy"
)

type ObjectKind string

const (
	KindVault      ObjectKind = "vault"
	KindNote       ObjectKind = "note"
	KindAsset      ObjectKind = "asset"
	KindProject    ObjectKind = "project"
	KindSubproject ObjectKind = "subproject"
	KindFolder     ObjectKind = "folder"
	KindView       ObjectKind = "view"
	KindTemplate   ObjectKind = "template"
	KindTask       ObjectKind = "task"
	KindFile       ObjectKind = "file"
)

func (kind ObjectKind) Valid() bool {
	switch kind {
	case KindVault, KindNote, KindAsset, KindProject, KindSubproject, KindFolder, KindView, KindTemplate, KindTask, KindFile:
		return true
	default:
		return false
	}
}

func NewObjectID() (ObjectID, error) {
	var value [16]byte
	millis := uint64(time.Now().UTC().UnixMilli())
	value[0] = byte(millis >> 40)
	value[1] = byte(millis >> 32)
	value[2] = byte(millis >> 24)
	value[3] = byte(millis >> 16)
	value[4] = byte(millis >> 8)
	value[5] = byte(millis)
	if _, err := rand.Read(value[6:]); err != nil {
		return "", fmt.Errorf("generate object id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80
	return ObjectID(formatUUID(value)), nil
}

func ParseObjectID(raw string) (ObjectID, error) {
	value, err := parseUUID(raw)
	if err != nil {
		return "", err
	}
	if value[6]>>4 != 7 || value[8]&0xc0 != 0x80 {
		return "", fmt.Errorf("invalid UUIDv7 object id %q", raw)
	}
	return ObjectID(raw), nil
}

func MustParseObjectID(raw string) ObjectID {
	id, err := ParseObjectID(raw)
	if err != nil {
		panic(err)
	}
	return id
}

func Classify(raw string) IDClass {
	if _, err := ParseObjectID(raw); err == nil {
		return IDClassCanonical
	}
	if isLegacyNoteID(raw) {
		return IDClassLegacy
	}
	return IDClassInvalid
}

func isLegacyNoteID(raw string) bool {
	if !strings.HasPrefix(raw, "note_") || len(raw) <= len("note_") || len(raw) > 133 {
		return false
	}
	for _, char := range strings.TrimPrefix(raw, "note_") {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case char == '_', char == '-':
		default:
			return false
		}
	}
	return true
}

func parseUUID(raw string) ([16]byte, error) {
	var value [16]byte
	if len(raw) != 36 || raw != strings.ToLower(raw) || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return value, fmt.Errorf("invalid canonical UUID %q", raw)
	}
	compact := strings.NewReplacer("-", "").Replace(raw)
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != len(value) {
		return value, fmt.Errorf("invalid canonical UUID %q", raw)
	}
	copy(value[:], decoded)
	return value, nil
}

func formatUUID(value [16]byte) string {
	var output [36]byte
	hex.Encode(output[0:8], value[0:4])
	output[8] = '-'
	hex.Encode(output[9:13], value[4:6])
	output[13] = '-'
	hex.Encode(output[14:18], value[6:8])
	output[18] = '-'
	hex.Encode(output[19:23], value[8:10])
	output[23] = '-'
	hex.Encode(output[24:36], value[10:16])
	return string(output[:])
}

type Generator func() (ObjectID, error)

type Allocator struct {
	mu        sync.Mutex
	allocated map[string]ObjectID
	generate  Generator
}

func NewAllocator() *Allocator {
	return NewAllocatorWithGenerator(NewObjectID)
}

func NewAllocatorWithGenerator(generate Generator) *Allocator {
	if generate == nil {
		generate = NewObjectID
	}
	return &Allocator{allocated: map[string]ObjectID{}, generate: generate}
}

func (allocator *Allocator) Allocate(idempotencyKey string) (ObjectID, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return "", ErrIdempotencyKeyRequired
	}
	allocator.mu.Lock()
	defer allocator.mu.Unlock()
	if id, ok := allocator.allocated[key]; ok {
		return id, nil
	}
	id, err := allocator.generate()
	if err != nil {
		return "", err
	}
	if _, err := ParseObjectID(id.String()); err != nil {
		return "", fmt.Errorf("identity generator returned invalid id: %w", err)
	}
	allocator.allocated[key] = id
	return id, nil
}

func UnixMillis(id ObjectID) (int64, error) {
	value, err := parseUUID(id.String())
	if err != nil {
		return 0, err
	}
	var buffer [8]byte
	copy(buffer[2:], value[0:6])
	return int64(binary.BigEndian.Uint64(buffer[:])), nil
}
