package pinaxclient

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const maxTokenFileBytes int64 = 8 << 10

var (
	errTokenFilePlatformUnsupported = errors.New("secure token files are unsupported on this platform")
	errTokenFilePolicy              = errors.New("token file policy rejected the file")
)

// LoadTokenFile reads one plaintext bearer token from an owner-only file.
// The path must be absolute, every path component must be non-symlink, and
// the platform-specific owner/access policy must pass before any bytes are
// returned. Errors intentionally omit both the path and file contents.
func LoadTokenFile(tokenPath string) (string, error) {
	return loadTokenFile(tokenPath, os.Open)
}

func loadTokenFile(tokenPath string, openFile func(string) (*os.File, error)) (string, error) {
	if !filepath.IsAbs(tokenPath) || filepath.Clean(tokenPath) != tokenPath {
		return "", tokenFileError(CodeTokenFileUnsafe, "Pinax token file path is unsafe")
	}

	before, err := inspectTokenFilePath(tokenPath)
	if err != nil {
		return "", classifyTokenFileError(err)
	}
	file, err := openFile(tokenPath)
	if err != nil {
		return "", tokenFileError(CodeTokenFileUnreadable, "Pinax token file could not be opened")
	}
	defer func() { _ = file.Close() }()

	descriptorInfo, err := file.Stat()
	if err != nil {
		return "", tokenFileError(CodeTokenFileUnreadable, "Pinax token file could not be inspected")
	}
	if !descriptorInfo.Mode().IsRegular() || !os.SameFile(before, descriptorInfo) {
		return "", tokenFileError(CodeTokenFileUnsafe, "Pinax token file changed while opening")
	}
	if err := validateTokenFilePlatform(tokenPath, descriptorInfo); err != nil {
		return "", classifyTokenFileError(err)
	}
	if descriptorInfo.Size() > maxTokenFileBytes {
		return "", tokenFileError(CodeTokenFileTooLarge, "Pinax token file exceeds the size limit")
	}

	encoded, err := io.ReadAll(io.LimitReader(file, maxTokenFileBytes+1))
	if err != nil {
		return "", tokenFileError(CodeTokenFileUnreadable, "Pinax token file could not be read")
	}
	if int64(len(encoded)) > maxTokenFileBytes {
		return "", tokenFileError(CodeTokenFileTooLarge, "Pinax token file exceeds the size limit")
	}

	after, err := inspectTokenFilePath(tokenPath)
	if err != nil || !os.SameFile(descriptorInfo, after) {
		return "", tokenFileError(CodeTokenFileUnsafe, "Pinax token file changed while reading")
	}
	token := strings.TrimSpace(string(encoded))
	if token == "" || strings.IndexFunc(token, func(character rune) bool {
		return unicode.IsControl(character) || unicode.IsSpace(character)
	}) >= 0 {
		return "", tokenFileError(CodeTokenFileInvalid, "Pinax token file does not contain one valid bearer token")
	}
	return token, nil
}

func inspectTokenFilePath(tokenPath string) (os.FileInfo, error) {
	info, err := os.Lstat(tokenPath)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errTokenFilePolicy
	}
	if err := validateTokenFilePathEntryPlatform(tokenPath, info, false); err != nil {
		return nil, err
	}
	if err := validateTokenFilePlatform(tokenPath, info); err != nil {
		return nil, err
	}

	for current := filepath.Dir(tokenPath); ; current = filepath.Dir(current) {
		ancestor, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if ancestor.Mode()&os.ModeSymlink != 0 || !ancestor.IsDir() {
			return nil, errTokenFilePolicy
		}
		if err := validateTokenFilePathEntryPlatform(current, ancestor, true); err != nil {
			return nil, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return info, nil
}

func classifyTokenFileError(err error) error {
	switch {
	case errors.Is(err, errTokenFilePlatformUnsupported):
		return tokenFileError(CodeTokenFileUnsafe, "Pinax token file security policy is unavailable on this platform")
	case errors.Is(err, errTokenFilePolicy):
		return tokenFileError(CodeTokenFileUnsafe, "Pinax token file does not satisfy the secure file policy")
	default:
		return tokenFileError(CodeTokenFileUnreadable, "Pinax token file could not be inspected")
	}
}

func tokenFileError(code, message string) *Error {
	return newError(code, message, nil)
}
