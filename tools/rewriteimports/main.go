// Command rewriteimports rewrites pgwatch import paths across the repository.
//
// It exists for the internal/ -> pkg/ relocation described in
// spec/oss-published-packages.md and may be removed once that is done.
//
// Usage:
//
//	go run ./tools/rewriteimports [flags] <old> <new> [<old> <new> ...]
//
// Paths may be given either in full or relative to the module path, so both
// of these rewrite the same thing:
//
//	go run ./tools/rewriteimports internal/log pkg/log
//	go run ./tools/rewriteimports github.com/cybertec-postgresql/pgwatch/v6/internal/log \
//	    github.com/cybertec-postgresql/pgwatch/v6/pkg/log
//
// A path is only replaced where it ends an import path or is followed by a
// slash, so rewriting internal/log leaves internal/logsomething alone.
// Rewritten Go files are reformatted, which also re-sorts the import block.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const modulePath = "github.com/cybertec-postgresql/pgwatch/v6"

// skipDirs are never walked: build output, dependencies and VCS metadata.
var skipDirs = map[string]bool{
	".git":         true,
	".task":        true,
	"build":        true,
	"dist":         true,
	"node_modules": true,
	"site":         true,
}

type replacement struct{ old, new string }

func main() {
	var (
		root   = flag.String("root", ".", "directory to walk")
		exts   = flag.String("ext", ".go", "comma-separated file extensions to rewrite")
		dryRun = flag.Bool("n", false, "report the files that would change without writing them")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: go run ./tools/rewriteimports [flags] <old> <new> [<old> <new> ...]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 || len(args)%2 != 0 {
		flag.Usage()
		os.Exit(2)
	}

	var reps []replacement
	for i := 0; i < len(args); i += 2 {
		reps = append(reps, replacement{old: qualify(args[i]), new: qualify(args[i+1])})
	}

	wanted := make(map[string]bool)
	for _, e := range strings.Split(*exts, ",") {
		if e = strings.TrimSpace(e); e != "" {
			wanted[e] = true
		}
	}

	changed, err := walk(*root, wanted, reps, *dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rewriteimports:", err)
		os.Exit(1)
	}
	fmt.Printf("rewriteimports: %d file(s) changed\n", changed)
}

// qualify turns a module-relative path into a full import path.
func qualify(p string) string {
	p = strings.Trim(p, "/")
	if strings.HasPrefix(p, modulePath) {
		return p
	}
	return modulePath + "/" + p
}

func walk(root string, exts map[string]bool, reps []replacement, dryRun bool) (int, error) {
	var changed int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !exts[filepath.Ext(path)] {
			return nil
		}
		ok, err := rewriteFile(path, reps, dryRun)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if ok {
			changed++
			fmt.Println(path)
		}
		return nil
	})
	return changed, err
}

func rewriteFile(path string, reps []replacement, dryRun bool) (bool, error) {
	before, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	after := before
	for _, r := range reps {
		// Only replace at an import-path boundary: the quote that closes the
		// import, or a slash starting a subpackage.
		after = []byte(strings.ReplaceAll(string(after), r.old+`"`, r.new+`"`))
		after = []byte(strings.ReplaceAll(string(after), r.old+"/", r.new+"/"))
	}
	if string(after) == string(before) {
		return false, nil
	}
	if filepath.Ext(path) == ".go" {
		// Reformatting re-sorts the import block, which the rewrite disturbs,
		// and fails loudly if the result is not valid Go.
		if after, err = format.Source(after); err != nil {
			return false, err
		}
	}
	if dryRun {
		return true, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, after, info.Mode().Perm())
}
