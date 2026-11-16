// Package treewalk is a small shared directory-walking helper for the
// template's maintenance programs (currently scripts/smoke; scripts/init is
// deliberately self-contained so it can rewrite a module that does not
// build).
package treewalk

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// maxFileSize caps the files maintenance programs will process (10 MiB).
const maxFileSize = 10 << 20

// Hooks are the callbacks invoked while walking root. rel paths are
// slash-separated and relative to root ("." is never passed to SkipDir).
type Hooks struct {
	// SkipDir prunes a directory subtree by its relative path.
	SkipDir func(rel string) bool
	// OnDir runs for each directory (except "."); used by scripts/smoke to
	// mirror the tree. Directories pruned by SkipDir never reach OnDir.
	OnDir func(rel string) error
	// OnFile runs for each regular file within size limits.
	OnFile func(path, rel string, d fs.DirEntry) error
}

// Walk enumerates root invoking the hooks. It does not follow symlinks:
// non-regular entries are skipped. The path passed to hooks is the join of
// root and rel, suitable for direct os.* calls.
func Walk(root string, h Hooks) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return walkDir(rel, h)
		}
		return walkFile(path, rel, d, h)
	})
}

func walkDir(rel string, h Hooks) error {
	if rel == "." {
		return nil
	}
	if h.SkipDir != nil && h.SkipDir(rel) {
		return filepath.SkipDir
	}
	if h.OnDir != nil {
		return h.OnDir(rel)
	}
	return nil
}

func walkFile(path, rel string, d fs.DirEntry, h Hooks) error {
	if !d.Type().IsRegular() {
		return nil
	}
	if h.OnFile == nil {
		return nil
	}
	info, err := d.Info()
	if err != nil {
		return err
	}
	if info.Size() > maxFileSize {
		return nil
	}
	return h.OnFile(path, rel, d)
}

// IsBinary reports whether data looks like binary content (NUL byte within
// the first 8000 bytes).
func IsBinary(data []byte) bool {
	const sniff = 8000
	if len(data) > sniff {
		data = data[:sniff]
	}
	return strings.ContainsRune(string(data), 0)
}
