// smoke copies this repository into a temporary directory, rewrites the
// module path using scripts/init, checks that no template-path references
// remain outside scripts/init, and runs `make verify` in the resulting
// project. It is the end-to-end proof that the template initializes into a
// green project on a clean copy.
//
// Usage:
//
//	go run ./scripts/smoke
//
// Requires: Go and make on PATH (same contract as `make verify` itself).
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kazzarahw/go-blueprint/internal/treewalk"
)

const (
	oldModulePath = "github.com/kazzarahw/go-blueprint"
	newModulePath = "github.com/example/smoke-test"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "smoke: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("smoke: PASS — template initializes into a green project")
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod")) // #nosec G304 -- fixed path: the working directory smoke was invoked from
	if err != nil {
		return fmt.Errorf("read go.mod (run from the repository root): %w", err)
	}
	if !strings.Contains(string(goMod), "module "+oldModulePath) {
		return fmt.Errorf("go.mod does not declare the template module %q — run from the template root", oldModulePath)
	}

	tmp, err := os.MkdirTemp("", "go-blueprint-smoke-*")
	if err != nil {
		return err
	}
	defer func() {
		if rmErr := os.RemoveAll(tmp); rmErr != nil {
			fmt.Fprintf(os.Stderr, "smoke: warning: remove temp dir %s: %v\n", tmp, rmErr)
		}
	}()

	fmt.Printf("smoke: copying template to %s\n", tmp)
	if copyErr := copyTree(root, tmp); copyErr != nil {
		return fmt.Errorf("copy template: %w", copyErr)
	}

	fmt.Printf("smoke: rewriting module path to %s\n", newModulePath)
	if initErr := runCmd(tmp, "go", "run", "./scripts/init", newModulePath); initErr != nil {
		return initErr
	}

	fmt.Println("smoke: checking for stale template-path references")
	stale, err := findStaleReferences(tmp)
	if err != nil {
		return err
	}
	if len(stale) > 0 {
		return fmt.Errorf("template path still referenced in: %s", strings.Join(stale, ", "))
	}

	fmt.Println("smoke: running make verify in the initialized project")
	if verifyErr := runCmd(tmp, "make", "verify"); verifyErr != nil {
		return verifyErr
	}
	return nil
}

// copyTree copies the template into dst, skipping .git and local build
// artifacts. Copied files are owner-read/write only (0o600); the directory
// tree is owner-only (0o750) — sufficient for `go` and `make` to run.
func copyTree(src, dst string) error {
	return treewalk.Walk(src, treewalk.Hooks{
		SkipDir: func(rel string) bool {
			switch rel {
			case ".git", "bin", "dist":
				return true
			}
			return false
		},
		OnDir: func(rel string) error {
			return os.MkdirAll(filepath.Join(dst, filepath.FromSlash(rel)), 0o750)
		},
		OnFile: func(path, rel string, _ fs.DirEntry) error {
			if rel == "coverage.out" {
				return nil
			}
			return copyFile(path, filepath.Join(dst, filepath.FromSlash(rel)))
		},
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src is inside the template root being copied
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := in.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "smoke: warning: close %s: %v\n", src, closeErr)
		}
	}()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- dst is inside the fresh temp dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		if closeErr := out.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "smoke: warning: close %s: %v\n", dst, closeErr)
		}
		return err
	}
	return out.Close()
}

// findStaleReferences walks the initialized project and returns files that
// still contain the template module path. scripts/init keeps the path as
// data by design and is excluded.
func findStaleReferences(root string) ([]string, error) {
	stale := []string{}
	err := treewalk.Walk(root, treewalk.Hooks{
		SkipDir: func(rel string) bool { return rel == ".git" || rel == "scripts/init" },
		OnFile: func(path, rel string, _ fs.DirEntry) error {
			data, err := os.ReadFile(path) // #nosec G304, G122 -- walking the fresh temp copy; not attacker input
			if err != nil {
				return err
			}
			if treewalk.IsBinary(data) {
				return nil
			}
			if strings.Contains(string(data), oldModulePath) {
				stale = append(stale, rel)
			}
			return nil
		},
	})
	return stale, err
}

func runCmd(dir, name string, args ...string) error {
	fmt.Printf("smoke: $ (cd %s) %s %s\n", dir, name, strings.Join(args, " "))
	cmd := exec.Command(name, args...) // #nosec G204 -- the command and arguments are fixed by this program; name is one of "go"/"make"
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
