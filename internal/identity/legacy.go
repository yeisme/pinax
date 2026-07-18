package identity

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
)

// LegacyNoteIDFromPath 仅用于读取和迁移尚未拥有 canonical UUID 的旧笔记。
// 新对象创建、复制、移动和重命名不得调用此函数。
func LegacyNoteIDFromPath(path string) string {
	sum := sha1.Sum([]byte(filepath.ToSlash(path)))
	return "note_" + hex.EncodeToString(sum[:])[:12]
}
