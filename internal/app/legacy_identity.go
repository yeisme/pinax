package app

import "github.com/yeisme/pinax/internal/identity"

func legacyNoteIDForPath(path string) string {
	return identity.LegacyNoteIDFromPath(path)
}
