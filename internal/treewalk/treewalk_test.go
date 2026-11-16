package treewalk_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kazzarahw/go-blueprint/internal/treewalk"
)

// writeFixture materializes a small deterministic tree for the walker tests.
func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"a.txt":            "alpha",
		"sub/b.txt":        "beta",
		"sub/deep/c.txt":   "gamma",
		"bin/blob.bin":     "bin\x00ary",
		"skipme/d.txt":     "pruned",
		"skipme/inner.txt": "pruned too",
		".git/e.txt":       "never seen",
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

func TestWalk_InvokesOnFileForEveryRegularFile(t *testing.T) {
	root := writeFixture(t)
	var got []string
	err := treewalk.Walk(root, treewalk.Hooks{
		SkipDir: func(rel string) bool { return rel == ".git" || rel == "skipme" },
		OnFile: func(_ string, rel string, _ fs.DirEntry) error {
			got = append(got, rel)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Strings(got)
	want := []string{"a.txt", "bin/blob.bin", "sub/b.txt", "sub/deep/c.txt"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Errorf("visited = %v, want %v", got, want)
	}
}

func TestWalk_OnDirSeesUnprunedDirectories(t *testing.T) {
	root := writeFixture(t)
	var got []string
	err := treewalk.Walk(root, treewalk.Hooks{
		SkipDir: func(rel string) bool { return rel == "skipme" },
		OnDir: func(rel string) error {
			got = append(got, rel)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Strings(got)
	want := []string{".git", "bin", "sub", "sub/deep"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Errorf("dirs = %v, want %v", got, want)
	}
}

func TestWalk_PropagatesFileErrors(t *testing.T) {
	root := writeFixture(t)
	boom := errors.New("boom")
	err := treewalk.Walk(root, treewalk.Hooks{
		OnFile: func(_ string, _ string, _ fs.DirEntry) error {
			return boom
		},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Walk error = %v, want boom", err)
	}
}

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, false},
		{"text", []byte("hello world\n"), false},
		{"nul byte", []byte("hello\x00world"), true},
		{"nul after sniff window", append(bytes.Repeat([]byte("a"), 9000), 0), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := treewalk.IsBinary(tt.data); got != tt.want {
				t.Errorf("IsBinary(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}
