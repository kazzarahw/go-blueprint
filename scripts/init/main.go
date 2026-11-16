// init-project rewrites this template to your own module path.
//
// Usage:
//
//	go run ./scripts/init github.com/you/yourproject
//
// The program walks the repository, replaces every occurrence of the
// template module path with the given one (and, for good measure, the
// regex-escaped form used in documentation), and prints each file it
// changed. It is the recommended initialization path because it works
// identically on Linux, macOS and Windows — no sed, no find, no shell.
//
// The program is deliberately self-contained: it imports nothing from the
// module it is about to rewrite, and holds the template path as data in
// exactly one constant. It therefore works even when the module does not
// yet build. It skips .git and its own source (which contains that
// constant). LICENSE and the internal/taskstate example are left alone:
// the copyright line is yours to update, and the example is a reference you
// may delete once you have real code.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const oldModulePath = "github.com/kazzarahw/go-blueprint"

// maxFileSize caps the files the rewriter processes (10 MiB).
const maxFileSize = 10 << 20

type replacement struct{ old, new string }

var modulePathPart = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./scripts/init <new-module-path>\n")
		fmt.Fprintf(os.Stderr, "example: go run ./scripts/init github.com/you/yourproject\n")
		os.Exit(2)
	}
	newPath, err := validateModulePath(os.Args[1])
	if err != nil {
		fatal(err)
	}
	if newPath == oldModulePath {
		fatal(errors.New("new module path is identical to the template path; nothing to do"))
	}

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}

	escapedOld := strings.ReplaceAll(oldModulePath, ".", `\.`)
	escapedNew := strings.ReplaceAll(newPath, ".", `\.`)

	count, files, err := rewrite(root, []replacement{
		{oldModulePath, newPath},
		{escapedOld, escapedNew},
	})
	if err != nil {
		fatal(err)
	}

	fmt.Printf("Rewrote %d occurrence(s) across %d file(s).\n", count, len(files))
	for _, f := range files {
		fmt.Printf("  %s\n", f)
	}
	fmt.Println("\nRemaining references to the template path (expected: this program only):")
	if err := printRemaining(root); err != nil {
		fmt.Fprintf(os.Stderr, "init-project: warning: could not scan for remaining references: %v\n", err)
	}
	fmt.Println("\nNext steps: update LICENSE (copyright), README badges if forked, and run: make verify")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "init-project: %v\n", err)
	os.Exit(1)
}

// validateModulePath applies a pragmatic subset of the module path grammar:
// two or more slash-separated parts, ASCII alphanumerics plus . _ ~ -, and a
// dotted first part (a host-like first segment). It rejects whitespace and
// anything that would be dangerous in shell contexts downstream.
func validateModulePath(p string) (string, error) {
	if p == "" {
		return "", errors.New("module path must not be empty")
	}
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("module path %q needs at least two path elements, e.g. github.com/you/project", p)
	}
	for _, part := range parts {
		if part == "" || !modulePathPart.MatchString(part) {
			return "", fmt.Errorf("module path %q contains invalid element %q", p, part)
		}
	}
	if !strings.Contains(parts[0], ".") {
		return "", fmt.Errorf("first element %q of module path %q must contain a dot (host-like), e.g. github.com/you/project", parts[0], p)
	}
	return p, nil
}

// repoRoot returns the directory containing go.mod, which must declare the
// template module path — otherwise the script was pointed at the wrong
// directory or the project was already initialized.
func repoRoot() (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	moduleLine, err := readModuleDirective(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	if moduleLine != "module "+oldModulePath {
		return "", fmt.Errorf("%s declares %q, not %q — run from the repository root of an un-initialized template", filepath.Join(root, "go.mod"), moduleLine, "module "+oldModulePath)
	}
	return root, nil
}

func readModuleDirective(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- fixed path derived from the working directory
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return line, nil
		}
	}
	return "", fmt.Errorf("%s has no module directive", path)
}

// walkFiles enumerates the regular files under root, skipping .git and this
// program's own source, and calls onFile for each. rel paths are
// slash-separated relative paths suitable for messages and keys.
func walkFiles(root string, onFile func(path, rel string) error) error {
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
			if rel == ".git" || rel == "scripts/init" {
				return filepath.SkipDir
			}
			return nil
		}
		return onRegularFile(path, rel, d, onFile)
	})
}

func onRegularFile(path, rel string, d fs.DirEntry, onFile func(path, rel string) error) error {
	if !d.Type().IsRegular() {
		return nil
	}
	info, err := d.Info()
	if err != nil {
		return err
	}
	if info.Size() > maxFileSize {
		return nil
	}
	return onFile(path, rel)
}

// rewrite replaces every replacement in every regular, text file under root
// and returns the number of replacements performed and the list of changed
// files (slash-separated).
func rewrite(root string, repls []replacement) (int, []string, error) {
	total := 0
	changed := []string{}
	err := walkFiles(root, func(path, rel string) error {
		n, err := rewriteFile(path, repls)
		if err != nil {
			return err
		}
		total += n
		if n > 0 {
			changed = append(changed, rel)
		}
		return nil
	})
	return total, changed, err
}

// rewriteFile applies the replacements to one file and returns how many
// replacements were made. The file keeps its original permission bits.
func rewriteFile(path string, repls []replacement) (int, error) {
	data, err := os.ReadFile(path) // #nosec G304, G122 -- the path comes from walking the repository root the user pointed us at; not attacker input
	if err != nil {
		return 0, err
	}
	if isBinary(data) {
		return 0, nil
	}
	updated := string(data)
	count := 0
	for _, r := range repls {
		n := strings.Count(updated, r.old)
		updated = strings.ReplaceAll(updated, r.old, r.new)
		count += n
	}
	if count == 0 {
		return 0, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil { // #nosec G703 -- walking the repository root; not attacker input
		return 0, err
	}
	return count, nil
}

// printRemaining reports any file outside scripts/init that still mentions
// the template module path, so the operator can eyeball the result.
func printRemaining(root string) error {
	found := 0
	err := walkFiles(root, func(path, rel string) error {
		data, err := os.ReadFile(path) // #nosec G304, G122 -- walking the repository root; not attacker input
		if err != nil {
			return err
		}
		if isBinary(data) {
			return nil
		}
		if strings.Contains(string(data), oldModulePath) {
			found++
			fmt.Printf("  %s\n", rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if found == 0 {
		fmt.Println("  (none)")
	}
	return nil
}

func isBinary(data []byte) bool {
	const sniff = 8000
	if len(data) > sniff {
		data = data[:sniff]
	}
	return strings.ContainsRune(string(data), 0)
}
